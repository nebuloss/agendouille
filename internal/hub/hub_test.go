package hub

import "testing"

func TestPublishNeverBlocksAndSaysWhatWasLost(t *testing.T) {
	h := New()
	l, stop := h.Listen(2)
	for i := range 5 {
		h.Publish(Event{Seq: int64(i)})
	}
	if len(l.C) != 2 || !l.Lost() || l.Lost() {
		t.Fatalf("buffered %d; lost should be reported once", len(l.C))
	}
	if e := <-l.C; e.Seq != 0 {
		t.Errorf("first event %d", e.Seq)
	}
	stop()
	h.Publish(Event{Seq: 9})
	if len(l.C) != 1 {
		t.Error("a stopped listener still receives")
	}
}
