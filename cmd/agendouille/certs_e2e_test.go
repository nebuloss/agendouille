package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/nebuloss/agendouille/internal/auth"
)

// Device keys (DECISIONS: device keys): the admin switches CalDAV between
// password, both and keys; a device gets a certificate as a .p12 once; the
// certificate counts only from a trusted proxy, in X-Client-Cert.
func TestDeviceCertificates(t *testing.T) {
	x := startServer(t)
	// The test client is the proxy: 127.0.0.1 is trusted, as 10.0.60.2 is
	// at home.
	addr := freePort(t)
	x.configure("public_listen: " + addr + "\npublic_url: http://" + addr + "\npublic_allow_insecure: true\ntrusted_proxies: [127.0.0.1]\n")
	deadline := time.Now().Add(5 * time.Second)
	for a := x.srv.PublicAddr(); a == nil || a.String() != addr; a = x.srv.PublicAddr() {
		if time.Now().After(deadline) {
			t.Fatal("the public listener did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	x.public = "http://" + addr
	ctx := context.Background()
	x.svc.dir.AddUser(ctx, "cli", "lea", "Léa", "", nil)
	L := x.browser("lea")
	A := signInAdmin(t, x)

	// Password mode: no certificate is handed out.
	d0, _ := L.json("POST", "/api/devices", map[string]any{"kind": "davx5", "label": "Vieux téléphone"}, 201)
	if d0["certificate_url"] != nil || d0["dav_auth"] != "password" {
		t.Fatalf("password mode: %v", d0)
	}
	// The admin asks for both: the old device has no certificate.
	m := A.json("PUT", "/dav-auth", map[string]any{"mode": "both"}, 200)
	if m["mode"] != "both" || m["devices_without_certificate"] != float64(1) {
		t.Fatalf("mode: %v", m)
	}
	if nt := A.json("GET", "/notices", nil, 200); !strings.Contains(asJSON(nt), "devices_without_certificate") {
		t.Errorf("no notice for the device without a certificate: %v", nt)
	}

	// A new device comes with its certificate, downloadable once.
	d, _ := L.json("POST", "/api/devices", map[string]any{"kind": "davx5", "label": "Pixel"}, 201)
	certURL, _ := d["certificate_url"].(string)
	if certURL == "" || d["certificate_password"] == "" {
		t.Fatalf("no certificate: %v", d)
	}
	file := L.call("GET", certURL, nil)
	if file.status != 200 || file.header.Get("Content-Type") != "application/x-pkcs12" {
		t.Fatalf("the .p12: %d %s", file.status, file.header.Get("Content-Type"))
	}
	if again := L.call("GET", certURL, nil); again.status != http.StatusGone {
		t.Errorf("the .p12 twice: %d", again.status)
	}
	_, cert, err := pkcs12.Decode([]byte(file.body), d["certificate_password"].(string))
	if err != nil {
		t.Fatalf("decoding the .p12: %v", err)
	}
	header := url.QueryEscape(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})))
	dev := davClient{x, "lea", d["secret"].(string)}
	old := davClient{x, "lea", d0["secret"].(string)}
	propfind := func(c *davClient, withCert bool) int {
		t.Helper()
		return x.do("PROPFIND", x.public+"/dav/", strings.NewReader(""), func(r *http.Request) {
			r.Header.Set("Depth", "0")
			if c != nil {
				r.SetBasicAuth(c.user, c.pass)
			}
			if withCert {
				r.Header.Set(auth.ClientCertHeader, header)
			}
		}).status
	}
	// Both: the password and the certificate of the same device.
	if s := propfind(&dev, true); s != 207 {
		t.Errorf("both, password and certificate: %d", s)
	}
	if s := propfind(&dev, false); s != 401 {
		t.Errorf("both, the password alone: %d", s)
	}
	if s := propfind(&old, true); s != 401 {
		t.Errorf("both, another device's password with this certificate: %d", s)
	}
	// Keys: the certificate alone.
	A.json("PUT", "/dav-auth", map[string]any{"mode": "keys"}, 200)
	if s := propfind(nil, true); s != 207 {
		t.Errorf("keys, the certificate alone: %d", s)
	}
	if s := propfind(&dev, false); s != 401 {
		t.Errorf("keys, the password alone: %d", s)
	}
	// A forged header is not a certificate of ours.
	if s := x.do("PROPFIND", x.public+"/dav/", strings.NewReader(""), func(r *http.Request) {
		r.Header.Set("Depth", "0")
		r.Header.Set(auth.ClientCertHeader, url.QueryEscape("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"))
	}).status; s != 401 {
		t.Errorf("a forged certificate: %d", s)
	}
	// A new certificate replaces the old one.
	nc, _ := L.json("POST", "/api/devices/"+fmtID(d["id"])+"/certificate", nil, 201)
	if nc["certificate_url"] == nil {
		t.Fatalf("a new certificate: %v", nc)
	}
	if s := propfind(nil, true); s != 401 {
		t.Errorf("the replaced certificate still works: %d", s)
	}
	// Back to passwords: the certificate is not needed.
	A.json("PUT", "/dav-auth", map[string]any{"mode": "password"}, 200)
	if s := propfind(&dev, false); s != 207 {
		t.Errorf("password mode again: %d", s)
	}
	if a := A.call("PUT", "/dav-auth", map[string]any{"mode": "nonsense"}); a.status != http.StatusBadRequest {
		t.Errorf("a bad mode: %d", a.status)
	}
}
