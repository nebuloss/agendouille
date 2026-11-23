package ical

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var paris, _ = time.LoadLocation("Europe/Paris")

func cal(events ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" + strings.Join(events, "") + "END:VCALENDAR\r\n"
}

func event(lines ...string) string {
	return "BEGIN:VEVENT\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\n"
}

func TestDecodeUnfoldsAndKeepsParams(t *testing.T) {
	data := "BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nUID:a\nDTSTART:20261123T090000Z\n" +
		"SUMMARY:Dentiste\\, avec L\xc3\xa9a\\nsalle 2\nDESCRIPTION:une ligne tr\n \xc3\xa8s longue\n" +
		"ATTENDEE;CN=\"Doe; John\";ROLE=REQ-PARTICIPANT:mailto:j@example.org\n" +
		"X-APPLE-STRUCTURED-LOCATION;VALUE=URI;X-TITLE=\"Gymnase, salle 2\":geo:48.8,2.3\nEND:VEVENT\nEND:VCALENDAR\n"
	roots, err := Decode([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	ev := roots[0].Children[0]
	if got := ev.Text("SUMMARY"); got != "Dentiste, avec Léa\nsalle 2" {
		t.Errorf("SUMMARY %q", got)
	}
	if got := ev.Text("DESCRIPTION"); got != "une ligne très longue" {
		t.Errorf("unfolded DESCRIPTION %q", got)
	}
	att := ev.Get("ATTENDEE")
	if att.Param("cn") != "Doe; John" || att.Value != "mailto:j@example.org" {
		t.Errorf("ATTENDEE %+v", att)
	}
	if loc := ev.Get("X-APPLE-STRUCTURED-LOCATION"); loc.Param("X-TITLE") != "Gymnase, salle 2" || loc.Value != "geo:48.8,2.3" {
		t.Errorf("an X- property %+v", loc)
	}
}

func TestDecodeRefusesBrokenStructure(t *testing.T) {
	for name, data := range map[string]string{
		"no calendar":       "BEGIN:VEVENT\r\nEND:VEVENT\r\n",
		"unclosed":          "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n",
		"mismatched END":    "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nEND:VCALENDAR\r\n",
		"a line without :":  "BEGIN:VCALENDAR\r\nVERSION\r\nEND:VCALENDAR\r\n",
		"an unclosed quote": "BEGIN:VCALENDAR\r\nX-A;P=\"x:y\r\nEND:VCALENDAR\r\n",
		"not UTF-8":         "BEGIN:VCALENDAR\r\nSUMMARY:\xff\r\nEND:VCALENDAR\r\n",
		"empty":             "",
	} {
		if _, err := Decode([]byte(data)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestEncodeFoldsAndRoundTrips(t *testing.T) {
	long := strings.Repeat("é", 60) // 120 octets
	c := &Component{Name: "VCALENDAR", Props: []Prop{{Name: "VERSION", Value: "2.0"}},
		Children: []*Component{{Name: "VEVENT", Props: []Prop{
			{Name: "UID", Value: "x"},
			{Name: "SUMMARY", Value: EscapeText(long + ", fin; et\nsuite")},
			{Name: "ATTENDEE", Params: map[string][]string{"CN": {"Doe; John"}}, Value: "mailto:j@example.org"},
		}}}}
	out := Encode(c)
	for _, line := range strings.Split(string(out), "\r\n") {
		if len(line) > 75 {
			t.Errorf("a line of %d octets", len(line))
		}
	}
	back, err := Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	ev := back[0].Children[0]
	if ev.Text("SUMMARY") != long+", fin; et\nsuite" || ev.Get("ATTENDEE").Param("CN") != "Doe; John" {
		t.Errorf("round trip: %q %+v", ev.Text("SUMMARY"), ev.Get("ATTENDEE"))
	}
}

func TestReadValidates(t *testing.T) {
	ok := event("UID:a", "DTSTART;TZID=Europe/Paris:20261123T090000", "DTEND;TZID=Europe/Paris:20261123T100000", "SUMMARY:Ok")
	cases := []struct {
		name, data string
		want       error
	}{
		{"a task", cal("BEGIN:VTODO\r\nUID:t\r\nEND:VTODO\r\n"), ErrUnsupported},
		{"no UID", cal(event("DTSTART:20261123T090000Z")), ErrInvalid},
		{"no DTSTART", cal(event("UID:a")), ErrInvalid},
		{"two UIDs", cal(ok, event("UID:b", "DTSTART:20261123T090000Z")), ErrInvalid},
		{"ends before it starts", cal(event("UID:a", "DTSTART:20261123T090000Z", "DTEND:20261123T080000Z")), ErrInvalid},
		{"two masters", cal(ok, ok), ErrInvalid},
		{"a bad RRULE", cal(event("UID:a", "DTSTART:20261123T090000Z", "RRULE:FREQ=SOMETIMES")), ErrInvalid},
		{"no event", cal(), ErrInvalid},
		{"two calendars", cal(ok) + cal(ok), ErrInvalid},
	}
	for _, c := range cases {
		if _, _, err := Read([]byte(c.data), 0, paris); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if _, _, err := Read([]byte(cal(ok)), 10, paris); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	if _, idx, err := Read([]byte(cal(ok)), 0, paris); err != nil || idx.Summary != "Ok" || idx.UID != "a" {
		t.Errorf("a good event: %+v %v", idx, err)
	}
}

func TestIndex(t *testing.T) {
	utc := func(s string) time.Time { tt, _ := time.Parse(time.RFC3339, s); return tt }
	cases := []struct {
		name      string
		data      string
		start     string
		end       string // "" for open-ended
		recurring bool
		allDay    bool
	}{
		{"a timed event", cal(event("UID:a", "DTSTART;TZID=Europe/Paris:20261123T090000", "DURATION:PT90M")),
			"2026-11-23T08:00:00Z", "2026-11-23T09:30:00Z", false, false},
		{"an all-day event with no end", cal(event("UID:a", "DTSTART;VALUE=DATE:20261123")),
			"2026-11-22T23:00:00Z", "2026-11-23T23:00:00Z", false, true},
		{"weekly, 3 times", cal(event("UID:a", "DTSTART;TZID=Europe/Paris:20261104T140000",
			"DTEND;TZID=Europe/Paris:20261104T153000", "RRULE:FREQ=WEEKLY;COUNT=3")),
			"2026-11-04T13:00:00Z", "2026-11-18T14:30:00Z", true, false},
		{"weekly forever", cal(event("UID:a", "DTSTART;TZID=Europe/Paris:20261104T140000", "RRULE:FREQ=WEEKLY")),
			"2026-11-04T13:00:00Z", "", true, false},
		{"an override moved later than the rule's end", cal(
			event("UID:a", "DTSTART:20261104T140000Z", "DTEND:20261104T150000Z", "RRULE:FREQ=WEEKLY;COUNT=2"),
			event("UID:a", "RECURRENCE-ID:20261111T140000Z", "DTSTART:20261201T140000Z", "DTEND:20261201T150000Z")),
			"2026-11-04T14:00:00Z", "2026-12-01T15:00:00Z", true, false},
	}
	for _, c := range cases {
		_, idx, err := Read([]byte(c.data), 0, paris)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !idx.Start.Equal(utc(c.start)) || idx.Recurring != c.recurring || idx.AllDay != c.allDay {
			t.Errorf("%s: %+v", c.name, idx)
		}
		switch {
		case c.end == "" && idx.End != nil:
			t.Errorf("%s: end %v, want open", c.name, idx.End)
		case c.end != "" && (idx.End == nil || !idx.End.Equal(utc(c.end))):
			t.Errorf("%s: end %v, want %s", c.name, idx.End, c.end)
		}
	}
}

func starts(occ []Occurrence, loc *time.Location) string {
	var out []string
	for _, o := range occ {
		s := o.Start.In(loc).Format("01-02T15:04")
		if o.Overridden {
			s += "*"
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

// Judo every Wednesday at 14:00 until the end of June, one Wednesday
// cancelled, one moved: the family case of PLAN.md 1.
func TestExpandWithExdateAndOverride(t *testing.T) {
	data := cal(
		event("UID:judo", "SUMMARY:Judo", "DTSTART;TZID=Europe/Paris:20261104T140000",
			"DTEND;TZID=Europe/Paris:20261104T153000", "RRULE:FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z",
			"EXDATE;TZID=Europe/Paris:20261118T140000"),
		event("UID:judo", "SUMMARY:Judo (gymnase B)", "RECURRENCE-ID;TZID=Europe/Paris:20261125T140000",
			"DTSTART;TZID=Europe/Paris:20261126T170000", "DTEND;TZID=Europe/Paris:20261126T183000"))
	obj, idx, err := Read([]byte(data), 0, paris)
	if err != nil {
		t.Fatal(err)
	}
	if idx.End == nil || idx.End.In(paris).Format("2006-01-02") != "2027-06-30" {
		t.Errorf("the last instance ends %v", idx.End)
	}
	occ, _, err := obj.Expand(paris, time.Date(2026, 11, 1, 0, 0, 0, 0, paris), time.Date(2026, 12, 10, 0, 0, 0, 0, paris), 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "11-04T14:00 11-11T14:00 11-26T17:00* 12-02T14:00 12-09T14:00" // 11-18 excluded, 11-25 moved
	if got := starts(occ, paris); got != want {
		t.Errorf("\n got  %s\n want %s", got, want)
	}
	// The moved one is found by a range around its new day only.
	if ok, _ := obj.Overlaps(paris, time.Date(2026, 11, 26, 0, 0, 0, 0, paris), time.Date(2026, 11, 27, 0, 0, 0, 0, paris)); !ok {
		t.Error("the override's day does not overlap")
	}
	if ok, _ := obj.Overlaps(paris, time.Date(2026, 11, 18, 0, 0, 0, 0, paris), time.Date(2026, 11, 19, 0, 0, 0, 0, paris)); ok {
		t.Error("the cancelled Wednesday overlaps")
	}
}

// The wall time stays 09:00 across the end of summer time (25 October 2026
// in Paris), and an all-day event keeps its length.
func TestExpandAcrossDST(t *testing.T) {
	data := cal(event("UID:a", "DTSTART;TZID=Europe/Paris:20261023T090000", "DTEND;TZID=Europe/Paris:20261023T100000",
		"RRULE:FREQ=DAILY;COUNT=4"))
	obj, _, _ := Read([]byte(data), 0, paris)
	occ, _, _ := obj.Expand(paris, time.Date(2026, 10, 1, 0, 0, 0, 0, paris), time.Date(2026, 11, 1, 0, 0, 0, 0, paris), 0)
	if got := starts(occ, paris); got != "10-23T09:00 10-24T09:00 10-25T09:00 10-26T09:00" {
		t.Errorf("got %s", got)
	}
	if d := occ[3].Start.Sub(occ[1].Start); d != 49*time.Hour {
		t.Errorf("two days across the change are %v, want 49h", d)
	}

	allDay := cal(event("UID:b", "DTSTART;VALUE=DATE:20261024", "DTEND;VALUE=DATE:20261026", "RRULE:FREQ=WEEKLY;COUNT=2"))
	obj, _, _ = Read([]byte(allDay), 0, paris)
	occ, _, _ = obj.Expand(paris, time.Date(2026, 10, 1, 0, 0, 0, 0, paris), time.Date(2026, 11, 30, 0, 0, 0, 0, paris), 0)
	if len(occ) != 2 || !occ[0].AllDay || occ[0].End.In(paris).Format("01-02T15:04") != "10-26T00:00" {
		t.Errorf("all-day across the change: %+v", occ)
	}
}

func TestFloatingAndForeignZones(t *testing.T) {
	floating := cal(event("UID:a", "DTSTART:20261123T090000"))
	_, idx, _ := Read([]byte(floating), 0, paris)
	if idx.Start.Format(time.RFC3339) != "2026-11-23T08:00:00Z" {
		t.Errorf("floating 09:00 in Paris: %v", idx.Start)
	}
	outlook := cal("BEGIN:VTIMEZONE\r\nTZID:Romance Standard Time\r\nEND:VTIMEZONE\r\n",
		event("UID:b", "DTSTART;TZID=Romance Standard Time:20260715T090000"))
	_, idx, err := Read([]byte(outlook), 0, time.UTC)
	if err != nil || idx.Start.Format(time.RFC3339) != "2026-07-15T07:00:00Z" {
		t.Errorf("Outlook's Paris in summer: %v %v", idx.Start, err)
	}
	mozilla := cal(event("UID:c", "DTSTART;TZID=/mozilla.org/20050126_1/Europe/Paris:20260715T090000"))
	if _, idx, _ := Read([]byte(mozilla), 0, time.UTC); idx.Start.Format(time.RFC3339) != "2026-07-15T07:00:00Z" {
		t.Errorf("a Mozilla TZID: %v", idx.Start)
	}
	unknown := cal(event("UID:d", "DTSTART;TZID=Somewhere/Else:20260715T090000"))
	if _, idx, _ := Read([]byte(unknown), 0, paris); idx.Start.Format(time.RFC3339) != "2026-07-15T07:00:00Z" {
		t.Errorf("an unknown TZID falls back to the calendar's zone: %v", idx.Start)
	}
}

func TestDurations(t *testing.T) {
	for in, want := range map[string]Duration{
		"PT90M": {Exact: 90 * time.Minute}, "P1D": {Days: 1}, "P2W": {Days: 14}, "-PT15M": {Exact: -15 * time.Minute},
		"P1DT2H3M4S": {Days: 1, Exact: 2*time.Hour + 3*time.Minute + 4*time.Second},
	} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("%s: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "P", "1D", "PT", "P1H", "PTD", "P1D2"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestRDateOnly(t *testing.T) {
	data := cal(event("UID:a", "DTSTART:20261123T090000Z", "RDATE:20261130T090000Z,20261207T090000Z"))
	obj, idx, _ := Read([]byte(data), 0, time.UTC)
	occ, _, _ := obj.Expand(time.UTC, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), 0)
	if got := starts(occ, time.UTC); got != "11-23T09:00 11-30T09:00 12-07T09:00" || !idx.Recurring {
		t.Errorf("got %s, %+v", got, idx)
	}
}
