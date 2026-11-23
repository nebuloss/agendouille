package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// Link is a read-only subscription link (PLAN.md 7), as listed.
type Link struct {
	ID          int64      `json:"id"`
	Label       string     `json:"label"`
	CalendarIDs []int64    `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// NewLink makes a link over calendars (ids the caller checked the user may
// read). The secret is 32 random bytes and is returned once.
func (s *Service) NewLink(ctx context.Context, userID int64, label string, calendarIDs []int64) (Link, string, error) {
	label = strings.TrimSpace(label)
	if label == "" || len(label) > 100 {
		return Link{}, "", fmt.Errorf("a link's label is 1 to 100 characters")
	}
	if len(calendarIDs) == 0 {
		return Link{}, "", fmt.Errorf("a link shows at least one calendar")
	}
	secret := newToken(32)
	now := s.now()
	var id int64
	err := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO links(user_id, label, secret_hash, created_at) VALUES (?, ?, ?, ?)`,
			userID, label, hash(secret), store.Time(now))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		for _, c := range calendarIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO link_calendars(link_id, calendar_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, c); err != nil {
				return err
			}
		}
		return nil
	})
	return Link{ID: id, Label: label, CalendarIDs: calendarIDs, CreatedAt: now}, secret, err
}

// Links lists a user's live links.
func (s *Service) Links(ctx context.Context, userID int64) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, created_at, last_used_at FROM links WHERE user_id = ? AND revoked_at IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	out := []Link{}
	for rows.Next() {
		var l Link
		var created string
		var used sql.NullString
		if err := rows.Scan(&l.ID, &l.Label, &created, &used); err != nil {
			rows.Close()
			return nil, err
		}
		l.CreatedAt, _ = store.ParseTime(created)
		if used.Valid {
			t, _ := store.ParseTime(used.String)
			l.LastUsedAt = &t
		}
		out = append(out, l)
	}
	rows.Close()
	for i := range out {
		if out[i].CalendarIDs, err = s.linkCalendars(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, rows.Err()
}

func (s *Service) linkCalendars(ctx context.Context, id int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT calendar_id FROM link_calendars WHERE link_id = ? ORDER BY calendar_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var c int64
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RevokeLink withdraws one of a user's links.
func (s *Service) RevokeLink(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE links SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, store.Time(s.now()), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalid
	}
	return nil
}

// OpenLink resolves a secret to its user and calendars. A revoked link, or
// one of a disabled or removed user, opens nothing.
func (s *Service) OpenLink(ctx context.Context, secret string) (userID int64, calendarIDs []int64, err error) {
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT l.id, l.user_id FROM links l JOIN users u ON u.id = l.user_id
		WHERE l.secret_hash = ? AND l.revoked_at IS NULL AND u.disabled = 0 AND u.removed_at IS NULL`, hash(secret)).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, ErrInvalid
	} else if err != nil {
		return 0, nil, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE links SET last_used_at = ? WHERE id = ?`, store.Time(s.now()), id)
	calendarIDs, err = s.linkCalendars(ctx, id)
	return userID, calendarIDs, err
}
