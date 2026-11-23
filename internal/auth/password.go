package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// The argon2id parameters of new hashes (PLAN.md 10). They are stored in
// the hash, so they can be raised later without locking the admin out.
const (
	argonTime    = 2
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

var errBadHash = errors.New("the stored admin password hash is malformed; set a new one with agendouille admin passwd")

// hashPassword is the PHC string "$argon2id$v=19$m=65536,t=2,p=1$salt$key".
func hashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

func checkPassword(password, stored string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, errBadHash
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m < 8*1024 || m > 1024*1024 || t < 1 || t > 16 || p < 1 {
		return false, errBadHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false, errBadHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) < 16 {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// derive runs one key derivation at a time at most twice over: each takes
// 64 MiB and most of a core, and the admin sign-in form is reachable
// before any authentication. It is the only guard on sign-in: there is no
// rate limit (DECISIONS.md 2026-11-23), but nobody can make the server
// derive more than two keys at once.
func (s *Service) derive(ctx context.Context, f func()) error {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.gate }()
	f()
	return nil
}
