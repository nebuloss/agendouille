/*
 * Shaped after Google's SampleSyncAdapter (Apache License 2.0,
 * Copyright (C) 2010 The Android Open Source Project): the service and the
 * adapter Android's sync manager runs.
 */
package io.github.nebuloss.agendouille;

import android.accounts.Account;
import android.app.Service;
import android.content.AbstractThreadedSyncAdapter;
import android.content.ContentProviderClient;
import android.content.Context;
import android.content.Intent;
import android.content.SyncResult;
import android.os.Bundle;
import android.os.IBinder;
import android.util.Log;

import java.util.List;
import java.util.Map;

/** The calendar sync adapter (PLAN.md 7.3): Android runs it, never the screens. */
public final class SyncService extends Service {
    private static final Object LOCK = new Object();
    private static Adapter adapter;

    @Override
    public void onCreate() {
        synchronized (LOCK) {
            if (adapter == null) adapter = new Adapter(getApplicationContext());
        }
    }

    @Override
    public IBinder onBind(Intent intent) {
        return adapter.getSyncAdapterBinder();
    }

    static final class Adapter extends AbstractThreadedSyncAdapter {
        Adapter(Context ctx) {
            super(ctx, true, false);
        }

        @Override
        public void onPerformSync(Account account, Bundle extras, String authority, ContentProviderClient provider, SyncResult result) {
            Context ctx = getContext();
            Credentials cred = Credentials.load(ctx);
            if (cred == null || !cred.account.equals(account)) return;
            SyncStatus.syncing(ctx);
            String ua = "agendouille-android/" + version(ctx) + " (Android " + android.os.Build.VERSION.RELEASE + ")";
            Dav dav = new Dav((method, url, depth, type, body) -> {
                Http.Answer a = Http.dav(method, url, cred.username, cred.secret, depth, type, body, null, ua);
                return new Dav.Answer(a.status, a.body, a.etag);
            }, cred.server);
            try {
                CalendarSync cals = new CalendarSync(account, provider);
                List<Dav.Collection> remote = dav.calendars();
                Map<String, Long> ids = cals.update(remote);
                result.stats.numEntries += ids.size();
                // The events of each calendar the person keeps on the phone.
                EventSync events = new EventSync(account, provider, dav, java.time.ZoneId.systemDefault());
                for (CalendarSync.Synced c : cals.synced()) {
                    result.stats.numUpdates += events.sync(c.id, c.url, c.token);
                }
                SyncStatus.done(ctx, cals.events());
            } catch (Dav.Unauthorized e) {
                result.stats.numAuthExceptions++;
                SyncStatus.failed(ctx, ctx.getString(R.string.sync_refused));
            } catch (java.io.IOException e) {
                Log.w("agendouille", "sync", e);
                result.stats.numIoExceptions++;
                SyncStatus.failed(ctx, ctx.getString(R.string.sync_offline));
            } catch (Exception e) {
                Log.e("agendouille", "sync", e);
                result.databaseError = true;
                SyncStatus.failed(ctx, String.valueOf(e.getMessage()));
            }
        }

        private static String version(Context ctx) {
            try {
                return ctx.getPackageManager().getPackageInfo(ctx.getPackageName(), 0).versionName;
            } catch (Exception e) {
                return "?";
            }
        }
    }
}
