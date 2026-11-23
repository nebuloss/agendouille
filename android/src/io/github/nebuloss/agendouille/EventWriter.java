package io.github.nebuloss.agendouille;

import java.time.Instant;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

/**
 * The phone's event as the iCalendar to upload (PLAN.md 7.3): the
 * server's text patched, never rewritten (PLAN 4.6). A field is written
 * only when the phone's value differs from what the server's text says,
 * so properties the phone does not know, and the way the server wrote the
 * others (a TZID, an EXDATE's form), are kept. A new event gets a new
 * object. Pure Java: tested on the JVM against testdata/ical.
 */
final class EventWriter {
    private EventWriter() {}

    /** An overridden or cancelled occurrence of the series, as the phone has it. */
    static final class Occurrence {
        final Map<String, Object> values;
        final List<int[]> reminders;
        final boolean cancelled; // deleted on the phone, or STATUS_CANCELED

        Occurrence(Map<String, Object> values, List<int[]> reminders, boolean cancelled) {
            this.values = values;
            this.reminders = reminders;
            this.cancelled = cancelled;
        }
    }

    private static final DateTimeFormatter STAMP = DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss'Z'").withZone(ZoneOffset.UTC);
    private static final DateTimeFormatter LOCAL = DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss");
    private static final DateTimeFormatter DATE = DateTimeFormatter.ofPattern("yyyyMMdd").withZone(ZoneOffset.UTC);

    /**
     * The object's text: original (the server's, or null for a new event)
     * with the master's values, its reminders and the occurrences applied.
     */
    static String write(String original, Map<String, Object> master, List<int[]> reminders, List<Occurrence> occurrences,
                        ZoneId deviceZone, long now) {
        ICal.Component cal;
        ICal.Component event;
        if (original != null) {
            cal = ICal.parse(original);
            event = null;
            for (ICal.Component e : cal.children("VEVENT")) if (e.get("RECURRENCE-ID") == null && event == null) event = e;
            if (event == null) throw new IllegalArgumentException("no master VEVENT to patch");
        } else {
            cal = new ICal.Component("VCALENDAR");
            cal.props.add(prop("VERSION", "2.0"));
            cal.props.add(prop("PRODID", "-//agendouille//Android//FR"));
            event = new ICal.Component("VEVENT");
            Object uid = master.get("uid2445");
            event.props.add(prop("UID", uid != null && !uid.toString().isEmpty() ? uid.toString() : UUID.randomUUID() + "@agendouille"));
            cal.children.add(event);
        }
        boolean changed = patch(event, master, reminders, true, deviceZone, original == null);
        EventMapper.When start = EventMapper.when(event.get("DTSTART"), deviceZone);
        for (Occurrence o : occurrences) {
            Object t = o.values.get("originalInstanceTime");
            if (!(t instanceof Number)) continue;
            long instance = ((Number) t).longValue();
            ICal.Component override = null;
            for (ICal.Component e : cal.children("VEVENT")) {
                EventMapper.When rid = EventMapper.when(e.get("RECURRENCE-ID"), deviceZone);
                if (rid != null && rid.millis == instance) override = e;
            }
            if (o.cancelled) {
                // A cancelled occurrence is an EXDATE of the series (what the
                // phone's own calendar apps understand), its override gone.
                if (override != null) cal.children.remove(override);
                if (addExdate(event, instance, start)) changed = true;
                continue;
            }
            if (override == null) {
                override = new ICal.Component("VEVENT");
                override.props.add(prop("UID", event.get("UID").value));
                override.props.add(prop("RECURRENCE-ID", start, instance));
                int at = cal.children.indexOf(event) + 1;
                cal.children.add(at, override);
            }
            if (patch(override, o.values, o.reminders, false, deviceZone, true)) changed = true;
        }
        if (changed || original == null) stamp(event, now);
        return ICal.write(cal);
    }

    /** Applies the phone's values to one VEVENT; true when something changed. */
    private static boolean patch(ICal.Component e, Map<String, Object> v, List<int[]> reminders, boolean master, ZoneId zone, boolean fresh) {
        boolean changed = false;
        changed |= text(e, "SUMMARY", v.get("title"));
        changed |= text(e, "LOCATION", v.get("eventLocation"));
        changed |= text(e, "DESCRIPTION", v.get("description"));

        boolean allDay = num(v.get("allDay")) == 1;
        long dtstart = lng(v.get("dtstart"));
        String tz = str(v.get("eventTimezone"));
        EventMapper.When was = EventMapper.when(e.get("DTSTART"), zone);
        if (was == null || was.millis != dtstart || was.allDay != allDay) {
            set(e, prop("DTSTART", allDay, tz, dtstart));
            changed = true;
        }
        // The end: DTEND, or DTSTART + DURATION for a series (Android's own form).
        long end = dtstart;
        Object dtend = v.get("dtend");
        if (dtend instanceof Number) end = ((Number) dtend).longValue();
        else if (v.get("duration") != null) {
            Long d = EventMapper.durationMillis(str(v.get("duration")));
            if (d != null) end = dtstart + d;
        } else if (allDay) end = dtstart + 86400000L;
        EventMapper.When wasEnd = EventMapper.when(e.get("DTEND"), zone);
        Long wasDur = e.get("DURATION") != null ? EventMapper.durationMillis(e.get("DURATION").value) : null;
        long oldEnd = wasEnd != null ? wasEnd.millis : wasDur != null && was != null ? was.millis + wasDur : -1;
        if (oldEnd != end || changed && fresh) {
            e.props.removeIf(p -> p.name.equals("DURATION"));
            set(e, prop("DTEND", allDay, tz, end));
            changed = true;
        }
        if (master) {
            String rrule = str(v.get("rrule"));
            ICal.Property had = e.get("RRULE");
            if (!rrule.equals(had == null ? "" : had.value)) {
                e.props.removeIf(p -> p.name.equals("RRULE"));
                if (!rrule.isEmpty()) e.props.add(prop("RRULE", rrule));
                changed = true;
            }
            String exdate = str(v.get("exdate"));
            if (!sameDates(exdate, e.all("EXDATE"), zone)) {
                e.props.removeIf(p -> p.name.equals("EXDATE"));
                for (String one : exdate.split(",")) {
                    if (one.trim().isEmpty()) continue;
                    long ms = Instant.from(STAMP.parse(one.trim())).toEpochMilli();
                    e.props.add(prop("EXDATE", allDay, tz, ms));
                }
                changed = true;
            }
        }
        // No status on the phone is a confirmed event, as in RFC 5545.
        int status = v.get("eventStatus") instanceof Number ? num(v.get("eventStatus")) : EventMapper.STATUS_CONFIRMED;
        String st = status == EventMapper.STATUS_CANCELED ? "CANCELLED" : status == EventMapper.STATUS_TENTATIVE ? "TENTATIVE" : "CONFIRMED";
        String hadStatus = e.text("STATUS");
        if (!st.equals(hadStatus == null ? "CONFIRMED" : hadStatus.trim().toUpperCase())) {
            set(e, prop("STATUS", st));
            changed = true;
        }
        boolean free = num(v.get("availability")) == EventMapper.AVAILABILITY_FREE;
        boolean wasFree = "TRANSPARENT".equalsIgnoreCase(String.valueOf(e.text("TRANSP")).trim());
        if (free != wasFree) {
            e.props.removeIf(p -> p.name.equals("TRANSP"));
            if (free) e.props.add(prop("TRANSP", "TRANSPARENT"));
            changed = true;
        }
        // Alarms: the ones the phone understands are replaced when they
        // differ; others (absolute, after the end) stay.
        List<int[]> had = new ArrayList<>();
        for (ICal.Component a : e.children("VALARM")) {
            int[] r = EventMapper.reminder(a);
            if (r != null) had.add(r);
        }
        if (!sameReminders(had, reminders)) {
            e.children.removeIf(a -> a.name.equals("VALARM") && EventMapper.reminder(a) != null);
            for (int[] r : reminders) {
                ICal.Component a = new ICal.Component("VALARM");
                a.props.add(prop("ACTION", "DISPLAY"));
                a.props.add(prop("DESCRIPTION", ICal.escape(str(v.get("title")).isEmpty() ? "agendouille" : str(v.get("title")))));
                a.props.add(prop("TRIGGER", "-PT" + r[0] + "M"));
                e.children.add(a);
            }
            changed = true;
        }
        return changed;
    }

    private static boolean sameReminders(List<int[]> a, List<int[]> b) {
        if (a.size() != b.size()) return false;
        Set<String> x = new LinkedHashSet<>();
        for (int[] r : a) x.add(r[0] + "/" + r[1]);
        for (int[] r : b) if (!x.contains(r[0] + "/" + r[1])) return false;
        return true;
    }

    private static boolean sameDates(String utcList, List<ICal.Property> props, ZoneId zone) {
        Set<Long> want = new LinkedHashSet<>();
        for (String one : utcList.split(",")) if (!one.trim().isEmpty()) want.add(Instant.from(STAMP.parse(one.trim())).toEpochMilli());
        Set<Long> have = new LinkedHashSet<>();
        for (ICal.Property p : props) {
            for (String one : p.value.split(",")) {
                if (one.trim().isEmpty()) continue;
                have.add(EventMapper.when(new ICal.Property(p.name, p.params, one.trim()), zone).millis);
            }
        }
        return want.equals(have);
    }

    /** Adds an occurrence to the series' EXDATE, in the form of its DTSTART. */
    private static boolean addExdate(ICal.Component event, long instance, EventMapper.When start) {
        for (ICal.Property p : event.all("EXDATE")) {
            for (String one : p.value.split(",")) {
                EventMapper.When w = EventMapper.when(new ICal.Property(p.name, p.params, one.trim()), ZoneId.of(start.zone.equals("UTC") ? "UTC" : start.zone));
                if (w.millis == instance) return false;
            }
        }
        event.props.add(prop("EXDATE", start, instance));
        return true;
    }

    private static void stamp(ICal.Component e, long now) {
        set(e, prop("DTSTAMP", STAMP.format(Instant.ofEpochMilli(now))));
        if (e.get("LAST-MODIFIED") != null) set(e, prop("LAST-MODIFIED", STAMP.format(Instant.ofEpochMilli(now))));
        ICal.Property seq = e.get("SEQUENCE");
        if (seq != null) {
            try {
                set(e, prop("SEQUENCE", Integer.toString(Integer.parseInt(seq.value.trim()) + 1)));
            } catch (NumberFormatException ignored) {
                // leave a SEQUENCE we cannot read as it was
            }
        }
    }

    /** SUMMARY and the like: written when the text differs, removed when emptied. */
    private static boolean text(ICal.Component e, String name, Object value) {
        String now = value == null ? "" : value.toString();
        String was = e.text(name);
        if (now.equals(was == null ? "" : was)) return false;
        e.props.removeIf(p -> p.name.equals(name));
        if (!now.isEmpty()) e.props.add(prop(name, ICal.escape(now)));
        return true;
    }

    /** Replaces a property in place, keeping the order of the others. */
    private static void set(ICal.Component e, ICal.Property p) {
        for (int i = 0; i < e.props.size(); i++) {
            if (e.props.get(i).name.equals(p.name)) {
                e.props.set(i, p);
                e.props.removeIf(q -> q != p && q.name.equals(p.name));
                return;
            }
        }
        e.props.add(p);
    }

    private static ICal.Property prop(String name, String value) {
        return new ICal.Property(name, new java.util.LinkedHashMap<>(), value);
    }

    /** A date or date-time property: VALUE=DATE, UTC, or TZID with local time. */
    private static ICal.Property prop(String name, boolean allDay, String tz, long millis) {
        Map<String, String> params = new java.util.LinkedHashMap<>();
        Instant at = Instant.ofEpochMilli(millis);
        if (allDay) {
            params.put("VALUE", "DATE");
            return new ICal.Property(name, params, DATE.format(at));
        }
        if (tz == null || tz.isEmpty() || tz.equals("UTC")) return new ICal.Property(name, params, STAMP.format(at));
        params.put("TZID", tz);
        return new ICal.Property(name, params, LOCAL.format(at.atZone(ZoneId.of(tz))));
    }

    /** A date property in the form of the series' start (an EXDATE, a RECURRENCE-ID). */
    private static ICal.Property prop(String name, EventMapper.When like, long millis) {
        return prop(name, like.allDay, like.zone, millis);
    }

    private static long lng(Object o) {
        return o instanceof Number ? ((Number) o).longValue() : 0;
    }

    private static int num(Object o) {
        return o instanceof Number ? ((Number) o).intValue() : 0;
    }

    private static String str(Object o) {
        return o == null ? "" : o.toString();
    }
}
