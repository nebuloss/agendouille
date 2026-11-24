package main

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
)

// An admin tab's stream is told when something it shows changes elsewhere
// (internal/live): a device added from the family app is a "devices" notice
// on a stream that names devices, and nothing on one that names backups.
func TestAdminStreamIsToldOfChanges(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	A := signInAdmin(t, x)
	L := x.browser("lea")

	open := func(topics string) (*bufio.Reader, func()) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, "GET", x.admin+"/api/admin/events?topics="+topics, nil)
		req.AddCookie(&http.Cookie{Name: auth.AdminCookie, Value: A.session})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("the stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
		}
		return bufio.NewReader(res.Body), func() { cancel(); res.Body.Close() }
	}
	devices, stopD := open("devices,people")
	defer stopD()
	backups, stopB := open("backups")
	defer stopB()

	L.json("POST", "/api/devices", map[string]any{"kind": "davx5", "label": "Pixel"}, 201)

	got := make(chan string, 1)
	go func() {
		for {
			line, err := devices.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "event: ") {
				got <- strings.TrimSpace(strings.TrimPrefix(line, "event: "))
				return
			}
		}
	}()
	select {
	case ev := <-got:
		if ev != "devices" {
			t.Errorf("the first notice is %q", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notice within 5 s of a new device")
	}

	other := make(chan string, 1)
	go func() {
		for {
			line, err := backups.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "event: ") {
				other <- line
				return
			}
		}
	}()
	select {
	case ev := <-other:
		t.Errorf("a stream on backups was told of a device: %q", ev)
	case <-time.After(700 * time.Millisecond):
	}

	// Without the admin's session, no stream.
	if a := x.do("GET", x.admin+"/api/admin/events?topics=devices", nil, nil); a.status != http.StatusUnauthorized {
		t.Errorf("a stream without a session: %d", a.status)
	}
}
