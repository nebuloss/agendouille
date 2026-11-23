// Package hub fans calendar changes out to whoever listens in this process:
// the web app's live feed (M4) and push (M9). Publishing never blocks: a
// listener that falls behind loses events and is told so, and the SSE
// handler then asks its client to reload.
package hub

import "sync"

// Event is one change, as the calendar service publishes it.
type Event struct {
	Seq        int64
	CalendarID int64
	Calendar   string // ref, "owner~slug"
	Name       string // "" for a change of the calendar itself
	Action     string
	By         string
}

// Listener receives events on C. Lost reports whether some were dropped
// since the last time it was read.
type Listener struct {
	C    chan Event
	mu   sync.Mutex
	lost bool
}

// Lost reports, and clears, whether events were dropped.
func (l *Listener) Lost() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lost := l.lost
	l.lost = false
	return lost
}

// Hub is the fan-out.
type Hub struct {
	mu        sync.Mutex
	listeners map[*Listener]struct{}
}

// New makes an empty hub.
func New() *Hub { return &Hub{listeners: map[*Listener]struct{}{}} }

// Listen adds a listener with room for buffer events; stop removes it.
func (h *Hub) Listen(buffer int) (l *Listener, stop func()) {
	l = &Listener{C: make(chan Event, buffer)}
	h.mu.Lock()
	h.listeners[l] = struct{}{}
	h.mu.Unlock()
	return l, func() {
		h.mu.Lock()
		delete(h.listeners, l)
		h.mu.Unlock()
	}
}

// Publish hands e to every listener that has room.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for l := range h.listeners {
		select {
		case l.C <- e:
		default:
			l.mu.Lock()
			l.lost = true
			l.mu.Unlock()
		}
	}
}
