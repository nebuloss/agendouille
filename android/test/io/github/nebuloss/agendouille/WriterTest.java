package io.github.nebuloss.agendouille;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;

import org.junit.Test;

/** The phone's changes written back as patches of the server's text. */
public class WriterTest {
    private static final ZoneId PARIS = ZoneId.of("Europe/Paris");
    private static final ZoneId DEVICE = ZoneId.of("Europe/Paris");
    private static final long NOW = ZonedDateTime.of(2026, 11, 23, 12, 0, 0, 0, ZoneId.of("UTC")).toInstant().toEpochMilli();

    private static String read(String name) throws Exception {
        return new String(Files.readAllBytes(Paths.get("../testdata/ical/" + name)), StandardCharsets.UTF_8);
    }

    private static List<EventMapper.Row> rows(String ics) {
        return EventMapper.map(ics, "/x.ics", "\"e\"", DEVICE);
    }

    private static String write(String original, EventMapper.Row master, List<EventWriter.Occurrence> occ) {
        return EventWriter.write(original, master.values, master.reminders, occ, DEVICE, NOW);
    }

    @Test
    public void unchangedIsUntouched() throws Exception {
        String ics = read("vendor-tz.ics");
        String out = write(ics, rows(ics).get(0), Collections.emptyList());
        // The vendor TZID, the folded description and X-MOZ-GENERATION are kept as they were.
        assertTrue(out.contains("DTSTART;TZID=/mozilla.org/20050126_1/Europe/Paris:20261201T090000"));
        assertTrue(out.contains("X-MOZ-GENERATION:3"));
        assertTrue(out.contains("DTSTAMP:20261120T100000Z")); // nothing changed: not even stamped
        assertEquals(rows(ics).get(0).values.get("description"), rows(out).get(0).values.get("description"));
    }

    @Test
    public void aNewTitleChangesOnlyTheTitle() throws Exception {
        String ics = read("simple.ics");
        EventMapper.Row r = rows(ics).get(0);
        r.values.put("title", "Judo; puis piscine");
        String out = write(ics, r, Collections.emptyList());
        assertTrue(out, out.contains("SUMMARY:Judo\\; puis piscine"));
        assertTrue(out.contains("DTSTART;TZID=Europe/Paris:20261125T140000"));
        assertTrue(out.contains("DTSTAMP:20261123T120000Z"));
        assertTrue(out.contains("TRIGGER:-PT15M"));
        EventMapper.Row back = rows(out).get(0);
        assertEquals("Judo; puis piscine", back.get("title"));
        assertEquals(r.get("dtstart"), back.get("dtstart"));
    }

    @Test
    public void movedTimeAndNewAlarm() throws Exception {
        String ics = read("simple.ics");
        EventMapper.Row r = rows(ics).get(0);
        long start = ZonedDateTime.of(2026, 11, 26, 9, 0, 0, 0, PARIS).toInstant().toEpochMilli();
        r.values.put("dtstart", start);
        r.values.put("dtend", start + 3600000L);
        r.reminders.clear();
        r.reminders.add(new int[]{60, EventMapper.METHOD_ALERT});
        String out = write(ics, r, Collections.emptyList());
        assertTrue(out.contains("DTSTART;TZID=Europe/Paris:20261126T090000"));
        assertTrue(out.contains("DTEND;TZID=Europe/Paris:20261126T100000"));
        assertTrue(out.contains("TRIGGER:-PT60M"));
        assertFalse(out.contains("TRIGGER:-PT15M"));
        EventMapper.Row back = rows(out).get(0);
        assertEquals(start, back.get("dtstart"));
        assertEquals(60, back.reminders.get(0)[0]);
    }

    @Test
    public void occurrencesOfASeries() throws Exception {
        String ics = read("recurring.ics");
        List<EventMapper.Row> all = rows(ics);
        EventMapper.Row master = all.get(0);
        List<EventWriter.Occurrence> occ = new ArrayList<>();
        // The moved one moves again; a new one is cancelled on the phone.
        EventMapper.Row moved = all.get(1);
        long later = ZonedDateTime.of(2026, 11, 18, 16, 0, 0, 0, PARIS).toInstant().toEpochMilli();
        moved.values.put("dtstart", later);
        moved.values.put("dtend", later + 5400000L);
        occ.add(new EventWriter.Occurrence(moved.values, moved.reminders, false));
        Map<String, Object> cancel = new java.util.HashMap<>();
        cancel.put("originalInstanceTime", ZonedDateTime.of(2026, 12, 2, 10, 0, 0, 0, PARIS).toInstant().toEpochMilli());
        occ.add(new EventWriter.Occurrence(cancel, Collections.emptyList(), true));
        String out = write(ics, master, occ);
        assertTrue(out, out.contains("EXDATE;TZID=Europe/Paris:20261202T100000"));
        assertTrue(out.contains("EXDATE;TZID=Europe/Paris:20261111T100000")); // the server's, kept
        List<EventMapper.Row> back = rows(out);
        assertEquals(3, back.size());
        assertEquals(later, back.get(1).get("dtstart"));
        assertTrue(((String) back.get(0).get("exdate")).contains("20261202T090000Z"));
    }

    @Test
    public void aNewEvent() {
        Map<String, Object> v = new java.util.HashMap<>();
        long start = ZonedDateTime.of(2026, 12, 3, 18, 30, 0, 0, PARIS).toInstant().toEpochMilli();
        v.put("title", "Dîner, chez Mamie");
        v.put("dtstart", start);
        v.put("dtend", start + 7200000L);
        v.put("eventTimezone", "Europe/Paris");
        v.put("allDay", 0);
        v.put("eventStatus", EventMapper.STATUS_CONFIRMED);
        String out = EventWriter.write(null, v, Collections.emptyList(), Collections.emptyList(), DEVICE, NOW);
        assertTrue(out.startsWith("BEGIN:VCALENDAR\r\nVERSION:2.0\r\n"));
        assertTrue(out.contains("UID:"));
        assertTrue(out.contains("SUMMARY:Dîner\\, chez Mamie"));
        assertTrue(out.contains("DTSTART;TZID=Europe/Paris:20261203T183000"));
        EventMapper.Row back = rows(out).get(0);
        assertEquals("Dîner, chez Mamie", back.get("title"));
        assertEquals(start + 7200000L, back.get("dtend"));
    }

    @Test
    public void allDayNewEvent() {
        Map<String, Object> v = new java.util.HashMap<>();
        long day = java.time.LocalDate.of(2026, 12, 24).atStartOfDay(java.time.ZoneOffset.UTC).toInstant().toEpochMilli();
        v.put("title", "Réveillon");
        v.put("dtstart", day);
        v.put("dtend", day + 86400000L);
        v.put("eventTimezone", "UTC");
        v.put("allDay", 1);
        String out = EventWriter.write(null, v, Collections.emptyList(), Collections.emptyList(), DEVICE, NOW);
        assertTrue(out, out.contains("DTSTART;VALUE=DATE:20261224"));
        assertTrue(out.contains("DTEND;VALUE=DATE:20261225"));
    }

    @Test
    public void foldsLongLinesOnCharacters() {
        StringBuilder b = new StringBuilder();
        ICal.fold("DESCRIPTION:" + "é".repeat(80), b);
        for (String line : b.toString().split("\r\n")) assertTrue(line.getBytes(StandardCharsets.UTF_8).length <= 75);
        assertEquals("DESCRIPTION:" + "é".repeat(80), String.join("", ICal.unfold(b.toString())));
    }
}
