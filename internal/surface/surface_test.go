package surface

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
)

func say(text string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, text) })
}

func fetch(t *testing.T, addr net.Addr) string {
	t.Helper()
	resp, err := http.Get("http://" + addr.String() + "/")
	if err != nil {
		return "down"
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestThePublicListenerStartsMovesAndStops(t *testing.T) {
	s := New(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), say("public"))
	defer s.Shutdown()
	admin, err := s.StartAdmin("127.0.0.1:0", say("admin"))
	if err != nil {
		t.Fatal(err)
	}
	if s.PublicAddr() != nil {
		t.Fatal("public listener up before the setting")
	}

	first := freeAddr(t)
	if err := s.ApplyPublic(first); err != nil {
		t.Fatal(err)
	}
	a1 := s.PublicAddr()
	if got := fetch(t, a1); got != "public" {
		t.Fatalf("first address: %q", got)
	}

	second := freeAddr(t)
	if err := s.ApplyPublic(second); err != nil {
		t.Fatal(err)
	}
	if got := fetch(t, s.PublicAddr()); got != "public" {
		t.Fatalf("second address: %q", got)
	}
	if got := fetch(t, a1); got != "down" {
		t.Errorf("the old address still answers: %q", got)
	}

	// The same setting again changes nothing.
	before := s.PublicAddr().String()
	if err := s.ApplyPublic(second); err != nil || s.PublicAddr().String() != before {
		t.Fatalf("re-applying: %v", err)
	}

	if err := s.ApplyPublic(""); err != nil {
		t.Fatal(err)
	}
	if s.PublicAddr() != nil {
		t.Error("public listener still up after clearing the setting")
	}
	if got := fetch(t, admin); got != "admin" {
		t.Errorf("the admin listener: %q", got)
	}
}

func TestABadAddressLeavesTheOldListenerServing(t *testing.T) {
	s := New(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), say("public"))
	defer s.Shutdown()
	if err := s.ApplyPublic(freeAddr(t)); err != nil {
		t.Fatal(err)
	}
	good := s.PublicAddr()
	if err := s.ApplyPublic("192.0.2.1:8791"); err == nil { // TEST-NET: not an address of this machine
		t.Fatal("binding an address this machine does not have succeeded")
	}
	if got := fetch(t, good); got != "public" {
		t.Errorf("after a failed move: %q", got)
	}
}

func TestAMoveToTheSamePortOnAnotherHost(t *testing.T) {
	s := New(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), say("public"))
	defer s.Shutdown()
	addr := freeAddr(t)
	_, port, _ := net.SplitHostPort(addr)
	if err := s.ApplyPublic("127.0.0.1:" + port); err != nil {
		t.Fatal(err)
	}
	// 0.0.0.0 on the same port needs the old listener gone first.
	if err := s.ApplyPublic("0.0.0.0:" + port); err != nil {
		t.Fatal(err)
	}
	if got := fetch(t, s.PublicAddr()); got != "public" {
		t.Errorf("after the move: %q", got)
	}
}
