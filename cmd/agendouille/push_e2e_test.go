package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/push"
)

// pushed is one message a test push service received.
type pushed struct {
	path   string
	header http.Header
	body   []byte
}

// M9's "done when", on the server's side: a device subscribes to a calendar
// over CalDAV (WebDAV-Push); a change made elsewhere reaches its push
// resource at once, encrypted for it and signed with VAPID; its own changes,
// and those a request says not to push, do not; a resource the push service
// says is gone is dropped.
func TestWebDAVPush(t *testing.T) {
	got := make(chan pushed, 16)
	ps := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- pushed{r.URL.Path, r.Header.Clone(), body}
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer ps.Close()
	push.Transport = ps.Client().Transport
	t.Cleanup(func() { push.Transport = nil })

	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	L := x.browser("lea")
	c, _ := L.json("POST", "/api/calendars", map[string]any{"name": "Perso"}, 201)
	ref := c["ref"].(string)
	dev := func(label string) davClient {
		d, _ := L.json("POST", "/api/devices", map[string]any{"kind": "thunderbird", "label": label}, 201)
		return davClient{x: x, user: "lea", pass: d["secret"].(string)}
	}
	A, B := dev("Téléphone"), dev("Portable")
	calPath := "/dav/calendars/lea/" + ref + "/"

	// Discovery: the DAV header, and the calendar's three properties.
	if h := A.req("OPTIONS", calPath, "").header.Get("DAV"); !strings.Contains(h, "webdav-push") {
		t.Errorf("DAV header: %q", h)
	}
	pf := A.req("PROPFIND", calPath, `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:P="https://bitfire.at/webdav-push">
<d:prop><P:transports/><P:topic/><P:supported-triggers/></d:prop></d:propfind>`, "Depth", "0")
	if pf.status != 207 || !strings.Contains(pf.body, `<p:vapid-public-key type="p256ecdsa">`) ||
		!strings.Contains(pf.body, "<p:content-update><d:depth>1</d:depth></p:content-update>") {
		t.Fatalf("the push properties: %d %s", pf.status, pf.body)
	}
	topic := between(pf.body, "<p:topic>", "</p:topic>")
	if len(topic) != 32 {
		t.Fatalf("topic %q", topic)
	}
	// allprop does not carry them.
	if all := A.req("PROPFIND", calPath, `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:allprop/></d:propfind>`, "Depth", "0"); strings.Contains(all.body, "transports") {
		t.Errorf("allprop has the push properties")
	}

	// A registers.
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	register := func(cl davClient, resource string) answer {
		return cl.req("POST", calPath, `<?xml version="1.0" encoding="utf-8"?>
<push-register xmlns="https://bitfire.at/webdav-push" xmlns:D="DAV:">
  <subscription><web-push-subscription>
    <push-resource>`+resource+`</push-resource>
    <content-encoding>aes128gcm</content-encoding>
    <subscription-public-key type="p256dh">`+base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())+`</subscription-public-key>
    <auth-secret>`+base64.RawURLEncoding.EncodeToString(auth)+`</auth-secret>
  </web-push-subscription></subscription>
  <trigger><content-update><D:depth>1</D:depth></content-update><property-update><D:depth>0</D:depth></property-update></trigger>
  <expires>`+time.Now().Add(72*time.Hour).UTC().Format(http.TimeFormat)+`</expires>
</push-register>`)
	}
	r1 := register(A, ps.URL+"/up/phone")
	loc := r1.header.Get("Location")
	if r1.status != http.StatusNoContent || !strings.Contains(loc, "/dav/push/") || r1.header.Get("Expires") == "" {
		t.Fatalf("registering: %d %v %s", r1.status, r1.header, r1.body)
	}
	// The same resource again: renewed, the same registration.
	if r2 := register(A, ps.URL+"/up/phone"); r2.status != http.StatusNoContent || r2.header.Get("Location") != loc {
		t.Errorf("registering again: %d %q, want %q", r2.status, r2.header.Get("Location"), loc)
	}
	// Not over https: refused.
	if r := register(A, "http://example.com/up"); r.status != http.StatusForbidden || !strings.Contains(r.body, "invalid-subscription") {
		t.Errorf("an http resource: %d %s", r.status, r.body)
	}

	wait := func(what string) pushed {
		t.Helper()
		select {
		case m := <-got:
			return m
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no push", what)
		}
		return pushed{}
	}
	none := func(what string) {
		t.Helper()
		select {
		case m := <-got:
			t.Errorf("%s: pushed anyway to %s", what, m.path)
		case <-time.After(1500 * time.Millisecond):
		}
	}

	// The web app adds an event: A hears of it, encrypted for it.
	L.json("POST", "/api/calendars/"+ref+"/events", map[string]any{"summary": "Piscine", "start": "2026-11-28T10:00"}, 201)
	m := wait("a change in the web app")
	if m.path != "/up/phone" || m.header.Get("Content-Encoding") != "aes128gcm" || m.header.Get("Topic") != topic ||
		!strings.HasPrefix(m.header.Get("Authorization"), "vapid t=") || m.header.Get("TTL") == "" {
		t.Errorf("the request: %s %v", m.path, m.header)
	}
	msg := openWebPush(t, m.body, ua, auth)
	if !strings.Contains(msg, "<P:topic>"+topic+"</P:topic>") || !strings.Contains(msg, "<P:content-update><D:sync-token>") || strings.Contains(msg, "Piscine") {
		t.Errorf("the message: %s", msg)
	}

	// A's own change: not pushed back to it.
	A.req("PUT", calPath+"a.ics", strings.Replace(judo, "judo-uid", "a-uid", 1))
	none("A's own change")
	// B's change, with Push-Dont-Notify naming A's registration: not pushed.
	B.req("PUT", calPath+"b.ics", strings.Replace(judo, "judo-uid", "b-uid", 1), "Push-Dont-Notify", `"`+loc+`"`)
	none("Push-Dont-Notify")
	// B's change: pushed.
	B.req("DELETE", calPath+"b.ics", "")
	wait("B's change")

	// A resource the push service says is gone: dropped.
	rg := register(B, ps.URL+"/up/gone")
	goneLoc := rg.header.Get("Location")
	A.req("PUT", calPath+"c.ics", strings.Replace(judo, "judo-uid", "c-uid", 1))
	if m := wait("to the gone resource"); m.path != "/up/gone" {
		t.Errorf("pushed to %s", m.path)
	}
	time.Sleep(200 * time.Millisecond)
	if s := B.req("DELETE", strings.TrimPrefix(goneLoc, x.public), "").status; s != http.StatusNotFound {
		t.Errorf("the gone registration is still there: %d", s)
	}

	// Unregistering: once, then unknown.
	path := strings.TrimPrefix(loc, x.public)
	if s := B.req("DELETE", path, "").status; s != http.StatusNoContent {
		t.Errorf("unregistering (the same user, another device): %d", s)
	}
	if s := A.req("DELETE", path, "").status; s != http.StatusNotFound {
		t.Errorf("unregistering again: %d", s)
	}
	L.json("POST", "/api/calendars/"+ref+"/events", map[string]any{"summary": "Après", "start": "2026-11-29T10:00"}, 201)
	none("after unregistering")
}

func between(s, from, to string) string {
	_, rest, ok := strings.Cut(s, from)
	if !ok {
		return ""
	}
	v, _, _ := strings.Cut(rest, to)
	return v
}

// openWebPush decrypts an aes128gcm Web Push body (RFC 8291) as a device would.
func openWebPush(t *testing.T, body []byte, ua *ecdh.PrivateKey, auth []byte) string {
	t.Helper()
	mac := func(k, d []byte) []byte { h := hmac.New(sha256.New, k); h.Write(d); return h.Sum(nil) }
	if len(body) < 86 || body[20] != 65 {
		t.Fatalf("not an aes128gcm body: %d bytes", len(body))
	}
	salt, asPub := body[:16], body[21:86]
	as, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	shared, _ := ua.ECDH(as)
	info := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asPub...)
	prk := mac(salt, mac(mac(auth, shared), append(info, 1)))
	block, _ := aes.NewCipher(mac(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16])
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, mac(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12], body[86:], nil)
	if err != nil {
		t.Fatalf("decrypting: %v", err)
	}
	return string(plain[:len(plain)-1])
}
