package calendar

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// CalendarStats is one calendar as the admin's stats page shows it: how
// big, how alive, changed by whom and from what. Counts, times, users and
// devices only: the change log's summaries are never selected (PLAN.md 10).
type CalendarStats struct {
	CalendarMeta
	Description string     `json:"description"`
	Recurring   int        `json:"recurring"`
	AllDay      int        `json:"all_day"`
	Changes     int        `json:"changes"`
	LastChange  *time.Time `json:"last_change,omitempty"`
	Daily       []DayCount `json:"daily"` // the last Days days, oldest first, every day present
	ByUser      []WhoCount `json:"by_user"`
	ByVia       []ViaCount `json:"by_via"`
	ByDevice    []DevCount `json:"by_device"`
	Days        int        `json:"days"`
}

// DayCount is one day of changes to a calendar's events.
type DayCount struct {
	Day     string `json:"day"` // YYYY-MM-DD in UTC
	Created int    `json:"created"`
	Changed int    `json:"changed"`
	Deleted int    `json:"deleted"`
}

// WhoCount is how much one user changed a calendar.
type WhoCount struct {
	User    string    `json:"user"`
	Changes int       `json:"changes"`
	Last    time.Time `json:"last"`
}

// ViaCount is how many changes came through one way in: dav, web, app, import.
type ViaCount struct {
	Via     string `json:"via"`
	Changes int    `json:"changes"`
}

// DevCount is one device that wrote to a calendar.
type DevCount struct {
	Label   string    `json:"label"`
	User    string    `json:"user"`
	Kind    string    `json:"kind"`
	Changes int       `json:"changes"`
	Last    time.Time `json:"last"`
}

// StatsDays is how far back the daily counts go.
const StatsDays = 90

// AdminStats is a calendar's stats (the admin's calendar page), by ref
// ("owner~slug").
func (s *Service) AdminStats(ctx context.Context, ref string) (CalendarStats, error) {
	owner, slug, ok := strings.Cut(ref, "~")
	if !ok || owner == "" || slug == "" {
		return CalendarStats{}, ErrNotFound
	}
	var st CalendarStats
	var id int64
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT c.id, c.name, c.color, c.timezone, c.description, c.created_at,
		(SELECT count(*) FROM objects o WHERE o.calendar_id = c.id),
		(SELECT COALESCE(sum(length(o.data)), 0) FROM objects o WHERE o.calendar_id = c.id),
		(SELECT count(*) FROM objects o WHERE o.calendar_id = c.id AND o.recurring = 1),
		(SELECT count(*) FROM objects o WHERE o.calendar_id = c.id AND o.all_day = 1)
		FROM calendars c JOIN users u ON u.id = c.owner_id WHERE u.name = ? AND c.slug = ? AND c.deleted_at IS NULL`, owner, slug).
		Scan(&id, &st.Name, &st.Color, &st.Timezone, &st.Description, &created, &st.Events, &st.Bytes, &st.Recurring, &st.AllDay)
	if errors.Is(err, sql.ErrNoRows) {
		return CalendarStats{}, ErrNotFound
	}
	if err != nil {
		return CalendarStats{}, err
	}
	st.Ref, st.Owner, st.Days = owner+"~"+slug, owner, StatsDays
	st.CreatedAt, _ = store.ParseTime(created)
	if st.Shares, err = s.sharing.Shares(ctx, id); err != nil {
		return CalendarStats{}, err
	}

	// Event changes only (name <> ''): the calendar's own are in the audit.
	var last sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT count(*), max(at) FROM changes WHERE calendar_id = ? AND name <> ''`, id).Scan(&st.Changes, &last); err != nil {
		return CalendarStats{}, err
	}
	if last.Valid {
		t, _ := store.ParseTime(last.String)
		st.LastChange = &t
	}

	now := s.now().UTC()
	first := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(StatsDays - 1))
	days := map[string]*DayCount{}
	for i := 0; i < StatsDays; i++ {
		d := first.AddDate(0, 0, i).Format("2006-01-02")
		st.Daily = append(st.Daily, DayCount{Day: d})
	}
	for i := range st.Daily {
		days[st.Daily[i].Day] = &st.Daily[i]
	}
	rows, err := s.db.QueryContext(ctx, `SELECT substr(at, 1, 10), action, count(*) FROM changes
		WHERE calendar_id = ? AND name <> '' AND at >= ? GROUP BY 1, 2`, id, store.Time(first))
	if err != nil {
		return CalendarStats{}, err
	}
	for rows.Next() {
		var day, action string
		var n int
		if err := rows.Scan(&day, &action, &n); err != nil {
			rows.Close()
			return CalendarStats{}, err
		}
		if d := days[day]; d != nil {
			switch action {
			case "created":
				d.Created += n
			case "deleted":
				d.Deleted += n
			default:
				d.Changed += n
			}
		}
	}
	rows.Close()

	st.ByUser = []WhoCount{}
	rows, err = s.db.QueryContext(ctx, `SELECT COALESCE(u.name, ''), count(*), max(ch.at) FROM changes ch LEFT JOIN users u ON u.id = ch.user_id
		WHERE ch.calendar_id = ? AND ch.name <> '' GROUP BY 1 ORDER BY 2 DESC`, id)
	if err != nil {
		return CalendarStats{}, err
	}
	for rows.Next() {
		var w WhoCount
		var at string
		if err := rows.Scan(&w.User, &w.Changes, &at); err != nil {
			rows.Close()
			return CalendarStats{}, err
		}
		w.Last, _ = store.ParseTime(at)
		st.ByUser = append(st.ByUser, w)
	}
	rows.Close()

	st.ByVia = []ViaCount{}
	rows, err = s.db.QueryContext(ctx, `SELECT via, count(*) FROM changes WHERE calendar_id = ? AND name <> '' GROUP BY 1 ORDER BY 2 DESC`, id)
	if err != nil {
		return CalendarStats{}, err
	}
	for rows.Next() {
		var v ViaCount
		if err := rows.Scan(&v.Via, &v.Changes); err != nil {
			rows.Close()
			return CalendarStats{}, err
		}
		st.ByVia = append(st.ByVia, v)
	}
	rows.Close()

	st.ByDevice = []DevCount{}
	rows, err = s.db.QueryContext(ctx, `SELECT d.label, u.name, d.kind, count(*), max(ch.at) FROM changes ch
		JOIN devices d ON d.id = ch.device_id JOIN users u ON u.id = d.user_id
		WHERE ch.calendar_id = ? AND ch.name <> '' GROUP BY d.id ORDER BY 4 DESC`, id)
	if err != nil {
		return CalendarStats{}, err
	}
	for rows.Next() {
		var d DevCount
		var at string
		if err := rows.Scan(&d.Label, &d.User, &d.Kind, &d.Changes, &at); err != nil {
			rows.Close()
			return CalendarStats{}, err
		}
		d.Last, _ = store.ParseTime(at)
		st.ByDevice = append(st.ByDevice, d)
	}
	rows.Close()
	return st, rows.Err()
}
