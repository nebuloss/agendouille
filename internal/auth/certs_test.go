package auth

import (
	"context"
	"encoding/pem"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"software.sslmate.com/src/go-pkcs12"
)

// The CA is made once and read back; a certificate counts only from a
// trusted proxy (fromProxy), and in keys mode it is the whole sign-in.
func TestCertificates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	ca, err := LoadCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dir, "ca.key")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ca.key: %v %v", info, err)
	}
	again, err := LoadCA(dir)
	if err != nil || Fingerprint(again.Cert) != Fingerprint(ca.Cert) {
		t.Fatalf("the CA read back differs: %v", err)
	}
	f.auth.UseCA(ca)
	d, err := f.auth.NewDevice(ctx, f.lea.ID, "davx5", "Pixel", "readwrite")
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.auth.IssueCertificate(ctx, f.lea.ID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, cert, err := pkcs12.Decode(c.P12, c.Password)
	if err != nil || Fingerprint(cert) != c.Fingerprint {
		t.Fatalf("the .p12: %v", err)
	}
	if err := f.auth.SetDavAuthMode(ctx, "admin", DavKeys); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PROPFIND", "/dav/", nil)
	req.Header.Set(ClientCertHeader, url.QueryEscape(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))))
	if p, err := f.auth.Dav(req, "10.0.0.9", true); err != nil || p.DeviceID != d.ID {
		t.Fatalf("from the proxy: %v %v", p, err)
	}
	if _, err := f.auth.Dav(req, "10.0.0.9", false); err == nil {
		t.Error("a certificate header from an untrusted peer counted")
	}
	// Another user's device cannot get a certificate for this one.
	if _, err := f.auth.IssueCertificate(ctx, f.lea.ID+1000, d.ID); err == nil {
		t.Error("issued for someone else's device")
	}
	// Revoked, the device's certificate no longer signs in.
	if err := f.auth.RevokeDevice(ctx, "lea", f.lea.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Dav(req, "10.0.0.9", true); err == nil {
		t.Error("a revoked device's certificate still signs in")
	}
}
