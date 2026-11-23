package ical

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Freq is a rule's FREQ.
type Freq int

const (
	Secondly Freq = iota
	Minutely
	Hourly
	Daily
	Weekly
	Monthly
	Yearly
)

var freqNames = map[string]Freq{"SECONDLY": Secondly, "MINUTELY": Minutely, "HOURLY": Hourly, "DAILY": Daily,
	"WEEKLY": Weekly, "MONTHLY": Monthly, "YEARLY": Yearly}

var dayNames = map[string]time.Weekday{"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday}

// WeekdayNum is one BYDAY entry: a weekday, and for MONTHLY and YEARLY an
// optional ordinal (1MO, -1FR); N is 0 when there is none.
type WeekdayNum struct {
	Day time.Weekday
	N   int
}

// Rule is a parsed RRULE (RFC 5545 3.3.10).
type Rule struct {
	Freq       Freq
	Interval   int
	Count      int       // 0: no COUNT
	Until      time.Time // zero: no UNTIL
	UntilDate  bool      // UNTIL was a DATE: compare dates, not instants
	BySecond   []int
	ByMinute   []int
	ByHour     []int
	ByDay      []WeekdayNum
	ByMonthDay []int
	ByYearDay  []int
	ByWeekNo   []int
	ByMonth    []int
	BySetPos   []int
	WeekStart  time.Weekday
}

// ParseRule reads an RRULE value. UNTIL without Z or as a DATE is read in
// loc (the zone of DTSTART).
func ParseRule(s string, loc *time.Location) (Rule, error) {
	r := Rule{Interval: 1, WeekStart: time.Monday, Freq: -1}
	for _, part := range strings.Split(strings.TrimSpace(s), ";") {
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return Rule{}, fmt.Errorf("%q is not NAME=VALUE", part)
		}
		k = strings.ToUpper(k)
		var err error
		switch k {
		case "FREQ":
			f, ok := freqNames[strings.ToUpper(v)]
			if !ok {
				return Rule{}, fmt.Errorf("FREQ=%s", v)
			}
			r.Freq = f
		case "INTERVAL":
			r.Interval, err = strconv.Atoi(v)
			if err == nil && r.Interval < 1 {
				err = fmt.Errorf("must be at least 1")
			}
		case "COUNT":
			r.Count, err = strconv.Atoi(v)
			if err == nil && r.Count < 1 {
				err = fmt.Errorf("must be at least 1")
			}
		case "UNTIL":
			switch {
			case len(v) == 8:
				r.Until, err = time.ParseInLocation("20060102", v, loc)
				r.UntilDate = true
			case strings.HasSuffix(v, "Z"):
				r.Until, err = time.Parse("20060102T150405Z", v)
			default:
				r.Until, err = time.ParseInLocation("20060102T150405", v, loc)
			}
		case "BYSECOND":
			r.BySecond, err = intList(v, 0, 60, false)
		case "BYMINUTE":
			r.ByMinute, err = intList(v, 0, 59, false)
		case "BYHOUR":
			r.ByHour, err = intList(v, 0, 23, false)
		case "BYMONTHDAY":
			r.ByMonthDay, err = intList(v, -31, 31, true)
		case "BYYEARDAY":
			r.ByYearDay, err = intList(v, -366, 366, true)
		case "BYWEEKNO":
			r.ByWeekNo, err = intList(v, -53, 53, true)
		case "BYMONTH":
			r.ByMonth, err = intList(v, 1, 12, false)
		case "BYSETPOS":
			r.BySetPos, err = intList(v, -366, 366, true)
		case "WKST":
			d, ok := dayNames[strings.ToUpper(v)]
			if !ok {
				err = fmt.Errorf("not a weekday")
			}
			r.WeekStart = d
		case "BYDAY":
			for _, item := range strings.Split(v, ",") {
				item = strings.ToUpper(strings.TrimSpace(item))
				if len(item) < 2 {
					return Rule{}, fmt.Errorf("BYDAY=%s", v)
				}
				d, ok := dayNames[item[len(item)-2:]]
				if !ok {
					return Rule{}, fmt.Errorf("BYDAY=%s", v)
				}
				n := 0
				if num := item[:len(item)-2]; num != "" {
					n, err = strconv.Atoi(strings.TrimPrefix(num, "+"))
					if err != nil || n == 0 || n < -53 || n > 53 {
						return Rule{}, fmt.Errorf("BYDAY=%s", v)
					}
				}
				r.ByDay = append(r.ByDay, WeekdayNum{Day: d, N: n})
			}
		default:
			if !strings.HasPrefix(k, "X-") {
				return Rule{}, fmt.Errorf("unknown part %s", k)
			}
		}
		if err != nil {
			return Rule{}, fmt.Errorf("%s=%s: %v", k, v, err)
		}
	}
	if r.Freq < 0 {
		return Rule{}, fmt.Errorf("no FREQ")
	}
	if r.Count > 0 && !r.Until.IsZero() {
		return Rule{}, fmt.Errorf("COUNT and UNTIL together")
	}
	return r, nil
}

func intList(s string, lo, hi int, noZero bool) ([]int, error) {
	var out []int
	for _, item := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(item), "+"))
		if err != nil || n < lo || n > hi || (noZero && n == 0) {
			return nil, fmt.Errorf("%q is out of range", item)
		}
		out = append(out, n)
	}
	return out, nil
}

// Ends reports whether the rule has an end (COUNT or UNTIL).
func (r Rule) Ends() bool { return r.Count > 0 || !r.Until.IsZero() }

// maxEmptyPeriods stops a rule that never matches again (FREQ=YEARLY;
// BYMONTH=2;BYMONTHDAY=30) instead of looping forever.
const maxEmptyPeriods = 10000

// Iterator returns the rule's instances from dtstart on, in order. DTSTART
// is always the first, whether or not it matches the rule, and counts
// toward COUNT (RFC 5545 3.8.5.3). Times are wall times in dtstart's zone.
func (r Rule) Iterator(dtstart time.Time) func() (time.Time, bool) {
	loc := dtstart.Location()
	emitted := 0
	firstDone := false
	var pending []time.Time
	period := r.firstPeriod(dtstart)
	empty := 0
	done := false
	untilOK := func(t time.Time) bool {
		if r.Until.IsZero() {
			return true
		}
		if r.UntilDate {
			ty, tm, td := t.Date()
			uy, um, ud := r.Until.Date()
			return time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC).Compare(time.Date(uy, um, ud, 0, 0, 0, 0, time.UTC)) <= 0
		}
		return !t.After(r.Until)
	}
	return func() (time.Time, bool) {
		if done {
			return time.Time{}, false
		}
		if !firstDone {
			firstDone = true
			if !untilOK(dtstart) {
				done = true
				return time.Time{}, false
			}
			emitted++
			return dtstart, true
		}
		for {
			if r.Count > 0 && emitted >= r.Count {
				done = true
				return time.Time{}, false
			}
			for len(pending) > 0 {
				t := pending[0]
				pending = pending[1:]
				if !t.After(dtstart) {
					continue // before DTSTART, or DTSTART itself again
				}
				if !untilOK(t) {
					done = true
					return time.Time{}, false
				}
				emitted++
				return t, true
			}
			pending = r.expand(period, dtstart, loc)
			period = r.nextPeriod(period)
			if len(pending) == 0 {
				empty++
				if empty > maxEmptyPeriods {
					done = true
					return time.Time{}, false
				}
			} else {
				empty = 0
			}
			// A period that starts past UNTIL cannot bring anything.
			if !r.Until.IsZero() && len(pending) == 0 && period.After(r.Until.Add(366*24*time.Hour)) {
				done = true
				return time.Time{}, false
			}
		}
	}
}

// A period is identified by its first instant in wall time: Jan 1 for a
// year, the 1st for a month, the week's WKST day, the day, the hour...
func (r Rule) firstPeriod(t time.Time) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	switch r.Freq {
	case Yearly:
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	case Monthly:
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	case Weekly:
		back := (int(t.Weekday()) - int(r.WeekStart) + 7) % 7
		return time.Date(y, m, d-back, 0, 0, 0, 0, loc)
	case Daily:
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	case Hourly:
		return time.Date(y, m, d, t.Hour(), 0, 0, 0, loc)
	case Minutely:
		return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, loc)
	}
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, loc)
}

func (r Rule) nextPeriod(p time.Time) time.Time {
	y, m, d := p.Date()
	loc := p.Location()
	switch r.Freq {
	case Yearly:
		return time.Date(y+r.Interval, 1, 1, 0, 0, 0, 0, loc)
	case Monthly:
		return time.Date(y, m+time.Month(r.Interval), 1, 0, 0, 0, 0, loc)
	case Weekly:
		return time.Date(y, m, d+7*r.Interval, 0, 0, 0, 0, loc)
	case Daily:
		return time.Date(y, m, d+r.Interval, 0, 0, 0, 0, loc)
	case Hourly:
		return p.Add(time.Duration(r.Interval) * time.Hour)
	case Minutely:
		return p.Add(time.Duration(r.Interval) * time.Minute)
	}
	return p.Add(time.Duration(r.Interval) * time.Second)
}

// day is a calendar date, free of any zone.
type day struct {
	y int
	m time.Month
	d int
}

func (d day) weekday() time.Weekday { return time.Date(d.y, d.m, d.d, 12, 0, 0, 0, time.UTC).Weekday() }
func (d day) yearDay() int          { return time.Date(d.y, d.m, d.d, 12, 0, 0, 0, time.UTC).YearDay() }

func daysIn(y int, m time.Month) int { return time.Date(y, m+1, 0, 12, 0, 0, 0, time.UTC).Day() }
func daysInYear(y int) int           { return time.Date(y, 12, 31, 12, 0, 0, 0, time.UTC).YearDay() }

// expand lists the instances of one period, sorted, BYSETPOS applied.
func (r Rule) expand(period, dtstart time.Time, loc *time.Location) []time.Time {
	var days []day
	py, pm, pd := period.Date()
	switch r.Freq {
	case Yearly:
		days = r.yearDays(py, dtstart)
	case Monthly:
		if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(pm)) {
			return nil
		}
		days = r.monthDays(py, pm, dtstart)
	case Weekly:
		for i := range 7 {
			t := time.Date(py, pm, pd+i, 12, 0, 0, 0, time.UTC)
			dd := day{t.Year(), t.Month(), t.Day()}
			if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(dd.m)) {
				continue
			}
			if len(r.ByDay) > 0 {
				if !r.dayIn(dd) {
					continue
				}
			} else if dd.weekday() != dtstart.Weekday() {
				continue
			}
			days = append(days, dd)
		}
	default: // Daily and finer: the period's own day, limited by the BY parts
		dd := day{py, pm, pd}
		if r.limitDay(dd) {
			days = []day{dd}
		}
	}
	var out []time.Time
	for _, dd := range days {
		for _, h := range r.hours(period, dtstart) {
			for _, mi := range r.minutes(period, dtstart) {
				for _, s := range r.seconds(period, dtstart) {
					// A wall time that does not exist (02:30 in a DST gap)
					// becomes the time after the gap, as RFC 5545 3.3.5
					// says; time.Date does exactly that. The days come from
					// real months, so no invalid date gets here.
					out = append(out, time.Date(dd.y, dd.m, dd.d, h, mi, s, 0, loc))
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	out = dedupe(out)
	if len(r.BySetPos) > 0 && len(out) > 0 {
		var picked []time.Time
		for _, pos := range r.BySetPos {
			i := pos - 1
			if pos < 0 {
				i = len(out) + pos
			}
			if i >= 0 && i < len(out) {
				picked = append(picked, out[i])
			}
		}
		sort.Slice(picked, func(i, j int) bool { return picked[i].Before(picked[j]) })
		out = dedupe(picked)
	}
	return out
}

func dedupe(ts []time.Time) []time.Time {
	out := ts[:0]
	for i, t := range ts {
		if i == 0 || !t.Equal(ts[i-1]) {
			out = append(out, t)
		}
	}
	return out
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// hours, minutes, seconds: the BY lists for coarser frequencies, the
// period's own value (limited by the BY list) for finer ones, else
// DTSTART's.
func (r Rule) hours(period, dtstart time.Time) []int {
	if r.Freq <= Hourly {
		if len(r.ByHour) > 0 && !containsInt(r.ByHour, period.Hour()) {
			return nil
		}
		return []int{period.Hour()}
	}
	if len(r.ByHour) > 0 {
		return r.ByHour
	}
	return []int{dtstart.Hour()}
}

func (r Rule) minutes(period, dtstart time.Time) []int {
	if r.Freq <= Minutely {
		if len(r.ByMinute) > 0 && !containsInt(r.ByMinute, period.Minute()) {
			return nil
		}
		return []int{period.Minute()}
	}
	if len(r.ByMinute) > 0 {
		return r.ByMinute
	}
	return []int{dtstart.Minute()}
}

func (r Rule) seconds(period, dtstart time.Time) []int {
	if r.Freq == Secondly {
		if len(r.BySecond) > 0 && !containsInt(r.BySecond, period.Second()) {
			return nil
		}
		return []int{period.Second()}
	}
	if len(r.BySecond) > 0 {
		return r.BySecond
	}
	return []int{dtstart.Second()}
}

// limitDay applies the day-level BY parts as limits (DAILY and finer).
func (r Rule) limitDay(dd day) bool {
	if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(dd.m)) {
		return false
	}
	if len(r.ByYearDay) > 0 && !r.yearDayIn(dd) {
		return false
	}
	if len(r.ByMonthDay) > 0 && !r.monthDayIn(dd) {
		return false
	}
	if len(r.ByDay) > 0 && !r.dayIn(dd) {
		return false
	}
	return true
}

func (r Rule) monthDayIn(dd day) bool {
	n := daysIn(dd.y, dd.m)
	for _, md := range r.ByMonthDay {
		if md == dd.d || (md < 0 && n+md+1 == dd.d) {
			return true
		}
	}
	return false
}

func (r Rule) yearDayIn(dd day) bool {
	n := daysInYear(dd.y)
	yd := dd.yearDay()
	for _, v := range r.ByYearDay {
		if v == yd || (v < 0 && n+v+1 == yd) {
			return true
		}
	}
	return false
}

// dayIn matches BYDAY without ordinals (any entry for this weekday).
func (r Rule) dayIn(dd day) bool {
	wd := dd.weekday()
	for _, b := range r.ByDay {
		if b.Day == wd {
			return true
		}
	}
	return false
}

// nthIn matches BYDAY with ordinals counted inside a span of days (a month
// or a year): 2MO is the second Monday of it, -1FR the last Friday.
func (r Rule) nthIn(dd day, span []day) bool {
	wd := dd.weekday()
	for _, b := range r.ByDay {
		if b.Day != wd {
			continue
		}
		if b.N == 0 {
			return true
		}
		var same []day
		for _, s := range span {
			if s.weekday() == wd {
				same = append(same, s)
			}
		}
		i := b.N - 1
		if b.N < 0 {
			i = len(same) + b.N
		}
		if i >= 0 && i < len(same) && same[i] == dd {
			return true
		}
	}
	return false
}

func monthSpan(y int, m time.Month) []day {
	n := daysIn(y, m)
	out := make([]day, n)
	for i := range n {
		out[i] = day{y, m, i + 1}
	}
	return out
}

// monthDays is a MONTHLY period's days: BYMONTHDAY and BYDAY intersect
// (MONTHLY;BYDAY=FR;BYMONTHDAY=13 is Friday the 13th), and with neither the
// day of DTSTART (skipped in months too short for it).
func (r Rule) monthDays(y int, m time.Month, dtstart time.Time) []day {
	span := monthSpan(y, m)
	var out []day
	for _, dd := range span {
		switch {
		case len(r.ByMonthDay) == 0 && len(r.ByDay) == 0:
			if dd.d != dtstart.Day() {
				continue
			}
		default:
			if len(r.ByMonthDay) > 0 && !r.monthDayIn(dd) {
				continue
			}
			if len(r.ByDay) > 0 && !r.nthIn(dd, span) {
				continue
			}
		}
		out = append(out, dd)
	}
	return out
}

// yearDays is a YEARLY period's days, the BY parts intersecting. Ordinals
// in BYDAY count in the month when BYMONTH is given, else in the year.
func (r Rule) yearDays(y int, dtstart time.Time) []day {
	noDayParts := len(r.ByWeekNo) == 0 && len(r.ByYearDay) == 0 && len(r.ByMonthDay) == 0 && len(r.ByDay) == 0
	var yearSpan []day
	for m := time.January; m <= time.December; m++ {
		yearSpan = append(yearSpan, monthSpan(y, m)...)
	}
	var out []day
	for _, dd := range yearSpan {
		if len(r.ByMonth) > 0 && !containsInt(r.ByMonth, int(dd.m)) {
			continue
		}
		if noDayParts {
			// DTSTART's day of the month, in DTSTART's month unless BYMONTH
			// names others.
			if dd.d != dtstart.Day() || (len(r.ByMonth) == 0 && dd.m != dtstart.Month()) {
				continue
			}
			out = append(out, dd)
			continue
		}
		if len(r.ByWeekNo) > 0 && !r.weekNoIn(dd) {
			continue
		}
		if len(r.ByYearDay) > 0 && !r.yearDayIn(dd) {
			continue
		}
		if len(r.ByMonthDay) > 0 && !r.monthDayIn(dd) {
			continue
		}
		if len(r.ByDay) > 0 {
			span := yearSpan
			if len(r.ByMonth) > 0 {
				span = monthSpan(y, dd.m)
			}
			if !r.nthIn(dd, span) {
				continue
			}
		} else if len(r.ByWeekNo) > 0 && len(r.ByYearDay) == 0 && len(r.ByMonthDay) == 0 && dd.weekday() != dtstart.Weekday() {
			continue // BYWEEKNO alone: DTSTART's weekday in those weeks
		}
		out = append(out, dd)
	}
	return out
}

// week1Start is the first day of week 1 of year y: the week, starting on
// WKST, that holds at least four days of the year (RFC 5545 3.3.10).
func (r Rule) week1Start(y int) time.Time {
	jan1 := time.Date(y, 1, 1, 12, 0, 0, 0, time.UTC)
	back := (int(jan1.Weekday()) - int(r.WeekStart) + 7) % 7
	start := jan1.AddDate(0, 0, -back)
	if back > 3 { // that week has fewer than four days of y
		start = start.AddDate(0, 0, 7)
	}
	return start
}

// weekNoIn matches BYWEEKNO for a day, counting weeks in the day's year
// (a day before week 1 is in the last week of the year before, and is not
// matched here).
func (r Rule) weekNoIn(dd day) bool {
	t := time.Date(dd.y, dd.m, dd.d, 12, 0, 0, 0, time.UTC)
	start := r.week1Start(dd.y)
	next := r.week1Start(dd.y + 1)
	if t.Before(start) || !t.Before(next) {
		return false
	}
	week := int(t.Sub(start).Hours()/24)/7 + 1
	weeks := int(next.Sub(start).Hours()/24) / 7
	for _, w := range r.ByWeekNo {
		if w == week || (w < 0 && weeks+w+1 == week) {
			return true
		}
	}
	return false
}
