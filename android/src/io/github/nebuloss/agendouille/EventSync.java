package io.github.nebuloss.agendouille;

import android.accounts.Account;
import android.content.ContentProviderClient;
import android.content.ContentProviderOperation;
import android.content.ContentValues;
import android.database.Cursor;
import android.net.Uri;
import android.provider.CalendarContract.Calendars;
import android.provider.CalendarContract.Events;
import android.provider.CalendarContract.Reminders;
import android.util.Log;

import java.io.IOException;
import java.time.ZoneId;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * The events of one calendar, from the server to the phone (PLAN.md 7.3):
 * what changed since the last sync token (sync-collection), the changed
 * objects fetched (calendar-multiget) and written as Events rows, removed
 * ones deleted. An expired token starts again from nothing. An event the
 * phone changed and has not uploaded yet (DIRTY) is left alone: the upload
 * comes first.
 */
final class EventSync {
    private static final int BATCH = 50;

    private final Account account;
    private final ContentProviderClient provider;
    private final Dav dav;
    private final ZoneId zone;

    EventSync(Account account, ContentProviderClient provider, Dav dav, ZoneId zone) {
        this.account = account;
        this.provider = provider;
        this.dav = dav;
        this.zone = zone;
    }

    private Uri events() {
        return CalendarSync.asSyncAdapter(Events.CONTENT_URI, account);
    }

    /** Syncs one calendar; returns how many objects changed on the phone. */
    int sync(long calendarId, String url, String token) throws Exception {
        Dav.Changes changes;
        boolean full = token == null || token.isEmpty();
        try {
            changes = dav.changes(url, token);
        } catch (Dav.TokenExpired e) {
            full = true;
            changes = dav.changes(url, null);
        }
        Map<String, String[]> local = local(calendarId); // href -> {etag, dirty}
        List<String> fetch = new ArrayList<>();
        Set<String> seen = new HashSet<>();
        int changed = 0;
        for (Dav.Member m : changes.members) {
            seen.add(m.href);
            String[] have = local.get(m.href);
            if (m.gone) {
                if (have != null && !"1".equals(have[1])) {
                    delete(calendarId, m.href);
                    changed++;
                }
            } else if (have == null || m.etag == null || !m.etag.equals(have[0])) {
                if (have == null || !"1".equals(have[1])) fetch.add(m.href);
            }
        }
        if (full) {
            // Everything the server has was listed: what it did not list is gone.
            for (Map.Entry<String, String[]> e : local.entrySet()) {
                if (!seen.contains(e.getKey()) && !"1".equals(e.getValue()[1])) {
                    delete(calendarId, e.getKey());
                    changed++;
                }
            }
        }
        for (int i = 0; i < fetch.size(); i += BATCH) {
            for (Dav.Member m : dav.multiget(url, fetch.subList(i, Math.min(fetch.size(), i + BATCH)))) {
                if (m.gone || m.data == null) continue;
                try {
                    write(calendarId, m);
                    changed++;
                } catch (IllegalArgumentException | java.time.DateTimeException bad) {
                    // One object the phone cannot read does not stop the others.
                    Log.w("agendouille", "skipped " + m.href + ": " + bad.getMessage());
                }
            }
        }
        if (changes.token != null) {
            ContentValues v = new ContentValues();
            v.put(Calendars.CAL_SYNC2, changes.token);
            provider.update(android.content.ContentUris.withAppendedId(CalendarSync.asSyncAdapter(Calendars.CONTENT_URI, account), calendarId), v, null, null);
        }
        return changed;
    }

    /** The phone's objects in a calendar: href -> {etag, dirty}. */
    private Map<String, String[]> local(long calendarId) throws android.os.RemoteException {
        Map<String, String[]> out = new HashMap<>();
        try (Cursor c = provider.query(events(), new String[]{Events._SYNC_ID, Events.SYNC_DATA1, Events.DIRTY},
                Events.CALENDAR_ID + "=? AND " + Events._SYNC_ID + " IS NOT NULL", new String[]{Long.toString(calendarId)}, null)) {
            while (c != null && c.moveToNext()) out.put(c.getString(0), new String[]{c.getString(1), Integer.toString(c.getInt(2))});
        }
        return out;
    }

    private void delete(long calendarId, String href) throws android.os.RemoteException {
        provider.delete(events(), Events.CALENDAR_ID + "=? AND (" + Events._SYNC_ID + "=? OR " + Events.ORIGINAL_SYNC_ID + "=?)",
                new String[]{Long.toString(calendarId), href, href});
    }

    /** Replaces an object's rows with the server's version, in one batch. */
    private void write(long calendarId, Dav.Member m) throws Exception {
        List<EventMapper.Row> rows = EventMapper.map(m.data, m.href, m.etag, zone);
        ArrayList<ContentProviderOperation> ops = new ArrayList<>();
        ops.add(ContentProviderOperation.newDelete(events())
                .withSelection(Events.CALENDAR_ID + "=? AND (" + Events._SYNC_ID + "=? OR " + Events.ORIGINAL_SYNC_ID + "=?)",
                        new String[]{Long.toString(calendarId), m.href, m.href}).build());
        for (EventMapper.Row r : rows) {
            ContentValues v = values(r.values);
            v.put(Events.CALENDAR_ID, calendarId);
            v.put(Events.DIRTY, 0);
            int at = ops.size();
            ops.add(ContentProviderOperation.newInsert(events()).withValues(v).build());
            for (int[] rem : r.reminders) {
                ops.add(ContentProviderOperation.newInsert(CalendarSync.asSyncAdapter(Reminders.CONTENT_URI, account))
                        .withValueBackReference(Reminders.EVENT_ID, at)
                        .withValue(Reminders.MINUTES, rem[0])
                        .withValue(Reminders.METHOD, rem[1]).build());
            }
        }
        provider.applyBatch(ops);
    }

    static ContentValues values(Map<String, Object> m) {
        ContentValues v = new ContentValues();
        for (Map.Entry<String, Object> e : m.entrySet()) {
            Object o = e.getValue();
            if (o == null) v.putNull(e.getKey());
            else if (o instanceof Integer) v.put(e.getKey(), (Integer) o);
            else if (o instanceof Long) v.put(e.getKey(), (Long) o);
            else v.put(e.getKey(), o.toString());
        }
        return v;
    }
}
