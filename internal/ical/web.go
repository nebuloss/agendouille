package ical

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// The web app's simple event shape (CONTRACTS.md 3.3) and the edits it
// makes on stored objects (PLAN.md 4.6). Edits patch: only the properties
// the form owns change; alarms, attendees and X- properties stay.
//
// Local times are "YYYY-MM-DDTHH:MM:SS" in the family zone; dates are
// "YYYY-MM-DD".

// EventInput is what the editor sends.
type EventInput struct {
	Summary     string  `json:"summary"`
	Location    string  `json:"location"`
	Description string  `json:"description"`
	AllDay      bool    `json:"all_day"`
	Start       string  `json:"start"`
	End         string  `json:"end"`
	RRule       *string `json:"rrule"`
}

// OverrideView is one moved or changed occurrence.
type OverrideView struct {
	RecurrenceID string `json:"recurrence_id"`
	Start        string `json:"start"`
	End          string `json:"end"`
	Summary      string `json:"summary"`
}

// EventView is what the editor shows of an object.
type EventView struct {
	UID           string         `json:"uid"`
	Summary       string         `json:"summary"`
	Location      string         `json:"location"`
	Description   string         `json:"description"`
	AllDay        bool           `json:"all_day"`
	Start         string         `json:"start"`
	End           string         `json:"end"`
	RRule         string         `json:"rrule"`
	Exdates       []string       `json:"exdates"`
	Overrides     []OverrideView `json:"overrides"`
	HasAlarms     bool           `json:"has_alarms"`
	HasAttendees  bool           `json:"has_attendees"`
	EditableRRule bool           `json:"editable_rrule"`
}

// Scope of an edit or a deletion of a recurring event.
type Scope string

const (
	ScopeThis      Scope = "this"
	ScopeFollowing Scope = "following"
	ScopeAll       Scope = "all"
)

const (
	localLayout = "2006-01-02T15:04:05"
	dateLayout  = "2006-01-02"
	prodID      = "-//agendouille//agendouille//FR"
)

// LocalString formats an instant (or a date) for the web app.
func LocalString(t time.Time, allDay bool, family *time.Location) string {
	if allDay {
		return t.Format(dateLayout)
	}
	return t.In(family).Format(localLayout)
}

// ParseLocal reads a web local time or date in the family zone.
func ParseLocal(s string, allDay bool, family *time.Location) (time.Time, error) {
	if allDay {
		if len(s) > 10 {
			s = s[:10]
		}
		return time.ParseInLocation(dateLayout, s, family)
	}
	if len(s) == 16 { // "YYYY-MM-DDTHH:MM" from an <input type=datetime-local>
		s += ":00"
	}
	return time.ParseInLocation(localLayout, s, family)
}

// NewUID is a fresh UID for an event the web app makes.
func NewUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:] + "@agendouille"
}

func stamp(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// timeProp writes a DTSTART-like property: a DATE, or a DATE-TIME with the
// family zone's TZID (UTC stays UTC).
func timeProp(name string, t time.Time, allDay bool) Prop {
	if allDay {
		return Prop{Name: name, Params: map[string][]string{"VALUE": {"DATE"}}, Value: t.Format("20060102")}
	}
	if t.Location() == time.UTC {
		return Prop{Name: name, Value: stamp(t)}
	}
	return Prop{Name: name, Params: map[string][]string{"TZID": {t.Location().String()}}, Value: t.Format("20060102T150405")}
}

// like writes t the way an existing property is written (same TZID and
// value type), for RECURRENCE-ID and EXDATE, which must match DTSTART.
func like(p *Prop, name string, t time.Time, loc *time.Location) Prop {
	if p == nil {
		return timeProp(name, t, false)
	}
	if strings.EqualFold(p.Param("VALUE"), "DATE") || len(strings.TrimSpace(p.Value)) == 8 {
		return Prop{Name: name, Params: map[string][]string{"VALUE": {"DATE"}}, Value: t.Format("20060102")}
	}
	if strings.HasSuffix(strings.TrimSpace(p.Value), "Z") {
		return Prop{Name: name, Value: stamp(t)}
	}
	zone := loc
	if tz := p.Param("TZID"); tz != "" {
		zone = Zone(tz, loc)
		return Prop{Name: name, Params: map[string][]string{"TZID": {tz}}, Value: t.In(zone).Format("20060102T150405")}
	}
	return Prop{Name: name, Value: t.In(zone).Format("20060102T150405")}
}

func (c *Component) set(p Prop) {
	for i := range c.Props {
		if c.Props[i].Name == p.Name {
			c.Props[i] = p
			c.del(p.Name, i+1)
			return
		}
	}
	c.Props = append(c.Props, p)
}

// del removes the properties named name from index from on.
func (c *Component) del(name string, from int) {
	out := c.Props[:from]
	for _, p := range c.Props[from:] {
		if p.Name != name {
			out = append(out, p)
		}
	}
	c.Props = out
}

func (c *Component) setText(name, v string) {
	if v == "" {
		c.del(name, 0)
		return
	}
	c.set(Prop{Name: name, Value: EscapeText(v)})
}

// touch marks an edit: DTSTAMP, LAST-MODIFIED, SEQUENCE + 1.
func touch(c *Component, now time.Time) {
	c.set(Prop{Name: "DTSTAMP", Value: stamp(now)})
	c.set(Prop{Name: "LAST-MODIFIED", Value: stamp(now)})
	seq := 0
	if p := c.Get("SEQUENCE"); p != nil {
		fmt.Sscanf(p.Value, "%d", &seq)
	}
	c.set(Prop{Name: "SEQUENCE", Value: fmt.Sprint(seq + 1)})
}

// input is a validated EventInput.
type input struct {
	in         EventInput
	start, end time.Time
}

func checkInput(in EventInput, family *time.Location) (input, error) {
	in.Summary = strings.TrimSpace(in.Summary)
	if in.Summary == "" {
		in.Summary = "(sans titre)"
	}
	if len(in.Summary) > 500 || len(in.Location) > 500 || len(in.Description) > 20000 {
		return input{}, ErrFieldTooLong
	}
	start, err := ParseLocal(in.Start, in.AllDay, family)
	if err != nil {
		return input{}, invalid("start %q: %v", in.Start, err)
	}
	end := start
	if in.End != "" {
		if end, err = ParseLocal(in.End, in.AllDay, family); err != nil {
			return input{}, invalid("end %q: %v", in.End, err)
		}
	} else if in.AllDay {
		end = start.AddDate(0, 0, 1)
	} else {
		end = start.Add(time.Hour)
	}
	if in.AllDay && !end.After(start) {
		end = start.AddDate(0, 0, 1) // a one-day event whose end the form left on the same day
	}
	if end.Before(start) {
		return input{}, ErrEndsBeforeStart
	}
	if in.RRule != nil && *in.RRule != "" {
		rule := untilOf(*in.RRule, in.AllDay, family)
		if _, err := ParseRule(rule, family); err != nil {
			return input{}, invalid("repeat: %v", err)
		}
		in.RRule = &rule
	}
	return input{in: in, start: start, end: end}, nil
}

// untilOf makes the editor's UNTIL (a date) what RFC 5545 3.3.10 wants:
// a DATE for an all-day event, else the last second of that day in the
// family zone, in UTC.
func untilOf(rule string, allDay bool, family *time.Location) string {
	var parts []string
	for _, p := range strings.Split(rule, ";") {
		k, v, _ := strings.Cut(p, "=")
		if strings.EqualFold(k, "UNTIL") && len(v) == 8 && !allDay {
			if d, err := time.ParseInLocation("20060102", v, family); err == nil {
				v = d.AddDate(0, 0, 1).Add(-time.Second).UTC().Format("20060102T150405Z")
				p = "UNTIL=" + v
			}
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ";")
}

// apply writes the form's fields onto an event.
func (x input) apply(c *Component, withRule bool) {
	c.setText("SUMMARY", x.in.Summary)
	c.setText("LOCATION", x.in.Location)
	c.setText("DESCRIPTION", x.in.Description)
	c.set(timeProp("DTSTART", x.start, x.in.AllDay))
	c.del("DURATION", 0)
	c.set(timeProp("DTEND", x.end, x.in.AllDay))
	if withRule {
		if x.in.RRule != nil && *x.in.RRule != "" {
			c.set(Prop{Name: "RRULE", Value: strings.ToUpper(*x.in.RRule)})
		} else if x.in.RRule != nil {
			c.del("RRULE", 0)
			c.del("RDATE", 0)
			c.del("EXDATE", 0)
		}
	}
}

// New makes the text of a new object from the form.
func New(in EventInput, family *time.Location, now time.Time) ([]byte, string, error) {
	x, err := checkInput(in, family)
	if err != nil {
		return nil, "", err
	}
	uid := NewUID()
	ev := &Component{Name: "VEVENT", Props: []Prop{
		{Name: "UID", Value: uid},
		{Name: "DTSTAMP", Value: stamp(now)},
		{Name: "CREATED", Value: stamp(now)},
		{Name: "LAST-MODIFIED", Value: stamp(now)},
		{Name: "SEQUENCE", Value: "0"},
	}}
	x.apply(ev, true)
	cal := &Component{Name: "VCALENDAR", Props: []Prop{{Name: "VERSION", Value: "2.0"}, {Name: "PRODID", Value: prodID}},
		Children: []*Component{ev}}
	return Encode(cal), uid, nil
}

// View reads an object for the editor.
func View(data []byte, cal, family *time.Location) (EventView, error) {
	obj, _, err := Read(data, 0, cal)
	if err != nil {
		return EventView{}, err
	}
	m := obj.Master
	if m == nil && len(obj.Overrides) > 0 {
		m = obj.Overrides[0]
	}
	v := EventView{UID: obj.UID, Summary: m.Text("SUMMARY"), Location: m.Text("LOCATION"), Description: m.Text("DESCRIPTION"),
		Exdates: []string{}, Overrides: []OverrideView{}}
	start, _ := startOf(m, cal)
	end, _ := endOf(m, start, cal)
	v.AllDay = start.allDay
	v.Start, v.End = LocalString(start.t, v.AllDay, family), LocalString(end.t, v.AllDay, family)
	if r := m.Get("RRULE"); r != nil && obj.Master != nil {
		v.RRule = r.Value
		rule, err := ParseRule(r.Value, cal)
		v.EditableRRule = err == nil && editable(rule) && m.Get("RDATE") == nil
		for _, p := range m.All("EXDATE") {
			for _, t := range listTimes(p, cal) {
				v.Exdates = append(v.Exdates, LocalString(t, v.AllDay, family))
			}
		}
	} else {
		v.EditableRRule = m.Get("RDATE") == nil
	}
	for _, c := range obj.Overrides {
		rid, _ := propTime(c.Get("RECURRENCE-ID"), cal)
		s, _ := startOf(c, cal)
		e, _ := endOf(c, s, cal)
		v.Overrides = append(v.Overrides, OverrideView{RecurrenceID: LocalString(rid.t, rid.allDay, family),
			Start: LocalString(s.t, s.allDay, family), End: LocalString(e.t, e.allDay, family), Summary: c.Text("SUMMARY")})
	}
	for _, c := range append([]*Component{obj.Master}, obj.Overrides...) {
		if c == nil {
			continue
		}
		v.HasAttendees = v.HasAttendees || c.Get("ATTENDEE") != nil
		for _, ch := range c.Children {
			v.HasAlarms = v.HasAlarms || ch.Name == "VALARM"
		}
	}
	return v, nil
}

// editable reports whether the editor can show a rule: daily, weekly on
// days, monthly by date, yearly, with an interval and an end.
func editable(r Rule) bool {
	if len(r.BySecond)+len(r.ByMinute)+len(r.ByHour)+len(r.ByYearDay)+len(r.ByWeekNo)+len(r.BySetPos) > 0 || len(r.ByMonth) > 1 {
		return false
	}
	switch r.Freq {
	case Daily:
		return len(r.ByDay)+len(r.ByMonthDay) == 0
	case Weekly:
		for _, d := range r.ByDay {
			if d.N != 0 {
				return false
			}
		}
		return len(r.ByMonthDay) == 0
	case Monthly:
		return len(r.ByDay) == 0 && len(r.ByMonthDay) <= 1
	case Yearly:
		return len(r.ByDay) == 0 && len(r.ByMonthDay) <= 1
	}
	return false
}

// Result is what an edit asks the calendar to store.
type Result struct {
	Data    []byte // the object, changed; nil when it is to be deleted
	Delete  bool
	NewData []byte // a second object (a split, scope "following"), or nil
}

// occurrenceAt finds the instance whose original start is occ (a local
// string in the family zone), as an instant.
func occurrenceAt(occ string, allDay bool, family *time.Location) (time.Time, error) {
	t, err := ParseLocal(occ, allDay, family)
	if err != nil {
		return time.Time{}, invalid("occurrence %q: %v", occ, err)
	}
	return t, nil
}

func parsed(data []byte, cal *time.Location) (*Object, error) {
	obj, _, err := Read(data, 0, cal)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// sameInstant compares a RECURRENCE-ID with an instant; for dates, the day.
func sameInstant(p *Prop, t time.Time, cal *time.Location) bool {
	w, err := propTime(p, cal)
	if err != nil {
		return false
	}
	if w.allDay {
		return w.t.Format(dateLayout) == t.Format(dateLayout)
	}
	return w.t.Equal(t)
}

func (o *Object) override(t time.Time, cal *time.Location) (int, *Component) {
	for i, c := range o.Overrides {
		if sameInstant(c.Get("RECURRENCE-ID"), t, cal) {
			return i, c
		}
	}
	return -1, nil
}

// Edit changes an object from the form (PLAN.md 4.6). occ is the
// occurrence edited, for a recurring event and a scope other than all.
func Edit(data []byte, in EventInput, scope Scope, occ string, cal, family *time.Location, now time.Time) (Result, error) {
	x, err := checkInput(in, family)
	if err != nil {
		return Result{}, err
	}
	obj, err := parsed(data, cal)
	if err != nil {
		return Result{}, err
	}
	recurring := obj.Master != nil && (obj.Master.Get("RRULE") != nil || obj.Master.Get("RDATE") != nil)
	if !recurring || scope == ScopeAll || scope == "" {
		return editAll(obj, x, cal, now)
	}
	masterStart, _ := startOf(obj.Master, cal)
	at, err := occurrenceAt(occ, masterStart.allDay, family)
	if err != nil {
		return Result{}, err
	}
	switch scope {
	case ScopeThis:
		_, ov := obj.override(at, cal)
		if ov == nil {
			ov = &Component{Name: "VEVENT"}
			for _, p := range obj.Master.Props {
				switch p.Name {
				case "RRULE", "RDATE", "EXDATE":
				default:
					ov.Props = append(ov.Props, p)
				}
			}
			ov.Children = append(ov.Children, obj.Master.Children...) // alarms follow
			ov.set(like(obj.Master.Get("DTSTART"), "RECURRENCE-ID", at, cal))
			obj.Cal.Children = append(obj.Cal.Children, ov)
		}
		x.apply(ov, false)
		ov.del("RRULE", 0)
		touch(ov, now)
		return Result{Data: Encode(obj.Cal)}, nil
	case ScopeFollowing:
		if !at.After(masterStart.t) {
			return editAll(obj, x, cal, now) // from the first one: the whole series
		}
		newObj, err := split(obj, at, cal, now)
		if err != nil {
			return Result{}, err
		}
		nm := masterOf(newObj)
		x.apply(nm, true) // a nil RRULE keeps the series' rule, from the new start
		touch(nm, now)
		return Result{Data: Encode(obj.Cal), NewData: Encode(newObj)}, nil
	}
	return Result{}, invalid("scope %q is not this, following or all", scope)
}

func masterOf(cal *Component) *Component {
	for _, c := range cal.Children {
		if c.Name == "VEVENT" && c.Get("RECURRENCE-ID") == nil {
			return c
		}
	}
	return nil
}

// editAll changes the master. When its start moves, the exceptions move
// with it, so a cancelled Wednesday stays cancelled.
func editAll(obj *Object, x input, cal *time.Location, now time.Time) (Result, error) {
	m := obj.Master
	if m == nil {
		m = obj.Overrides[0]
	}
	old, _ := startOf(m, cal)
	x.apply(m, obj.Master != nil)
	touch(m, now)
	if obj.Master != nil && !old.t.Equal(x.start) && old.allDay == x.in.AllDay {
		shift := func(t time.Time) time.Time {
			if x.in.AllDay {
				days := int(x.start.Sub(old.t).Round(24*time.Hour) / (24 * time.Hour))
				return t.AddDate(0, 0, days)
			}
			return t.Add(x.start.Sub(old.t))
		}
		var exs []time.Time
		for _, p := range m.All("EXDATE") {
			exs = append(exs, listTimes(p, cal)...)
		}
		if len(exs) > 0 {
			m.del("EXDATE", 0)
			for _, t := range exs {
				m.Props = append(m.Props, like(m.Get("DTSTART"), "EXDATE", shift(t), cal))
			}
		}
		for _, ov := range obj.Overrides {
			if rid, err := propTime(ov.Get("RECURRENCE-ID"), cal); err == nil {
				ov.set(like(m.Get("DTSTART"), "RECURRENCE-ID", shift(rid.t), cal))
			}
		}
	}
	return Result{Data: Encode(obj.Cal)}, nil
}

// split ends obj's series just before at and returns a new object (new
// UID) from at on, with the overrides and exceptions after at.
func split(obj *Object, at time.Time, cal *time.Location, now time.Time) (*Component, error) {
	m := obj.Master
	start, _ := startOf(m, cal)
	rr := m.Get("RRULE")
	newM := &Component{Name: "VEVENT", Children: append([]*Component(nil), m.Children...)}
	uid := NewUID()
	for _, p := range m.Props {
		switch p.Name {
		case "UID":
			newM.Props = append(newM.Props, Prop{Name: "UID", Value: uid})
		case "EXDATE", "RDATE":
		default:
			newM.Props = append(newM.Props, p)
		}
	}
	newM.set(like(m.Get("DTSTART"), "DTSTART", at, cal))
	if end, err := endOf(m, start, cal); err == nil && m.Get("DTEND") != nil {
		newM.set(like(m.Get("DTEND"), "DTEND", at.Add(end.t.Sub(start.t)), cal))
	}
	newM.set(Prop{Name: "CREATED", Value: stamp(now)})
	newM.set(Prop{Name: "SEQUENCE", Value: "0"})
	if rr != nil {
		r, err := ParseRule(rr.Value, start.t.Location())
		if err != nil {
			return nil, invalid("RRULE: %v", err)
		}
		value := rr.Value
		if r.Count > 0 { // the new series has what the old one had not used yet
			used := 0
			next := r.Iterator(start.t)
			for {
				t, ok := next()
				if !ok || !t.Before(at) {
					break
				}
				used++
			}
			value = replacePart(value, "COUNT", fmt.Sprint(max(1, r.Count-used)))
		}
		newM.set(Prop{Name: "RRULE", Value: value})
		// The old series ends the instant before this occurrence.
		until := at.Add(-time.Second).UTC().Format("20060102T150405Z")
		if start.allDay {
			until = at.AddDate(0, 0, -1).Format("20060102")
		}
		old := replacePart(replacePart(rr.Value, "COUNT", ""), "UNTIL", until)
		m.set(Prop{Name: "RRULE", Value: old})
	}
	// Exceptions and overrides after the split go with the new series.
	var keepEx, moveEx []time.Time
	for _, p := range m.All("EXDATE") {
		for _, t := range listTimes(p, cal) {
			if t.Before(at) {
				keepEx = append(keepEx, t)
			} else {
				moveEx = append(moveEx, t)
			}
		}
	}
	m.del("EXDATE", 0)
	for _, t := range keepEx {
		m.Props = append(m.Props, like(m.Get("DTSTART"), "EXDATE", t, cal))
	}
	for _, t := range moveEx {
		newM.Props = append(newM.Props, like(newM.Get("DTSTART"), "EXDATE", t, cal))
	}
	touch(m, now)
	newCal := &Component{Name: "VCALENDAR", Props: []Prop{{Name: "VERSION", Value: "2.0"}, {Name: "PRODID", Value: prodID}}}
	newCal.Children = append(newCal.Children, obj.Timezones...)
	newCal.Children = append(newCal.Children, newM)
	var kept []*Component
	for _, c := range obj.Cal.Children {
		if c.Name == "VEVENT" && c.Get("RECURRENCE-ID") != nil {
			if rid, err := propTime(c.Get("RECURRENCE-ID"), cal); err == nil && !rid.t.Before(at) {
				c.set(Prop{Name: "UID", Value: uid})
				newCal.Children = append(newCal.Children, c)
				continue
			}
		}
		kept = append(kept, c)
	}
	obj.Cal.Children = kept
	return newCal, nil
}

// replacePart sets (or with "" removes) NAME=value in an RRULE value.
func replacePart(rule, name, value string) string {
	var parts []string
	found := false
	for _, p := range strings.Split(rule, ";") {
		k, _, _ := strings.Cut(p, "=")
		if strings.EqualFold(k, name) {
			found = true
			if value != "" {
				parts = append(parts, name+"="+value)
			}
			continue
		}
		if p != "" {
			parts = append(parts, p)
		}
	}
	if !found && value != "" {
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, ";")
}

// Remove deletes from an object: the whole of it (scope all, or an event
// that does not recur), one occurrence (an EXDATE), or the occurrences
// from occ on (the series ends before it).
func Remove(data []byte, scope Scope, occ string, cal, family *time.Location, now time.Time) (Result, error) {
	obj, err := parsed(data, cal)
	if err != nil {
		return Result{}, err
	}
	recurring := obj.Master != nil && (obj.Master.Get("RRULE") != nil || obj.Master.Get("RDATE") != nil)
	if !recurring || scope == ScopeAll || scope == "" {
		return Result{Delete: true}, nil
	}
	masterStart, _ := startOf(obj.Master, cal)
	at, err := occurrenceAt(occ, masterStart.allDay, family)
	if err != nil {
		return Result{}, err
	}
	switch scope {
	case ScopeThis:
		if i, _ := obj.override(at, cal); i >= 0 {
			ov := obj.Overrides[i]
			var kept []*Component
			for _, c := range obj.Cal.Children {
				if c != ov {
					kept = append(kept, c)
				}
			}
			obj.Cal.Children = kept
		}
		obj.Master.Props = append(obj.Master.Props, like(obj.Master.Get("DTSTART"), "EXDATE", at, cal))
		touch(obj.Master, now)
		return Result{Data: Encode(obj.Cal)}, nil
	case ScopeFollowing:
		if !at.After(masterStart.t) {
			return Result{Delete: true}, nil
		}
		if _, err := split(obj, at, cal, now); err != nil {
			return Result{}, err
		}
		return Result{Data: Encode(obj.Cal)}, nil // the new series is dropped
	}
	return Result{}, invalid("scope %q is not this, following or all", scope)
}
