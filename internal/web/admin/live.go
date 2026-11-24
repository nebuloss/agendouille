package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/live"
)

// How a tab learns what changed (internal/live, claudication's
// docs/data-architecture.md): one stream of notices per tab, naming the
// topics its page shows; the tab reads again, through the API, what changed.

// streamKeepalive is how often a quiet stream writes a comment, so nothing
// between the tab and the server closes it as idle: nginx's default
// proxy_read_timeout is a minute.
const streamKeepalive = 15 * time.Second

// streamLife bounds one stream. The tab connects again at once, and the
// session is checked again then: a stream must not outlive the sign-in that
// opened it.
const streamLife = 30 * time.Minute

// events is a tab's stream: ?topics= names what its page shows, and each
// change to one of them is one event naming the topic and its version.
func (h *handlers) events(w http.ResponseWriter, r *http.Request) {
	var topics []live.Topic
	for t := range strings.SplitSeq(r.URL.Query().Get("topics"), ",") {
		topics = append(topics, live.Topic(strings.TrimSpace(t)))
	}
	if h.d.Live == nil {
		http.Error(w, "no live updates", http.StatusServiceUnavailable)
		return
	}
	sub := h.d.Live.Subscribe(topics)
	defer h.d.Live.Unsubscribe(sub)

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	// nginx buffers a proxied answer unless told not to, and a notice held
	// in its buffer is a screen that does not move.
	hd.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	_ = rc.Flush()

	keepalive := time.NewTicker(streamKeepalive)
	defer keepalive.Stop()
	life := time.NewTimer(streamLife)
	defer life.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return
		case <-life.C:
			return
		case <-keepalive.C:
			_, err = fmt.Fprint(w, ": keepalive\n\n")
		case n, ok := <-sub.C:
			if !ok {
				return
			}
			if n.Resync {
				_, err = fmt.Fprintf(w, "id: %d\nevent: resync\ndata: {}\n\n", n.Seq)
			} else {
				_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: {\"v\":%d}\n\n", n.Seq, n.Topic, n.Version)
			}
		}
		if err != nil || rc.Flush() != nil {
			return
		}
	}
}
