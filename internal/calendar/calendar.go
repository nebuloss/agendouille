// Package calendar is the one write path (PLAN.md 4.3, CONTRACTS.md 1.3):
// CalDAV, the web API, import and the CLI all create, change and delete
// calendars and events through it, and nothing else writes the calendars,
// objects or changes tables. Every write appends to the change log, which
// sync tokens and ctags are made from, and is published on the hub after
// it commits.
package calendar

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/hub"
	"github.com/nebuloss/agendouille/internal/ical"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

var (
	ErrNotFound        = errors.New("not found")             // also: no access
	ErrForbidden       = errors.New("forbidden")             // can read, cannot write
	ErrPrecondition    = errors.New("precondition failed")   // If-Match / If-None-Match
	ErrUIDConflict     = errors.New("uid conflict")          // the UID is in another object of the calendar
	ErrInvalidData     = ical.ErrInvalid                     // valid-calendar-data
	ErrUnsupportedComp = ical.ErrUnsupported                 // supported-calendar-component
	ErrTooLarge        = ical.ErrTooLarge                    // max-resource-size
	ErrQuota           = errors.New("quota exceeded")        // a limit setting
	ErrTokenExpired    = errors.New("sync token expired")    // valid-sync-token
	ErrExists          = errors.New("calendar exists")       // slug taken (or once taken)
	ErrBadSlug         = errors.New("bad calendar slug")     //
	ErrBadName         = errors.New("bad object name")       //
	ErrNotOwner        = errors.New("only the owner can do") //
)

// UIDConflict carries the name of the object that holds the UID.
type UIDConflict struct{ Name string }

func (e *UIDConflict) Error() string { return "uid conflict with " + e.Name }
func (e *UIDConflict) Unwrap() error { return ErrUIDConflict }

// Actor is who writes. UserID 0 is the operator (the CLI, and the admin UI
// for metadata): it may manage any calendar.
type Actor struct {
	UserID   int64
	Name     string // for the change log and the audit
	DeviceID int64
	Cap      string // "readwrite" or "read"
	Via      string // caldav, web, app, import, cli, admin
}

// Operator is the CLI's actor.
func Operator(via string) Actor { return Actor{Name: via, Cap: "readwrite", Via: via} }

// Ref names a calendar outside the database: "<owner>~<slug>".
type Ref struct{ Owner, Slug string }

func (r Ref) String() string { return r.Owner + "~" + r.Slug }

// ParseRef reads "<owner>~<slug>".
func ParseRef(s string) (Ref, error) {
	owner, slug, ok := strings.Cut(s, "~")
	if !ok || directory.CheckName(owner) != nil || !slugRule.MatchString(slug) {
		return Ref{}, fmt.Errorf("%w: %q is not <owner>~<slug>", ErrNotFound, s)
	}
	return Ref{owner, slug}, nil
}

var (
	slugRule  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	nameRule  = regexp.MustCompile(`^[A-Za-z0-9._@+-]{1,200}\.ics$`)
	colorRule = regexp.MustCompile(`^#[0-9a-f]{6}$`)
)

// NormalizeSlug turns what a client asked for at MKCALENDAR (often an
// upper-case UUID) into a slug: lower case, other characters as dashes.
func NormalizeSlug(s string) (string, error) {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, s)
	s = strings.Trim(s, "-")
	if !slugRule.MatchString(s) {
		return "", fmt.Errorf("%w: %q", ErrBadSlug, s)
	}
	return s, nil
}

// ValidName reports whether an object name is one agendouille keeps.
func ValidName(name string) bool { return nameRule.MatchString(name) }

// NormalizeColor accepts #rrggbb and Apple's #rrggbbaa, any case.
func NormalizeColor(c string) (string, bool) {
	c = strings.ToLower(strings.TrimSpace(c))
	if len(c) == 9 {
		c = c[:7]
	}
	return c, colorRule.MatchString(c)
}

// Info is a calendar as one user sees it.
type Info struct {
	ID          int64
	Ref         Ref
	OwnerID     int64
	Name        string // the owner's name for it
	Color       string
	Timezone    string
	Description string
	Seq         int64
	PrunedSeq   int64
	Access      sharing.Access
	View        sharing.View
}

// DisplayName is the name this user sees: their own, else the owner's.
func (i Info) DisplayName() string {
	if i.View.DisplayName != "" {
		return i.View.DisplayName
	}
	return i.Name
}

// DisplayColor is the colour this user sees.
func (i Info) DisplayColor() string {
	if i.View.Color != "" {
		return i.View.Color
	}
	return i.Color
}

// Location is the calendar's zone.
func (i Info) Location() *time.Location {
	if loc, err := time.LoadLocation(i.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// Object is one stored calendar object.
type Object struct {
	Name       string
	UID        string
	ETag       string
	Data       []byte
	Summary    string
	CreatedBy  int64
	ModifiedBy int64
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// Precondition is If-Match / If-None-Match: *.
type Precondition struct {
	IfMatch     []string // quoted etags, or "*"
	IfNoneMatch bool
}

// Service is the calendar.
type Service struct {
	db       *sql.DB
	now      func() time.Time
	settings *settings.Service
	dir      *directory.Service
	sharing  *sharing.Service
	hub      *hub.Hub
}

// New wraps the database.
func New(db *sql.DB, now func() time.Time, set *settings.Service, dir *directory.Service, sh *sharing.Service, h *hub.Hub) *Service {
	return &Service{db: db, now: now, settings: set, dir: dir, sharing: sh, hub: h}
}

// Sharing is the sharing service, for the handlers that manage shares.
func (s *Service) Sharing() *sharing.Service { return s.sharing }

const calCols = `c.id, u.name, c.slug, c.owner_id, c.name, c.color, c.timezone, c.description, c.seq, c.pruned_seq`

func scanInfo(row interface{ Scan(...any) error }) (Info, error) {
	var i Info
	err := row.Scan(&i.ID, &i.Ref.Owner, &i.Ref.Slug, &i.OwnerID, &i.Name, &i.Color, &i.Timezone, &i.Description, &i.Seq, &i.PrunedSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return Info{}, ErrNotFound
	}
	return i, err
}

// lookup finds a live calendar by ref, with the actor's access and view.
func (s *Service) lookup(ctx context.Context, a Actor, r Ref) (Info, error) {
	i, err := scanInfo(s.db.QueryRowContext(ctx, `SELECT `+calCols+` FROM calendars c JOIN users u ON u.id = c.owner_id
		WHERE u.name = ? AND c.slug = ? AND c.deleted_at IS NULL`, r.Owner, r.Slug))
	if err != nil {
		return Info{}, err
	}
	return s.withAccess(ctx, a, i)
}

func (s *Service) withAccess(ctx context.Context, a Actor, i Info) (Info, error) {
	if a.UserID == 0 {
		i.Access = sharing.Owner
		return i, nil
	}
	acc, err := s.sharing.Access(ctx, a.UserID, i.ID)
	if err != nil {
		return Info{}, err
	}
	i.Access = sharing.Effective(acc, a.Cap)
	if !i.Access.CanRead() {
		return Info{}, ErrNotFound
	}
	if i.View, err = s.sharing.ViewOf(ctx, a.UserID, i.ID); err != nil {
		return Info{}, err
	}
	return i, nil
}

// recheck repeats, inside a write transaction, what lookup saw before it:
// the calendar is still live and the actor may still write it. Transactions
// take the write lock first (_txlock=immediate), so no other write can
// land between this check and the caller's (PLAN.md 4.3, step 1).
func (s *Service) recheck(ctx context.Context, tx *sql.Tx, a Actor, i Info, needOwner bool) error {
	var deleted sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM calendars WHERE id = ?`, i.ID).Scan(&deleted); err != nil || deleted.Valid {
		return ErrNotFound
	}
	if a.UserID == 0 {
		return nil
	}
	acc, err := s.sharing.AccessTx(ctx, tx, a.UserID, i.ID)
	if err != nil {
		return err
	}
	acc = sharing.Effective(acc, a.Cap)
	switch {
	case !acc.CanRead():
		return ErrNotFound
	case needOwner && acc != sharing.Owner:
		return ErrNotOwner
	case !acc.CanWrite():
		return ErrForbidden
	}
	return nil
}

// Info is a calendar the actor may read.
func (s *Service) Info(ctx context.Context, a Actor, r Ref) (Info, error) { return s.lookup(ctx, a, r) }

// Home lists the calendars the actor may read, hidden ones left out unless
// withHidden (the web app shows them, to show them again).
func (s *Service) Home(ctx context.Context, a Actor, withHidden bool) ([]Info, error) {
	entries, err := s.sharing.Readable(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range entries {
		if e.Hidden && !withHidden {
			continue
		}
		i, err := scanInfo(s.db.QueryRowContext(ctx, `SELECT `+calCols+` FROM calendars c JOIN users u ON u.id = c.owner_id WHERE c.id = ?`, e.CalendarID))
		if err != nil {
			return nil, err
		}
		i.Access = sharing.Effective(e.Access, a.Cap)
		if i.View, err = s.sharing.ViewOf(ctx, a.UserID, i.ID); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// Props are a calendar's own properties; nil fields are left as they are.
type Props struct {
	Name        *string
	Color       *string
	Timezone    *string
	Description *string
	Order       *int // a user's own order, never the calendar's
}

func (s *Service) change(ctx context.Context, tx *sql.Tx, a Actor, calID int64, name, summary, action string) (int64, error) {
	var dev any
	if a.DeviceID != 0 {
		dev = a.DeviceID
	}
	var uid any
	if a.UserID != 0 {
		uid = a.UserID
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO changes(calendar_id, name, summary, action, user_id, device_id, via, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, calID, name, summary, action, uid, dev, a.Via, store.Time(s.now()))
	if err != nil {
		return 0, err
	}
	seq, _ := res.LastInsertId()
	_, err = tx.ExecContext(ctx, `UPDATE calendars SET seq = ? WHERE id = ?`, seq, calID)
	return seq, err
}

func (s *Service) publish(i Info, seq int64, name, action, by string) {
	if s.hub != nil {
		s.hub.Publish(hub.Event{Seq: seq, CalendarID: i.ID, Calendar: i.Ref.String(), Name: name, Action: action, By: by})
	}
}

// Create makes a calendar owned by owner (the actor itself, unless the
// actor is the operator).
func (s *Service) Create(ctx context.Context, a Actor, owner, slug string, p Props) (Info, error) {
	slug, err := NormalizeSlug(slug)
	if err != nil {
		return Info{}, err
	}
	u, err := s.dir.User(ctx, owner)
	if err != nil || !u.Active() {
		return Info{}, fmt.Errorf("owner %q: %w", owner, ErrNotFound)
	}
	if a.UserID != 0 && a.UserID != u.ID {
		return Info{}, ErrNotOwner
	}
	set := s.settings.Get()
	name := slug
	if p.Name != nil && strings.TrimSpace(*p.Name) != "" {
		name = strings.TrimSpace(*p.Name)
	}
	if len(name) > 100 {
		name = name[:100]
	}
	color := ""
	if p.Color != nil {
		if c, ok := NormalizeColor(*p.Color); ok {
			color = c
		}
	}
	zone := set.FamilyTimezone
	if p.Timezone != nil && *p.Timezone != "" {
		if _, err := time.LoadLocation(*p.Timezone); err == nil {
			zone = *p.Timezone
		}
	}
	desc := ""
	if p.Description != nil {
		desc = *p.Description
	}
	var id int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM calendars WHERE owner_id = ? AND deleted_at IS NULL`, u.ID).Scan(&n); err != nil {
			return err
		}
		if n >= set.MaxCalendarsPerUser {
			return fmt.Errorf("%w: %d calendars (max_calendars_per_user)", ErrQuota, set.MaxCalendarsPerUser)
		}
		if color == "" {
			color = set.Palette[n%len(set.Palette)]
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO calendars(owner_id, slug, name, color, timezone, description, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(owner_id, slug) DO NOTHING`, u.ID, slug, name, color, zone, desc, store.Time(s.now()))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: %s~%s", ErrExists, owner, slug)
		}
		id, _ = res.LastInsertId()
		if _, err := s.change(ctx, tx, a, id, "", "", "calendar_created"); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, calendar_id, detail) VALUES (?, ?, 'calendar', 'created', ?, ?)`,
			store.Time(s.now()), a.Name, id, owner+"~"+slug)
		return err
	})
	if err != nil {
		return Info{}, err
	}
	i, err := s.lookup(ctx, a, Ref{owner, slug})
	if err == nil {
		s.publish(i, i.Seq, "", "calendar_created", a.Name)
	}
	return i, err
}

// Update changes a calendar's properties. The owner (or the operator)
// changes them for everyone; anyone else with access changes only their
// own view (PLAN.md 5.2, PROPPATCH).
func (s *Service) Update(ctx context.Context, a Actor, r Ref, p Props) (Info, error) {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return Info{}, err
	}
	var color *string
	if p.Color != nil {
		c, ok := NormalizeColor(*p.Color)
		if !ok && *p.Color != "" {
			return Info{}, fmt.Errorf("colour %q is not #rrggbb", *p.Color)
		}
		color = &c
	}
	if a.UserID != 0 && (p.Order != nil || i.Access != sharing.Owner) {
		name := p.Name
		if i.Access == sharing.Owner {
			name, color = nil, nil // the owner renames the calendar itself, below
		}
		if err := s.sharing.SetView(ctx, a.UserID, i.ID, nil, nil, name, color, p.Order); err != nil {
			return Info{}, err
		}
		if i.Access != sharing.Owner {
			return s.lookup(ctx, a, r)
		}
	}
	sets, args := []string{}, []any{}
	if p.Name != nil && strings.TrimSpace(*p.Name) != "" {
		sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(*p.Name))
	}
	if color != nil && *color != "" {
		sets, args = append(sets, "color = ?"), append(args, *color)
	}
	if p.Timezone != nil {
		if _, err := time.LoadLocation(*p.Timezone); err != nil || *p.Timezone == "" {
			return Info{}, fmt.Errorf("zone %q is not a name like Europe/Paris", *p.Timezone)
		}
		sets, args = append(sets, "timezone = ?"), append(args, *p.Timezone)
	}
	if p.Description != nil {
		sets, args = append(sets, "description = ?"), append(args, *p.Description)
	}
	if len(sets) == 0 {
		return s.lookup(ctx, a, r)
	}
	var seq int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.recheck(ctx, tx, a, i, true); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE calendars SET `+strings.Join(sets, ", ")+` WHERE id = ?`, append(args, i.ID)...); err != nil {
			return err
		}
		seq, err = s.change(ctx, tx, a, i.ID, "", "", "calendar_changed")
		return err
	})
	if err != nil {
		return Info{}, err
	}
	s.publish(i, seq, "", "calendar_changed", a.Name)
	return s.lookup(ctx, a, r)
}

// DeleteCalendar deletes a calendar and its events, for everyone. Only its
// owner or the operator may. The row stays, so its slug is never reused
// and the activity log keeps its name.
func (s *Service) DeleteCalendar(ctx context.Context, a Actor, r Ref) error {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return err
	}
	if i.Access != sharing.Owner {
		return ErrNotOwner
	}
	var seq int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.recheck(ctx, tx, a, i, true); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM objects WHERE calendar_id = ?`, i.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM shares WHERE calendar_id = ?`, i.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE calendars SET deleted_at = ? WHERE id = ?`, store.Time(s.now()), i.ID); err != nil {
			return err
		}
		if seq, err = s.change(ctx, tx, a, i.ID, "", "", "calendar_deleted"); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, calendar_id, detail) VALUES (?, ?, 'calendar', 'deleted', ?, ?)`,
			store.Time(s.now()), a.Name, i.ID, r.String())
		return err
	})
	if err == nil {
		s.publish(i, seq, "", "calendar_deleted", a.Name)
	}
	return err
}

// Transfer gives a calendar to another user (the owner or the operator
// does it). Its ref changes, so clients see the old one go and the new one
// come; the old owner keeps read-write unless keep is false.
func (s *Service) Transfer(ctx context.Context, a Actor, r Ref, to string, keep bool) (Info, error) {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return Info{}, err
	}
	if i.Access != sharing.Owner {
		return Info{}, ErrNotOwner
	}
	u, err := s.dir.User(ctx, to)
	if err != nil || !u.Active() {
		return Info{}, fmt.Errorf("user %q: %w", to, ErrNotFound)
	}
	if u.ID == i.OwnerID {
		return i, nil
	}
	if a.UserID != 0 {
		if ok, err := s.dir.Sees(ctx, a.UserID, u.ID); err != nil || !ok {
			return Info{}, sharing.ErrNotVisible
		}
	}
	var seq int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.recheck(ctx, tx, a, i, true); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM calendars WHERE owner_id = ? AND slug = ?`, u.ID, i.Ref.Slug).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: %s already has a calendar %s", ErrExists, to, i.Ref.Slug)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE calendars SET owner_id = ? WHERE id = ?`, u.ID, i.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM shares WHERE calendar_id = ? AND user_id = ?`, i.ID, u.ID); err != nil {
			return err
		}
		if keep {
			if _, err := tx.ExecContext(ctx, `INSERT INTO shares(calendar_id, user_id, access, created_at) VALUES (?, ?, 'readwrite', ?)
				ON CONFLICT(calendar_id, user_id) DO UPDATE SET access = 'readwrite'`, i.ID, i.OwnerID, store.Time(s.now())); err != nil {
				return err
			}
		}
		if seq, err = s.change(ctx, tx, a, i.ID, "", "", "calendar_changed"); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, calendar_id, detail) VALUES (?, ?, 'calendar', 'transferred', ?, ?)`,
			store.Time(s.now()), a.Name, i.ID, r.String()+" -> "+to)
		return err
	})
	if err != nil {
		return Info{}, err
	}
	s.publish(i, seq, "", "calendar_changed", a.Name)
	return s.lookup(ctx, Operator(a.Via), Ref{to, i.Ref.Slug})
}

// Owned lists the live calendars a user owns (for removing a user).
func (s *Service) Owned(ctx context.Context, userID int64) ([]Info, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+calCols+` FROM calendars c JOIN users u ON u.id = c.owner_id
		WHERE c.owner_id = ? AND c.deleted_at IS NULL ORDER BY c.slug`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Info{}
	for rows.Next() {
		i, err := scanInfo(rows)
		if err != nil {
			return nil, err
		}
		i.Access = sharing.Owner
		out = append(out, i)
	}
	return out, rows.Err()
}

const objCols = `name, uid, etag, data, summary, created_by, modified_by, created_at, modified_at`

func scanObject(row interface{ Scan(...any) error }) (Object, error) {
	var o Object
	var created, modified string
	err := row.Scan(&o.Name, &o.UID, &o.ETag, &o.Data, &o.Summary, &o.CreatedBy, &o.ModifiedBy, &created, &modified)
	if errors.Is(err, sql.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	o.CreatedAt, _ = store.ParseTime(created)
	o.ModifiedAt, _ = store.ParseTime(modified)
	return o, err
}

// Get is one object of a calendar the actor may read.
func (s *Service) Get(ctx context.Context, a Actor, r Ref, name string) (Info, Object, error) {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return Info{}, Object{}, err
	}
	o, err := scanObject(s.db.QueryRowContext(ctx, `SELECT `+objCols+` FROM objects WHERE calendar_id = ? AND name = ?`, i.ID, name))
	return i, o, err
}

// List is every object of a calendar, with its data.
func (s *Service) List(ctx context.Context, i Info) ([]Object, error) {
	return s.objects(ctx, `SELECT `+objCols+` FROM objects WHERE calendar_id = ? ORDER BY name`, i.ID)
}

func (s *Service) objects(ctx context.Context, q string, args ...any) ([]Object, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Object{}
	for rows.Next() {
		o, err := scanObject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetMany is the objects of names that exist (calendar-multiget).
func (s *Service) GetMany(ctx context.Context, i Info, names []string) (map[string]Object, error) {
	out := map[string]Object{}
	for _, n := range names {
		o, err := scanObject(s.db.QueryRowContext(ctx, `SELECT `+objCols+` FROM objects WHERE calendar_id = ? AND name = ?`, i.ID, n))
		if errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		out[n] = o
	}
	return out, nil
}

// Range is the objects with an occurrence overlapping [from, to): the
// index picks candidates, recurrence is expanded to decide (RFC 4791 9.9).
func (s *Service) Range(ctx context.Context, i Info, from, to time.Time) ([]Object, error) {
	cands, err := s.objects(ctx, `SELECT `+objCols+` FROM objects WHERE calendar_id = ? AND start_utc < ?
		AND (end_utc IS NULL OR end_utc > ? OR (end_utc = start_utc AND start_utc >= ?)) ORDER BY start_utc`,
		i.ID, store.Time(to), store.Time(from), store.Time(from))
	if err != nil {
		return nil, err
	}
	loc := i.Location()
	out := cands[:0]
	for _, o := range cands {
		obj, _, err := ical.Read(o.Data, 0, loc)
		if err != nil {
			continue // stored before a stricter parser: not in any range
		}
		if ok, _ := obj.Overlaps(loc, from, to); ok {
			out = append(out, o)
		}
	}
	return out, nil
}

// Changes is what a sync-collection reports since a token.
type Changes struct {
	Names     []string // changed or deleted since, in the order they last changed
	Seq       int64    // the token to hand out
	Truncated bool     // limit was hit: the client asks again from Seq
}

// ChangesSince lists the objects changed after seq (RFC 6578). seq 0 is a
// first sync: every object, as changed. limit 0 is no limit.
func (s *Service) ChangesSince(ctx context.Context, i Info, seq int64, limit int) (Changes, error) {
	if seq == 0 {
		objs, err := s.objects(ctx, `SELECT `+objCols+` FROM objects WHERE calendar_id = ? ORDER BY name`, i.ID)
		if err != nil {
			return Changes{}, err
		}
		c := Changes{Seq: i.Seq}
		for _, o := range objs {
			c.Names = append(c.Names, o.Name)
		}
		return c, nil
	}
	if seq < i.PrunedSeq || seq > i.Seq {
		return Changes{}, ErrTokenExpired
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, name FROM changes WHERE calendar_id = ? AND seq > ? ORDER BY seq`, i.ID, seq)
	if err != nil {
		return Changes{}, err
	}
	defer rows.Close()
	c := Changes{Seq: i.Seq}
	seen := map[string]bool{}
	for rows.Next() {
		var n int64
		var name string
		if err := rows.Scan(&n, &name); err != nil {
			return Changes{}, err
		}
		if name == "" || seen[name] {
			continue
		}
		if limit > 0 && len(c.Names) == limit {
			c.Truncated = true
			c.Seq = n - 1
			break
		}
		seen[name] = true
		c.Names = append(c.Names, name)
	}
	return c, rows.Err()
}

// ETag of stored data: SHA-256, 16 bytes, quoted.
func ETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

func (p Precondition) check(exists bool, etag string) error {
	if p.IfNoneMatch && exists {
		return ErrPrecondition
	}
	if len(p.IfMatch) == 0 {
		return nil
	}
	if !exists {
		return ErrPrecondition
	}
	for _, m := range p.IfMatch {
		if m == "*" || m == etag || strings.TrimPrefix(m, "W/") == etag {
			return nil
		}
	}
	return ErrPrecondition
}

// Put stores an object as the client sent it: the bytes are kept, the
// index is computed beside them (PLAN.md 4.3, 4.5).
func (s *Service) Put(ctx context.Context, a Actor, r Ref, name string, data []byte, p Precondition) (Object, bool, error) {
	if !ValidName(name) {
		return Object{}, false, fmt.Errorf("%w: %q", ErrBadName, name)
	}
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return Object{}, false, err
	}
	if !i.Access.CanWrite() {
		return Object{}, false, ErrForbidden
	}
	set := s.settings.Get()
	_, idx, err := ical.Read(data, set.MaxObjectBytes, i.Location())
	if err != nil {
		return Object{}, false, err
	}
	etag := ETag(data)
	now := s.now()
	created := false
	var seq int64
	var dev any
	if a.DeviceID != 0 {
		dev = a.DeviceID
	}
	var end any
	if idx.End != nil {
		end = store.Time(*idx.End)
	}
	author := a.UserID
	if author == 0 {
		author = i.OwnerID // the operator imports in the owner's name
	}
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.recheck(ctx, tx, a, i, false); err != nil {
			return err
		}
		var oldETag, oldUID string
		err := tx.QueryRowContext(ctx, `SELECT etag, uid FROM objects WHERE calendar_id = ? AND name = ?`, i.ID, name).Scan(&oldETag, &oldUID)
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := p.check(exists, oldETag); err != nil {
			return err
		}
		var holder string
		err = tx.QueryRowContext(ctx, `SELECT name FROM objects WHERE calendar_id = ? AND uid = ? AND name <> ?`, i.ID, idx.UID, name).Scan(&holder)
		if err == nil {
			return &UIDConflict{Name: holder}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		action := "changed"
		if !exists {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM objects WHERE calendar_id = ?`, i.ID).Scan(&n); err != nil {
				return err
			}
			if n >= set.MaxEventsPerCalendar {
				return fmt.Errorf("%w: %d events (max_events_per_calendar)", ErrQuota, set.MaxEventsPerCalendar)
			}
			action, created = "created", true
			_, err = tx.ExecContext(ctx, `INSERT INTO objects(calendar_id, name, uid, etag, data, summary, start_utc, end_utc, recurring, all_day,
				created_by, modified_by, created_via, modified_via, device_id, created_at, modified_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				i.ID, name, idx.UID, etag, data, idx.Summary, store.Time(idx.Start), end, idx.Recurring, idx.AllDay,
				author, author, a.Via, a.Via, dev, store.Time(now), store.Time(now))
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE objects SET uid = ?, etag = ?, data = ?, summary = ?, start_utc = ?, end_utc = ?,
				recurring = ?, all_day = ?, modified_by = ?, modified_via = ?, device_id = ?, modified_at = ?
				WHERE calendar_id = ? AND name = ?`,
				idx.UID, etag, data, idx.Summary, store.Time(idx.Start), end, idx.Recurring, idx.AllDay,
				author, a.Via, dev, store.Time(now), i.ID, name)
		}
		if err != nil {
			return err
		}
		seq, err = s.change(ctx, tx, a, i.ID, name, idx.Summary, action)
		return err
	})
	if err != nil {
		return Object{}, false, err
	}
	action := "changed"
	if created {
		action = "created"
	}
	s.publish(i, seq, name, action, a.Name)
	return Object{Name: name, UID: idx.UID, ETag: etag, Data: data, Summary: idx.Summary, CreatedBy: author, ModifiedBy: author,
		CreatedAt: now, ModifiedAt: now}, created, nil
}

// Delete removes an object.
func (s *Service) Delete(ctx context.Context, a Actor, r Ref, name string, p Precondition) error {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return err
	}
	if !i.Access.CanWrite() {
		return ErrForbidden
	}
	var seq int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.recheck(ctx, tx, a, i, false); err != nil {
			return err
		}
		var etag, summary string
		err := tx.QueryRowContext(ctx, `SELECT etag, summary FROM objects WHERE calendar_id = ? AND name = ?`, i.ID, name).Scan(&etag, &summary)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := p.check(true, etag); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM objects WHERE calendar_id = ? AND name = ?`, i.ID, name); err != nil {
			return err
		}
		seq, err = s.change(ctx, tx, a, i.ID, name, summary, "deleted")
		return err
	})
	if err == nil {
		s.publish(i, seq, name, "deleted", a.Name)
	}
	return err
}

// Prune drops the change rows older than sync_token_retention_days. A token
// older than what is left gets valid-sync-token, and its client syncs in
// full (RFC 6578 3.2).
func (s *Service) Prune(ctx context.Context) (int64, error) {
	cutoff := store.Time(s.now().AddDate(0, 0, -s.settings.Get().SyncTokenRetentionDays))
	var removed int64
	err := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE calendars SET pruned_seq = max(pruned_seq,
			COALESCE((SELECT max(seq) FROM changes ch WHERE ch.calendar_id = calendars.id AND ch.at < ?), 0))`, cutoff); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM changes WHERE at < ?`, cutoff)
		if err != nil {
			return err
		}
		removed, _ = res.RowsAffected()
		return nil
	})
	return removed, err
}

// Token is a calendar's sync token on a server reached at base.
func Token(base string, i Info) string {
	return strings.TrimRight(base, "/") + "/ns/sync/" + strconv.FormatInt(i.ID, 10) + "/" + strconv.FormatInt(i.Seq, 10)
}

// ParseToken reads a token for calendar i; anything else is expired.
func ParseToken(base string, i Info, token string) (int64, error) {
	prefix := strings.TrimRight(base, "/") + "/ns/sync/" + strconv.FormatInt(i.ID, 10) + "/"
	if !strings.HasPrefix(token, prefix) {
		return 0, ErrTokenExpired
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(token, prefix), 10, 64)
	if err != nil || n < 1 {
		return 0, ErrTokenExpired
	}
	return n, nil
}
