package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/web/assets"
)

// A DAVx5 device's QR code leads to a page of ours that opens DAVx5 (DAVx5
// has no scanner, a camera app does not open davx5://); the app's pieces
// answer as a build without the APK should.
func TestDAVx5PageAndAppPieces(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	L := x.browser("lea")
	d, _ := L.json("POST", "/api/devices", map[string]any{"kind": "davx5", "label": "Pixel"}, 201)
	page, _ := d["davx5_page"].(string)
	if !strings.Contains(page, "/davx5/") || d["qr_svg"] == nil {
		t.Fatalf("no page for the QR code: %v", d)
	}
	path := page[strings.Index(page, "/davx5/"):]
	a := x.do("GET", x.public+path, nil, nil) // the phone has no session
	if a.status != 200 || !strings.Contains(a.body, `href="davx5://lea:`) || !strings.Contains(a.body, "Ouvrir dans DAVx5") {
		t.Errorf("the DAVx5 page: %d %s", a.status, a.body)
	}
	if a.header.Get("Cache-Control") != "no-store" {
		t.Errorf("the page with a password is cacheable: %q", a.header.Get("Cache-Control"))
	}
	if a := x.do("GET", x.public+"/davx5/nope", nil, nil); a.status != http.StatusGone {
		t.Errorf("an unknown token: %d", a.status)
	}

	// The agendouille app scans the same code: it asks for the link the page holds.
	token := path[len("/davx5/"):]
	hdr := func(r *http.Request) { r.Header.Set("Content-Type", "application/json") }
	if a := x.do("POST", x.public+"/api/app/davx5", strings.NewReader(`{"token":"`+token+`"}`), hdr); a.status != 200 ||
		!strings.Contains(a.body, `"link":"davx5://lea:`) || a.header.Get("Cache-Control") != "no-store" {
		t.Errorf("the link for the app: %d %s", a.status, a.body)
	}
	if a := x.do("POST", x.public+"/api/app/davx5", strings.NewReader(`{"token":"nope"}`), hdr); a.status != http.StatusGone {
		t.Errorf("the app, an unknown token: %d %s", a.status, a.body)
	}

	// The download and the App Links exist exactly when the build embeds
	// the APK: not in a plain test build, yes in a release (release.yml
	// embeds it before the tests run).
	want := http.StatusNotFound
	if assets.AndroidApp() != nil {
		want = http.StatusOK
	}
	for _, p := range []string{"/app/agendouille.apk", "/.well-known/assetlinks.json"} {
		if a := x.do("GET", x.public+p, nil, nil); a.status != want {
			t.Errorf("%s: %d, want %d (APK embedded: %v)", p, a.status, want, want == http.StatusOK)
		}
	}

	// The sign-in page on a phone: never the QR code (a phone cannot scan
	// its own screen); on Android, the app alone when the server has it.
	android := func(r *http.Request) {
		r.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14; 23078PND5G) Mobile")
	}
	iphone := func(r *http.Request) {
		r.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Mobile")
	}
	a = x.do("GET", x.public+"/", nil, android)
	if assets.AndroidApp() != nil {
		if !strings.Contains(a.body, "/app/agendouille.apk") || strings.Contains(a.body, "signin-short-code") || strings.Contains(a.body, "signin-qr") {
			t.Errorf("Android, the app embedded: the download only, got %s", a.body)
		}
	} else if strings.Contains(a.body, "signin-qr") || !strings.Contains(a.body, "signin-short-code") {
		t.Errorf("Android, no app: the code without the QR code, got %s", a.body)
	}
	if a := x.do("GET", x.public+"/", nil, iphone); strings.Contains(a.body, "signin-qr") || !strings.Contains(a.body, "signin-short-code") {
		t.Errorf("iPhone: the code without the QR code, got %s", a.body)
	}
	if a := x.do("GET", x.public+"/", nil, nil); !strings.Contains(a.body, "signin-qr") {
		t.Errorf("desktop: the QR code, got %s", a.body)
	}
}
