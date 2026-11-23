package calendar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"

	"github.com/nebuloss/agendouille/internal/ical"
)

// ImportResult counts what an import did.
type ImportResult struct {
	Added    int      `json:"added"`
	Replaced int      `json:"replaced"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

// prodID is what agendouille writes in the calendars it assembles.
const prodID = "-//agendouille//agendouille//FR"

var safeUID = regexp.MustCompile(`^[A-Za-z0-9._@+-]{1,195}$`)

// objectName is the name an imported UID gets: the UID itself when it is a
// valid name, else a hash of it.
func objectName(uid string) string {
	if safeUID.MatchString(uid) {
		return uid + ".ics"
	}
	sum := sha256.Sum256([]byte(uid))
	return hex.EncodeToString(sum[:16]) + ".ics"
}

// Import splits an .ics file (a Google Calendar export, one or more
// VCALENDARs) into one object per UID, each with the VTIMEZONEs it names,
// and puts them through the one write path. A UID already in the calendar
// is skipped, or replaced when replace is set.
func (s *Service) Import(ctx context.Context, a Actor, r Ref, data []byte, replace bool) (ImportResult, error) {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return ImportResult{}, err
	}
	if !i.Access.CanWrite() {
		return ImportResult{}, ErrForbidden
	}
	roots, err := ical.Decode(data)
	if err != nil {
		return ImportResult{}, err
	}
	zones := map[string]*ical.Component{}
	type group struct{ events []*ical.Component }
	byUID := map[string]*group{}
	var order []string
	res := ImportResult{Errors: []string{}}
	for _, root := range roots {
		for _, c := range root.Children {
			switch c.Name {
			case "VTIMEZONE":
				zones[c.Text("TZID")] = c
			case "VEVENT":
				uid := c.Text("UID")
				if uid == "" {
					res.Errors = append(res.Errors, "an event without a UID was left out")
					continue
				}
				if byUID[uid] == nil {
					byUID[uid] = &group{}
					order = append(order, uid)
				}
				byUID[uid].events = append(byUID[uid].events, c)
			case "VTODO", "VJOURNAL":
				res.Skipped++
			}
		}
	}
	existing, err := s.List(ctx, i)
	if err != nil {
		return ImportResult{}, err
	}
	nameOfUID := map[string]string{}
	for _, o := range existing {
		nameOfUID[o.UID] = o.Name
	}
	for _, uid := range order {
		cal := &ical.Component{Name: "VCALENDAR", Props: []ical.Prop{{Name: "VERSION", Value: "2.0"}, {Name: "PRODID", Value: prodID}}}
		used := map[string]bool{}
		for _, ev := range byUID[uid].events {
			for _, p := range ev.Props {
				if tz := p.Param("TZID"); tz != "" && zones[tz] != nil && !used[tz] {
					used[tz] = true
					cal.Children = append(cal.Children, zones[tz])
				}
			}
		}
		cal.Children = append(cal.Children, byUID[uid].events...)
		body := ical.Encode(cal)
		name, pre := objectName(uid), Precondition{IfNoneMatch: true}
		if have, ok := nameOfUID[uid]; ok {
			if !replace {
				res.Skipped++
				continue
			}
			name, pre = have, Precondition{}
		}
		_, created, err := s.Put(ctx, a, r, name, body, pre)
		switch {
		case err == nil && created:
			res.Added++
		case err == nil:
			res.Replaced++
		case errors.Is(err, ErrQuota):
			res.Errors = append(res.Errors, err.Error())
			return res, nil
		default:
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", uid, err))
		}
	}
	return res, nil
}

// Export writes a calendar as one .ics: its VTIMEZONEs once each, then
// every event, under the calendar's name.
func (s *Service) Export(ctx context.Context, a Actor, r Ref) ([]byte, error) {
	i, err := s.lookup(ctx, a, r)
	if err != nil {
		return nil, err
	}
	return s.ExportInfo(ctx, i)
}

// ExportInfo is Export for a calendar already looked up.
func (s *Service) ExportInfo(ctx context.Context, i Info) ([]byte, error) {
	objs, err := s.List(ctx, i)
	if err != nil {
		return nil, err
	}
	out := &ical.Component{Name: "VCALENDAR", Props: []ical.Prop{
		{Name: "VERSION", Value: "2.0"}, {Name: "PRODID", Value: prodID},
		{Name: "X-WR-CALNAME", Value: ical.EscapeText(i.DisplayName())},
		{Name: "X-WR-TIMEZONE", Value: i.Timezone},
	}}
	zones := map[string]bool{}
	var events []*ical.Component
	for _, o := range objs {
		roots, err := ical.Decode(o.Data)
		if err != nil || len(roots) == 0 {
			continue
		}
		for _, c := range roots[0].Children {
			if c.Name == "VTIMEZONE" {
				if tz := c.Text("TZID"); !zones[tz] {
					zones[tz] = true
					out.Children = append(out.Children, c)
				}
				continue
			}
			events = append(events, c)
		}
	}
	out.Children = append(out.Children, events...)
	return ical.Encode(out), nil
}
