package io.github.nebuloss.agendouille;

import android.Manifest;
import android.app.job.JobInfo;
import android.app.job.JobParameters;
import android.app.job.JobScheduler;
import android.app.job.JobService;
import android.content.BroadcastReceiver;
import android.content.ComponentName;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.os.Bundle;
import android.provider.CalendarContract;
import android.provider.CalendarContract.Events;
import android.util.Log;

/**
 * An event created, changed or deleted on the phone (in any calendar app)
 * goes to the server within seconds, the app open or not: Android runs this
 * job when the calendar provider changes, and it asks for a sync when one
 * of the account's events waits to be uploaded (DIRTY or DELETED). The
 * sync's own writes clear those flags, so it never starts itself again.
 * A content trigger fires once: the job schedules itself anew each time,
 * and the app, each sync and a reboot schedule it too.
 */
public final class ChangeJob extends JobService {
    private static final int ID = 0x616764; // "agd"

    /** Watches the phone's calendars, when there is an account to sync. */
    static void schedule(Context ctx) {
        if (Credentials.load(ctx) == null) return;
        JobScheduler js = ctx.getSystemService(JobScheduler.class);
        if (js == null) return;
        JobInfo job = new JobInfo.Builder(ID, new ComponentName(ctx, ChangeJob.class))
                .addTriggerContentUri(new JobInfo.TriggerContentUri(CalendarContract.CONTENT_URI, JobInfo.TriggerContentUri.FLAG_NOTIFY_FOR_DESCENDANTS))
                .setTriggerContentUpdateDelay(1_000) // a burst of changes, one sync
                .setTriggerContentMaxDelay(5_000)
                .build();
        try {
            js.schedule(job);
        } catch (RuntimeException e) {
            Log.w("agendouille", "scheduling the change job", e);
        }
    }

    @Override
    public boolean onStartJob(JobParameters params) {
        new Thread(() -> {
            try {
                String pending = waiting(this);
                if (pending != null && fresh(this, pending)) upload(this);
            } catch (RuntimeException e) {
                Log.w("agendouille", "change job", e);
            } finally {
                schedule(this);
                jobFinished(params, false);
            }
        }, "agendouille-change").start();
        return true;
    }

    @Override
    public boolean onStopJob(JobParameters params) {
        return false; // schedule() in onStartJob's thread puts it back
    }

    /**
     * The events of the account waiting to go to the server, as a signature
     * (their ids); null when none. Only those the upload handles: in a
     * calendar the phone syncs and may write to. A change in a read-only or
     * unsynced calendar stays on the phone, and must not wake a sync: the
     * sync's own writes wake this job, and it looped, draining the battery.
     */
    private static String waiting(Context ctx) {
        Credentials c = Credentials.load(ctx);
        if (c == null || ctx.checkSelfPermission(Manifest.permission.READ_CALENDAR) != PackageManager.PERMISSION_GRANTED) return null;
        StringBuilder calendars = new StringBuilder();
        try (Cursor q = ctx.getContentResolver().query(CalendarSync.asSyncAdapter(CalendarContract.Calendars.CONTENT_URI, c.account),
                new String[]{CalendarContract.Calendars._ID},
                CalendarContract.Calendars.ACCOUNT_NAME + "=? AND " + CalendarContract.Calendars.ACCOUNT_TYPE + "=? AND "
                        + CalendarContract.Calendars.SYNC_EVENTS + "=1 AND " + CalendarContract.Calendars.CALENDAR_ACCESS_LEVEL + ">="
                        + CalendarContract.Calendars.CAL_ACCESS_CONTRIBUTOR,
                new String[]{c.account.name, c.account.type}, null)) {
            while (q != null && q.moveToNext()) calendars.append(calendars.length() == 0 ? "" : ",").append(q.getLong(0));
        }
        if (calendars.length() == 0) return null;
        StringBuilder ids = new StringBuilder();
        try (Cursor q = ctx.getContentResolver().query(CalendarSync.asSyncAdapter(Events.CONTENT_URI, c.account), new String[]{Events._ID},
                Events.ACCOUNT_NAME + "=? AND " + Events.ACCOUNT_TYPE + "=? AND " + Events.CALENDAR_ID + " IN (" + calendars + ") AND ("
                        + Events.DIRTY + "=1 OR " + Events.DELETED + "=1)",
                new String[]{c.account.name, c.account.type}, Events._ID)) {
            while (q != null && q.moveToNext()) ids.append(q.getLong(0)).append(' ');
        }
        return ids.length() == 0 ? null : ids.toString();
    }

    /**
     * Whether these waiting events are new since the last sync this job
     * asked for. The same ones still there after it means the upload cannot
     * send them (the server refuses one, say): not again before half an
     * hour, the periodic sync tries meanwhile.
     */
    private static boolean fresh(Context ctx, String pending) {
        android.content.SharedPreferences p = ctx.getSharedPreferences("change_job", Context.MODE_PRIVATE);
        long now = System.currentTimeMillis();
        if (pending.equals(p.getString("pending", null)) && now - p.getLong("asked", 0) < 30 * 60_000L) return false;
        p.edit().putString("pending", pending).putLong("asked", now).apply();
        return true;
    }

    /** Asks Android for a sync now: the phone's changes first, then the server's. */
    static void upload(Context ctx) {
        Credentials c = Credentials.load(ctx);
        if (c == null) return;
        Bundle b = new Bundle();
        b.putBoolean(ContentResolver.SYNC_EXTRAS_MANUAL, true);
        b.putBoolean(ContentResolver.SYNC_EXTRAS_EXPEDITED, true);
        ContentResolver.requestSync(c.account, CalendarContract.AUTHORITY, b);
    }

    /** After a reboot or an update of the app: the job again (content triggers are not kept). */
    public static final class Boot extends BroadcastReceiver {
        @Override
        public void onReceive(Context ctx, Intent intent) {
            schedule(ctx);
            PushService.update(ctx); // instant sync, when on
        }
    }
}
