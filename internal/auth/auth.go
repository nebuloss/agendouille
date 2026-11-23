// Package auth signs people and devices in (PLAN.md 8, 10): the admin's
// password and sessions, users' web sessions, the admin's one-time
// enrolment links, approvals of a browser from a signed-in device, and the
// device credentials CalDAV clients use.
//
// No user has a password, and nothing here sends email. Every secret but
// the admin password is random and long (128 bits or more), so none is
// rate limited: there is nothing to guess. The admin password is guarded
// by argon2 and by the admin listener staying on the LAN.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

// Surfaces a session belongs to. A session of one never opens the other.
const (
	SurfaceApp   = "app"
	SurfaceAdmin = "admin"
)

const (
	// AppCookie is the family web app's session; AdminCookie the admin UI's.
	AppCookie   = "agd_app"
	AdminCookie = "agd_admin"
	// AdminSessionTTL is fixed: an admin session is not renewed by use.
	AdminSessionTTL = 12 * time.Hour
	// MinAdminPassword counts characters, not bytes.
	MinAdminPassword = 8
)

var (
	// ErrInvalid is any credential that does not open anything: unknown,
	// revoked, of a disabled user. One error, so answers tell nothing.
	ErrInvalid = errors.New("invalid credential")
	// ErrWrongPassword is a password that does not match, or one tried
	// while none is set.
	ErrWrongPassword = errors.New("wrong password")
	ErrShortPassword = fmt.Errorf("the admin password needs at least %d characters", MinAdminPassword)
	ErrExpired       = errors.New("expired")
	ErrUsed          = errors.New("already used")
	ErrWrongNumber   = errors.New("wrong number: the sign-in was cancelled")
	ErrBusy          = errors.New("too many sign-ins waiting; try again in a minute")
	ErrTooMany       = errors.New("device limit reached")
)

// Principal is who a request acts as.
type Principal struct {
	User      directory.User // zero for the admin
	Admin     bool
	DeviceID  int64  // the device of a CalDAV request or of the app's WebView
	DeviceCap string // "readwrite" or "read"; "readwrite" for a browser
	Surface   string // SurfaceApp, SurfaceAdmin, or "dav"
	SessionID int64
}

// Service is the authentication state, all of it in the database.
type Service struct {
	ca       *CA // device certificates; nil until UseCA
	db       *sql.DB
	now      func() time.Time
	settings *settings.Service
	dir      *directory.Service
	gate     chan struct{}

	mu      sync.Mutex
	waiters map[string]chan struct{} // approval id -> closed when it is decided
}

// New wraps the database.
func New(db *sql.DB, now func() time.Time, set *settings.Service, dir *directory.Service) *Service {
	// ca is set by UseCA: device certificates (certs.go).
	return &Service{db: db, now: now, settings: set, dir: dir, gate: make(chan struct{}, 2), waiters: map[string]chan struct{}{}}
}

// HasAdminPassword reports whether the admin password is set. serve
// refuses to start without one.
func (s *Service) HasAdminPassword(ctx context.Context) (bool, error) {
	h, err := s.adminHash(ctx)
	return h != "", err
}

func (s *Service) adminHash(ctx context.Context) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM admin WHERE id = 1`).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return h, err
}

// SetAdminPassword replaces the admin password without the old one (the
// shell is the recovery path) and ends every admin session.
func (s *Service) SetAdminPassword(ctx context.Context, actor, password string) error {
	if utf8.RuneCountInString(password) < MinAdminPassword {
		return ErrShortPassword
	}
	var h string
	if err := s.derive(ctx, func() { h = hashPassword(password) }); err != nil {
		return err
	}
	now := store.Time(s.now())
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO admin(id, password_hash, updated_at) VALUES (1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at`, h, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE surface = 'admin'`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'admin', 'password', '')`, now, actor)
		return err
	})
}

// RandomPassword is a password for install.sh to print once: 24 base32
// characters (120 bits) in six groups of four.
func RandomPassword() string {
	return newDeviceSecret()[4 : 4+29]
}

// AdminSignIn checks the password and opens an admin session. It costs one
// derivation whatever happens, so timing does not tell whether a password
// is set.
func (s *Service) AdminSignIn(ctx context.Context, password, userAgent string) (secret string, err error) {
	stored, err := s.adminHash(ctx)
	if err != nil {
		return "", err
	}
	match := false
	var cerr error
	if err := s.derive(ctx, func() {
		if stored == "" {
			hashPassword(password)
			return
		}
		match, cerr = checkPassword(password, stored)
	}); err != nil {
		return "", err
	}
	if cerr != nil {
		return "", cerr
	}
	if !match {
		return "", ErrWrongPassword
	}
	return s.newSession(ctx, SurfaceAdmin, 0, 0, userAgent, AdminSessionTTL)
}
