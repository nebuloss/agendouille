package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
)

// live is /api/live (CONTRACTS.md 7): Server-Sent Events telling an open
// page that a calendar it can read changed ("changed"), or that the list
// of its calendars may have ("access"). The page reloads what it shows;
// nothing is replayed, so a reconnect is followed by a reload ("resync").
func (h *handlers) live(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	flusher, ok := w.(http.Flusher)
	if !ok || h.d.Hub == nil {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	l, stop := h.d.Hub.Listen(64)
	defer stop()
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	send := func(event string, data any) bool {
		b, _ := json.Marshal(data)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	fmt.Fprint(w, "retry: 3000\n")
	if !send("hello", map[string]any{}) {
		return
	}
	if r.Header.Get("Last-Event-ID") != "" && !send("resync", map[string]any{}) {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if !send("ping", map[string]any{}) {
				return
			}
		case e := <-l.C:
			if l.Lost() && !send("resync", map[string]any{}) {
				return
			}
			if e.Action == "access" || e.Name == "" {
				if !send("access", map[string]any{}) {
					return
				}
				continue
			}
			acc, err := h.d.Calendar.Sharing().Access(ctx, p.User.ID, e.CalendarID)
			if err != nil || !acc.CanRead() {
				continue
			}
			fmt.Fprintf(w, "id: %d\n", e.Seq)
			if !send("changed", map[string]any{"seq": e.Seq, "calendar": e.Calendar, "name": e.Name, "action": e.Action, "by": e.By}) {
				return
			}
		}
	}
}

func newProfileToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func uuidOf() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	x := hex.EncodeToString(b)
	return x[:8] + "-" + x[8:12] + "-4" + x[13:16] + "-a" + x[17:20] + "-" + x[20:32]
}

// mobileconfig is an Apple configuration profile with one CalDAV account
// (PLAN.md 7): opened on an iPhone, it adds the account in one tap. It is
// unsigned, so iOS says "not verified", which is fine for a family.
func mobileconfig(host, user, secret, label string, id int64) []byte {
	e := html.EscapeString
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>CalDAVAccountDescription</key><string>agendouille</string>
      <key>CalDAVHostName</key><string>` + e(host) + `</string>
      <key>CalDAVPort</key><integer>443</integer>
      <key>CalDAVPrincipalURL</key><string>/dav/principals/` + e(user) + `/</string>
      <key>CalDAVUseSSL</key><true/>
      <key>CalDAVUsername</key><string>` + e(user) + `</string>
      <key>CalDAVPassword</key><string>` + e(secret) + `</string>
      <key>PayloadDescription</key><string>Les calendriers d'agendouille</string>
      <key>PayloadDisplayName</key><string>agendouille (` + e(label) + `)</string>
      <key>PayloadIdentifier</key><string>agendouille.caldav.` + fmt.Sprint(id) + `</string>
      <key>PayloadType</key><string>com.apple.caldav.account</string>
      <key>PayloadUUID</key><string>` + uuidOf() + `</string>
      <key>PayloadVersion</key><integer>1</integer>
    </dict>
  </array>
  <key>PayloadDisplayName</key><string>agendouille</string>
  <key>PayloadIdentifier</key><string>agendouille.profile.` + fmt.Sprint(id) + `</string>
  <key>PayloadRemovalDisallowed</key><false/>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadUUID</key><string>` + uuidOf() + `</string>
  <key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
`)
}
