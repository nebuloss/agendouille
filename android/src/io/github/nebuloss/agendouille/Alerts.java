package io.github.nebuloss.agendouille;

import android.Manifest;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.os.PowerManager;

/**
 * What the person must know when the app is not open, in its own
 * notification category « Alertes » (instant sync's notification is
 * another): sign in again, sync failing while the phone is online, a
 * permission the mode needs. Each is one notification, replaced, and gone
 * once things are right again. Without the notifications permission
 * (Android 13 on) Android shows none of them.
 */
final class Alerts {
    private Alerts() {}

    static final String CHANNEL = "alerts";
    private static final int LOGIN = 11, SYNC = 12, PERMISSIONS = 13, UPDATE = 14;
    /** Failures in a row, online, before saying so: one is often a blip. */
    private static final int FAILURES = 2;

    private static SharedPreferences prefs(Context ctx) {
        return ctx.getSharedPreferences("alerts", Context.MODE_PRIVATE);
    }

    private static void show(Context ctx, int id, String title, String text, String open) {
        NotificationManager nm = ctx.getSystemService(NotificationManager.class);
        NotificationChannel ch = new NotificationChannel(CHANNEL, ctx.getString(R.string.alerts_channel), NotificationManager.IMPORTANCE_DEFAULT);
        ch.setDescription(ctx.getString(R.string.alerts_channel_help));
        nm.createNotificationChannel(ch);
        Intent tap = new Intent(ctx, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        if (open != null) tap.putExtra("open", open);
        Notification n = new Notification.Builder(ctx, CHANNEL)
                // A problem shows a warning; an update, the download arrow.
                .setSmallIcon(id == UPDATE ? android.R.drawable.stat_sys_download_done : android.R.drawable.stat_notify_error)
                .setContentTitle(title)
                .setContentText(text)
                .setStyle(new Notification.BigTextStyle().bigText(text))
                .setContentIntent(PendingIntent.getActivity(ctx, id, tap, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT))
                .setAutoCancel(true)
                .build();
        try {
            nm.notify(id, n);
        } catch (SecurityException ignored) {
            // no permission to notify: the app shows it when opened
        }
    }

    private static void cancel(Context ctx, int id) {
        ctx.getSystemService(NotificationManager.class).cancel(id);
    }

    /** The server refused the device: it must be signed in again. */
    static void login(Context ctx) {
        show(ctx, LOGIN, ctx.getString(R.string.alert_login_title), ctx.getString(R.string.alert_login_text), null);
    }

    /** A sync that worked: no failure to tell any more. */
    static void synced(Context ctx) {
        prefs(ctx).edit().putInt("failures", 0).apply();
        cancel(ctx, SYNC);
        cancel(ctx, LOGIN);
    }

    /** A sync that failed: told when the phone is online and it failed twice in a row. */
    static void failed(Context ctx, String why) {
        if (!online(ctx)) return; // offline: nothing to tell, it will sync later
        int n = prefs(ctx).getInt("failures", 0) + 1;
        prefs(ctx).edit().putInt("failures", n).apply();
        if (n >= FAILURES) show(ctx, SYNC, ctx.getString(R.string.alert_sync_title), ctx.getString(R.string.alert_sync_text, why), null);
    }

    /**
     * What the mode needs and lacks: the calendar, and for instant sync the
     * battery exemption. Told once for each new lack; gone when all is there.
     */
    static void permissions(Context ctx) {
        StringBuilder missing = new StringBuilder();
        if (ctx.checkSelfPermission(Manifest.permission.WRITE_CALENDAR) != PackageManager.PERMISSION_GRANTED) {
            missing.append(ctx.getString(R.string.alert_perm_calendar));
        }
        if (UnifiedPush.prefs(ctx).getBoolean("instant", false)) {
            PowerManager pm = ctx.getSystemService(PowerManager.class);
            if (pm != null && !pm.isIgnoringBatteryOptimizations(ctx.getPackageName())) {
                if (missing.length() > 0) missing.append(", ");
                missing.append(ctx.getString(R.string.alert_perm_battery));
            }
        }
        String now = missing.toString();
        SharedPreferences p = prefs(ctx);
        if (now.isEmpty()) {
            p.edit().remove("permissions").apply();
            cancel(ctx, PERMISSIONS);
        } else if (!now.equals(p.getString("permissions", ""))) {
            p.edit().putString("permissions", now).apply();
            show(ctx, PERMISSIONS, ctx.getString(R.string.alert_perm_title), ctx.getString(R.string.alert_perm_text, now), "permissions");
        }
    }

    /** Whether the phone has a network that reaches the internet now. */
    static boolean online(Context ctx) {
        try {
            ConnectivityManager cm = ctx.getSystemService(ConnectivityManager.class);
            Network n = cm == null ? null : cm.getActiveNetwork();
            NetworkCapabilities c = n == null ? null : cm.getNetworkCapabilities(n);
            return c != null && c.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED);
        } catch (RuntimeException e) {
            return false;
        }
    }

    /** An update for this phone: told once per version, never one « Plus tard » was said to. */
    static void update(Context ctx, String version) {
        if (version.equals(ctx.getSharedPreferences("agendouille", Context.MODE_PRIVATE).getString("update_dismissed", ""))) return;
        if (version.equals(prefs(ctx).getString("update", ""))) return;
        prefs(ctx).edit().putString("update", version).apply();
        show(ctx, UPDATE, ctx.getString(R.string.alert_update_title), ctx.getString(R.string.alert_update_text, version), "update");
    }

    /** The app is open: its dialog offers the update, the notification is done. */
    static void updateSeen(Context ctx) {
        cancel(ctx, UPDATE);
    }

    /** A sign-out: nothing of the account stays on screen. */
    static void clear(Context ctx) {
        prefs(ctx).edit().clear().apply();
        ctx.getSystemService(NotificationManager.class).cancelAll();
    }
}
