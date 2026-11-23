package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// Enrolment is a one-time sign-in link for a user's first device (PLAN.md
// 7.2, 8.1). Token is in the URL and shown once.
type Enrolment struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"-"`
	Token     string     `json:"-"`
	MadeBy    string     `json:"made_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// NewEnrolment makes a link for a user. madeBy is "admin", "cli", or the
// user's own name when they enrol another phone of theirs. ttl 0 takes the
// enrolment_link_ttl_hours setting.
func (s *Service) NewEnrolment(ctx context.Context, madeBy string, userID int64, ttl time.Duration) (Enrolment, error) {
	if ttl == 0 {
		ttl = s.settings.Get().EnrolmentLinkTTL()
	}
	u, err := s.dir.UserByID(ctx, userID)
	if err != nil || !u.Active() {
		return Enrolment{}, ErrInvalid
	}
	token := newToken(32)
	now := s.now()
	var id int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO enrolments(user_id, token_hash, made_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
			userID, hash(token), madeBy, store.Time(now), store.Time(now.Add(ttl)))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		_, err = tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'enrolment', 'made', ?)`,
			store.Time(now), madeBy, u.Name)
		return err
	})
	if err != nil {
		return Enrolment{}, err
	}
	return Enrolment{ID: id, UserID: userID, Token: token, MadeBy: madeBy, CreatedAt: now, ExpiresAt: now.Add(ttl)}, nil
}

// redeem marks an enrolment used and returns its user. Used, expired and
// unknown links are told apart, so the page can say what happened: the
// token is 256 random bits, so that tells an attacker nothing.
func (s *Service) redeem(ctx context.Context, tx *sql.Tx, token string) (enrolmentID, userID int64, err error) {
	var expires string
	var used, revoked sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, user_id, expires_at, used_at, revoked_at FROM enrolments WHERE token_hash = ?`, hash(token)).
		Scan(&enrolmentID, &userID, &expires, &used, &revoked)
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
	res, err := tx.ExecContext(ctx, `UPDATE enrolments SET used_at = ? WHERE id = ? AND used_at IS NULL`, store.Time(now), enrolmentID)
	if err != nil {
		return 0, 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, 0, ErrUsed // a concurrent redeem won
	}
	return enrolmentID, userID, nil
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

// RedeemForApp uses a link to enrol the agendouille app: it makes a device
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
		_, err = tx.ExecContext(ctx, `UPDATE enrolments SET used_by_device = ? WHERE id = ?`, out.ID, eid)
		return err
	})
	return out, err
}

// Enrolments lists a user's links, newest first, used ones included.
func (s *Service) Enrolments(ctx context.Context, userID int64) ([]Enrolment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, made_by, created_at, expires_at, used_at FROM enrolments
		WHERE user_id = ? AND revoked_at IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Enrolment{}
	for rows.Next() {
		var e Enrolment
		var created, expires string
		var used sql.NullString
		if err := rows.Scan(&e.ID, &e.UserID, &e.MadeBy, &created, &expires, &used); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = store.ParseTime(created)
		e.ExpiresAt, _ = store.ParseTime(expires)
		if used.Valid {
			t, _ := store.ParseTime(used.String)
			e.UsedAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RevokeEnrolment withdraws a link that was not used yet.
func (s *Service) RevokeEnrolment(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE enrolments SET revoked_at = ? WHERE id = ? AND user_id = ? AND used_at IS NULL AND revoked_at IS NULL`,
		store.Time(s.now()), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalid
	}
	return nil
}
