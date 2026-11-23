package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
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

	// No APK in a test build: no download, no App Links, nothing offered.
	for _, p := range []string{"/app/agendouille.apk", "/.well-known/assetlinks.json"} {
		if a := x.do("GET", x.public+p, nil, nil); a.status != http.StatusNotFound {
			t.Errorf("%s without an APK: %d", p, a.status)
		}
	}
}
