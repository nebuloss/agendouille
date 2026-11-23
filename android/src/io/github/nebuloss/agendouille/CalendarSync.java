package io.github.nebuloss.agendouille;

import android.accounts.Account;
import android.content.ContentProviderClient;
import android.content.ContentValues;
import android.database.Cursor;
import android.net.Uri;
import android.os.RemoteException;
import android.provider.CalendarContract.Calendars;
import android.provider.CalendarContract.Events;

import java.io.IOException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.TimeZone;

/**
 * The calendars of the account on the phone (PLAN.md 7.3): one Android
 * calendar per CalDAV calendar of the user's home, its name, colour and
 * read-only flag the server's; gone from the server, gone from the phone.
 * Whether the phone syncs it (SYNC_EVENTS) is the person's choice, kept.
 */
final class CalendarSync {
    private final Account account;
    private final ContentProviderClient provider;

    CalendarSync(Account account, ContentProviderClient provider) {
        this.account = account;
        this.provider = provider;
    }

    static Uri asSyncAdapter(Uri uri, Account a) {
        return uri.buildUpon()
                .appendQueryParameter(android.provider.CalendarContract.CALLER_IS_SYNCADAPTER, "true")
                .appendQueryParameter(Calendars.ACCOUNT_NAME, a.name)
                .appendQueryParameter(Calendars.ACCOUNT_TYPE, a.type)
                .build();
    }

    /** The phone's calendars of the account, by their CalDAV URL. */
    Map<String, Long> local() throws RemoteException {
        Map<String, Long> out = new HashMap<>();
        try (Cursor c = provider.query(asSyncAdapter(Calendars.CONTENT_URI, account), new String[]{Calendars._ID, Calendars._SYNC_ID},
                Calendars.ACCOUNT_NAME + "=? AND " + Calendars.ACCOUNT_TYPE + "=?", new String[]{account.name, account.type}, null)) {
            while (c != null && c.moveToNext()) out.put(c.getString(1), c.getLong(0));
        }
        return out;
    }

    /** Brings the phone's calendars in line with the server's; returns the URL -> id of each. */
    Map<String, Long> update(List<Dav.Collection> remote) throws RemoteException, IOException {
        Map<String, Long> have = local();
        Map<String, Long> out = new HashMap<>();
        Uri uri = asSyncAdapter(Calendars.CONTENT_URI, account);
        for (Dav.Collection c : remote) {
            ContentValues v = new ContentValues();
            v.put(Calendars.CALENDAR_DISPLAY_NAME, c.name);
            v.put(Calendars.CALENDAR_COLOR, color(c.color));
            v.put(Calendars.CALENDAR_ACCESS_LEVEL, c.readOnly ? Calendars.CAL_ACCESS_READ : Calendars.CAL_ACCESS_OWNER);
            Long id = have.remove(c.url);
            if (id == null) {
                v.put(Calendars.ACCOUNT_NAME, account.name);
                v.put(Calendars.ACCOUNT_TYPE, account.type);
                v.put(Calendars.NAME, c.url);
                v.put(Calendars._SYNC_ID, c.url);
                v.put(Calendars.OWNER_ACCOUNT, account.name);
                v.put(Calendars.VISIBLE, 1);
                v.put(Calendars.SYNC_EVENTS, 1);
                v.put(Calendars.CALENDAR_TIME_ZONE, TimeZone.getDefault().getID());
                v.put(Calendars.ALLOWED_REMINDERS, "0,1");
                v.put(Calendars.ALLOWED_AVAILABILITY, "0,1");
                v.put(Calendars.ALLOWED_ATTENDEE_TYPES, "0,1,2");
                Uri made = provider.insert(uri, v);
                if (made == null) throw new IOException("the calendar provider refused " + c.url);
                id = Long.parseLong(made.getLastPathSegment());
            } else {
                provider.update(android.content.ContentUris.withAppendedId(uri, id), v, null, null);
            }
            out.put(c.url, id);
        }
        for (Long gone : have.values()) provider.delete(android.content.ContentUris.withAppendedId(uri, gone), null, null);
        return out;
    }

    /** A calendar the person syncs: its id, URL and last sync token. */
    static final class Synced {
        final long id;
        final String url;
        final String token;

        Synced(long id, String url, String token) {
            this.id = id;
            this.url = url;
            this.token = token;
        }
    }

    /** The calendars whose events the phone keeps (SYNC_EVENTS). */
    List<Synced> synced() throws RemoteException {
        List<Synced> out = new java.util.ArrayList<>();
        try (Cursor c = provider.query(asSyncAdapter(Calendars.CONTENT_URI, account), new String[]{Calendars._ID, Calendars._SYNC_ID, Calendars.CAL_SYNC2},
                Calendars.ACCOUNT_NAME + "=? AND " + Calendars.ACCOUNT_TYPE + "=? AND " + Calendars.SYNC_EVENTS + "=1",
                new String[]{account.name, account.type}, null)) {
            while (c != null && c.moveToNext()) out.add(new Synced(c.getLong(0), c.getString(1), c.getString(2)));
        }
        return out;
    }

    /** How many events of the account the phone holds. */
    int events() throws RemoteException {
        try (Cursor c = provider.query(asSyncAdapter(Events.CONTENT_URI, account), new String[]{Events._ID},
                Events.ACCOUNT_NAME + "=? AND " + Events.ACCOUNT_TYPE + "=? AND " + Events.DELETED + "=0", new String[]{account.name, account.type}, null)) {
            return c == null ? 0 : c.getCount();
        }
    }

    static int color(String hex) {
        if (hex != null && hex.matches("#[0-9a-f]{6}")) return 0xff000000 | Integer.parseInt(hex.substring(1), 16);
        return 0xff8f4c38; // the andouille's
    }
}
