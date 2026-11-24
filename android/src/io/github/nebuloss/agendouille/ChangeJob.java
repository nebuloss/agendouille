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
                if (waiting(this)) upload(this);
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

    /** Whether an event of the account waits to go to the server. */
    private static boolean waiting(Context ctx) {
        Credentials c = Credentials.load(ctx);
        if (c == null || ctx.checkSelfPermission(Manifest.permission.READ_CALENDAR) != PackageManager.PERMISSION_GRANTED) return false;
        try (Cursor q = ctx.getContentResolver().query(CalendarSync.asSyncAdapter(Events.CONTENT_URI, c.account), new String[]{Events._ID},
                Events.ACCOUNT_NAME + "=? AND " + Events.ACCOUNT_TYPE + "=? AND (" + Events.DIRTY + "=1 OR " + Events.DELETED + "=1)",
                new String[]{c.account.name, c.account.type}, null)) {
            return q != null && q.getCount() > 0;
        }
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
        }
    }
}
