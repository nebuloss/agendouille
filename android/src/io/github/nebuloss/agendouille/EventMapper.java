package io.github.nebuloss.agendouille;

import java.time.Duration;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * One CalDAV object (all the VEVENTs of one UID) as the rows of Android's
 * CalendarContract.Events (PLAN.md 7.3): the master, with its recurrence,
 * and each overridden occurrence as an exception row pointing at it. Column
 * names are CalendarContract's, written out so this stays pure Java and is
 * tested on the JVM against the shared fixtures (testdata/ical).
 */
final class EventMapper {
    private EventMapper() {}

    /** A row of Events, and its reminders (minutes before, method). */
    static final class Row {
        final Map<String, Object> values = new LinkedHashMap<>();
        final List<int[]> reminders = new ArrayList<>();
        boolean exception;

        Object get(String column) {
            return values.get(column);
        }
    }

    // CalendarContract.Events / Reminders, as constants there are.
    static final int STATUS_TENTATIVE = 0, STATUS_CONFIRMED = 1, STATUS_CANCELED = 2;
    static final int AVAILABILITY_BUSY = 0, AVAILABILITY_FREE = 1;
    static final int METHOD_ALERT = 1, METHOD_EMAIL = 2;

    /** A DTSTART-like value: the instant, whether it is a date, its zone. */
    static final class When {
        final long millis;
        final boolean allDay;
        final String zone; // an Olson id, or "UTC"

        When(long millis, boolean allDay, String zone) {
            this.millis = millis;
            this.allDay = allDay;
            this.zone = zone;
        }
    }

    /** The rows of one object; empty when it holds no VEVENT. */
    static List<Row> map(String ics, String href, String etag, ZoneId deviceZone) {
        ICal.Component cal = ICal.parse(ics);
        List<ICal.Component> events = cal.children("VEVENT");
        ICal.Component master = null;
        for (ICal.Component e : events) if (e.get("RECURRENCE-ID") == null && master == null) master = e;
        List<Row> out = new ArrayList<>();
        When masterStart = null;
        if (master != null) {
            Row r = row(master, deviceZone, true);
            masterStart = when(master.get("DTSTART"), deviceZone);
            r.values.put("_sync_id", href);
            r.values.put("sync_data1", etag);
            r.values.put("sync_data2", ics); // the server's text, patched on upload
            r.values.put("uid2445", master.text("UID"));
            out.add(r);
        }
        for (ICal.Component e : events) {
            if (e.get("RECURRENCE-ID") == null) continue;
            if (master == null) {
                // An occurrence without its series (a client's half): shown on its own.
                Row r = row(e, deviceZone, false);
                r.values.put("_sync_id", href);
                r.values.put("sync_data1", etag);
                r.values.put("sync_data2", ics);
                r.values.put("uid2445", e.text("UID"));
                out.add(r);
                master = e;
                continue;
            }
            Row r = row(e, deviceZone, false);
            r.exception = true;
            When rid = when(e.get("RECURRENCE-ID"), deviceZone);
            r.values.put("original_sync_id", href);
            r.values.put("originalInstanceTime", rid.millis);
            r.values.put("originalAllDay", masterStart != null && masterStart.allDay ? 1 : 0);
            out.add(r);
        }
        return out;
    }

    private static Row row(ICal.Component e, ZoneId deviceZone, boolean master) {
        Row r = new Row();
        Map<String, Object> v = r.values;
        v.put("title", nz(e.text("SUMMARY")));
        v.put("eventLocation", nz(e.text("LOCATION")));
        v.put("description", nz(e.text("DESCRIPTION")));
        When start = when(e.get("DTSTART"), deviceZone);
        if (start == null) throw new IllegalArgumentException("a VEVENT without DTSTART");
        long end = end(e, start, deviceZone);
        boolean recurring = master && (e.get("RRULE") != null || e.get("RDATE") != null);
        v.put("dtstart", start.millis);
        v.put("allDay", start.allDay ? 1 : 0);
        v.put("eventTimezone", start.zone);
        if (recurring) {
            // Android computes a series' occurrences from DTSTART and DURATION.
            long secs = Math.max(0, (end - start.millis) / 1000);
            v.put("duration", start.allDay ? "P" + Math.max(1, secs / 86400) + "D" : "PT" + secs + "S");
            v.put("dtend", null);
            ICal.Property rr = e.get("RRULE");
            if (rr != null) v.put("rrule", rr.value);
            String rdate = dates(e.all("RDATE"), deviceZone);
            if (!rdate.isEmpty()) v.put("rdate", rdate);
            String exdate = dates(e.all("EXDATE"), deviceZone);
            if (!exdate.isEmpty()) v.put("exdate", exdate);
        } else {
            v.put("dtend", end);
            v.put("eventEndTimezone", start.zone);
        }
        String status = upper(e.text("STATUS"));
        v.put("eventStatus", "CANCELLED".equals(status) ? STATUS_CANCELED : "TENTATIVE".equals(status) ? STATUS_TENTATIVE : STATUS_CONFIRMED);
        v.put("availability", "TRANSPARENT".equals(upper(e.text("TRANSP"))) ? AVAILABILITY_FREE : AVAILABILITY_BUSY);
        String cls = upper(e.text("CLASS"));
        v.put("accessLevel", "PRIVATE".equals(cls) ? 2 : "CONFIDENTIAL".equals(cls) ? 1 : "PUBLIC".equals(cls) ? 3 : 0);
        for (ICal.Component alarm : e.children("VALARM")) {
            int[] rem = reminder(alarm);
            if (rem != null) r.reminders.add(rem);
        }
        v.put("hasAlarm", r.reminders.isEmpty() ? 0 : 1);
        return r;
    }

    private static String nz(String s) {
        return s == null ? "" : s;
    }

    private static String upper(String s) {
        return s == null ? "" : s.trim().toUpperCase(Locale.ROOT);
    }

    /** DTEND, or DTSTART + DURATION, or the RFC 5545 default (a day, or nothing). */
    private static long end(ICal.Component e, When start, ZoneId deviceZone) {
        When end = when(e.get("DTEND"), deviceZone);
        if (end != null && end.millis >= start.millis) return end.millis;
        ICal.Property d = e.get("DURATION");
        if (d != null) {
            Long ms = durationMillis(d.value);
            if (ms != null && ms >= 0) return start.millis + ms;
        }
        return start.allDay ? start.millis + 86400000L : start.millis;
    }

    /** A date or date-time property, in its zone; null when absent. */
    static When when(ICal.Property p, ZoneId deviceZone) {
        if (p == null) return null;
        String v = p.value.trim();
        if ("DATE".equalsIgnoreCase(p.param("VALUE")) || v.matches("\\d{8}")) {
            LocalDate d = LocalDate.parse(v.substring(0, 8), DateTimeFormatter.BASIC_ISO_DATE);
            return new When(d.atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli(), true, "UTC");
        }
        LocalDateTime t = LocalDateTime.parse(v.replace("Z", ""), DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss"));
        if (v.endsWith("Z")) return new When(t.toInstant(ZoneOffset.UTC).toEpochMilli(), false, "UTC");
        ZoneId zone = zone(p.param("TZID"), deviceZone);
        return new When(t.atZone(zone).toInstant().toEpochMilli(), false, zone.getId());
    }

    /** A TZID as Java knows it (Olson ids, and "/mozilla.org/…/Europe/Paris"), else the device's. */
    static ZoneId zone(String tzid, ZoneId deviceZone) {
        if (tzid == null || tzid.isEmpty()) return deviceZone;
        String id = tzid;
        for (int tries = 0; tries < 2; tries++) {
            try {
                return ZoneId.of(id);
            } catch (Exception e) {
                // A vendor prefix: keep the last two segments ("Europe/Paris").
                String[] parts = tzid.split("/");
                if (parts.length < 2) break;
                id = parts[parts.length - 2] + "/" + parts[parts.length - 1];
            }
        }
        return deviceZone;
    }

    private static final DateTimeFormatter UTC_STAMP = DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmmss'Z'").withZone(ZoneOffset.UTC);

    /** EXDATE/RDATE values as Android keeps them: UTC stamps, commas. */
    private static String dates(List<ICal.Property> props, ZoneId deviceZone) {
        StringBuilder b = new StringBuilder();
        for (ICal.Property p : props) {
            for (String one : p.value.split(",")) {
                if (one.trim().isEmpty()) continue;
                When w = when(new ICal.Property(p.name, p.params, one.trim()), deviceZone);
                if (b.length() > 0) b.append(',');
                b.append(UTC_STAMP.format(Instant.ofEpochMilli(w.millis)));
            }
        }
        return b.toString();
    }

    private static final Pattern DUR = Pattern.compile("([+-])?P(?:(\\d+)W)?(?:(\\d+)D)?(?:T(?:(\\d+)H)?(?:(\\d+)M)?(?:(\\d+)S)?)?");

    /** An RFC 5545 duration in milliseconds, or null. */
    static Long durationMillis(String v) {
        Matcher m = DUR.matcher(v.trim().toUpperCase(Locale.ROOT));
        if (!m.matches()) return null;
        long s = 0;
        if (m.group(2) != null) s += Long.parseLong(m.group(2)) * 7 * 86400;
        if (m.group(3) != null) s += Long.parseLong(m.group(3)) * 86400;
        if (m.group(4) != null) s += Long.parseLong(m.group(4)) * 3600;
        if (m.group(5) != null) s += Long.parseLong(m.group(5)) * 60;
        if (m.group(6) != null) s += Long.parseLong(m.group(6));
        return "-".equals(m.group(1)) ? -s * 1000 : s * 1000;
    }

    /** A VALARM before the start as {minutes, method}; others (absolute, after) are not kept. */
    static int[] reminder(ICal.Component alarm) {
        ICal.Property trigger = alarm.get("TRIGGER");
        if (trigger == null || "DATE-TIME".equalsIgnoreCase(trigger.param("VALUE")) || "END".equalsIgnoreCase(trigger.param("RELATED"))) return null;
        Long ms = durationMillis(trigger.value);
        if (ms == null || ms > 0) return null;
        int minutes = (int) Duration.ofMillis(-ms).toMinutes();
        String action = upper(alarm.text("ACTION"));
        return new int[]{minutes, "EMAIL".equals(action) ? METHOD_EMAIL : METHOD_ALERT};
    }
}
