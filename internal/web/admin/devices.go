package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// The admin sets a device up for someone (DECISIONS: devices from the
// admin): a new device with its manual settings (address, username,
// password, shown once), and a client certificate for any device, as a
// .p12 to download once. Both are in the audit.

// p12s are the certificates waiting to be downloaded: ten minutes, once.
var p12s = struct {
	sync.Mutex
	byToken map[string]p12
}{byToken: map[string]p12{}}

type p12 struct {
	userID, deviceID int64
	body             []byte
	expires          time.Time
}

func (h *handlers) keepP12(userID, deviceID int64, body []byte) string {
	p12s.Lock()
	defer p12s.Unlock()
	now := h.d.Clock()
	for k, v := range p12s.byToken {
		if now.After(v.expires) {
			delete(p12s.byToken, k)
		}
	}
	token := auth.RandomToken()
	p12s.byToken[token] = p12{userID: userID, deviceID: deviceID, body: body, expires: now.Add(10 * time.Minute)}
	return token
}

// addDevice is a new device for a user, made by the admin.
func (h *handlers) addDevice(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Cap   string `json:"cap"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Cap == "" {
		body.Cap = "readwrite"
	}
	ctx := r.Context()
	d, err := h.d.Auth.NewDevice(ctx, u.ID, body.Kind, body.Label, body.Cap)
	if err != nil {
		if errors.Is(err, auth.ErrTooMany) || errors.Is(err, auth.ErrInvalid) {
			h.failIn(w, r, err)
		} else if strings.TrimSpace(body.Label) == "" || len(body.Label) > 100 {
			h.failIn(w, r, validation{key: "error.device.label"})
		} else {
			h.failIn(w, r, validation{key: "error.device.invalid"})
		}
		return
	}
	_ = h.d.Auth.Audit(ctx, "admin", "device", "added", fmt.Sprintf("%s: %s (%s)", u.Name, d.Label, d.Kind))
	base := h.d.PublicURL()
	out := map[string]any{"id": d.ID, "kind": d.Kind, "label": d.Label, "cap": d.Cap,
		"server": base + "/dav/", "username": d.Username, "secret": d.Secret, "dav_auth": h.d.Auth.DavAuthMode(ctx)}
	if mode := h.d.Auth.DavAuthMode(ctx); mode != auth.DavPassword {
		cert, err := h.certificate(r, u.ID, u.Name, d.ID)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		for k, v := range cert {
			out[k] = v
		}
	}
	webx.JSON(w, http.StatusCreated, out)
}

func (h *handlers) certificate(r *http.Request, userID int64, user string, deviceID int64) (map[string]any, error) {
	c, err := h.d.Auth.IssueCertificate(r.Context(), userID, deviceID)
	if err != nil {
		return nil, err
	}
	_ = h.d.Auth.Audit(r.Context(), "admin", "device", "certificate issued", fmt.Sprintf("%s: device %d", user, deviceID))
	token := h.keepP12(userID, deviceID, c.P12)
	return map[string]any{
		"certificate_url":        fmt.Sprintf("/api/admin/users/%s/devices/%d/certificate.p12?t=%s", user, deviceID, token),
		"certificate_password":   c.Password,
		"certificate_expires_at": c.ExpiresAt,
	}, nil
}

// newCertificate gives a user's device a new client certificate; the one it
// had stops working.
func (h *handlers) newCertificate(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	id, idOK := pathID(r, "id")
	if !ok || !idOK {
		return
	}
	out, err := h.certificate(r, u.ID, u.Name, id)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusCreated, out)
}

// certificateFile hands a new certificate over, once.
func (h *handlers) certificateFile(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	id, err := strconv.ParseInt(strings.TrimSuffix(r.PathValue("id"), ".p12"), 10, 64)
	if !ok || err != nil {
		return
	}
	p12s.Lock()
	p, found := p12s.byToken[r.URL.Query().Get("t")]
	if found && p.userID == u.ID && p.deviceID == id {
		delete(p12s.byToken, r.URL.Query().Get("t"))
	}
	p12s.Unlock()
	if !found || p.userID != u.ID || p.deviceID != id || h.d.Clock().After(p.expires) {
		webx.FailT(w, r, http.StatusGone, "gone", "error.certificate.linkGone")
		return
	}
	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="agendouille-%s-%d.p12"`, u.Name, id))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(p.body)
}
