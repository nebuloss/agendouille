package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"math/big"
	"strings"
)

// newToken is n random bytes in base64url, for anything carried in a URL
// or a cookie: session ids, enrolment tokens, approval codes.
func newToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error()) // never fails on Linux; nothing sane to do if it does
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// hash is what the database keeps of a secret. SHA-256 is enough: every
// secret is at least 128 random bits, so there is nothing to brute-force
// and no reason for a slow hash on every request.
func hash(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

var lowerBase32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// newDeviceSecret is 20 random bytes (160 bits) as "agd_" and base32 in
// groups of four: easy to read off a screen into Thunderbird.
func newDeviceSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	raw := lowerBase32.EncodeToString(b) // 32 characters
	var groups []string
	for i := 0; i < len(raw); i += 4 {
		groups = append(groups, raw[i:i+4])
	}
	return "agd_" + strings.Join(groups, "-")
}

// normalizeDeviceSecret accepts a secret typed by hand: any case, with or
// without the dashes, with stray spaces.
func normalizeDeviceSecret(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "agd_")
	s = strings.NewReplacer("-", "", " ", "").Replace(s)
	return "agd_" + s
}

// shortCodeAlphabet has no 0/O, 1/I/L, 5/S, 8/B: a short code is read off
// one screen and typed on another.
const shortCodeAlphabet = "ACDEFGHJKMNPQRTUVWXY2345679"

// newShortCode is 6 characters (about 28 bits), shown as "KQ4-7XM". That is
// enough: a code lives two minutes, only a signed-in user can try it, and
// a wrong number cancels it.
func newShortCode() string {
	out := make([]byte, 6)
	max := big.NewInt(int64(len(shortCodeAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("crypto/rand: " + err.Error())
		}
		out[i] = shortCodeAlphabet[n.Int64()]
	}
	return string(out[:3]) + "-" + string(out[3:])
}

// normalizeShortCode accepts "kq4 7xm", "KQ47XM" and "KQ4-7XM" alike.
func normalizeShortCode(s string) string {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
	if len(s) != 6 {
		return s
	}
	return s[:3] + "-" + s[3:]
}

// randomInt is a uniform integer in [lo, hi].
func randomInt(lo, hi int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	if err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return lo + int(n.Int64())
}
