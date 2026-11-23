# Shared iCalendar fixtures

Cases the Android app's mapping (android/src/…/EventMapper.java, tested by
android/test/…/MappingTest.java) must get right, each a CalDAV object as a
server holds it. The Go side (internal/ical) can read the same files; a
change here is checked on both sides (AGENTS.md).

- `simple.ics`: a timed event in a named zone, escaped text, an alarm.
- `allday.ics`: two days, all day.
- `recurring.ics`: weekly, an excluded date, a moved occurrence and a
  cancelled one.
- `utc-duration.ics`: UTC start, DURATION instead of DTEND, free, private.
- `vendor-tz.ics`: a zone named the way some clients write it, a long
  folded description.
