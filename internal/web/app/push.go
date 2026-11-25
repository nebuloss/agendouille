package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/push"
	"github.com/nebuloss/agendouille/internal/web/assets"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// The push service (DECISIONS: push without ntfy, CONTRACTS 3.7): Web Push
// in at /push/<id> (RFC 8030 section 5, open to any sender: the URL is the
// secret), and out to the device over one Server-Sent Events stream, which
// the agendouille app holds open and hands on as a UnifiedPush distributor.

// streamPing keeps proxies from closing a quiet stream (nginx: 60 s).
var streamPing = 25 * time.Second

func (h *handlers) pushRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /push/{id}", h.pushIn)
	mux.Handle("GET /api/app/push/stream", h.basic(h.pushStream))
	mux.Handle("POST /api/app/push/endpoints", h.basic(h.pushEndpoint))
	mux.Handle("DELETE /api/app/push/endpoints/{id}", h.basic(h.pushDropEndpoint))
	mux.Handle("POST /api/app/push/ack", h.basic(h.pushAck))
}

// pushIn takes a Web Push message for an endpoint: 201, as RFC 8030 says.
func (h *handlers) pushIn(w http.ResponseWriter, r *http.Request) {
	relay := h.d.Push.Relay()
	body, err := io.ReadAll(io.LimitReader(r.Body, push.MaxMessage+1))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	// TTL is mandatory in RFC 8030; UnifiedPush senders do not all send it.
	ttl := 24 * time.Hour
	if v := r.Header.Get("TTL"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 {
			http.Error(w, "bad TTL", http.StatusBadRequest)
			return
		}
		ttl = time.Duration(n) * time.Second
	}
	id := r.PathValue("id")
	err = relay.Accept(r.Context(), id, body, ttl, r.Header.Get("Topic"), r.Header.Get("Urgency"))
	switch {
	case errors.Is(err, push.ErrNoEndpoint):
		http.NotFound(w, r)
	case errors.Is(err, push.ErrTooLarge):
		http.Error(w, "message too large", http.StatusRequestEntityTooLarge)
	case err != nil:
		h.d.Log.Error("push: accepting a message", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	default:
		w.Header().Set("Location", relay.EndpointURL(id))
		w.Header().Set("TTL", strconv.Itoa(int(ttl/time.Second)))
		w.WriteHeader(http.StatusCreated)
	}
}

// pushStream is the device's messages, as they come: Server-Sent Events,
// each message an event whose id the device acknowledges (Last-Event-ID on
// the next connection, or /api/app/push/ack).
func (h *handlers) pushStream(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if p.DeviceID == 0 {
		webx.FailT(w, r, http.StatusForbidden, "forbidden", "error.app.sessionOnly")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	relay := h.d.Push.Relay()
	ctx := r.Context()
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if after > 0 {
		if err := relay.Ack(ctx, p.DeviceID, after); err != nil {
			h.d.Log.Warn("push: acknowledging", "err", err)
		}
	}
	wake, stop := relay.Wait(p.DeviceID)
	defer stop()
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "retry: 5000\n\n")
	// Which app this server carries, first: a phone hears of an update as
	// soon as it reconnects after a deploy (no id: nothing to acknowledge).
	apkSHA, apkVersion := "", ""
	if a := assets.AndroidApp(); a != nil {
		apkSHA, apkVersion = a.SHA256, a.Version
	}
	server, _ := json.Marshal(map[string]string{"apk_version": apkVersion, "apk_sha256": apkSHA, "server_version": h.d.Version})
	_, _ = fmt.Fprintf(w, "event: server\ndata: %s\n\n", server)
	flusher.Flush()
	ping := time.NewTicker(streamPing)
	defer ping.Stop()
	for {
		msgs, err := relay.Pending(ctx, p.DeviceID, after)
		if err != nil {
			if ctx.Err() == nil {
				h.d.Log.Warn("push: the stream", "err", err)
			}
			return
		}
		for _, m := range msgs {
			data, _ := json.Marshal(map[string]string{"endpoint": m.Endpoint, "urgency": m.Urgency,
				"data": base64.RawURLEncoding.EncodeToString(m.Body)})
			if _, err := fmt.Fprintf(w, "id: %d\nevent: message\ndata: %s\n\n", m.Seq, data); err != nil {
				return
			}
			after = m.Seq
		}
		flusher.Flush()
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// pushEndpoint gives the device an endpoint for one app on it.
func (h *handlers) pushEndpoint(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if p.DeviceID == 0 {
		webx.FailT(w, r, http.StatusForbidden, "forbidden", "error.app.sessionOnly")
		return
	}
	var body struct {
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil || body.Instance == "" {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	relay := h.d.Push.Relay()
	id, err := relay.Endpoint(r.Context(), p.DeviceID, body.Instance)
	if errors.Is(err, push.ErrTooMany) {
		http.Error(w, `{"error":"too_many"}`, http.StatusConflict)
		return
	}
	if err != nil {
		h.d.Log.Error("push: an endpoint", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]string{"id": id, "endpoint": relay.EndpointURL(id)})
}

func (h *handlers) pushDropEndpoint(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ok, err := h.d.Push.Relay().DropEndpoint(r.Context(), p.DeviceID, r.PathValue("id"))
	switch {
	case err != nil:
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	case !ok:
		http.NotFound(w, r)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// pushAck is the device saying it has every message up to seq.
func (h *handlers) pushAck(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Seq int64 `json:"seq"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil || body.Seq <= 0 {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	if err := h.d.Push.Relay().Ack(r.Context(), p.DeviceID, body.Seq); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
