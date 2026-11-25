package main

import (
	"bufio"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type sseEvent struct {
	id       int64
	endpoint string
	data     []byte
}

// openStream holds the device's push stream open, its events on the channel.
func openStream(t *testing.T, x *e2e, user, pass string, lastID int64) (<-chan sseEvent, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, x.public+"/api/app/push/stream", nil)
	req.SetBasicAuth(user, pass)
	if lastID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(lastID, 10))
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		cancel()
		t.Fatalf("the stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	out := make(chan sseEvent, 32)
	go func() {
		defer res.Body.Close()
		sc := bufio.NewScanner(res.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "id: "):
				ev.id, _ = strconv.ParseInt(line[4:], 10, 64)
			case strings.HasPrefix(line, "data: "):
				var d map[string]string
				_ = json.Unmarshal([]byte(line[6:]), &d)
				ev.endpoint = d["endpoint"]
				ev.data, _ = base64.RawURLEncoding.DecodeString(d["data"])
			case line == "" && ev.id != 0:
				out <- ev
				ev = sseEvent{}
			}
		}
	}()
	return out, cancel
}

// The push service in agendouille itself: an endpoint per app on a device,
// Web Push in (RFC 8030), out over the device's stream until acknowledged;
// WebDAV-Push's messages to one of these endpoints go straight in.
func TestPushRelay(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	L := x.browser("lea")
	d, _ := L.json("POST", "/api/devices", map[string]any{"kind": "thunderbird", "label": "Téléphone"}, 201)
	secret := d["secret"].(string)
	basic := func(r *http.Request) {
		r.SetBasicAuth("lea", secret)
		r.Header.Set("Content-Type", "application/json")
	}
	endpoint := func(instance string) (string, string) {
		a := x.do("POST", x.public+"/api/app/push/endpoints", strings.NewReader(`{"instance":"`+instance+`"}`), basic)
		var v map[string]string
		_ = json.Unmarshal([]byte(a.body), &v)
		if a.status != 200 || v["id"] == "" || !strings.HasSuffix(v["endpoint"], "/push/"+v["id"]) {
			t.Fatalf("an endpoint: %d %s", a.status, a.body)
		}
		return v["id"], v["endpoint"]
	}
	selfID, selfURL := endpoint("io.github.nebuloss.agendouille")
	if again, _ := endpoint("io.github.nebuloss.agendouille"); again != selfID {
		t.Errorf("the same app again: %s, want %s", again, selfID)
	}
	davID, davURL := endpoint("at.bitfire.davdroid:token-1")
	if x.do("GET", x.public+"/api/app/config", nil, basic).body == "" || !strings.Contains(x.do("GET", x.public+"/api/app/config", nil, basic).body, `"push_service":true`) {
		t.Errorf("the config does not offer the push service")
	}

	send := func(url, body string, hdr ...string) int {
		return x.do("POST", url, strings.NewReader(body), func(r *http.Request) {
			for i := 0; i+1 < len(hdr); i += 2 {
				r.Header.Set(hdr[i], hdr[i+1])
			}
		}).status
	}
	next := func(c <-chan sseEvent, what string) sseEvent {
		t.Helper()
		select {
		case e := <-c:
			return e
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: nothing on the stream", what)
		}
		return sseEvent{}
	}
	quiet := func(c <-chan sseEvent, what string) {
		t.Helper()
		select {
		case e := <-c:
			t.Errorf("%s: got %d %q", what, e.id, e.data)
		case <-time.After(700 * time.Millisecond):
		}
	}

	// Anyone with the URL sends (a sender has no account); the device gets it.
	stream, stop := openStream(t, x, "lea", secret, 0)
	if s := send(davURL, "bonjour", "TTL", "60", "Urgency", "high"); s != http.StatusCreated {
		t.Fatalf("sending: %d", s)
	}
	e1 := next(stream, "a message to DAVx5's endpoint")
	if e1.endpoint != davID || string(e1.data) != "bonjour" {
		t.Errorf("the event: %+v", e1)
	}

	// WebDAV-Push to our own endpoint: no network, the message is in the queue.
	c, _ := L.json("POST", "/api/calendars", map[string]any{"name": "Perso"}, 201)
	ref := c["ref"].(string)
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	other, _ := L.json("POST", "/api/devices", map[string]any{"kind": "thunderbird", "label": "Autre"}, 201)
	reg := (davClient{x: x, user: "lea", pass: secret}).req("POST", "/dav/calendars/lea/"+ref+"/", `<?xml version="1.0"?>
<push-register xmlns="https://bitfire.at/webdav-push" xmlns:D="DAV:"><subscription><web-push-subscription>
<push-resource>`+selfURL+`</push-resource>
<subscription-public-key type="p256dh">`+base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())+`</subscription-public-key>
<auth-secret>`+base64.RawURLEncoding.EncodeToString(auth)+`</auth-secret></web-push-subscription></subscription>
<trigger><content-update><D:depth>1</D:depth></content-update></trigger></push-register>`)
	if reg.status != http.StatusNoContent {
		t.Fatalf("registering our own endpoint: %d %s", reg.status, reg.body)
	}
	(davClient{x: x, user: "lea", pass: other["secret"].(string)}).req("PUT", "/dav/calendars/lea/"+ref+"/x.ics", judo)
	e2 := next(stream, "a calendar change")
	if e2.endpoint != selfID || !strings.Contains(openWebPush(t, e2.data, ua, auth), "<P:content-update>") {
		t.Errorf("the calendar's message: %+v", e2)
	}
	stop()

	// Unacknowledged, it comes again; acknowledged (Last-Event-ID), never.
	again, stop2 := openStream(t, x, "lea", secret, e1.id)
	if e := next(again, "after reconnecting"); e.id != e2.id {
		t.Errorf("redelivered %d, want %d", e.id, e2.id)
	}
	quiet(again, "nothing else")
	stop2()

	// While away: a newer message of a topic replaces the older; TTL 0 is dropped.
	send(davURL, "un", "TTL", "60", "Topic", "cal")
	send(davURL, "deux", "TTL", "60", "Topic", "cal")
	send(davURL, "jetable", "TTL", "0")
	back, stop3 := openStream(t, x, "lea", secret, e2.id)
	if e := next(back, "the topic's latest"); string(e.data) != "deux" {
		t.Errorf("got %q, want the newer message", e.data)
	}
	quiet(back, "the replaced and the TTL 0 ones")
	stop3()

	// The errors: unknown, too large; a dropped endpoint is unknown.
	if s := send(x.public+"/push/nope", "x"); s != http.StatusNotFound {
		t.Errorf("an unknown endpoint: %d", s)
	}
	if s := send(davURL, strings.Repeat("x", 4097)); s != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: %d", s)
	}
	if s := x.do("DELETE", x.public+"/api/app/push/endpoints/"+davID, nil, basic).status; s != http.StatusNoContent {
		t.Errorf("dropping: %d", s)
	}
	if s := send(davURL, "x"); s != http.StatusNotFound {
		t.Errorf("a dropped endpoint: %d", s)
	}
	// The stream is a device's: a browser session has none.
	if s := x.do("GET", x.public+"/api/app/push/stream", nil, nil).status; s != http.StatusUnauthorized {
		t.Errorf("the stream without a device: %d", s)
	}
}
