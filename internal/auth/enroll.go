package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// Enrollment is a one-time sign-in link for a user's first device (PLAN.md
// 7.2, 8.1). Token is in the URL and shown once.
type Enrollment struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"-"`
	Token     string     `json:"-"`
	Kind      string     `json:"kind"` // phone, browser
	MadeBy    string     `json:"made_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	// State is pending, used or expired (a cancelled one is not listed).
	State string `json:"state"`
}

// BrowserLinkTTL is how long a link made to sign a browser in lasts: it is
// for testing, opened at once.
const BrowserLinkTTL = time.Hour

// NewEnrollment makes a link for a user. madeBy is "admin", "cli", or the
// user's own name when they enroll another phone of theirs. ttl 0 takes the
// enrollment_link_ttl_hours setting.
func (s *Service) NewEnrollment(ctx context.Context, madeBy string, userID int64, ttl time.Duration) (Enrollment, error) {
	return s.NewEnrollmentKind(ctx, madeBy, userID, ttl, "phone")
}

// NewEnrollmentKind is NewEnrollment for a kind of link: "phone" (a QR for a
// phone's camera) or "browser" (to sign a browser in, for testing). The
// token is kept while the link is pending, so it can be shown again.
func (s *Service) NewEnrollmentKind(ctx context.Context, madeBy string, userID int64, ttl time.Duration, kind string) (Enrollment, error) {
	if kind != "phone" && kind != "browser" {
		return Enrollment{}, ErrInvalid
	}
	if ttl == 0 && kind == "browser" {
		ttl = BrowserLinkTTL
	}
	if ttl == 0 {
		ttl = s.settings.Get().EnrollmentLinkTTL()
	}
	u, err := s.dir.UserByID(ctx, userID)
	if err != nil || !u.Active() {
		return Enrollment{}, ErrInvalid
	}
	token := newToken(32)
	now := s.now()
	var id int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO enrollments(user_id, token_hash, token, kind, made_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			userID, hash(token), token, kind, madeBy, store.Time(now), store.Time(now.Add(ttl)))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		_, err = tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'enrollment', 'made', ?)`,
			store.Time(now), madeBy, u.Name)
		return err
	})
	if err != nil {
		return Enrollment{}, err
	}
	return Enrollment{ID: id, UserID: userID, Token: token, Kind: kind, MadeBy: madeBy, CreatedAt: now, ExpiresAt: now.Add(ttl), State: "pending"}, nil
}

// redeem marks an enrollment used and returns its user. Used, expired and
// unknown links are told apart, so the page can say what happened: the
// token is 256 random bits, so that tells an attacker nothing.
func (s *Service) redeem(ctx context.Context, tx *sql.Tx, token string) (enrollmentID, userID int64, err error) {
	var expires string
	var used, revoked sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, user_id, expires_at, used_at, revoked_at FROM enrollments WHERE token_hash = ?`, hash(token)).
		Scan(&enrollmentID, &userID, &expires, &used, &revoked)
	if errors.Is(err, sql.ErrNoRows) || revoked.Valid {
		return 0, 0, ErrInvalid
	} else if err != nil {
		return 0, 0, err
	}
	if used.Valid {
		return 0, 0, ErrUsed
	}
	now := s.now()
	if exp, _ := store.ParseTime(expires); !now.Before(exp) {
		return 0, 0, ErrExpired
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT (disabled = 0 AND removed_at IS NULL) FROM users WHERE id = ?`, userID).Scan(&active); err != nil || active == 0 {
		return 0, 0, ErrInvalid
	}
	res, err := tx.ExecContext(ctx, `UPDATE enrollments SET used_at = ?, token = NULL WHERE id = ? AND used_at IS NULL`, store.Time(now), enrollmentID)
	if err != nil {
		return 0, 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, 0, ErrUsed // a concurrent redeem won
	}
	return enrollmentID, userID, nil
}

// RedeemForBrowser uses a link to sign a browser in (an iPhone, or a phone
// without the app). It returns the session secret for the agd_app cookie.
func (s *Service) RedeemForBrowser(ctx context.Context, token, userAgent string) (string, error) {
	var userID int64
	err := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		_, userID, err = s.redeem(ctx, tx, token)
		return err
	})
	if err != nil {
		return "", err
	}
	return s.newSession(ctx, SurfaceApp, userID, 0, userAgent, s.settings.Get().SessionTTL())
}

// RedeemForApp uses a link to enroll the agendouille app: it makes a device
// of kind app and returns its credential, once (CONTRACTS.md 2.2).
func (s *Service) RedeemForApp(ctx context.Context, token, model string) (DeviceSecret, error) {
	if model == "" {
		model = "Android phone"
	}
	var out DeviceSecret
	err := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		eid, userID, err := s.redeem(ctx, tx, token)
		if err != nil {
			return err
		}
		out, err = s.newDeviceTx(ctx, tx, userID, "app", truncate(model, 100), "readwrite")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE enrollments SET used_by_device = ? WHERE id = ?`, out.ID, eid)
		return err
	})
	return out, err
}

// Enrollments lists a user's links, newest first, used ones included.
func (s *Service) Enrollments(ctx context.Context, userID int64) ([]Enrollment, error) {
	// An expired link's token is no use to anyone: forget it.
	if _, err := s.db.ExecContext(ctx, `UPDATE enrollments SET token = NULL WHERE user_id = ? AND token IS NOT NULL AND expires_at <= ?`,
		userID, store.Time(s.now())); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, kind, made_by, created_at, expires_at, used_at FROM enrollments
		WHERE user_id = ? AND revoked_at IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Enrollment{}
	for rows.Next() {
		var e Enrollment
		var created, expires string
		var used sql.NullString
		if err := rows.Scan(&e.ID, &e.UserID, &e.Kind, &e.MadeBy, &created, &expires, &used); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = store.ParseTime(created)
		e.ExpiresAt, _ = store.ParseTime(expires)
		e.State = "pending"
		if used.Valid {
			t, _ := store.ParseTime(used.String)
			e.UsedAt = &t
			e.State = "used"
		} else if !s.now().Before(e.ExpiresAt) {
			e.State = "expired"
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RevokeEnrollment withdraws a link that was not used yet.
func (s *Service) RevokeEnrollment(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE enrollments SET revoked_at = ?, token = NULL WHERE id = ? AND user_id = ? AND used_at IS NULL AND revoked_at IS NULL`,
		store.Time(s.now()), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalid
	}
	return nil
}

// PendingEnrollment is a link still waiting, with its token, to show again.
// A used, cancelled or expired one is ErrUsed, ErrInvalid or ErrExpired.
func (s *Service) PendingEnrollment(ctx context.Context, userID, id int64) (Enrollment, error) {
	var e Enrollment
	var created, expires string
	var token, used, revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, kind, made_by, created_at, expires_at, used_at, revoked_at, token FROM enrollments
		WHERE id = ? AND user_id = ?`, id, userID).Scan(&e.ID, &e.UserID, &e.Kind, &e.MadeBy, &created, &expires, &used, &revoked, &token)
	if errors.Is(err, sql.ErrNoRows) || revoked.Valid {
		return Enrollment{}, ErrInvalid
	} else if err != nil {
		return Enrollment{}, err
	}
	if used.Valid {
		return Enrollment{}, ErrUsed
	}
	e.CreatedAt, _ = store.ParseTime(created)
	e.ExpiresAt, _ = store.ParseTime(expires)
	if !s.now().Before(e.ExpiresAt) || !token.Valid {
		_, _ = s.db.ExecContext(ctx, `UPDATE enrollments SET token = NULL WHERE id = ?`, id)
		return Enrollment{}, ErrExpired
	}
	e.Token, e.State = token.String, "pending"
	return e, nil
}
