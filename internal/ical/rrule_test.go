package ical

import (
	"strings"
	"testing"
	"time"
)

// The examples of RFC 5545 3.8.5.3, with the instances the RFC lists, in
// America/New_York as the RFC writes them. Each checks the first instances
// (all of them when the rule ends early enough).
func TestRFC5545Examples(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	type example struct {
		name, dtstart, rule string
		want                []string // "YYYYMMDDTHHMM" local wall times
		total               int      // when > 0, the rule ends after exactly this many
	}
	days := func(prefix string, ds ...int) []string { // same month, 09:00
		var out []string
		for _, d := range ds {
			out = append(out, prefix+twoDigits(d)+"T0900")
		}
		return out
	}
	join := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	examples := []example{
		{"daily for 10", "19970902T090000", "FREQ=DAILY;COUNT=10",
			days("199709", 2, 3, 4, 5, 6, 7, 8, 9, 10, 11), 10},
		{"daily until Dec 24", "19970902T090000", "FREQ=DAILY;UNTIL=19971224T000000Z",
			days("199709", 2, 3, 4), 113},
		{"every other day", "19970902T090000", "FREQ=DAILY;INTERVAL=2",
			days("199709", 2, 4, 6, 8, 10, 12), 0},
		{"every 10 days, 5 times", "19970902T090000", "FREQ=DAILY;INTERVAL=10;COUNT=5",
			join(days("199709", 2, 12, 22), days("199710", 2, 12)), 5},
		{"every day in January, 3 years (yearly)", "19980101T090000",
			"FREQ=YEARLY;UNTIL=20000131T140000Z;BYMONTH=1;BYDAY=SU,MO,TU,WE,TH,FR,SA",
			days("199801", 1, 2, 3), 93},
		{"every day in January, 3 years (daily)", "19980101T090000",
			"FREQ=DAILY;UNTIL=20000131T140000Z;BYMONTH=1", days("199801", 1, 2, 3), 93},
		{"weekly for 10", "19970902T090000", "FREQ=WEEKLY;COUNT=10",
			join(days("199709", 2, 9, 16, 23, 30), days("199710", 7, 14, 21, 28), days("199711", 4)), 10},
		{"weekly until Dec 24", "19970902T090000", "FREQ=WEEKLY;UNTIL=19971224T000000Z",
			days("199709", 2, 9, 16), 17},
		{"every other week", "19970902T090000", "FREQ=WEEKLY;INTERVAL=2;WKST=SU",
			join(days("199709", 2, 16, 30), days("199710", 14, 28)), 0},
		{"Tuesdays and Thursdays for 5 weeks", "19970902T090000",
			"FREQ=WEEKLY;UNTIL=19971007T000000Z;WKST=SU;BYDAY=TU,TH",
			join(days("199709", 2, 4, 9, 11, 16, 18, 23, 25, 30), days("199710", 2)), 10},
		{"same with COUNT", "19970902T090000", "FREQ=WEEKLY;COUNT=10;WKST=SU;BYDAY=TU,TH",
			join(days("199709", 2, 4, 9, 11, 16, 18, 23, 25, 30), days("199710", 2)), 10},
		{"every other week on MO,WE,FR", "19970901T090000",
			"FREQ=WEEKLY;INTERVAL=2;UNTIL=19971224T000000Z;WKST=SU;BYDAY=MO,WE,FR",
			join(days("199709", 1, 3, 5, 15, 17, 19, 29), days("199710", 1, 3, 13, 15, 17, 27, 29, 31),
				days("199711", 10, 12, 14, 24, 26, 28), days("199712", 8, 10, 12, 22)), 25},
		{"every other week on TU,TH, 8 times", "19970902T090000",
			"FREQ=WEEKLY;INTERVAL=2;COUNT=8;WKST=SU;BYDAY=TU,TH",
			join(days("199709", 2, 4, 16, 18, 30), days("199710", 2, 14, 16)), 8},
		{"first Friday, 10 months", "19970905T090000", "FREQ=MONTHLY;COUNT=10;BYDAY=1FR",
			join(days("199709", 5), days("199710", 3), days("199711", 7), days("199712", 5), days("199801", 2),
				days("199802", 6), days("199803", 6), days("199804", 3), days("199805", 1), days("199806", 5)), 10},
		{"first and last Sunday, every other month", "19970907T090000",
			"FREQ=MONTHLY;INTERVAL=2;COUNT=10;BYDAY=1SU,-1SU",
			join(days("199709", 7, 28), days("199711", 2, 30), days("199801", 4, 25), days("199803", 1, 29),
				days("199805", 3, 31)), 10},
		{"second-to-last Monday", "19970922T090000", "FREQ=MONTHLY;COUNT=6;BYDAY=-2MO",
			join(days("199709", 22), days("199710", 20), days("199711", 17), days("199712", 22), days("199801", 19),
				days("199802", 16)), 6},
		{"third-to-last day", "19970928T090000", "FREQ=MONTHLY;BYMONTHDAY=-3",
			join(days("199709", 28), days("199710", 29), days("199711", 28), days("199712", 29), days("199801", 29),
				days("199802", 26)), 0},
		{"2nd and 15th", "19970902T090000", "FREQ=MONTHLY;COUNT=10;BYMONTHDAY=2,15",
			join(days("199709", 2, 15), days("199710", 2, 15), days("199711", 2, 15), days("199712", 2, 15),
				days("199801", 2, 15)), 10},
		{"first and last day", "19970930T090000", "FREQ=MONTHLY;COUNT=10;BYMONTHDAY=1,-1",
			join(days("199709", 30), days("199710", 1, 31), days("199711", 1, 30), days("199712", 1, 31),
				days("199801", 1, 31), days("199802", 1)), 10},
		{"every 18 months, 10th to 15th", "19970910T090000",
			"FREQ=MONTHLY;INTERVAL=18;COUNT=10;BYMONTHDAY=10,11,12,13,14,15",
			join(days("199709", 10, 11, 12, 13, 14, 15), days("199903", 10, 11, 12, 13)), 10},
		{"every Tuesday, every other month", "19970902T090000", "FREQ=MONTHLY;INTERVAL=2;BYDAY=TU",
			join(days("199709", 2, 9, 16, 23, 30), days("199711", 4, 11, 18, 25), days("199801", 6)), 0},
		{"June and July, 10 times", "19970610T090000", "FREQ=YEARLY;COUNT=10;BYMONTH=6,7",
			join(days("199706", 10), days("199707", 10), days("199806", 10), days("199807", 10), days("199906", 10),
				days("199907", 10), days("200006", 10), days("200007", 10), days("200106", 10), days("200107", 10)), 10},
		{"Jan, Feb, Mar every other year", "19970310T090000", "FREQ=YEARLY;INTERVAL=2;COUNT=10;BYMONTH=1,2,3",
			join(days("199703", 10), days("199901", 10), days("199902", 10), days("199903", 10), days("200101", 10),
				days("200102", 10), days("200103", 10), days("200301", 10), days("200302", 10), days("200303", 10)), 10},
		{"1st, 100th, 200th day every third year", "19970101T090000",
			"FREQ=YEARLY;INTERVAL=3;COUNT=10;BYYEARDAY=1,100,200",
			join(days("199701", 1), days("199704", 10), days("199707", 19), days("200001", 1), days("200004", 9),
				days("200007", 18), days("200301", 1), days("200304", 10), days("200307", 19), days("200601", 1)), 10},
		{"20th Monday of the year", "19970519T090000", "FREQ=YEARLY;BYDAY=20MO",
			join(days("199705", 19), days("199805", 18), days("199905", 17)), 0},
		{"Monday of week 20", "19970512T090000", "FREQ=YEARLY;BYWEEKNO=20;BYDAY=MO",
			join(days("199705", 12), days("199805", 11), days("199905", 17)), 0},
		{"every Thursday in March", "19970313T090000", "FREQ=YEARLY;BYMONTH=3;BYDAY=TH",
			join(days("199703", 13, 20, 27), days("199803", 5, 12, 19, 26), days("199903", 4, 11, 18, 25)), 0},
		{"every Thursday, only in summer", "19970605T090000", "FREQ=YEARLY;BYDAY=TH;BYMONTH=6,7,8",
			join(days("199706", 5, 12, 19, 26), days("199707", 3, 10, 17, 24, 31), days("199708", 7, 14, 21, 28)), 0},
		{"first Saturday after the first Sunday", "19970913T090000",
			"FREQ=MONTHLY;BYDAY=SA;BYMONTHDAY=7,8,9,10,11,12,13",
			join(days("199709", 13), days("199710", 11), days("199711", 8), days("199712", 13), days("199801", 10),
				days("199802", 7), days("199803", 7), days("199804", 11), days("199805", 9), days("199806", 13)), 0},
		{"US presidential election day", "19961105T090000",
			"FREQ=YEARLY;INTERVAL=4;BYMONTH=11;BYDAY=TU;BYMONTHDAY=2,3,4,5,6,7,8",
			join(days("199611", 5), days("200011", 7), days("200411", 2)), 0},
		{"third of TU,WE,TH, 3 months", "19970904T090000", "FREQ=MONTHLY;COUNT=3;BYDAY=TU,WE,TH;BYSETPOS=3",
			join(days("199709", 4), days("199710", 7), days("199711", 6)), 3},
		{"second-to-last weekday", "19970929T090000", "FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-2",
			join(days("199709", 29), days("199710", 30), days("199711", 27), days("199712", 30), days("199801", 29),
				days("199802", 26), days("199803", 30)), 0},
		{"every 15 minutes, 6 times", "19970902T090000", "FREQ=MINUTELY;INTERVAL=15;COUNT=6",
			[]string{"19970902T0900", "19970902T0915", "19970902T0930", "19970902T0945", "19970902T1000", "19970902T1015"}, 6},
		{"every hour and a half, 4 times", "19970902T090000", "FREQ=MINUTELY;INTERVAL=90;COUNT=4",
			[]string{"19970902T0900", "19970902T1030", "19970902T1200", "19970902T1330"}, 4},
		{"every 20 minutes from 9 to 16:40", "19970902T090000", "FREQ=DAILY;BYHOUR=9,10,11,12,13,14,15,16;BYMINUTE=0,20,40",
			[]string{"19970902T0900", "19970902T0920", "19970902T0940", "19970902T1000"}, 0},
		{"WKST=MO", "19970805T090000", "FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=MO",
			join(days("199708", 5, 10, 19, 24)), 4},
		{"WKST=SU", "19970805T090000", "FREQ=WEEKLY;INTERVAL=2;COUNT=4;BYDAY=TU,SU;WKST=SU",
			join(days("199708", 5, 17, 19, 31)), 4},
		{"invalid dates are skipped", "20070115T090000", "FREQ=MONTHLY;BYMONTHDAY=15,30;COUNT=5",
			join(days("200701", 15, 30), days("200702", 15), days("200703", 15, 30)), 5},
	}
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			start, err := time.ParseInLocation("20060102T150405", ex.dtstart, ny)
			if err != nil {
				t.Fatal(err)
			}
			r, err := ParseRule(ex.rule, ny)
			if err != nil {
				t.Fatal(err)
			}
			next := r.Iterator(start)
			var got []string
			total := 0
			for total < 2000 {
				ti, ok := next()
				if !ok {
					break
				}
				if len(got) < len(ex.want) {
					got = append(got, ti.In(ny).Format("20060102T1504"))
				}
				total++
			}
			if strings.Join(got, " ") != strings.Join(ex.want, " ") {
				t.Errorf("\n got  %v\n want %v", got, ex.want)
			}
			if ex.total > 0 && total != ex.total {
				t.Errorf("%d instances in all, want %d", total, ex.total)
			}
		})
	}
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// The Friday-13th example: the RFC excludes DTSTART with an EXDATE,
// because DTSTART always counts as an instance.
func TestFridayThe13thWithExdate(t *testing.T) {
	data := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\nBEGIN:VEVENT\r\nUID:f13\r\n" +
		"DTSTART;TZID=America/New_York:19970902T090000\r\nDTEND;TZID=America/New_York:19970902T100000\r\n" +
		"EXDATE;TZID=America/New_York:19970902T090000\r\nRRULE:FREQ=MONTHLY;BYDAY=FR;BYMONTHDAY=13\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	ny, _ := time.LoadLocation("America/New_York")
	obj, _, err := Read([]byte(data), 0, ny)
	if err != nil {
		t.Fatal(err)
	}
	occ, _, err := obj.Expand(ny, time.Date(1997, 1, 1, 0, 0, 0, 0, ny), time.Date(2001, 1, 1, 0, 0, 0, 0, ny), 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range occ {
		got = append(got, o.Start.In(ny).Format("2006-01-02"))
	}
	want := "1998-02-13 1998-03-13 1998-11-13 1999-08-13 2000-10-13"
	if strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestRuleErrors(t *testing.T) {
	for _, bad := range []string{"", "COUNT=3", "FREQ=FORTNIGHTLY", "FREQ=DAILY;INTERVAL=0", "FREQ=DAILY;COUNT=2;UNTIL=20200101",
		"FREQ=MONTHLY;BYMONTHDAY=0", "FREQ=WEEKLY;BYDAY=XX", "FREQ=DAILY;BYHOUR=24", "FREQ=DAILY;NOPE=1"} {
		if _, err := ParseRule(bad, time.UTC); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if _, err := ParseRule("FREQ=DAILY;X-NAME=1", time.UTC); err != nil {
		t.Errorf("an X- part: %v", err)
	}
}

func TestARuleThatNeverMatchesEnds(t *testing.T) {
	r, err := ParseRule("FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=30", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	next := r.Iterator(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	if _, ok := next(); !ok {
		t.Fatal("DTSTART itself is an instance")
	}
	if _, ok := next(); ok {
		t.Fatal("30 February was produced")
	}
}
