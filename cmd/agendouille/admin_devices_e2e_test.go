package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/nebuloss/agendouille/internal/auth"
)

// The admin adds a device for someone and sees its manual settings once,
// and gives any device a client certificate, downloaded once.
func TestAdminDevicesAndCertificates(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	A := signInAdmin(t, x)

	// The stamp an open admin page polls changes with what it shows.
	before := A.json("GET", "/stamp", nil, 200)["stamp"]
	d := A.json("POST", "/users/lea/devices", map[string]any{"kind": "thunderbird", "label": "Portable de Léa"}, 201)
	if after := A.json("GET", "/stamp", nil, 200)["stamp"]; after == before || after == "" {
		t.Errorf("the stamp did not change with a new device: %v %v", before, after)
	}
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
	if a := A.call("POST", "/users/lea/devices", map[string]any{"kind": "toaster", "label": "x"}); a.status != http.StatusBadRequest {
		t.Errorf("a bad kind: %d", a.status)
	}

	// A certificate for it, downloaded once, with its password.
	id := fmtID(d["id"])
	c := A.json("POST", "/users/lea/devices/"+id+"/certificate", nil, 201)
	url, _ := c["certificate_url"].(string)
	if !strings.HasPrefix(url, "/api/admin/users/lea/devices/"+id+"/certificate.p12?t=") {
		t.Fatalf("certificate: %v", c)
	}
	f := x.do("GET", x.admin+url, nil, cookie(auth.AdminCookie, A.session))
	if f.status != 200 || f.header.Get("Content-Type") != "application/x-pkcs12" {
		t.Fatalf("the .p12: %d", f.status)
	}
	if _, _, err := pkcs12.Decode([]byte(f.body), c["certificate_password"].(string)); err != nil {
		t.Errorf("the .p12 does not open with its password: %v", err)
	}
	if again := x.do("GET", x.admin+url, nil, cookie(auth.AdminCookie, A.session)); again.status != http.StatusGone {
		t.Errorf("the .p12 twice: %d", again.status)
	}
	if devs := A.json("GET", "/users/lea", nil, 200); !strings.Contains(asJSON(devs["devices"]), `"has_certificate":true`) {
		t.Errorf("the device has no certificate after: %v", devs["devices"])
	}
	// In "keys" mode, a new device comes with its certificate.
	A.json("PUT", "/dav-auth", map[string]any{"mode": "keys"}, 200)
	k := A.json("POST", "/users/lea/devices", map[string]any{"kind": "davx5", "label": "Pixel"}, 201)
	if k["certificate_url"] == nil || k["dav_auth"] != "keys" {
		t.Errorf("keys mode: %v", k)
	}
	act := asJSON(A.json("GET", "/activity?kind=device", nil, 200))
	if !strings.Contains(act, "certificate issued") || !strings.Contains(act, "Portable de Léa") {
		t.Errorf("not in the activity log: %s", act)
	}
}
