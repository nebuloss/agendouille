package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// Device kinds (CONTRACTS.md 5).
var deviceKinds = map[string]bool{"app": true, "davx5": true, "ios": true, "thunderbird": true, "other": true}

// Device is one sync credential, as listed (never with its secret).
type Device struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"-"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"`
	Cap        string     `json:"cap"`
	Client     string     `json:"client"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	LastIP     string     `json:"last_ip,omitempty"`
}

// DeviceSecret is a new device with its secret, shown once.
type DeviceSecret struct {
	Device
	Username string `json:"username"`
	Secret   string `json:"secret"`
}

// NewDevice makes a sync credential for a user.
func (s *Service) NewDevice(ctx context.Context, userID int64, kind, label, cap string) (DeviceSecret, error) {
	if !deviceKinds[kind] {
		return DeviceSecret{}, fmt.Errorf("device kind %q is not one of app, davx5, ios, thunderbird, other", kind)
	}
	if cap != "readwrite" && cap != "read" {
		return DeviceSecret{}, fmt.Errorf("device cap %q is not readwrite or read", cap)
	}
	label = strings.TrimSpace(label)
	if label == "" || len(label) > 100 {
		return DeviceSecret{}, fmt.Errorf("a device label is 1 to 100 characters")
	}
	var out DeviceSecret
	err := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		d, err := s.newDeviceTx(ctx, tx, userID, kind, label, cap)
		out = d
		return err
	})
	return out, err
}

func (s *Service) newDeviceTx(ctx context.Context, tx *sql.Tx, userID int64, kind, label, cap string) (DeviceSecret, error) {
	var name string
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT name, (disabled = 0 AND removed_at IS NULL) FROM users WHERE id = ?`, userID).
		Scan(&name, &active); err != nil || active == 0 {
		return DeviceSecret{}, ErrInvalid
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM devices WHERE user_id = ? AND revoked_at IS NULL`, userID).Scan(&n); err != nil {
		return DeviceSecret{}, err
	}
	if limit := s.settings.Get().MaxDevicesPerUser; n >= limit {
		return DeviceSecret{}, fmt.Errorf("%w: %d devices (max_devices_per_user); revoke one first", ErrTooMany, limit)
	}
	secret := newDeviceSecret()
	now := s.now()
	res, err := tx.ExecContext(ctx, `INSERT INTO devices(user_id, kind, label, cap, secret_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, kind, label, cap, hash(normalizeDeviceSecret(secret)), store.Time(now))
	if err != nil {
		return DeviceSecret{}, err
	}
	id, _ := res.LastInsertId()
	return DeviceSecret{
		Device:   Device{ID: id, UserID: userID, Kind: kind, Label: label, Cap: cap, CreatedAt: now},
		Username: name, Secret: secret,
	}, nil
}

// Basic resolves an HTTP Basic credential: the user's name and a device
// secret. A user's web session or anything else is refused; so is a
// device of a disabled or removed user.
func (s *Service) Basic(r *http.Request, clientIP string) (*Principal, error) {
	name, secret, ok := r.BasicAuth()
	if !ok || name == "" || secret == "" {
		return nil, ErrInvalid
	}
	ctx := r.Context()
	var (
		id, userID int64
		cap        string
		lastUsed   sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `SELECT d.id, d.user_id, d.cap, d.last_used_at FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.secret_hash = ? AND u.name = ? AND d.revoked_at IS NULL AND u.disabled = 0 AND u.removed_at IS NULL`,
		hash(normalizeDeviceSecret(secret)), name).Scan(&id, &userID, &cap, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	u, err := s.dir.UserByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalid
	}
	now := s.now()
	// Recorded at most once a minute: a phone syncing makes many requests.
	if last, err := store.ParseTime(lastUsed.String); !lastUsed.Valid || err != nil || now.Sub(last) > time.Minute {
		_, _ = s.db.ExecContext(ctx, `UPDATE devices SET last_used_at = ?, last_ip = ?, client = ? WHERE id = ?`,
			store.Time(now), clientIP, truncate(r.UserAgent(), 200), id)
		s.dir.Touch(ctx, userID)
	}
	return &Principal{User: u, DeviceID: id, DeviceCap: cap, Surface: "dav"}, nil
}

// Devices lists a user's devices that are not revoked, newest first.
func (s *Service) Devices(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, kind, label, cap, client, created_at, last_used_at, COALESCE(last_ip, '')
		FROM devices WHERE user_id = ? AND revoked_at IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var created string
		var used sql.NullString
		if err := rows.Scan(&d.ID, &d.UserID, &d.Kind, &d.Label, &d.Cap, &d.Client, &created, &used, &d.LastIP); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = store.ParseTime(created)
		if used.Valid {
			t, _ := store.ParseTime(used.String)
			d.LastUsedAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice stops one of a user's devices, and the WebView session it
// opened if it is the app.
func (s *Service) RevokeDevice(ctx context.Context, actor string, userID, deviceID int64) error {
	now := store.Time(s.now())
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE devices SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, now, deviceID, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrInvalid
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE device_id = ?`, deviceID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'device', 'revoked', ?)`,
			now, actor, fmt.Sprint(deviceID))
		return err
	})
}

// AppSession trades the app's device credential for a web session for its
// WebView (CONTRACTS.md 2.2). Only a device of kind app may.
func (s *Service) AppSession(ctx context.Context, p *Principal, userAgent string) (string, error) {
	var kind string
	if err := s.db.QueryRowContext(ctx, `SELECT kind FROM devices WHERE id = ?`, p.DeviceID).Scan(&kind); err != nil || kind != "app" {
		return "", ErrInvalid
	}
	return s.newSession(ctx, SurfaceApp, p.User.ID, p.DeviceID, userAgent, s.settings.Get().SessionTTL())
}
