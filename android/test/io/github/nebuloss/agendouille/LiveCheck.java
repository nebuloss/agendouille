package io.github.nebuloss.agendouille;

import java.nio.charset.StandardCharsets;
import java.time.ZoneId;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * The app's sync code against a real agendouille server, on the JVM (no
 * phone, no emulator): discovery, sync-collection, calendar-multiget and
 * the mapping of what the server really sends. Everything but the
 * phone's calendar database.
 *
 *   java ... LiveCheck <server> <user> <device password>
 */
public final class LiveCheck {
    public static void main(String[] args) throws Exception {
        String server = args[0], user = args[1], pass = args[2];
        String auth = "Basic " + Base64.getEncoder().encodeToString((user + ":" + pass).getBytes(StandardCharsets.UTF_8));
        Dav dav = new Dav((method, url, depth, type, body) -> {
            Map<String, String> h = new LinkedHashMap<>();
            h.put("Authorization", auth);
            h.put("User-Agent", "agendouille-android/live-check");
            if (depth != null) h.put("Depth", depth);
            if (body != null) h.put("Content-Type", type);
            RawHttp.Answer a = RawHttp.request(null, method, url, h, body == null ? null : body.getBytes(StandardCharsets.UTF_8));
            return new Dav.Answer(a.status, a.text(), a.headers.get("etag"));
        }, server);
        List<Dav.Collection> cals = dav.calendars();
        System.out.println("calendars: " + cals.size());
        int objects = 0, rows = 0, exceptions = 0, reminders = 0;
        for (Dav.Collection c : cals) {
            Dav.Changes ch = dav.changes(c.url, null);
            List<String> hrefs = new ArrayList<>();
            for (Dav.Member m : ch.members) if (!m.gone) hrefs.add(m.href);
            System.out.println("  " + c.name + " " + c.color + (c.readOnly ? " (read-only)" : "") + ": " + hrefs.size() + " objects, token " + (ch.token != null));
            if (ch.token == null) throw new AssertionError("no sync token for " + c.url);
            if (!hrefs.isEmpty()) {
                for (Dav.Member m : dav.multiget(c.url, hrefs)) {
                    if (m.data == null) throw new AssertionError("no data for " + m.href);
                    for (EventMapper.Row r : EventMapper.map(m.data, m.href, m.etag, ZoneId.of("Europe/Paris"))) {
                        rows++;
                        if (r.exception) exceptions++;
                        reminders += r.reminders.size();
                        System.out.println("    " + r.get("title") + " start=" + r.get("dtstart") + " tz=" + r.get("eventTimezone")
                                + (r.get("rrule") != null ? " rrule=" + r.get("rrule") : "") + (r.exception ? " (exception)" : ""));
                    }
                    objects++;
                }
            }
            // A second sync from the token lists nothing new.
            Dav.Changes again = dav.changes(c.url, ch.token);
            if (!again.members.isEmpty()) throw new AssertionError("changes right after a sync: " + again.members.size());
        }
        System.out.println("objects " + objects + ", rows " + rows + ", exceptions " + exceptions + ", reminders " + reminders);
        if (objects == 0) throw new AssertionError("no objects at all");

        // The phone's changes, as UploadSync sends them: a new event, a
        // patch with If-Match, a stale ETag refused, then a delete.
        Dav.Collection mine = null;
        for (Dav.Collection c : cals) if (!c.readOnly) mine = c;
        String token = dav.changes(mine.url, null).token;
        java.util.Map<String, Object> v = new java.util.HashMap<>();
        long start = java.time.ZonedDateTime.of(2026, 12, 3, 18, 30, 0, 0, ZoneId.of("Europe/Paris")).toInstant().toEpochMilli();
        v.put("title", "Dîner chez Mamie");
        v.put("dtstart", start);
        v.put("dtend", start + 7200000L);
        v.put("eventTimezone", "Europe/Paris");
        v.put("allDay", 0);
        String ics = EventWriter.write(null, v, java.util.Collections.singletonList(new int[]{30, 1}), java.util.Collections.emptyList(), ZoneId.of("Europe/Paris"), System.currentTimeMillis());
        String url = java.net.URI.create(mine.url).resolve("live-" + System.nanoTime() + ".ics").toString();
        RawHttp.Answer put = object(auth, "PUT", url, ics, null, "*");
        System.out.println("PUT new: " + put.status + " etag " + put.headers.get("etag"));
        if (put.status != 201) throw new AssertionError("PUT new: " + put.status + " " + put.text());
        if (object(auth, "PUT", url, ics, null, "*").status != 412) throw new AssertionError("a second If-None-Match: * was accepted");
        Dav.Changes after = dav.changes(mine.url, token);
        if (after.members.size() != 1) throw new AssertionError("the new event is not in the changes: " + after.members.size());
        String etag = after.members.get(0).etag;
        String stored = dav.multiget(mine.url, java.util.Collections.singletonList(after.members.get(0).href)).get(0).data;
        EventMapper.Row row = EventMapper.map(stored, url, etag, ZoneId.of("Europe/Paris")).get(0);
        if (!"Dîner chez Mamie".equals(row.get("title")) || row.reminders.size() != 1) throw new AssertionError("stored: " + row.values);
        row.values.put("title", "Dîner chez Mamie et Papi");
        String patched = EventWriter.write(stored, row.values, row.reminders, java.util.Collections.emptyList(), ZoneId.of("Europe/Paris"), System.currentTimeMillis());
        RawHttp.Answer upd = object(auth, "PUT", url, patched, etag, null);
        System.out.println("PUT patch: " + upd.status);
        if (upd.status / 100 != 2) throw new AssertionError("PUT patch: " + upd.status + " " + upd.text());
        RawHttp.Answer stale = object(auth, "PUT", url, patched, etag, null);
        System.out.println("PUT with the old ETag: " + stale.status);
        if (stale.status != 412) throw new AssertionError("a stale ETag was accepted: " + stale.status);
        String now = dav.multiget(mine.url, java.util.Collections.singletonList(after.members.get(0).href)).get(0).data;
        if (!now.contains("SUMMARY:Dîner chez Mamie et Papi")) throw new AssertionError("not patched: " + now);
        RawHttp.Answer del = object(auth, "DELETE", url, null, null, null);
        System.out.println("DELETE: " + del.status);
        if (del.status / 100 != 2) throw new AssertionError("DELETE: " + del.status);
        System.out.println("LIVE OK");
    }

    private static RawHttp.Answer object(String auth, String method, String url, String ics, String ifMatch, String ifNoneMatch) throws Exception {
        Map<String, String> h = new LinkedHashMap<>();
        h.put("Authorization", auth);
        h.put("User-Agent", "agendouille-android/live-check");
        if (ifMatch != null) h.put("If-Match", ifMatch);
        if (ifNoneMatch != null) h.put("If-None-Match", ifNoneMatch);
        if (ics != null) h.put("Content-Type", "text/calendar; charset=utf-8");
        return RawHttp.request(null, method, url, h, ics == null ? null : ics.getBytes(StandardCharsets.UTF_8));
    }
}
