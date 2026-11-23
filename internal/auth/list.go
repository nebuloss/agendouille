package auth

import (
	"context"
	"database/sql"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// Session is one signed-in browser, as listed.
type Session struct {
	ID        int64     `json:"id"`
	UserAgent string    `json:"user_agent"`
	App       bool      `json:"app"` // the agendouille app's WebView
	CreatedAt time.Time `json:"created_at"`
	SeenAt    time.Time `json:"seen_at"`
}

// Sessions lists a user's live web sessions, the most recently seen first.
func (s *Service) Sessions(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_agent, device_id IS NOT NULL, created_at, seen_at FROM sessions
		WHERE user_id = ? AND surface = 'app' AND expires_at > ? ORDER BY seen_at DESC`, userID, store.Time(s.now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		var created, seen string
		if err := rows.Scan(&x.ID, &x.UserAgent, &x.App, &created, &seen); err != nil {
			return nil, err
		}
		x.CreatedAt, _ = store.ParseTime(created)
		x.SeenAt, _ = store.ParseTime(seen)
		out = append(out, x)
	}
	return out, rows.Err()
}

// DeviceCounts is the overview's view of devices.
type DeviceCounts struct {
	Devices int `json:"devices"`
	Stale   int `json:"stale"` // not seen for a week, or never
}

// CountDevices counts the live devices of active users.
func (s *Service) CountDevices(ctx context.Context) (DeviceCounts, error) {
	var c DeviceCounts
	week := store.Time(s.now().Add(-7 * 24 * time.Hour))
	var stale sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT count(*), sum(last_used_at IS NULL OR last_used_at < ?) FROM devices d
		JOIN users u ON u.id = d.user_id WHERE d.revoked_at IS NULL AND u.removed_at IS NULL`, week).Scan(&c.Devices, &stale)
	c.Stale = int(stale.Int64)
	return c, err
}

// StaleDevices lists, per user name, the devices not seen for a week.
type StaleDevice struct {
	User       string     `json:"user"`
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	Kind       string     `json:"kind"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func (s *Service) StaleDevices(ctx context.Context) ([]StaleDevice, error) {
	week := store.Time(s.now().Add(-7 * 24 * time.Hour))
	rows, err := s.db.QueryContext(ctx, `SELECT u.name, d.id, d.label, d.kind, d.last_used_at FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.revoked_at IS NULL AND u.removed_at IS NULL AND (d.last_used_at IS NULL OR d.last_used_at < ?) ORDER BY u.name, d.id`, week)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaleDevice{}
	for rows.Next() {
		var d StaleDevice
		var used sql.NullString
		if err := rows.Scan(&d.User, &d.ID, &d.Label, &d.Kind, &used); err != nil {
			return nil, err
		}
		if used.Valid {
			t, _ := store.ParseTime(used.String)
			d.LastUsedAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
