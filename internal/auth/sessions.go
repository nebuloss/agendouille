package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// newSession stores a session and returns its secret. userID is 0 for the
// admin; deviceID is set for the app's WebView, whose session ends with
// its device.
func (s *Service) newSession(ctx context.Context, surface string, userID, deviceID int64, userAgent string, ttl time.Duration) (string, error) {
	secret := newToken(32)
	now := s.now()
	var uid, did any
	if userID != 0 {
		uid = userID
	}
	if deviceID != 0 {
		did = deviceID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(surface, user_id, device_id, token_hash, user_agent, created_at, seen_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, surface, uid, did, hash(secret), truncate(userAgent, 200),
		store.Time(now), store.Time(now), store.Time(now.Add(ttl)))
	if err != nil {
		return "", err
	}
	return secret, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Session resolves the request's cookie for surface: agd_app on the app
// surface, agd_admin on the admin surface. A cookie of the other surface is
// ignored, never upgraded. App sessions are renewed by use, at most once
// an hour; admin sessions are not.
func (s *Service) Session(r *http.Request, surface string) (*Principal, error) {
	name := AppCookie
	if surface == SurfaceAdmin {
		name = AdminCookie
	}
	c, err := r.Cookie(name)
	if err != nil || c.Value == "" {
		return nil, ErrInvalid
	}
	return s.lookupSession(r.Context(), surface, c.Value)
}

func (s *Service) lookupSession(ctx context.Context, surface, secret string) (*Principal, error) {
	var (
		id                 int64
		userID, deviceID   sql.NullInt64
		expires, seen      string
		deviceCap          sql.NullString
		deviceRevoked      sql.NullString
		deviceIsGone       bool
		disabled, inactive bool
	)
	err := s.db.QueryRowContext(ctx, `SELECT s.id, s.user_id, s.device_id, s.expires_at, s.seen_at,
			d.cap, d.revoked_at, (s.device_id IS NOT NULL AND d.id IS NULL),
			COALESCE(u.disabled, 0), (u.removed_at IS NOT NULL)
		FROM sessions s LEFT JOIN users u ON u.id = s.user_id LEFT JOIN devices d ON d.id = s.device_id
		WHERE s.token_hash = ? AND s.surface = ?`, hash(secret), surface).
		Scan(&id, &userID, &deviceID, &expires, &seen, &deviceCap, &deviceRevoked, &deviceIsGone, &disabled, &inactive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	now := s.now()
	exp, _ := store.ParseTime(expires)
	if !now.Before(exp) || disabled || inactive || deviceRevoked.Valid || deviceIsGone {
		return nil, ErrInvalid
	}
	p := &Principal{Surface: surface, SessionID: id, DeviceCap: "readwrite"}
	if surface == SurfaceAdmin {
		p.Admin = true
		return p, nil
	}
	u, err := s.dir.UserByID(ctx, userID.Int64)
	if err != nil {
		return nil, ErrInvalid
	}
	p.User = u
	if deviceID.Valid {
		p.DeviceID = deviceID.Int64
		p.DeviceCap = deviceCap.String
	}
	if last, _ := store.ParseTime(seen); now.Sub(last) > time.Hour {
		ttl := s.settings.Get().SessionTTL()
		_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET seen_at = ?, expires_at = ? WHERE id = ?`,
			store.Time(now), store.Time(now.Add(ttl)), id)
		s.dir.Touch(ctx, u.ID)
	}
	return p, nil
}

// SignOut ends the session behind the request's cookie for surface.
func (s *Service) SignOut(r *http.Request, surface string) {
	name := AppCookie
	if surface == SurfaceAdmin {
		name = AdminCookie
	}
	if c, err := r.Cookie(name); err == nil {
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = ? AND surface = ?`, hash(c.Value), surface)
	}
}

// RevokeSession ends one of a user's sessions (the user's own page, or the
// admin's view of them).
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, sessionID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalid
	}
	return nil
}

// SetCookie gives the browser its session for surface. HttpOnly: no
// script needs the secret. The app's is Lax, so a link from elsewhere lands
// signed in (cross-site writes are refused by the same-origin check); the
// admin's is Strict and lives as long as its session.
func SetCookie(w http.ResponseWriter, surface, secret string, secure bool, appTTL time.Duration) {
	c := &http.Cookie{Name: AppCookie, Value: secret, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(appTTL.Seconds())}
	if surface == SurfaceAdmin {
		c.Name, c.SameSite, c.MaxAge = AdminCookie, http.SameSiteStrictMode, int(AdminSessionTTL.Seconds())
	}
	http.SetCookie(w, c)
}

// ClearCookie removes the browser's session cookie for surface.
func ClearCookie(w http.ResponseWriter, surface string, secure bool) {
	c := &http.Cookie{Name: AppCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode}
	if surface == SurfaceAdmin {
		c.Name, c.SameSite = AdminCookie, http.SameSiteStrictMode
	}
	http.SetCookie(w, c)
}
