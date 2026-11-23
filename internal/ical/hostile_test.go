package ical

import (
	"strings"
	"testing"
	"time"
)

// Inputs a hostile client could PUT (the review of 2026-11-23): each must
// be refused or answered quickly, never hang or fill memory.

func within(t *testing.T, limit time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	start := time.Now()
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("still running after %v", limit)
	}
	if d := time.Since(start); d > limit {
		t.Fatalf("took %v, more than %v", d, limit)
	}
}

func TestHostileRules(t *testing.T) {
	bad := []string{
		"FREQ=SECONDLY;INTERVAL=36028797018963968;COUNT=2", // a step of zero once multiplied
		"FREQ=HOURLY;INTERVAL=2562048",                     // a negative step
		"FREQ=DAILY;COUNT=999999999",
		"FREQ=DAILY;BYHOUR=" + list(24) + ";BYMINUTE=" + list(60) + ";BYSECOND=" + list(60), // 86 400 a day
		"FREQ=YEARLY;BYHOUR=" + list(24) + ";BYMINUTE=" + list(60),                          // 366 x 1440
	}
	for _, rule := range bad {
		if _, err := ParseRule(rule, time.UTC); err == nil {
			t.Errorf("%.60s... was accepted", rule)
		}
	}
	// Duplicates add nothing: a list of a hundred thousand zeros is one zero.
	r, err := ParseRule("FREQ=DAILY;COUNT=2;BYHOUR="+strings.Repeat("0,", 100000)+"0", time.UTC)
	if err != nil || len(r.ByHour) != 1 {
		t.Fatalf("a repeated BYHOUR: %v %v", err, len(r.ByHour))
	}
	r, _ = ParseRule("FREQ=YEARLY;BYDAY="+strings.Repeat("1MO,", 5000)+"1MO", time.UTC)
	if len(r.ByDay) != 1 {
		t.Errorf("a repeated BYDAY kept %d entries", len(r.ByDay))
	}
}

func list(n int) string {
	var s []string
	for i := range n {
		s = append(s, itoa(i))
	}
	return strings.Join(s, ",")
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// A sparse sub-daily rule: periods that cannot match are skipped whole, so
// it neither runs for minutes nor stops after its first day.
func TestSparseSubDailyRules(t *testing.T) {
	r, err := ParseRule("FREQ=SECONDLY;BYHOUR=9;BYMINUTE=0;BYSECOND=0", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	within(t, 2*time.Second, func() {
		next := r.Iterator(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
		for range 4 {
			ti, ok := next()
			if !ok {
				break
			}
			got = append(got, ti.Format("01-02T15:04:05"))
		}
	})
	if strings.Join(got, " ") != "01-01T09:00:00 01-02T09:00:00 01-03T09:00:00 01-04T09:00:00" {
		t.Errorf("got %v", got)
	}
	// Once a minute but only on Mondays in March.
	r, _ = ParseRule("FREQ=MINUTELY;BYMONTH=3;BYDAY=MO;BYHOUR=8;BYMINUTE=30", time.UTC)
	within(t, 2*time.Second, func() {
		next := r.Iterator(time.Date(2026, 1, 1, 8, 30, 0, 0, time.UTC))
		next()
		ti, ok := next()
		if !ok || ti.Format("2006-01-02T15:04") != "2026-03-02T08:30" {
			t.Errorf("the first Monday of March: %v %v", ti, ok)
		}
	})
}

// An open-ended hourly event made twelve years ago still has instances
// today: expansion starts near the range, not at DTSTART.
func TestExpandStartsNearTheRange(t *testing.T) {
	data := cal(event("UID:h", "DTSTART:20140101T000000Z", "DTEND:20140101T001500Z", "RRULE:FREQ=HOURLY"))
	obj, idx, err := Read([]byte(data), 0, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if idx.End != nil {
		t.Fatal("an open-ended rule has an end")
	}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	var occ []Occurrence
	within(t, time.Second, func() {
		occ, _, err = obj.Expand(time.UTC, from, from.Add(24*time.Hour), 0)
	})
	if err != nil || len(occ) != 24 || !occ[0].Start.Equal(from) {
		t.Fatalf("got %d occurrences, first %v (%v)", len(occ), first(occ), err)
	}
	// Weekly from 1990, every other week: still the right phase today.
	data = cal(event("UID:w", "DTSTART:19900101T100000Z", "DTEND:19900101T110000Z", "RRULE:FREQ=WEEKLY;INTERVAL=2"))
	obj, _, _ = Read([]byte(data), 0, time.UTC)
	occ, _, _ = obj.Expand(time.UTC, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 0)
	for _, o := range occ {
		weeks := int(o.Start.Sub(time.Date(1990, 1, 1, 10, 0, 0, 0, time.UTC)).Hours() / 24 / 7)
		if weeks%2 != 0 || o.Start.Weekday() != time.Monday {
			t.Errorf("%v is off the every-other-Monday grid", o.Start)
		}
	}
	if len(occ) < 2 {
		t.Errorf("only %d occurrences in November", len(occ))
	}
}

func first(occ []Occurrence) any {
	if len(occ) == 0 {
		return nil
	}
	return occ[0].Start
}

func TestHostileText(t *testing.T) {
	// 256 KiB of continuation lines used to cost gigabytes of copying.
	long := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTART:20261123T090000Z\r\nDESCRIPTION:x" +
		strings.Repeat("\r\n y", 85000) + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	within(t, time.Second, func() {
		if _, _, err := Read([]byte(long), 0, time.UTC); err != nil {
			t.Errorf("a long folded line: %v", err)
		}
	})
	// A TZID of a hundred thousand segments used to be a hundred thousand lookups.
	tz := strings.Repeat("/a", 100000)
	within(t, time.Second, func() {
		if Zone(tz, time.UTC) != time.UTC {
			t.Error("a hostile TZID did not fall back")
		}
	})
	if Zone("/mozilla.org/20050126_1/America/Argentina/Buenos_Aires", time.UTC).String() != "America/Argentina/Buenos_Aires" {
		t.Error("a three-segment IANA name behind a prefix")
	}
	// Durations and dates past the year 9999 are refused or clamped.
	if _, err := ParseDuration("P3000000D"); err == nil {
		t.Error("a duration of 8000 years was accepted")
	}
	data := cal(event("UID:far", "DTSTART:99990101T000000Z", "RRULE:FREQ=YEARLY;INTERVAL=1000;COUNT=10"))
	_, idx, err := Read([]byte(data), 0, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if idx.End == nil || idx.End.Year() > 9999 {
		t.Errorf("the index end %v is not clamped", idx.End)
	}
}
