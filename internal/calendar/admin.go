package calendar

import (
	"context"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

// These are the only calendar calls the admin UI makes (CONTRACTS.md 1.1):
// they answer with metadata and never with an event's text, title or place
// (PLAN.md 10, "privacy from the admin").

// CalendarMeta is a calendar as the admin sees it.
type CalendarMeta struct {
	Ref       string          `json:"ref"`
	Owner     string          `json:"owner"`
	Name      string          `json:"name"`
	Color     string          `json:"color"`
	Timezone  string          `json:"timezone"`
	Events    int             `json:"events"`
	Bytes     int64           `json:"bytes"`
	Shares    []sharing.Share `json:"shares"`
	CreatedAt time.Time       `json:"created_at"`
}

// AdminCalendars lists every live calendar, or one user's when owner is
// set.
func (s *Service) AdminCalendars(ctx context.Context, owner string) ([]CalendarMeta, error) {
	q := `SELECT c.id, u.name, c.slug, c.name, c.color, c.timezone, c.created_at,
		(SELECT count(*) FROM objects o WHERE o.calendar_id = c.id),
		(SELECT COALESCE(sum(length(o.data)), 0) FROM objects o WHERE o.calendar_id = c.id)
		FROM calendars c JOIN users u ON u.id = c.owner_id WHERE c.deleted_at IS NULL`
	args := []any{}
	if owner != "" {
		q += ` AND u.name = ?`
		args = append(args, owner)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY u.name, c.slug`, args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		id int64
		m  CalendarMeta
	}
	var all []row
	for rows.Next() {
		var r row
		var slug, created string
		if err := rows.Scan(&r.id, &r.m.Owner, &slug, &r.m.Name, &r.m.Color, &r.m.Timezone, &created, &r.m.Events, &r.m.Bytes); err != nil {
			rows.Close()
			return nil, err
		}
		r.m.Ref = r.m.Owner + "~" + slug
		r.m.CreatedAt, _ = store.ParseTime(created)
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CalendarMeta, 0, len(all))
	for _, r := range all {
		if r.m.Shares, err = s.sharing.Shares(ctx, r.id); err != nil {
			return nil, err
		}
		out = append(out, r.m)
	}
	return out, nil
}

// SharedWith lists the calendars shared with a user, and through what.
type SharedWith struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	Access string `json:"access"`
	Via    string `json:"via"` // u:<user> for a direct share, g:<group> through a group
}

// AdminSharedWith is what is shared with a user (the admin's user page).
func (s *Service) AdminSharedWith(ctx context.Context, userID int64) ([]SharedWith, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.name || '~' || c.slug, c.name, sh.access, 'u:' || me.name
		FROM shares sh JOIN calendars c ON c.id = sh.calendar_id JOIN users u ON u.id = c.owner_id JOIN users me ON me.id = sh.user_id
		WHERE sh.user_id = ? AND c.deleted_at IS NULL
		UNION ALL
		SELECT u.name || '~' || c.slug, c.name, sh.access, 'g:' || g.name
		FROM shares sh JOIN calendars c ON c.id = sh.calendar_id JOIN users u ON u.id = c.owner_id JOIN groups g ON g.id = sh.group_id
		WHERE sh.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?) AND c.deleted_at IS NULL AND c.owner_id <> ?
		ORDER BY 1, 4`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SharedWith{}
	for rows.Next() {
		var w SharedWith
		if err := rows.Scan(&w.Ref, &w.Name, &w.Access, &w.Via); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Activity is one line of the admin's activity log (CONTRACTS.md 3.5.2):
// who did what, never what the event said.
type Activity struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"` // event, calendar, share, user, group, device, enrolment, setting, admin, approval
	Action   string    `json:"action"`
	By       string    `json:"by"`
	Calendar string    `json:"calendar,omitempty"`
	Via      string    `json:"via,omitempty"`
	Device   string    `json:"device,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

// ActivityFilter narrows the log. Before pages: the next page starts
// before the last line's time.
type ActivityFilter struct {
	User     string
	Calendar string
	Kind     string
	Before   time.Time
	Limit    int
}

// AdminActivity reads the change log (events) and the audit log (everything
// else), newest first. Event titles are in the change log for the users'
// own history; this query never selects them.
func (s *Service) AdminActivity(ctx context.Context, f ActivityFilter) ([]Activity, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	q := `SELECT at, kind, action, by, calendar, via, device, detail FROM (
		SELECT ch.at AS at, 'event' AS kind, ch.action AS action, COALESCE(u.name, '') AS by,
		       ou.name || '~' || c.slug AS calendar, ch.via AS via, COALESCE(d.label, '') AS device, '' AS detail
		FROM changes ch JOIN calendars c ON c.id = ch.calendar_id JOIN users ou ON ou.id = c.owner_id
		LEFT JOIN users u ON u.id = ch.user_id LEFT JOIN devices d ON d.id = ch.device_id
		WHERE ch.name <> ''
		UNION ALL
		SELECT a.at, a.kind, a.action, a.actor, COALESCE(ou.name || '~' || c.slug, ''), '', '', a.detail
		FROM audit a LEFT JOIN calendars c ON c.id = a.calendar_id LEFT JOIN users ou ON ou.id = c.owner_id
	) WHERE 1 = 1`
	var args []any
	if f.User != "" {
		q += ` AND (by = ? OR detail = ? OR detail LIKE ? OR detail LIKE ? OR calendar LIKE ?)`
		args = append(args, f.User, f.User, f.User+" %", "% "+f.User+"%", f.User+"~%")
	}
	if f.Calendar != "" {
		q += ` AND calendar = ?`
		args = append(args, f.Calendar)
	}
	if f.Kind != "" {
		q += ` AND kind = ?`
		args = append(args, f.Kind)
	}
	if !f.Before.IsZero() {
		q += ` AND at < ?`
		args = append(args, store.Time(f.Before))
	}
	q += ` ORDER BY at DESC LIMIT ?`
	args = append(args, f.Limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		var at string
		if err := rows.Scan(&at, &a.Kind, &a.Action, &a.By, &a.Calendar, &a.Via, &a.Device, &a.Detail); err != nil {
			return nil, err
		}
		a.At, _ = store.ParseTime(at)
		if a.Kind == "setting" {
			a.Detail = strings.TrimSpace(a.Detail)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Counts are the overview's numbers.
type Counts struct {
	Calendars int   `json:"calendars"`
	Events    int   `json:"events"`
	Bytes     int64 `json:"bytes"`
}

// AdminCounts counts live calendars and their events.
func (s *Service) AdminCounts(ctx context.Context) (Counts, error) {
	var c Counts
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM calendars WHERE deleted_at IS NULL),
		(SELECT count(*) FROM objects),
		(SELECT COALESCE(sum(length(data)), 0) FROM objects)`).Scan(&c.Calendars, &c.Events, &c.Bytes)
	return c, err
}
