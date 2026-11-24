package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/auth"
)

// The admin adds a device for someone and sees its manual settings once;
// CalDAV signs it in with its password, the only way (DECISIONS: no device
// certificates).
func TestAdminDevices(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	A := signInAdmin(t, x)

	d := A.json("POST", "/users/lea/devices", map[string]any{"kind": "thunderbird", "label": "Portable de Léa"}, 201)
	if d["username"] != "lea" || d["secret"] == "" || !strings.HasSuffix(d["server"].(string), "/dav/") || d["certificate_url"] != nil {
		t.Fatalf("a device added by the admin: %v", d)
	}
	// Its password works on CalDAV.
	if s := x.do("PROPFIND", x.public+"/dav/", strings.NewReader(""), func(r *http.Request) {
		r.Header.Set("Depth", "0")
		r.SetBasicAuth("lea", d["secret"].(string))
	}).status; s != 207 {
		t.Errorf("the admin's device on CalDAV: %d", s)
	}
	// A client certificate header alone signs nobody in any more.
	if s := x.do("PROPFIND", x.public+"/dav/", strings.NewReader(""), func(r *http.Request) {
		r.Header.Set("Depth", "0")
		r.Header.Set("X-Client-Cert", "-----BEGIN%20CERTIFICATE-----")
	}).status; s != http.StatusUnauthorized {
		t.Errorf("a certificate header alone: %d", s)
	}
	if a := A.call("POST", "/users/lea/devices", map[string]any{"kind": "toaster", "label": "x"}); a.status != http.StatusBadRequest {
		t.Errorf("a bad kind: %d", a.status)
	}
	// The certificate and sign-in mode routes are gone.
	id := fmtID(d["id"])
	for _, p := range []string{"/users/lea/devices/" + id + "/certificate", "/dav-auth"} {
		if a := x.do("POST", x.admin+"/api/admin"+p, nil, cookie(auth.AdminCookie, A.session)); a.status != http.StatusNotFound && a.status != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d, want it gone", p, a.status)
		}
	}
	if devs := A.json("GET", "/users/lea", nil, 200); strings.Contains(asJSON(devs), "certificate") || strings.Contains(asJSON(devs), "dav_auth") {
		t.Errorf("a user's page still speaks of certificates: %v", devs)
	}
	act := asJSON(A.json("GET", "/activity?kind=device", nil, 200))
	if !strings.Contains(act, "Portable de Léa") {
		t.Errorf("not in the activity log: %s", act)
	}
}
