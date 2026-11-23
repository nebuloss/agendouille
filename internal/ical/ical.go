// Package ical reads the iCalendar objects clients store (RFC 5545): it
// parses them, validates them (PLAN.md 4.5), computes the index columns the
// calendar keeps beside the text, and expands recurrence for time-range
// queries. Parser and recurrence engine are this project's own (DECISIONS
// 2026-11-23, "no iCalendar library").
//
// It never rewrites what a client sent: the calendar stores the client's
// bytes, and this package only reads them. It is pure: no store, no clock.
package ical

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // TZIDs must resolve on a host without zoneinfo
)

var (
	// ErrInvalid is data that is not a calendar object agendouille can keep
	// (CALDAV:valid-calendar-data).
	ErrInvalid = errors.New("invalid calendar data")
	// ErrUnsupported is a component other than VEVENT
	// (CALDAV:supported-calendar-component).
	ErrUnsupported = errors.New("only events (VEVENT) are supported")
	// ErrTooLarge is over the size limit (CALDAV:max-resource-size).
	ErrTooLarge = errors.New("calendar object too large")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Index is what the calendar keeps in columns next to an object's text.
type Index struct {
	UID       string
	Summary   string
	Start     time.Time  // the first instance's start, UTC
	End       *time.Time // the last instance's end, UTC; nil when the recurrence never ends
	Recurring bool
	AllDay    bool
}

// MaxIterations bounds the instances any one object is walked through, so
// a FREQ=SECONDLY rule cannot keep the server busy.
const MaxIterations = 100000

// Object is a parsed calendar object: all the VEVENTs of one UID.
type Object struct {
	Cal       *Component
	Master    *Component   // the VEVENT without RECURRENCE-ID, or nil
	Overrides []*Component // the VEVENTs with one
	Timezones []*Component
	UID       string
}

// Read validates data and returns the object and its index. loc is the
// calendar's zone, for floating times and unknown TZIDs.
func Read(data []byte, maxBytes int, loc *time.Location) (*Object, Index, error) {
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, Index{}, ErrTooLarge
	}
	roots, err := Decode(data)
	if err != nil {
		return nil, Index{}, err
	}
	if len(roots) != 1 {
		return nil, Index{}, invalid("a calendar object is one VCALENDAR, not %d", len(roots))
	}
	return FromComponent(roots[0], loc)
}

// FromComponent validates one VCALENDAR already decoded.
func FromComponent(cal *Component, loc *time.Location) (*Object, Index, error) {
	obj := &Object{Cal: cal}
	seen := map[int64]bool{}
	for _, c := range cal.Children {
		switch c.Name {
		case "VEVENT":
		case "VTIMEZONE":
			obj.Timezones = append(obj.Timezones, c)
			continue
		case "VTODO", "VJOURNAL", "VFREEBUSY":
			return nil, Index{}, ErrUnsupported
		default:
			continue // X- components: kept in the text, ignored here
		}
		uid := c.Text("UID")
		if uid == "" {
			return nil, Index{}, invalid("an event has no UID")
		}
		if obj.UID == "" {
			obj.UID = uid
		} else if uid != obj.UID {
			return nil, Index{}, invalid("one object holds the events of one UID; found %q and %q", obj.UID, uid)
		}
		start, err := startOf(c, loc)
		if err != nil {
			return nil, Index{}, err
		}
		end, err := endOf(c, start, loc)
		if err != nil {
			return nil, Index{}, err
		}
		if end.t.Before(start.t) {
			return nil, Index{}, invalid("an event ends before it starts")
		}
		if rid := c.Get("RECURRENCE-ID"); rid != nil {
			t, err := propTime(rid, loc)
			if err != nil {
				return nil, Index{}, invalid("RECURRENCE-ID: %v", err)
			}
			if seen[t.t.Unix()] {
				return nil, Index{}, invalid("two overrides of the same occurrence")
			}
			seen[t.t.Unix()] = true
			obj.Overrides = append(obj.Overrides, c)
			continue
		}
		if obj.Master != nil {
			return nil, Index{}, invalid("two events without RECURRENCE-ID share a UID")
		}
		if r := c.Get("RRULE"); r != nil {
			if _, err := ParseRule(r.Value, start.t.Location()); err != nil {
				return nil, Index{}, invalid("RRULE: %v", err)
			}
		}
		obj.Master = c
	}
	if obj.UID == "" {
		return nil, Index{}, invalid("no event in the object")
	}
	idx, err := obj.index(loc)
	if err != nil {
		return nil, Index{}, err
	}
	return obj, idx, nil
}

// when is a parsed DTSTART-like value.
type when struct {
	t      time.Time
	allDay bool
}

// propTime reads a DATE or DATE-TIME value: UTC with Z, in its TZID's zone,
// or floating (in loc).
func propTime(p *Prop, loc *time.Location) (when, error) {
	return parseTime(strings.TrimSpace(p.Value), p.Param("VALUE"), p.Param("TZID"), loc)
}

func parseTime(v, valueType, tzid string, loc *time.Location) (when, error) {
	if strings.EqualFold(valueType, "DATE") || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, loc)
		return when{t: t, allDay: true}, err
	}
	if strings.HasSuffix(v, "Z") || strings.HasSuffix(v, "z") {
		t, err := time.Parse("20060102T150405Z", strings.ToUpper(v))
		return when{t: t}, err
	}
	zone := loc
	if tzid != "" {
		zone = Zone(tzid, loc)
	}
	t, err := time.ParseInLocation("20060102T150405", v, zone)
	return when{t: t}, err
}

func startOf(c *Component, loc *time.Location) (when, error) {
	p := c.Get("DTSTART")
	if p == nil {
		return when{}, invalid("an event has no DTSTART")
	}
	w, err := propTime(p, loc)
	if err != nil {
		return when{}, invalid("DTSTART %q: %v", p.Value, err)
	}
	return w, nil
}

// endOf is DTEND, or DTSTART + DURATION, or the end RFC 5545 3.6.1 gives an
// event with neither: the next day for a date, the same instant otherwise.
func endOf(c *Component, start when, loc *time.Location) (when, error) {
	if p := c.Get("DTEND"); p != nil {
		w, err := propTime(p, loc)
		if err != nil {
			return when{}, invalid("DTEND %q: %v", p.Value, err)
		}
		return w, nil
	}
	if p := c.Get("DURATION"); p != nil {
		d, err := ParseDuration(p.Value)
		if err != nil {
			return when{}, invalid("DURATION %q: %v", p.Value, err)
		}
		return when{t: d.AddTo(start.t), allDay: start.allDay}, nil
	}
	if start.allDay {
		return when{t: start.t.AddDate(0, 0, 1), allDay: true}, nil
	}
	return start, nil
}

// Duration is an RFC 5545 3.3.6 duration. Days and weeks are nominal (a day
// keeps the wall time across a DST change); hours, minutes and seconds
// are exact.
type Duration struct {
	Days  int
	Exact time.Duration
}

// AddTo adds the duration to t.
func (d Duration) AddTo(t time.Time) time.Time { return t.AddDate(0, 0, d.Days).Add(d.Exact) }

// ParseDuration reads [+-]P[nW][nD][T[nH][nM][nS]].
func ParseDuration(s string) (Duration, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if !strings.HasPrefix(s, "P") || len(s) < 3 {
		return Duration{}, fmt.Errorf("not a duration")
	}
	s = s[1:]
	var d Duration
	inTime := false
	num := ""
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			num += string(r)
		case r == 'T':
			inTime = true
		default:
			if num == "" {
				return Duration{}, fmt.Errorf("not a duration")
			}
			n, err := strconv.Atoi(num)
			if err != nil || n > 1000000 {
				return Duration{}, fmt.Errorf("not a duration")
			}
			num = ""
			switch {
			case r == 'W' && !inTime:
				d.Days += 7 * n
			case r == 'D' && !inTime:
				d.Days += n
			case r == 'H' && inTime:
				d.Exact += time.Duration(n) * time.Hour
			case r == 'M' && inTime:
				d.Exact += time.Duration(n) * time.Minute
			case r == 'S' && inTime:
				d.Exact += time.Duration(n) * time.Second
			default:
				return Duration{}, fmt.Errorf("not a duration")
			}
		}
	}
	if num != "" {
		return Duration{}, fmt.Errorf("not a duration")
	}
	if d.Days > 36600 || d.Exact > 36600*24*time.Hour || d.Exact < 0 {
		return Duration{}, fmt.Errorf("longer than a century")
	}
	if neg {
		d.Days, d.Exact = -d.Days, -d.Exact
	}
	return d, nil
}

// Occurrence is one instance of an object in a range.
type Occurrence struct {
	// RecurrenceID is the original start of the instance, or nil for an
	// event that does not recur.
	RecurrenceID *time.Time
	Start, End   time.Time
	AllDay       bool
	Overridden   bool
	Event        *Component // the master, or the override that replaces this instance
}

func overlaps(start, end, from, to time.Time) bool {
	if end.Equal(start) {
		return !start.Before(from) && start.Before(to)
	}
	return start.Before(to) && end.After(from)
}

// span is the master's start and its length, as a nominal-days duration
// for dates so every instance keeps its length.
func (o *Object) span(loc *time.Location) (when, Duration, error) {
	start, err := startOf(o.Master, loc)
	if err != nil {
		return when{}, Duration{}, err
	}
	if p := o.Master.Get("DURATION"); p != nil && o.Master.Get("DTEND") == nil {
		d, err := ParseDuration(p.Value)
		return start, d, err
	}
	end, err := endOf(o.Master, start, loc)
	if err != nil {
		return when{}, Duration{}, err
	}
	if start.allDay {
		days := int(end.t.Sub(start.t).Round(24*time.Hour) / (24 * time.Hour))
		return start, Duration{Days: days}, nil
	}
	return start, Duration{Exact: end.t.Sub(start.t)}, nil
}

// instances returns the master's recurrence set as an iterator, or nil
// when it does not recur: RRULE from DTSTART, plus RDATEs, minus EXDATEs.
// from, when not zero, lets the rule start near it (Rule.IteratorFrom).
func (o *Object) instances(loc *time.Location, from time.Time) (func() (time.Time, bool), bool, error) {
	start, _, err := o.span(loc)
	if err != nil {
		return nil, false, err
	}
	rr := o.Master.Get("RRULE")
	rdates := o.Master.All("RDATE")
	if rr == nil && len(rdates) == 0 {
		return nil, false, nil
	}
	var rule func() (time.Time, bool)
	ends := true
	if rr != nil {
		r, err := ParseRule(rr.Value, start.t.Location())
		if err != nil {
			return nil, false, invalid("RRULE: %v", err)
		}
		if from.IsZero() {
			rule = r.Iterator(start.t)
		} else {
			rule = r.IteratorFrom(start.t, from)
		}
		ends = r.Ends()
	} else {
		done := false
		rule = func() (time.Time, bool) { // RDATE alone: DTSTART is the first instance
			if done {
				return time.Time{}, false
			}
			done = true
			return start.t, true
		}
	}
	var extra []time.Time
	for _, p := range rdates {
		extra = append(extra, listTimes(p, loc)...)
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Before(extra[j]) })
	excluded := map[int64]bool{}
	for _, p := range o.Master.All("EXDATE") {
		for _, t := range listTimes(p, loc) {
			excluded[t.Unix()] = true
		}
	}
	// Merge the rule's instances and the RDATEs in order, dropping EXDATEs
	// and duplicates.
	var next time.Time
	haveNext, ruleDone := false, false
	var last time.Time
	started := false
	return func() (time.Time, bool) {
		for {
			if !haveNext && !ruleDone {
				next, haveNext = rule()
				ruleDone = !haveNext
			}
			var t time.Time
			switch {
			case haveNext && (len(extra) == 0 || !extra[0].Before(next)):
				t, haveNext = next, false
			case len(extra) > 0:
				t, extra = extra[0], extra[1:]
			default:
				return time.Time{}, false
			}
			if excluded[t.Unix()] || (started && t.Equal(last)) {
				continue
			}
			started, last = true, t
			return t, true
		}
	}, ends, nil
}

// listTimes reads a comma-separated RDATE or EXDATE. PERIOD values are
// skipped: no client in PLAN.md 5.4 writes them.
func listTimes(p *Prop, loc *time.Location) []time.Time {
	if strings.EqualFold(p.Param("VALUE"), "PERIOD") {
		return nil
	}
	var out []time.Time
	for _, v := range strings.Split(p.Value, ",") {
		if w, err := parseTime(strings.TrimSpace(v), p.Param("VALUE"), p.Param("TZID"), loc); err == nil {
			out = append(out, w.t)
		}
	}
	return out
}

// Expand lists the occurrences overlapping [from, to), sorted by start, at
// most max of them (truncated reports a cut).
func (o *Object) Expand(loc *time.Location, from, to time.Time, max int) (occ []Occurrence, truncated bool, err error) {
	overridden := map[int64]bool{}
	for _, c := range o.Overrides {
		rid, err := propTime(c.Get("RECURRENCE-ID"), loc)
		if err != nil {
			continue
		}
		overridden[rid.t.Unix()] = true
		start, err := startOf(c, loc)
		if err != nil {
			return nil, false, err
		}
		end, err := endOf(c, start, loc)
		if err != nil {
			return nil, false, err
		}
		if overlaps(start.t, end.t, from, to) {
			r := rid.t
			occ = append(occ, Occurrence{RecurrenceID: &r, Start: start.t, End: end.t, AllDay: start.allDay, Overridden: true, Event: c})
		}
	}
	if o.Master != nil {
		start, dur, err := o.span(loc)
		if err != nil {
			return nil, false, err
		}
		// Start the rule a little before the range: an instance that began
		// before from may still overlap it.
		next, _, err := o.instances(loc, from.AddDate(0, 0, -dur.Days-1).Add(-dur.Exact))
		if err != nil {
			return nil, false, err
		}
		if next == nil {
			if end := dur.AddTo(start.t); overlaps(start.t, end, from, to) {
				occ = append(occ, Occurrence{Start: start.t, End: end, AllDay: start.allDay, Event: o.Master})
			}
		} else {
			for i := 0; i < MaxIterations; i++ {
				t, ok := next()
				if !ok || !t.Before(to) {
					break
				}
				end := dur.AddTo(t)
				if overridden[t.Unix()] || !overlaps(t, end, from, to) {
					continue
				}
				r := t
				occ = append(occ, Occurrence{RecurrenceID: &r, Start: t, End: end, AllDay: start.allDay, Event: o.Master})
			}
		}
	}
	sort.SliceStable(occ, func(i, j int) bool { return occ[i].Start.Before(occ[j].Start) })
	if max > 0 && len(occ) > max {
		return occ[:max], true, nil
	}
	return occ, false, nil
}

// Overlaps reports whether any occurrence overlaps [from, to) (RFC 4791
// 9.9).
func (o *Object) Overlaps(loc *time.Location, from, to time.Time) (bool, error) {
	occ, _, err := o.Expand(loc, from, to, 1)
	return len(occ) > 0, err
}

// index computes the columns: the first start, and the last end unless the
// recurrence is open-ended.
func (o *Object) index(loc *time.Location) (Index, error) {
	idx := Index{UID: o.UID}
	var first, last time.Time
	have := false
	consider := func(s, e time.Time) {
		if !have || s.Before(first) {
			first = s
		}
		if !have || e.After(last) {
			last = e
		}
		have = true
	}
	open := false
	if o.Master != nil {
		idx.Summary = o.Master.Text("SUMMARY")
		start, dur, err := o.span(loc)
		if err != nil {
			return Index{}, err
		}
		idx.AllDay = start.allDay
		next, ends, err := o.instances(loc, time.Time{})
		if err != nil {
			return Index{}, err
		}
		switch {
		case next == nil:
			consider(start.t, dur.AddTo(start.t))
		case !ends:
			idx.Recurring, open = true, true
			consider(start.t, dur.AddTo(start.t))
		default:
			idx.Recurring = true
			n := 0
			for ; n < MaxIterations; n++ {
				t, ok := next()
				if !ok {
					break
				}
				consider(t, dur.AddTo(t))
			}
			if n == MaxIterations {
				open = true // too long to walk: treated as never ending
			}
			if !have { // every instance excluded: DTSTART stays the start
				consider(start.t, start.t)
			}
		}
	}
	for _, c := range o.Overrides {
		if idx.Summary == "" {
			idx.Summary = c.Text("SUMMARY")
		}
		start, err := startOf(c, loc)
		if err != nil {
			return Index{}, err
		}
		end, err := endOf(c, start, loc)
		if err != nil {
			return Index{}, err
		}
		if o.Master == nil {
			idx.AllDay = start.allDay
		}
		consider(start.t, end.t)
	}
	idx.Start = clamp(first.UTC())
	if !open {
		e := clamp(last.UTC())
		idx.End = &e
	}
	return idx, nil
}

// latest is the last instant the index stores: times are kept as text,
// and a year of five digits would sort before every other.
var latest = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
var earliest = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)

func clamp(t time.Time) time.Time {
	if t.After(latest) {
		return latest
	}
	if t.Before(earliest) {
		return earliest
	}
	return t
}
