package ical

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

// judo, as a phone made it: an alarm and an Apple property the web app
// knows nothing about.
const judoFromPhone = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iOS 18//EN\r\nBEGIN:VEVENT\r\nUID:judo\r\n" +
	"SUMMARY:Judo\r\nDTSTART;TZID=Europe/Paris:20261104T140000\r\nDTEND;TZID=Europe/Paris:20261104T153000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z\r\nEXDATE;TZID=Europe/Paris:20261118T140000\r\n" +
	"X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC\r\nATTENDEE;CN=Maman:mailto:maman@example.org\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nDESCRIPTION:Judo\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func occurrences(t *testing.T, data []byte, from, to string) string {
	t.Helper()
	obj, _, err := Read(data, 0, paris)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	f, _ := time.ParseInLocation(dateLayout, from, paris)
	e, _ := time.ParseInLocation(dateLayout, to, paris)
	occ, _, _ := obj.Expand(paris, f, e, 0)
	var out []string
	for _, o := range occ {
		s := o.Start.In(paris).Format("01-02T15:04")
		if o.Overridden {
			s += "*" + o.Event.Text("SUMMARY")
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func kept(t *testing.T, data []byte) {
	t.Helper()
	s := string(data)
	for _, want := range []string{"BEGIN:VALARM", "TRIGGER:-PT15M", "X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC", "ATTENDEE;CN=Maman"} {
		if !strings.Contains(s, want) {
			t.Errorf("an edit from the web lost %s:\n%s", want, s)
		}
	}
}

func str(s string) *string { return &s }

func TestNewAndView(t *testing.T) {
	data, uid, err := New(EventInput{Summary: "Dentiste, salle 2", Start: "2026-11-26T17:30", End: "2026-11-26T18:00", Location: "Rue; 12"}, paris, now)
	if err != nil {
		t.Fatal(err)
	}
	v, err := View(data, paris, paris)
	if err != nil {
		t.Fatal(err)
	}
	if v.UID != uid || v.Summary != "Dentiste, salle 2" || v.Location != "Rue; 12" || v.Start != "2026-11-26T17:30:00" || v.End != "2026-11-26T18:00:00" {
		t.Errorf("view %+v", v)
	}
	allDay, _, _ := New(EventInput{Summary: "Vacances", AllDay: true, Start: "2026-12-19", End: "2027-01-04", RRule: str("FREQ=YEARLY")}, paris, now)
	v, _ = View(allDay, paris, paris)
	if !v.AllDay || v.Start != "2026-12-19" || v.End != "2027-01-04" || v.RRule != "FREQ=YEARLY" || !v.EditableRRule {
		t.Errorf("an all-day yearly event: %+v", v)
	}
	if _, _, err := New(EventInput{Summary: "x", Start: "2026-11-26T18:00", End: "2026-11-26T17:00"}, paris, now); err == nil {
		t.Error("an end before the start was accepted")
	}
	if _, _, err := New(EventInput{Summary: "x", Start: "2026-11-26T18:00", RRule: str("FREQ=SOMETIMES")}, paris, now); err == nil {
		t.Error("a bad rule was accepted")
	}
	v, _ = View([]byte(judoFromPhone), paris, paris)
	if !v.HasAlarms || !v.HasAttendees || len(v.Exdates) != 1 || v.Exdates[0] != "2026-11-18T14:00:00" || !v.EditableRRule {
		t.Errorf("judo: %+v", v)
	}
}

func TestEditEveryScope(t *testing.T) {
	in := EventInput{Summary: "Judo (gymnase B)", Start: "2026-11-25T15:00:00", End: "2026-11-25T16:30:00"}

	// this: one Wednesday moves; the series is unchanged.
	r, err := Edit([]byte(judoFromPhone), in, ScopeThis, "2026-11-25T14:00:00", paris, paris, now)
	if err != nil {
		t.Fatal(err)
	}
	kept(t, r.Data)
	if got := occurrences(t, r.Data, "2026-11-01", "2026-12-03"); got != "11-04T14:00 11-11T14:00 11-25T15:00*Judo (gymnase B) 12-02T14:00" {
		t.Errorf("this: %s", got)
	}
	// Editing the same occurrence again changes its override, not a new one.
	again, _ := Edit(r.Data, EventInput{Summary: "Judo annulé ?", Start: "2026-11-25T15:00:00", End: "2026-11-25T16:00:00"}, ScopeThis, "2026-11-25T14:00:00", paris, paris, now)
	if n := strings.Count(string(again.Data), "RECURRENCE-ID"); n != 1 {
		t.Errorf("two overrides of one occurrence: %d", n)
	}

	// following: the series splits; two objects, the second a new UID.
	r, err = Edit([]byte(judoFromPhone), EventInput{Summary: "Judo, nouvel horaire", Start: "2026-12-02T17:00:00", End: "2026-12-02T18:00:00"},
		ScopeFollowing, "2026-12-02T14:00:00", paris, paris, now)
	if err != nil {
		t.Fatal(err)
	}
	kept(t, r.Data)
	kept(t, r.NewData)
	if got := occurrences(t, r.Data, "2026-11-25", "2026-12-20"); got != "11-25T14:00" {
		t.Errorf("following, the old series: %s", got)
	}
	if got := occurrences(t, r.NewData, "2026-11-25", "2026-12-20"); got != "12-02T17:00 12-09T17:00 12-16T17:00" {
		t.Errorf("following, the new series: %s", got)
	}
	if v, _ := View(r.NewData, paris, paris); v.UID == "judo" || v.Summary != "Judo, nouvel horaire" {
		t.Errorf("the new series: %+v", v)
	}

	// all: the whole series moves an hour; the cancelled Wednesday stays cancelled.
	r, err = Edit([]byte(judoFromPhone), EventInput{Summary: "Judo", Start: "2026-11-04T15:00:00", End: "2026-11-04T16:30:00"},
		ScopeAll, "", paris, paris, now)
	if err != nil {
		t.Fatal(err)
	}
	kept(t, r.Data)
	if got := occurrences(t, r.Data, "2026-11-01", "2026-11-26"); got != "11-04T15:00 11-11T15:00 11-25T15:00" {
		t.Errorf("all: %s", got)
	}
	if !strings.Contains(string(r.Data), "SEQUENCE:1") {
		t.Error("SEQUENCE was not bumped")
	}
}

func TestRemoveEveryScope(t *testing.T) {
	r, err := Remove([]byte(judoFromPhone), ScopeThis, "2026-11-25T14:00:00", paris, paris, now)
	if err != nil {
		t.Fatal(err)
	}
	kept(t, r.Data)
	if got := occurrences(t, r.Data, "2026-11-01", "2026-12-03"); got != "11-04T14:00 11-11T14:00 12-02T14:00" {
		t.Errorf("this: %s", got)
	}
	r, _ = Remove([]byte(judoFromPhone), ScopeFollowing, "2026-11-25T14:00:00", paris, paris, now)
	if got := occurrences(t, r.Data, "2026-11-01", "2027-07-01"); got != "11-04T14:00 11-11T14:00" {
		t.Errorf("following: %s", got)
	}
	r, _ = Remove([]byte(judoFromPhone), ScopeFollowing, "2026-11-04T14:00:00", paris, paris, now)
	if !r.Delete {
		t.Error("following from the first occurrence is the whole series")
	}
	r, _ = Remove([]byte(judoFromPhone), ScopeAll, "", paris, paris, now)
	if !r.Delete {
		t.Error("all")
	}
	once, _, _ := New(EventInput{Summary: "x", Start: "2026-11-26T17:30"}, paris, now)
	if r, _ := Remove(once, ScopeThis, "2026-11-26T17:30:00", paris, paris, now); !r.Delete {
		t.Error("an event that does not recur is deleted whole")
	}
}

func TestSplitKeepsTheCount(t *testing.T) {
	data, _, _ := New(EventInput{Summary: "Séances", Start: "2026-11-02T18:00", End: "2026-11-02T19:00", RRule: str("FREQ=WEEKLY;COUNT=6")}, paris, now)
	r, err := Edit(data, EventInput{Summary: "Séances (salle 2)", Start: "2026-11-16T18:00", End: "2026-11-16T19:00"}, ScopeFollowing, "2026-11-16T18:00:00", paris, paris, now)
	if err != nil {
		t.Fatal(err)
	}
	a := occurrences(t, r.Data, "2026-10-01", "2027-03-01")
	b := occurrences(t, r.NewData, "2026-10-01", "2027-03-01")
	if strings.Count(a, " ")+1 != 2 || strings.Count(b, " ")+1 != 4 {
		t.Errorf("6 sessions split into 2 and 4: %q / %q", a, b)
	}
}
