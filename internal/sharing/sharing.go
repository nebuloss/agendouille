// Package sharing decides who may do what with a calendar (PLAN.md 4.1, 6):
// its owner may do anything; a share gives a user, or every member of a
// group, read or read-write; the highest wins; a device's cap lowers it.
// It also keeps each user's own view of a calendar: hidden or not, and the
// name and colour a sharee gave it on their own devices.
package sharing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/store"
)

// Access to a calendar, in increasing order.
type Access int

const (
	None Access = iota
	Read
	ReadWrite
	Owner
)

func (a Access) String() string {
	return [...]string{"none", "read", "readwrite", "owner"}[a]
}

// CanRead, CanWrite: what an access allows on the calendar's events.
func (a Access) CanRead() bool  { return a >= Read }
func (a Access) CanWrite() bool { return a >= ReadWrite }

// ParseAccess reads "read" or "readwrite", the two a share can give.
func ParseAccess(s string) (Access, error) {
	switch s {
	case "read":
		return Read, nil
	case "readwrite", "read-write", "rw":
		return ReadWrite, nil
	}
	return None, fmt.Errorf("access %q is not read or readwrite", s)
}

// Effective is the lower of a user's access and their device's cap: a
// read-only device never writes, even to the user's own calendar.
func Effective(a Access, deviceCap string) Access {
	if deviceCap == "read" && a > Read {
		return Read
	}
	return a
}

// Target is who a share is for: one user, or a group.
type Target struct {
	User  string
	Group string
}

// ParseTarget reads "u:<user>" or "g:<group>" (CONTRACTS.md 2.2).
func ParseTarget(s string) (Target, error) {
	kind, name, ok := strings.Cut(s, ":")
	switch {
	case ok && kind == "u" && name != "":
		return Target{User: name}, nil
	case ok && kind == "g" && name != "":
		return Target{Group: name}, nil
	}
	return Target{}, fmt.Errorf("share target %q is not u:<user> or g:<group>", s)
}

func (t Target) String() string {
	if t.Group != "" {
		return "g:" + t.Group
	}
	return "u:" + t.User
}

var (
	ErrNotFound   = errors.New("not found")
	ErrNotVisible = errors.New("the owner does not see this user or group")
	ErrNotOwner   = errors.New("only the calendar's owner can do this")
)

// Share is one row of a calendar's shares.
type Share struct {
	Target Target `json:"-"`
	Name   string `json:"target"` // u:lea, g:famille
	Access string `json:"access"`
}

// Service reads and writes shares and views.
type Service struct {
	db  *sql.DB
	dir *directory.Service
}

// New wraps the database.
func New(db *sql.DB, dir *directory.Service) *Service { return &Service{db: db, dir: dir} }

// Access is a user's access to a calendar. A deleted calendar, or one
// whose owner was removed, gives None.
func (s *Service) Access(ctx context.Context, userID, calendarID int64) (Access, error) {
	return access(ctx, s.db, userID, calendarID)
}

// AccessTx is Access inside a transaction, for the write path's re-check.
func (s *Service) AccessTx(ctx context.Context, tx *sql.Tx, userID, calendarID int64) (Access, error) {
	return access(ctx, tx, userID, calendarID)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func access(ctx context.Context, q querier, userID, calendarID int64) (Access, error) {
	var owner int64
	var deleted sql.NullString
	err := q.QueryRowContext(ctx, `SELECT owner_id, deleted_at FROM calendars WHERE id = ?`, calendarID).Scan(&owner, &deleted)
	if errors.Is(err, sql.ErrNoRows) || deleted.Valid {
		return None, nil
	} else if err != nil {
		return None, err
	}
	if owner == userID {
		return Owner, nil
	}
	var best sql.NullString
	err = q.QueryRowContext(ctx, `SELECT max(access) FROM shares WHERE calendar_id = ? AND (user_id = ?
		OR group_id IN (SELECT group_id FROM group_members WHERE user_id = ?))`, calendarID, userID, userID).Scan(&best)
	if err != nil {
		return None, err
	}
	switch best.String { // 'readwrite' > 'read' as text, so max() picks the higher
	case "readwrite":
		return ReadWrite, nil
	case "read":
		return Read, nil
	}
	return None, nil
}

// Entry is one calendar a user may read.
type Entry struct {
	CalendarID int64
	Access     Access
	Hidden     bool
}

// Readable lists the calendars a user owns or has a share of, hidden ones
// included (the caller decides), not deleted ones.
func (s *Service) Readable(ctx context.Context, userID int64) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,
		       CASE WHEN c.owner_id = ? THEN 'owner'
		            ELSE (SELECT max(sh.access) FROM shares sh WHERE sh.calendar_id = c.id AND (sh.user_id = ?
		                  OR sh.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?))) END,
		       COALESCE((SELECT hidden FROM calendar_views v WHERE v.calendar_id = c.id AND v.user_id = ?), 0)
		FROM calendars c WHERE c.deleted_at IS NULL ORDER BY c.id`, userID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var access sql.NullString
		if err := rows.Scan(&e.CalendarID, &access, &e.Hidden); err != nil {
			return nil, err
		}
		switch access.String {
		case "owner":
			e.Access = Owner
		case "readwrite":
			e.Access = ReadWrite
		case "read":
			e.Access = Read
		default:
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func audit(ctx context.Context, tx *sql.Tx, at, actor, action string, calendarID int64, detail string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, calendar_id, detail) VALUES (?, ?, 'share', ?, ?, ?)`,
		at, actor, action, calendarID, detail)
	return err
}

// ShareWith shares a calendar. byOwner is false for the admin, who may
// share with anyone; an owner may share only with users they see and with
// groups they are in (PLAN.md 4.1).
func (s *Service) ShareWith(ctx context.Context, actor string, ownerID int64, byOwner bool, calendarID int64, t Target, a Access, at string) error {
	if a != Read && a != ReadWrite {
		return fmt.Errorf("a share gives read or readwrite")
	}
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if t.User != "" {
			u, err := s.dir.User(ctx, t.User)
			if err != nil || !u.Active() {
				return fmt.Errorf("user %q: %w", t.User, ErrNotFound)
			}
			if u.ID == ownerID {
				return fmt.Errorf("the owner has the calendar already")
			}
			if byOwner {
				if ok, err := s.dir.Sees(ctx, ownerID, u.ID); err != nil {
					return err
				} else if !ok {
					return ErrNotVisible
				}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO shares(calendar_id, user_id, access, created_at) VALUES (?, ?, ?, ?)
				ON CONFLICT(calendar_id, user_id) DO UPDATE SET access = excluded.access`, calendarID, u.ID, a.String(), at); err != nil {
				return err
			}
		} else {
			var gid int64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE name = ?`, t.Group).Scan(&gid); err != nil {
				return fmt.Errorf("group %q: %w", t.Group, ErrNotFound)
			}
			if byOwner {
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM group_members WHERE group_id = ? AND user_id = ?`, gid, ownerID).Scan(&n); err != nil {
					return err
				}
				if n == 0 {
					return ErrNotVisible
				}
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO shares(calendar_id, group_id, access, created_at) VALUES (?, ?, ?, ?)
				ON CONFLICT(calendar_id, group_id) DO UPDATE SET access = excluded.access`, calendarID, gid, a.String(), at); err != nil {
				return err
			}
		}
		return audit(ctx, tx, at, actor, "shared", calendarID, t.String()+" "+a.String())
	})
}

// Unshare removes a share.
func (s *Service) Unshare(ctx context.Context, actor string, calendarID int64, t Target, at string) error {
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var res sql.Result
		var err error
		if t.User != "" {
			res, err = tx.ExecContext(ctx, `DELETE FROM shares WHERE calendar_id = ? AND user_id = (SELECT id FROM users WHERE name = ?)`, calendarID, t.User)
		} else {
			res, err = tx.ExecContext(ctx, `DELETE FROM shares WHERE calendar_id = ? AND group_id = (SELECT id FROM groups WHERE name = ?)`, calendarID, t.Group)
		}
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, at, actor, "unshared", calendarID, t.String())
	})
}

// Shares lists a calendar's shares, users then groups, by name.
func (s *Service) Shares(ctx context.Context, calendarID int64) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT 'u', u.name, sh.access FROM shares sh JOIN users u ON u.id = sh.user_id WHERE sh.calendar_id = ?
		UNION ALL
		SELECT 'g', g.name, sh.access FROM shares sh JOIN groups g ON g.id = sh.group_id WHERE sh.calendar_id = ?
		ORDER BY 1 DESC, 2`, calendarID, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Share{}
	for rows.Next() {
		var kind, name, access string
		if err := rows.Scan(&kind, &name, &access); err != nil {
			return nil, err
		}
		t := Target{User: name}
		if kind == "g" {
			t = Target{Group: name}
		}
		out = append(out, Share{Target: t, Name: t.String(), Access: access})
	}
	return out, rows.Err()
}

// View is a user's own view of a calendar.
type View struct {
	Shown       bool
	Hidden      bool
	DisplayName string // "" when the user did not rename it
	Color       string
	Order       *int
}

// ViewOf is a user's view of a calendar (the defaults when none is kept).
func (s *Service) ViewOf(ctx context.Context, userID, calendarID int64) (View, error) {
	v := View{Shown: true}
	var name, color sql.NullString
	var order sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT shown, hidden, displayname, color, sort_order FROM calendar_views
		WHERE calendar_id = ? AND user_id = ?`, calendarID, userID).Scan(&v.Shown, &v.Hidden, &name, &color, &order)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	} else if err != nil {
		return v, err
	}
	v.DisplayName, v.Color = name.String, color.String
	if order.Valid {
		n := int(order.Int64)
		v.Order = &n
	}
	return v, nil
}

// SetView updates the parts of a user's view that are non-nil.
func (s *Service) SetView(ctx context.Context, userID, calendarID int64, shown, hidden *bool, name, color *string, order *int) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO calendar_views(calendar_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		calendarID, userID); err != nil {
		return err
	}
	set := func(col string, v any) error {
		_, err := s.db.ExecContext(ctx, `UPDATE calendar_views SET `+col+` = ? WHERE calendar_id = ? AND user_id = ?`, v, calendarID, userID)
		return err
	}
	for col, v := range map[string]any{"shown": shown, "hidden": hidden, "displayname": name, "color": color, "sort_order": order} {
		switch x := v.(type) {
		case *bool:
			if x != nil {
				if err := set(col, *x); err != nil {
					return err
				}
			}
		case *string:
			if x != nil {
				var val any = *x
				if *x == "" {
					val = nil
				}
				if err := set(col, val); err != nil {
					return err
				}
			}
		case *int:
			if x != nil {
				if err := set(col, *x); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
