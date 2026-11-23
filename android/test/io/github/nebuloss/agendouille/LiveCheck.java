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
        System.out.println("LIVE OK");
    }
}
