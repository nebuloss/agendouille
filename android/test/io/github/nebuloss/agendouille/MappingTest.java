package io.github.nebuloss.agendouille;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.util.List;

import org.junit.Test;

/** iCalendar to the phone's event rows, over the shared fixtures (testdata/ical). */
public class MappingTest {
    private static final ZoneId PARIS = ZoneId.of("Europe/Paris");
    private static final ZoneId DEVICE = ZoneId.of("America/New_York");

    private static List<EventMapper.Row> map(String name) throws Exception {
        String ics = new String(Files.readAllBytes(Paths.get("../testdata/ical/" + name)), StandardCharsets.UTF_8);
        return EventMapper.map(ics, "/dav/calendars/lea/lea~perso/" + name, "\"etag-1\"", DEVICE);
    }

    private static long paris(int y, int m, int d, int h, int mi) {
        return ZonedDateTime.of(y, m, d, h, mi, 0, 0, PARIS).toInstant().toEpochMilli();
    }

    @Test
    public void simple() throws Exception {
        List<EventMapper.Row> rows = map("simple.ics");
        assertEquals(1, rows.size());
        EventMapper.Row r = rows.get(0);
        assertEquals("Judo, puis goûter", r.get("title"));
        assertEquals("Apporter le kimono\net une gourde", r.get("description"));
        assertEquals("Gymnase", r.get("eventLocation"));
        assertEquals(paris(2026, 11, 25, 14, 0), r.get("dtstart"));
        assertEquals(paris(2026, 11, 25, 15, 30), r.get("dtend"));
        assertEquals("Europe/Paris", r.get("eventTimezone"));
        assertEquals(0, r.get("allDay"));
        assertEquals(EventMapper.STATUS_CONFIRMED, r.get("eventStatus"));
        assertEquals("/dav/calendars/lea/lea~perso/simple.ics", r.get("_sync_id"));
        assertEquals("\"etag-1\"", r.get("sync_data1"));
        assertTrue(((String) r.get("sync_data2")).contains("UID:simple-1@agendouille"));
        assertEquals("simple-1@agendouille", r.get("uid2445"));
        assertEquals(1, r.reminders.size());
        assertEquals(15, r.reminders.get(0)[0]);
        assertEquals(EventMapper.METHOD_ALERT, r.reminders.get(0)[1]);
        assertEquals(1, r.get("hasAlarm"));
    }

    @Test
    public void allDay() throws Exception {
        EventMapper.Row r = map("allday.ics").get(0);
        assertEquals(1, r.get("allDay"));
        assertEquals("UTC", r.get("eventTimezone"));
        assertEquals(java.time.LocalDate.of(2026, 11, 27).atStartOfDay(java.time.ZoneOffset.UTC).toInstant().toEpochMilli(), r.get("dtstart"));
        assertEquals(java.time.LocalDate.of(2026, 11, 29).atStartOfDay(java.time.ZoneOffset.UTC).toInstant().toEpochMilli(), r.get("dtend"));
    }

    @Test
    public void recurringWithExceptions() throws Exception {
        List<EventMapper.Row> rows = map("recurring.ics");
        assertEquals(3, rows.size());
        EventMapper.Row master = rows.get(0);
        assertFalse(master.exception);
        assertEquals("FREQ=WEEKLY;BYDAY=WE;COUNT=10", master.get("rrule"));
        assertEquals("PT5400S", master.get("duration"));
        assertNull(master.get("dtend"));
        // 10:00 in Paris on 11 November is 09:00 UTC.
        assertEquals("20261111T090000Z", master.get("exdate"));
        EventMapper.Row moved = rows.get(1);
        assertTrue(moved.exception);
        assertEquals("/dav/calendars/lea/lea~perso/recurring.ics", moved.get("original_sync_id"));
        assertEquals(paris(2026, 11, 18, 10, 0), moved.get("originalInstanceTime"));
        assertEquals(paris(2026, 11, 18, 14, 0), moved.get("dtstart"));
        assertEquals(paris(2026, 11, 18, 15, 30), moved.get("dtend"));
        assertEquals("Piscine (l’après-midi)", moved.get("title"));
        assertNull(moved.get("_sync_id"));
        EventMapper.Row cancelled = rows.get(2);
        assertEquals(EventMapper.STATUS_CANCELED, cancelled.get("eventStatus"));
        assertEquals(paris(2026, 11, 25, 10, 0), cancelled.get("originalInstanceTime"));
    }

    @Test
    public void utcDurationFreePrivate() throws Exception {
        EventMapper.Row r = map("utc-duration.ics").get(0);
        long start = java.time.Instant.parse("2026-11-26T17:00:00Z").toEpochMilli();
        assertEquals(start, r.get("dtstart"));
        assertEquals(start + 90 * 60000L, r.get("dtend"));
        assertEquals("UTC", r.get("eventTimezone"));
        assertEquals(EventMapper.AVAILABILITY_FREE, r.get("availability"));
        assertEquals(2, r.get("accessLevel"));
        // The e-mail alarm a day before is kept; the absolute one is not.
        assertEquals(1, r.reminders.size());
        assertEquals(1440, r.reminders.get(0)[0]);
        assertEquals(EventMapper.METHOD_EMAIL, r.reminders.get(0)[1]);
    }

    @Test
    public void vendorZoneAndFolding() throws Exception {
        EventMapper.Row r = map("vendor-tz.ics").get(0);
        assertEquals("Europe/Paris", r.get("eventTimezone"));
        assertEquals(paris(2026, 12, 1, 9, 0), r.get("dtstart"));
        assertTrue(((String) r.get("description")).contains("repliée sur plusieurs lignes par le client"));
        assertTrue(((String) r.get("description")).endsWith("au-delà de soixante-quinze octets."));
    }

    @Test
    public void floatingTimesAreTheDevicesZone() {
        String ics = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:f\r\nDTSTART:20261125T080000\r\nSUMMARY:x\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n";
        EventMapper.Row r = EventMapper.map(ics, "/x.ics", null, DEVICE).get(0);
        assertEquals(ZonedDateTime.of(2026, 11, 25, 8, 0, 0, 0, DEVICE).toInstant().toEpochMilli(), r.get("dtstart"));
        assertEquals(r.get("dtstart"), r.get("dtend"));
    }

    @Test(expected = IllegalArgumentException.class)
    public void notICalendar() {
        EventMapper.map("<html>nope</html>", "/x.ics", null, DEVICE);
    }
}
