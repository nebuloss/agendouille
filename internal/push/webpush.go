package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Web Push by hand, as the rest of the protocols here (DECISIONS: no
// library): the message encryption of RFC 8291 (aes128gcm, RFC 8188) and
// the VAPID header of RFC 8292. Both are short, and their RFCs carry test
// vectors (webpush_test.go).

// recordSize is the aes128gcm record size: one record holds any message here.
const recordSize = 4096

// b64 is the base64url of Web Push, without padding.
var b64 = base64.RawURLEncoding

// decode64 reads base64url with or without padding, or standard base64:
// clients are not all strict.
func decode64(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	s = strings.NewReplacer("+", "-", "/", "_").Replace(s)
	return b64.DecodeString(s)
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// encrypt seals plain for the subscription's key (uaPublic, 65 bytes
// uncompressed) and auth secret (16 bytes), with the server's one-time key
// asPrivate and salt (16 random bytes): RFC 8291 section 3.4. The answer is
// the request body, its aes128gcm header included.
func encrypt(plain, uaPublic, authSecret []byte, asPrivate *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(authSecret) != 16 || len(salt) != 16 {
		return nil, errors.New("webpush: the auth secret and the salt are 16 bytes")
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("webpush: the subscription's key: %w", err)
	}
	shared, err := asPrivate.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPublic := asPrivate.PublicKey().Bytes()
	// IKM = HKDF(auth_secret, ecdh_secret, "WebPush: info" 0 ua_public as_public, 32)
	prkKey := hmacSHA256(authSecret, shared)
	info := append([]byte("WebPush: info\x00"), uaPublic...)
	info = append(info, asPublic...)
	ikm := hmacSHA256(prkKey, append(info, 1))
	// CEK and NONCE from HKDF(salt, IKM, ...): RFC 8188 section 2.2.
	prk := hmacSHA256(salt, ikm)
	cek := hmacSHA256(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16]
	nonce := hmacSHA256(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12]
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// One record, the last: its delimiter is 2, no padding.
	if len(plain)+1+gcm.Overhead() > recordSize {
		return nil, errors.New("webpush: message too long")
	}
	header := make([]byte, 0, 21+len(asPublic))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, recordSize)
	header = append(header, byte(len(asPublic)))
	header = append(header, asPublic...)
	return gcm.Seal(header, nonce, append(append([]byte{}, plain...), 2), nil), nil
}

// seal is encrypt with a fresh key and salt.
func seal(plain, uaPublic, authSecret []byte) ([]byte, error) {
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return encrypt(plain, uaPublic, authSecret, as, salt)
}

// vapid is the Authorization header for a request to endpoint (RFC 8292):
// a JWT signed with the server's key, for the endpoint's origin, and the
// public key itself.
func vapid(key *ecdsa.PrivateKey, endpoint, subject string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": now.Add(12 * time.Hour).Unix(),
		"sub": subject,
	})
	if err != nil {
		return "", err
	}
	unsigned := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	pub, err := publicBytes(key)
	if err != nil {
		return "", err
	}
	return "vapid t=" + unsigned + "." + b64.EncodeToString(sig) + ", k=" + b64.EncodeToString(pub), nil
}

// publicBytes is a P-256 key's public half, uncompressed (65 bytes).
func publicBytes(key *ecdsa.PrivateKey) ([]byte, error) {
	k, err := key.ECDH()
	if err != nil {
		return nil, err
	}
	return k.PublicKey().Bytes(), nil
}
