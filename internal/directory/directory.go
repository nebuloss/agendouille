// Package directory owns users, groups and group membership, and answers
// the visibility question: user A sees user B when they share a group
// (PLAN.md 4.1). Only the admin changes any of it.
//
// Users are never deleted: removing one sets removed_at, so the events
// they made keep an author. A disabled or removed user is not Active, and
// every credential lookup in internal/auth checks that.
package directory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nebuloss/agendouille/internal/store"
)

// User is a person of the family.
type User struct {
	ID          int64      `json:"-"`
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Color       string     `json:"color"`
	Disabled    bool       `json:"disabled"`
	RemovedAt   *time.Time `json:"removed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
}

// Active reports whether the user may sign in and sync.
func (u User) Active() bool { return !u.Disabled && u.RemovedAt == nil }

// Group is a set of users who see each other.
type Group struct {
	ID          int64     `json:"-"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"created_at"`
}

var (
	ErrNotFound  = errors.New("not found")
	ErrNameTaken = errors.New("name taken")
	ErrBadName   = errors.New("bad name")
)

var nameRule = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// reserved names would read as someone else in the audit log, or in a
// calendar ref.
var reserved = map[string]bool{"admin": true, "cli": true, "system": true, "root": true}

// CheckName validates a user or group name.
func CheckName(name string) error {
	if !nameRule.MatchString(name) {
		return fmt.Errorf("%w: %q must be a lowercase letter, then up to 31 lowercase letters, digits or dashes", ErrBadName, name)
	}
	if reserved[name] {
		return fmt.Errorf("%w: %q is reserved", ErrBadName, name)
	}
	return nil
}

func checkDisplay(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 64 || strings.ContainsAny(s, "\x00\r\n\t") {
		return "", fmt.Errorf("%w: a display name is 1 to 64 characters on one line", ErrBadName)
	}
	return s, nil
}

var colorRule = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// Service is the directory.
type Service struct {
	db      *sql.DB
	now     func() time.Time
	palette func() []string
}

// New wraps the database. palette gives the colours new users and groups
// take in turn when none is given.
func New(db *sql.DB, now func() time.Time, palette func() []string) *Service {
	return &Service{db: db, now: now, palette: palette}
}

func (s *Service) pickColor(ctx context.Context, tx *sql.Tx, table, color string) (string, error) {
	if color != "" {
		if !colorRule.MatchString(color) {
			return "", fmt.Errorf("%w: %q is not a colour like #3b6ea5", ErrBadName, color)
		}
		return color, nil
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		return "", err
	}
	p := s.palette()
	return p[n%len(p)], nil
}

func audit(ctx context.Context, tx *sql.Tx, now time.Time, actor, kind, action, detail string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, ?, ?, ?)`,
		store.Time(now), actor, kind, action, detail)
	return err
}

const userCols = `id, name, display_name, color, disabled, removed_at, created_at, last_seen_at`

type scanner interface{ Scan(...any) error }

func scanUser(r scanner) (User, error) {
	var u User
	var removed, seen sql.NullString
	var created string
	if err := r.Scan(&u.ID, &u.Name, &u.DisplayName, &u.Color, &u.Disabled, &removed, &created, &seen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u.CreatedAt, _ = store.ParseTime(created)
	if removed.Valid {
		t, _ := store.ParseTime(removed.String)
		u.RemovedAt = &t
	}
	if seen.Valid {
		t, _ := store.ParseTime(seen.String)
		u.LastSeenAt = &t
	}
	return u, nil
}

// AddUser creates a user in the given groups. An empty color takes the
// next one of the palette.
func (s *Service) AddUser(ctx context.Context, actor, name, displayName, color string, groups []string) (User, error) {
	if err := CheckName(name); err != nil {
		return User{}, err
	}
	if displayName == "" {
		displayName = strings.ToUpper(name[:1]) + name[1:]
	}
	display, err := checkDisplay(displayName)
	if err != nil {
		return User{}, err
	}
	now := s.now()
	var u User
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		c, err := s.pickColor(ctx, tx, "users", color)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO users(name, display_name, color, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(name) DO NOTHING`, name, display, c, store.Time(now))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: a user named %q exists (or existed: names are never reused)", ErrNameTaken, name)
		}
		id, _ := res.LastInsertId()
		for _, g := range groups {
			if err := addMember(ctx, tx, g, id); err != nil {
				return err
			}
		}
		if err := audit(ctx, tx, now, actor, "user", "added", name); err != nil {
			return err
		}
		u, err = scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
		return err
	})
	return u, err
}

// User finds a user by name, removed ones included.
func (s *Service) User(ctx context.Context, name string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE name = ?`, name))
}

// UserByID finds a user by id, removed ones included.
func (s *Service) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// Users lists the users not removed, by name.
func (s *Service) Users(ctx context.Context) ([]User, error) {
	return s.queryUsers(ctx, `SELECT `+userCols+` FROM users WHERE removed_at IS NULL ORDER BY name`)
}

func (s *Service) queryUsers(ctx context.Context, q string, args ...any) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetDisabled disables or enables a user. A disabled user's sessions,
// devices and links stop working until enabled; nothing is deleted.
func (s *Service) SetDisabled(ctx context.Context, actor, name string, disabled bool) error {
	action := "enabled"
	if disabled {
		action = "disabled"
	}
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET disabled = ? WHERE name = ? AND removed_at IS NULL`, disabled, name)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, s.now(), actor, "user", action, name)
	})
}

// Remove marks a user removed and takes them out of every group. Their
// credentials stop working at once (internal/auth checks Active); their
// calendars are transferred or deleted by the caller first (M2, M3).
func (s *Service) Remove(ctx context.Context, actor, name string) error {
	now := s.now()
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE name = ? AND removed_at IS NULL`, name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET removed_at = ?, disabled = 1 WHERE id = ?`, store.Time(now), id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE user_id = ?`, id); err != nil {
			return err
		}
		return audit(ctx, tx, now, actor, "user", "removed", name)
	})
}

// Touch records that a user was seen, at most once a minute.
func (s *Service) Touch(ctx context.Context, id int64) {
	now := s.now()
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET last_seen_at = ? WHERE id = ? AND (last_seen_at IS NULL OR last_seen_at < ?)`,
		store.Time(now), id, store.Time(now.Add(-time.Minute)))
}

const groupCols = `id, name, display_name, color, created_at`

func scanGroup(r scanner) (Group, error) {
	var g Group
	var created string
	if err := r.Scan(&g.ID, &g.Name, &g.DisplayName, &g.Color, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Group{}, ErrNotFound
		}
		return Group{}, err
	}
	g.CreatedAt, _ = store.ParseTime(created)
	return g, nil
}

// AddGroup creates a group.
func (s *Service) AddGroup(ctx context.Context, actor, name, displayName, color string) (Group, error) {
	if err := CheckName(name); err != nil {
		return Group{}, err
	}
	if displayName == "" {
		displayName = strings.ToUpper(name[:1]) + name[1:]
	}
	display, err := checkDisplay(displayName)
	if err != nil {
		return Group{}, err
	}
	now := s.now()
	var g Group
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		c, err := s.pickColor(ctx, tx, "groups", color)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO groups(name, display_name, color, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(name) DO NOTHING`, name, display, c, store.Time(now))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: a group named %q exists", ErrNameTaken, name)
		}
		id, _ := res.LastInsertId()
		if err := audit(ctx, tx, now, actor, "group", "added", name); err != nil {
			return err
		}
		g, err = scanGroup(tx.QueryRowContext(ctx, `SELECT `+groupCols+` FROM groups WHERE id = ?`, id))
		return err
	})
	return g, err
}

// Groups lists every group, by name.
func (s *Service) Groups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+groupCols+` FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// RemoveGroup deletes a group and its memberships. Shares to it go with it
// once calendars exist (M2).
func (s *Service) RemoveGroup(ctx context.Context, actor, name string) error {
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM groups WHERE name = ?`, name)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, s.now(), actor, "group", "removed", name)
	})
}

func addMember(ctx context.Context, tx *sql.Tx, group string, userID int64) error {
	var gid int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE name = ?`, group).Scan(&gid)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("group %q: %w", group, ErrNotFound)
	} else if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO group_members(group_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, gid, userID)
	return err
}

// AddMember puts a user in a group.
func (s *Service) AddMember(ctx context.Context, actor, group, user string) error {
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var uid int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE name = ? AND removed_at IS NULL`, user).Scan(&uid)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user %q: %w", user, ErrNotFound)
		} else if err != nil {
			return err
		}
		if err := addMember(ctx, tx, group, uid); err != nil {
			return err
		}
		return audit(ctx, tx, s.now(), actor, "group", "member_added", group+" "+user)
	})
}

// RemoveMember takes a user out of a group.
func (s *Service) RemoveMember(ctx context.Context, actor, group, user string) error {
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM group_members
			WHERE group_id = (SELECT id FROM groups WHERE name = ?) AND user_id = (SELECT id FROM users WHERE name = ?)`, group, user)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, s.now(), actor, "group", "member_removed", group+" "+user)
	})
}

// GroupsOf lists the groups a user is in, by name.
func (s *Service) GroupsOf(ctx context.Context, userID int64) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.name, g.display_name, g.color, g.created_at
		FROM groups g JOIN group_members m ON m.group_id = g.id WHERE m.user_id = ? ORDER BY g.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Members lists the active users of a group, by name.
func (s *Service) Members(ctx context.Context, group string) ([]User, error) {
	return s.queryUsers(ctx, `SELECT u.id, u.name, u.display_name, u.color, u.disabled, u.removed_at, u.created_at, u.last_seen_at
		FROM users u JOIN group_members m ON m.user_id = u.id JOIN groups g ON g.id = m.group_id
		WHERE g.name = ? AND u.removed_at IS NULL ORDER BY u.name`, group)
}

// Sees reports whether user a sees user b: the same user, or a shared group.
func (s *Service) Sees(ctx context.Context, a, b int64) (bool, error) {
	if a == b {
		return true, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM group_members x JOIN group_members y ON x.group_id = y.group_id
		WHERE x.user_id = ? AND y.user_id = ?`, a, b).Scan(&n)
	return n > 0, err
}

// Visible lists the other active users a user sees, by name.
func (s *Service) Visible(ctx context.Context, userID int64) ([]User, error) {
	return s.queryUsers(ctx, `SELECT DISTINCT u.id, u.name, u.display_name, u.color, u.disabled, u.removed_at, u.created_at, u.last_seen_at
		FROM users u JOIN group_members y ON y.user_id = u.id JOIN group_members x ON x.group_id = y.group_id
		WHERE x.user_id = ? AND u.id <> ? AND u.removed_at IS NULL AND u.disabled = 0 ORDER BY u.name`, userID, userID)
}
