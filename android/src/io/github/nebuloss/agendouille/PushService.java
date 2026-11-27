package io.github.nebuloss.agendouille;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ServiceInfo;
import android.net.ConnectivityManager;
import android.net.Network;
import android.os.Build;
import android.os.IBinder;
import android.util.Log;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Base64;

/**
 * Instant sync, when the person turns it on (Réglages): holds the device's
 * push stream open (Server-Sent Events from agendouille's own push service,
 * DECISIONS: push without ntfy) and hands each message on, to this app (a
 * sync) or to DAVx5 (Distributor). Android lets an app stay connected only
 * as a foreground service, which must show a notification: it is on the
 * lowest channel (no icon in the status bar), and the person can turn that
 * channel off.
 */
public final class PushService extends Service {
    static final String CHANNEL = "push";
    private static final int NOTIFICATION = 7;

    private volatile boolean running;
    private volatile HttpURLConnection current;
    private Thread loop;
    private ConnectivityManager.NetworkCallback network;
    private final Object wake = new Object();

    /** Whether the stream should be open: signed in, turned on, and the server has the service. */
    static boolean wanted(Context ctx) {
        SharedPreferences p = UnifiedPush.prefs(ctx);
        return Credentials.load(ctx) != null && p.getBoolean("instant", false) && p.getBoolean("service", false);
    }

    /** Starts it when wanted, stops it when not. */
    static void update(Context ctx) {
        Intent i = new Intent(ctx, PushService.class);
        if (!wanted(ctx)) {
            ctx.stopService(i);
            return;
        }
        try {
            ctx.startForegroundService(i);
        } catch (RuntimeException e) {
            // In the background without the battery exemption, Android refuses: the app starts it when opened.
            Log.i("agendouille", "push: the service cannot start now", e);
        }
    }

    /** The notification's category, which the person can turn off in Android's settings. */
    static String channel(Context ctx) {
        NotificationManager nm = ctx.getSystemService(NotificationManager.class);
        NotificationChannel ch = new NotificationChannel(CHANNEL, ctx.getString(R.string.push_channel), NotificationManager.IMPORTANCE_MIN);
        ch.setDescription(ctx.getString(R.string.push_channel_help));
        ch.setShowBadge(false);
        nm.createNotificationChannel(ch);
        nm.deleteNotificationChannel("push_hidden"); // a category of an earlier build
        return CHANNEL;
    }

    @Override
    public void onCreate() {
        super.onCreate();
        String channel = channel(this);
        Intent open = new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        Notification n = new Notification.Builder(this, channel)
                .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
                .setContentTitle(getString(R.string.push_notification))
                .setContentIntent(PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE))
                .setOngoing(true)
                .setShowWhen(false)
                .build();
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        } else {
            startForeground(NOTIFICATION, n);
        }
        running = true;
        // Back on the network: reconnect at once rather than at the end of the wait.
        network = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onAvailable(Network net) {
                synchronized (wake) {
                    wake.notifyAll();
                }
            }
        };
        try {
            getSystemService(ConnectivityManager.class).registerDefaultNetworkCallback(network);
        } catch (RuntimeException e) {
            // Without it the stream still reconnects, after its wait.
            Log.w("agendouille", "push: no network callback", e);
            network = null;
        }
        loop = new Thread(this::run, "agendouille-push");
        loop.start();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (!wanted(this)) {
            stopSelf();
            return START_NOT_STICKY;
        }
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        running = false;
        HttpURLConnection c = current;
        if (c != null) c.disconnect();
        synchronized (wake) {
            wake.notifyAll();
        }
        try {
            if (network != null) getSystemService(ConnectivityManager.class).unregisterNetworkCallback(network);
        } catch (RuntimeException ignored) {
            // not registered
        }
        UnifiedPush.prefs(this).edit().putBoolean("connected", false).apply();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    /** Connects, reads, and connects again: at once after a good connection, then waiting longer and longer. */
    private void run() {
        long backoff = 2_000;
        while (running) {
            long opened = System.currentTimeMillis();
            try {
                stream();
            } catch (Exception e) {
                Log.i("agendouille", "push: the stream ended", e);
            }
            UnifiedPush.prefs(this).edit().putBoolean("connected", false).apply();
            if (!running) return;
            backoff = System.currentTimeMillis() - opened > 60_000 ? 2_000 : Math.min(backoff * 2, 5 * 60_000);
            synchronized (wake) {
                try {
                    wake.wait(backoff);
                } catch (InterruptedException e) {
                    return;
                }
            }
        }
    }

    private void stream() throws Exception {
        Credentials cred = Credentials.load(this);
        if (cred == null) {
            running = false;
            stopSelf();
            return;
        }
        SharedPreferences p = UnifiedPush.prefs(this);
        HttpURLConnection c = (HttpURLConnection) new URL(cred.server + "/api/app/push/stream").openConnection();
        current = c;
        try {
            c.setConnectTimeout(15_000);
            c.setReadTimeout(150_000); // the server pings every 55 s
            c.setRequestProperty("Accept", "text/event-stream");
            String raw = cred.username + ":" + cred.secret;
            c.setRequestProperty("Authorization", "Basic " + android.util.Base64.encodeToString(raw.getBytes(StandardCharsets.UTF_8), android.util.Base64.NO_WRAP));
            long last = p.getLong("last_seq", 0);
            if (last > 0) c.setRequestProperty("Last-Event-ID", Long.toString(last));
            if (c.getResponseCode() != 200) throw new java.io.IOException("the stream answered " + c.getResponseCode());
            p.edit().putBoolean("connected", true).apply();
            BufferedReader in = new BufferedReader(new InputStreamReader(c.getInputStream(), StandardCharsets.UTF_8));
            long id = 0;
            String data = null;
            String event = null;
            for (String line; running && (line = in.readLine()) != null; ) {
                if (line.startsWith("event: ")) {
                    event = line.substring(7).trim();
                } else if (line.isEmpty() && "server".equals(event) && data != null) {
                    // The APK the server carries: an update, told at once.
                    JSONObject o = new JSONObject(data);
                    UpdateNotice.fromServer(this, o.optString("apk_version"), !o.optString("apk_sha256").isEmpty());
                    event = null;
                    data = null;
                } else if (line.startsWith("id: ")) {
                    id = Long.parseLong(line.substring(4).trim());
                } else if (line.startsWith("data: ")) {
                    data = line.substring(6);
                } else if (line.isEmpty() && id > 0 && data != null) {
                    JSONObject o = new JSONObject(data);
                    Distributor.deliver(this, o.optString("endpoint"), Base64.getUrlDecoder().decode(o.optString("data")), id);
                    p.edit().putLong("last_seq", id).apply();
                    ack(cred, id);
                    id = 0;
                    data = null;
                    event = null;
                }
            }
        } finally {
            current = null;
            c.disconnect();
        }
    }

    /** Tells the server the message arrived, so it does not wait for the next connection to drop it. */
    private void ack(Credentials cred, long seq) {
        new Thread(() -> {
            try {
                Http.call("POST", cred.server + "/api/app/push/ack", cred.username, cred.secret, null, "{\"seq\":" + seq + "}", "agendouille-android");
            } catch (Exception ignored) {
                // Last-Event-ID does it at the next connection
            }
        }, "agendouille-push-ack").start();
    }
}
