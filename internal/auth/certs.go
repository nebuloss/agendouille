package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/nebuloss/agendouille/internal/store"
)

// Device keys for CalDAV (DECISIONS: device keys). A device can hold a
// client certificate issued by agendouille's own small CA, its private
// key generated here and handed over once as a .p12 file, never stored.
// TLS ends at the DMZ proxy, which asks the client for a certificate and
// passes it in ClientCertHeader on /dav/; agendouille reads that header
// only from a trusted proxy and verifies the certificate itself against
// its CA. The admin chooses how CalDAV signs a device in (DavAuthMode).

// ClientCertHeader is where the proxy passes the client certificate: PEM,
// URL-escaped (nginx's $ssl_client_escaped_cert).
const ClientCertHeader = "X-Client-Cert"

// Modes of CalDAV sign-in.
const (
	DavPassword = "password" // a device's password, as before
	DavBoth     = "both"     // its password and its certificate
	DavKeys     = "keys"     // its certificate alone
)

// DeviceCertLife is how long a device certificate lasts.
const DeviceCertLife = 5 * 365 * 24 * time.Hour

// CA is agendouille's certificate authority for device certificates.
type CA struct {
	Cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

// LoadCA reads the CA from dataDir (ca.pem, ca.key), making it the first
// time. The key is readable by the service only.
func LoadCA(dataDir string) (*CA, error) {
	certPath, keyPath := filepath.Join(dataDir, "ca.pem"), filepath.Join(dataDir, "ca.key")
	certPEM, err1 := os.ReadFile(certPath)
	keyPEM, err2 := os.ReadFile(keyPath)
	if errors.Is(err1, os.ErrNotExist) && errors.Is(err2, os.ErrNotExist) {
		return makeCA(certPath, keyPath)
	}
	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, fmt.Errorf("%s or %s is not PEM", certPath, keyPath)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	return newCA(cert, key), nil
}

func newCA(cert *x509.Certificate, key *ecdsa.PrivateKey) *CA {
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &CA{Cert: cert, key: key, pool: pool}
}

func makeCA(certPath, keyPath string) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "agendouille devices", Organization: []string{"agendouille"}},
		// A year back: a host whose clock is off, or a device certificate
		// dated by a test's clock, is still inside the CA's validity.
		NotBefore:             now.AddDate(-1, 0, 0),
		NotAfter:              now.Add(20 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, err
	}
	return newCA(cert, key), nil
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 126))
	return n
}

// Fingerprint is a certificate's SHA-256, hex.
func Fingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	return hex.EncodeToString(sum[:])
}

// UseCA lets the service issue and verify device certificates.
func (s *Service) UseCA(ca *CA) { s.ca = ca }

// DeviceCert is a device's new certificate, handed over once.
type DeviceCert struct {
	P12         []byte // the key and the certificate, for Android's or Thunderbird's store
	Password    string // the .p12's password
	Fingerprint string
	ExpiresAt   time.Time
}

// IssueCertificate makes a new key pair and certificate for a user's
// device, replacing the one it had, and returns them as a .p12.
func (s *Service) IssueCertificate(ctx context.Context, userID, deviceID int64) (DeviceCert, error) {
	if s.ca == nil {
		return DeviceCert{}, fmt.Errorf("no certificate authority")
	}
	var user, label string
	err := s.db.QueryRowContext(ctx, `SELECT u.name, d.label FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.id = ? AND d.user_id = ? AND d.revoked_at IS NULL`, deviceID, userID).Scan(&user, &label)
	if errors.Is(err, sql.ErrNoRows) {
		return DeviceCert{}, ErrInvalid
	} else if err != nil {
		return DeviceCert{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return DeviceCert{}, err
	}
	now := s.now()
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: user + " · " + label, Organization: []string{"agendouille"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(DeviceCertLife),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, s.ca.Cert, &key.PublicKey, s.ca.key)
	if err != nil {
		return DeviceCert{}, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return DeviceCert{}, err
	}
	password := strings.ToLower(newToken(10))
	p12, err := pkcs12.LegacyDES.Encode(key, cert, nil, password)
	if err != nil {
		return DeviceCert{}, err
	}
	fp := Fingerprint(cert)
	if _, err := s.db.ExecContext(ctx, `UPDATE devices SET cert_fingerprint = ?, cert_expires_at = ? WHERE id = ?`,
		fp, store.Time(cert.NotAfter), deviceID); err != nil {
		return DeviceCert{}, err
	}
	return DeviceCert{P12: p12, Password: password, Fingerprint: fp, ExpiresAt: cert.NotAfter}, nil
}

// DavAuthMode is how CalDAV signs a device in: password (the default),
// both, or keys.
func (s *Service) DavAuthMode(ctx context.Context) string {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM switches WHERE key = 'dav_auth'`).Scan(&v); err != nil {
		return DavPassword
	}
	if v != DavBoth && v != DavKeys {
		return DavPassword
	}
	return v
}

// SetDavAuthMode changes it, from the admin UI.
func (s *Service) SetDavAuthMode(ctx context.Context, actor, mode string) error {
	if mode != DavPassword && mode != DavBoth && mode != DavKeys {
		return fmt.Errorf("%w: dav_auth is password, both or keys", ErrInvalid)
	}
	now := store.Time(s.now())
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO switches(key, value, updated_at) VALUES ('dav_auth', ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, mode, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'setting', 'changed', ?)`,
			now, actor, "dav_auth -> "+mode)
		return err
	})
}

// DevicesWithoutCert counts the devices in use that have no certificate:
// they cannot sync once the mode needs one.
func (s *Service) DevicesWithoutCert(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.revoked_at IS NULL AND u.removed_at IS NULL AND (d.cert_fingerprint IS NULL OR d.cert_expires_at <= ?)`,
		store.Time(s.now())).Scan(&n)
	return n, err
}

// clientCert is the certificate the proxy saw, when the request came from
// a trusted proxy and the certificate is one of ours and current. "" when
// there is none: nothing in the header, or not from a trusted proxy.
func (s *Service) clientCert(r *http.Request, fromProxy bool) (string, error) {
	raw := r.Header.Get(ClientCertHeader)
	if raw == "" || !fromProxy || s.ca == nil {
		return "", nil
	}
	text, err := url.QueryUnescape(raw)
	if err != nil {
		return "", ErrInvalid
	}
	b, _ := pem.Decode([]byte(text))
	if b == nil {
		return "", ErrInvalid
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return "", ErrInvalid
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: s.ca.pool, CurrentTime: s.now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return "", ErrInvalid
	}
	return Fingerprint(cert), nil
}

// Dav signs a CalDAV request in, the way the admin chose (DavAuthMode):
// password, both (the password's device must hold the certificate), or
// keys (the certificate alone names the device). fromProxy says the TCP
// peer is a trusted proxy, the only one whose certificate header counts.
func (s *Service) Dav(r *http.Request, clientIP string, fromProxy bool) (*Principal, error) {
	ctx := r.Context()
	mode := s.DavAuthMode(ctx)
	fp, err := s.clientCert(r, fromProxy)
	if err != nil {
		return nil, err
	}
	switch mode {
	case DavBoth:
		if fp == "" {
			return nil, ErrInvalid
		}
		p, err := s.Basic(r, clientIP)
		if err != nil {
			return nil, err
		}
		var holder sql.NullString
		if err := s.db.QueryRowContext(ctx, `SELECT cert_fingerprint FROM devices WHERE id = ?`, p.DeviceID).Scan(&holder); err != nil || holder.String != fp {
			return nil, ErrInvalid
		}
		return p, nil
	case DavKeys:
		if fp == "" {
			return nil, ErrInvalid
		}
		return s.deviceByCert(r, fp, clientIP)
	default:
		return s.Basic(r, clientIP)
	}
}

// deviceByCert signs in the device holding the certificate fp.
func (s *Service) deviceByCert(r *http.Request, fp, clientIP string) (*Principal, error) {
	ctx := r.Context()
	var id, userID int64
	var cap string
	var lastUsed sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT d.id, d.user_id, d.cap, d.last_used_at FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.cert_fingerprint = ? AND d.revoked_at IS NULL AND u.disabled = 0 AND u.removed_at IS NULL`, fp).Scan(&id, &userID, &cap, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	return s.signedIn(r, id, userID, cap, lastUsed, clientIP)
}

// Audit records an action in the audit log (the admin's, done here
// rather than through a service of its own).
func (s *Service) Audit(ctx context.Context, actor, kind, action, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, ?, ?, ?)`,
		store.Time(s.now()), actor, kind, action, detail)
	return err
}

// RandomToken is a random string for a one-time link.
func RandomToken() string { return newToken(24) }

// CertState says what became of a request's client certificate, for the
// log of a refused CalDAV sign-in: never the certificate, only its fate.
func (s *Service) CertState(r *http.Request, fromProxy bool) string {
	raw := r.Header.Get(ClientCertHeader)
	switch {
	case raw == "" && !fromProxy:
		return "none (not through the proxy)"
	case raw == "":
		return "none (the client sent no certificate)"
	case !fromProxy:
		return "ignored (the header came from an untrusted peer)"
	case s.ca == nil:
		return "ignored (no certificate authority)"
	}
	text, err := url.QueryUnescape(raw)
	if err != nil {
		return "unreadable (bad escaping)"
	}
	b, _ := pem.Decode([]byte(text))
	if b == nil {
		return "unreadable (not PEM)"
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return "unreadable (" + err.Error() + ")"
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: s.ca.pool, CurrentTime: s.now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return "not ours (" + err.Error() + "), subject " + cert.Subject.CommonName
	}
	fp := Fingerprint(cert)
	var id int64
	var revoked sql.NullString
	if err := s.db.QueryRowContext(r.Context(), `SELECT id, revoked_at FROM devices WHERE cert_fingerprint = ?`, fp).Scan(&id, &revoked); err != nil {
		return "valid but held by no device (replaced by a newer one?), subject " + cert.Subject.CommonName
	}
	if revoked.Valid {
		return fmt.Sprintf("valid, of device %d, which is revoked", id)
	}
	return fmt.Sprintf("valid, of device %d", id)
}
