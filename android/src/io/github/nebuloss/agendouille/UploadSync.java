package io.github.nebuloss.agendouille;

import android.accounts.Account;
import android.content.ContentProviderClient;
import android.content.ContentUris;
import android.content.ContentValues;
import android.database.Cursor;
import android.net.Uri;
import android.provider.CalendarContract.Events;
import android.provider.CalendarContract.Reminders;
import android.util.Log;

import java.io.IOException;
import java.net.URI;
import java.time.ZoneId;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

/**
 * The phone's own changes to one calendar, sent to the server (PLAN.md
 * 7.3) before the download: a new event is a new object (If-None-Match),
 * a changed one the server's text patched (If-Match its ETag), a deleted
 * one a DELETE. On 412 the server's version wins: the change is dropped and
 * the download brings the server's back (a merge of the fields the two
 * did not both touch is for later).
 */
final class UploadSync {
    /** One PUT or DELETE: the app's is RawHttp with the device's credential. */
    interface Sender {
        Http.Answer send(String method, String url, String body, String ifMatch, String ifNoneMatch) throws IOException;
    }

    private static final String[] COLUMNS = {
            Events._ID, Events._SYNC_ID, Events.SYNC_DATA1, Events.SYNC_DATA2, Events.DIRTY, Events.DELETED,
            Events.ORIGINAL_ID, Events.ORIGINAL_SYNC_ID, Events.ORIGINAL_INSTANCE_TIME,
            Events.TITLE, Events.EVENT_LOCATION, Events.DESCRIPTION, Events.DTSTART, Events.DTEND, Events.DURATION,
            Events.ALL_DAY, Events.EVENT_TIMEZONE, Events.RRULE, Events.EXDATE, Events.STATUS, Events.AVAILABILITY, Events.UID_2445,
    };

    private final Account account;
    private final ContentProviderClient provider;
    private final Sender sender;
    private final ZoneId zone;
    int conflicts;

    UploadSync(Account account, ContentProviderClient provider, Sender sender, ZoneId zone) {
        this.account = account;
        this.provider = provider;
        this.sender = sender;
        this.zone = zone;
    }

    private Uri events() {
        return CalendarSync.asSyncAdapter(Events.CONTENT_URI, account);
    }

    /** Uploads what the phone changed in one calendar; returns how many objects. */
    int sync(long calendarId, String calendarUrl) throws Exception {
        Set<Long> masters = new LinkedHashSet<>();
        for (Map<String, Object> r : query(Events.CALENDAR_ID + "=? AND (" + Events.DIRTY + "=1 OR " + Events.DELETED + "=1)",
                new String[]{Long.toString(calendarId)})) {
            Long master = masterOf(calendarId, r);
            if (master != null) masters.add(master);
        }
        int done = 0;
        for (long id : masters) {
            if (upload(calendarId, calendarUrl, id)) done++;
        }
        return done;
    }

    /** The series an exception belongs to, or the row itself. */
    private Long masterOf(long calendarId, Map<String, Object> r) throws Exception {
        Object origId = r.get(Events.ORIGINAL_ID);
        Object origSync = r.get(Events.ORIGINAL_SYNC_ID);
        if (origId == null && origSync == null) return (Long) r.get(Events._ID);
        if (origId != null) return ((Number) origId).longValue();
        List<Map<String, Object>> m = query(Events.CALENDAR_ID + "=? AND " + Events._SYNC_ID + "=?", new String[]{Long.toString(calendarId), origSync.toString()});
        return m.isEmpty() ? null : (Long) m.get(0).get(Events._ID);
    }

    private boolean upload(long calendarId, String calendarUrl, long id) throws Exception {
        List<Map<String, Object>> found = query(Events._ID + "=?", new String[]{Long.toString(id)});
        if (found.isEmpty()) return false;
        Map<String, Object> master = found.get(0);
        String href = (String) master.get(Events._SYNC_ID);
        String etag = (String) master.get(Events.SYNC_DATA1);
        List<Map<String, Object>> exceptions = query(Events.ORIGINAL_ID + "=?" + (href != null ? " OR " + Events.ORIGINAL_SYNC_ID + "=?" : ""),
                href != null ? new String[]{Long.toString(id), href} : new String[]{Long.toString(id)});

        if (num(master.get(Events.DELETED)) == 1) {
            if (href != null) {
                Http.Answer a = sender.send("DELETE", absolute(calendarUrl, href), null, etag != null && !etag.isEmpty() ? etag : null, null);
                if (a.status == 412) conflicts++;
                else if (a.status / 100 != 2 && a.status != 404) throw new IOException("DELETE " + href + ": HTTP " + a.status);
            }
            purge(id, exceptions);
            return true;
        }

        List<EventWriter.Occurrence> occ = new ArrayList<>();
        for (Map<String, Object> e : exceptions) {
            boolean cancelled = num(e.get(Events.DELETED)) == 1 || num(e.get(Events.STATUS)) == Events.STATUS_CANCELED;
            occ.add(new EventWriter.Occurrence(e, reminders((Long) e.get(Events._ID)), cancelled));
        }
        String original = (String) master.get(Events.SYNC_DATA2);
        String ics = EventWriter.write(original, master, reminders(id), occ, zone, System.currentTimeMillis());
        boolean fresh = href == null;
        if (fresh) href = URI.create(calendarUrl).getRawPath() + UUID.randomUUID() + ".ics";
        Http.Answer a = sender.send("PUT", absolute(calendarUrl, href), ics, fresh ? null : (etag != null && !etag.isEmpty() ? etag : null), fresh ? "*" : null);
        if (a.status == 412 || a.status == 409 || a.status == 403) {
            // The server's version wins: forget the change, the download restores it.
            conflicts++;
            Log.w("agendouille", "upload of " + href + " refused (" + a.status + "): the server's version is kept");
            ContentValues v = new ContentValues();
            v.put(Events.DIRTY, 0);
            v.put(Events.SYNC_DATA1, "");
            provider.update(ContentUris.withAppendedId(events(), id), v, null, null);
            for (Map<String, Object> e : exceptions) clean((Long) e.get(Events._ID), href);
            return false;
        }
        if (a.status / 100 != 2) throw new IOException("PUT " + href + ": HTTP " + a.status);
        ContentValues v = new ContentValues();
        v.put(Events._SYNC_ID, href);
        v.put(Events.SYNC_DATA1, a.etag); // null when the server sent none: the download fetches it
        v.put(Events.SYNC_DATA2, ics);
        v.put(Events.DIRTY, 0);
        v.put(Events.UID_2445, ICal.parse(ics).children("VEVENT").get(0).text("UID"));
        provider.update(ContentUris.withAppendedId(events(), id), v, null, null);
        for (Map<String, Object> e : exceptions) {
            if (num(e.get(Events.DELETED)) == 1) provider.delete(ContentUris.withAppendedId(events(), (Long) e.get(Events._ID)), null, null);
            else clean((Long) e.get(Events._ID), href);
        }
        return true;
    }

    private void clean(long id, String href) throws Exception {
        ContentValues v = new ContentValues();
        v.put(Events.DIRTY, 0);
        v.put(Events.ORIGINAL_SYNC_ID, href);
        provider.update(ContentUris.withAppendedId(events(), id), v, null, null);
    }

    private void purge(long id, List<Map<String, Object>> exceptions) throws Exception {
        for (Map<String, Object> e : exceptions) provider.delete(ContentUris.withAppendedId(events(), (Long) e.get(Events._ID)), null, null);
        provider.delete(ContentUris.withAppendedId(events(), id), null, null);
    }

    static String absolute(String calendarUrl, String href) {
        return URI.create(calendarUrl).resolve(href).toString();
    }

    private List<int[]> reminders(long eventId) throws Exception {
        List<int[]> out = new ArrayList<>();
        try (Cursor c = provider.query(CalendarSync.asSyncAdapter(Reminders.CONTENT_URI, account), new String[]{Reminders.MINUTES, Reminders.METHOD},
                Reminders.EVENT_ID + "=?", new String[]{Long.toString(eventId)}, null)) {
            while (c != null && c.moveToNext()) {
                int minutes = c.getInt(0);
                out.add(new int[]{minutes < 0 ? 15 : minutes, c.getInt(1) == Reminders.METHOD_EMAIL ? EventMapper.METHOD_EMAIL : EventMapper.METHOD_ALERT});
            }
        }
        return out;
    }

    private List<Map<String, Object>> query(String where, String[] args) throws Exception {
        List<Map<String, Object>> out = new ArrayList<>();
        try (Cursor c = provider.query(events(), COLUMNS, where, args, null)) {
            while (c != null && c.moveToNext()) {
                Map<String, Object> m = new HashMap<>();
                for (int i = 0; i < COLUMNS.length; i++) {
                    switch (c.getType(i)) {
                        case Cursor.FIELD_TYPE_NULL: m.put(COLUMNS[i], null); break;
                        case Cursor.FIELD_TYPE_INTEGER: m.put(COLUMNS[i], c.getLong(i)); break;
                        default: m.put(COLUMNS[i], c.getString(i));
                    }
                }
                // Integer-valued columns the writer reads as numbers.
                for (String k : new String[]{Events.ALL_DAY, Events.STATUS, Events.AVAILABILITY}) {
                    if (m.get(k) instanceof Long) m.put(k, ((Long) m.get(k)).intValue());
                }
                out.add(m);
            }
        }
        return out;
    }

    private static int num(Object o) {
        return o instanceof Number ? ((Number) o).intValue() : 0;
    }
}
