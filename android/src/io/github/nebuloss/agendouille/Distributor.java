package io.github.nebuloss.agendouille;

import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;

import org.json.JSONException;
import org.json.JSONObject;

import java.util.Iterator;

/**
 * The app as a UnifiedPush distributor (spec AND_3, by hand, no library):
 * DAVx5, or any UnifiedPush app, registers here; the app asks the server's
 * push service for an endpoint for it and hands it over; each message the
 * stream brings for that endpoint goes to that app. The app's own endpoint
 * (UnifiedPush.selfEndpoint) needs no intent: its messages make it sync.
 *
 * Kept in the "push" preferences under "apps": endpoint id -> {package, token}.
 */
final class Distributor {
    private Distributor() {}

    private static final String CONNECTOR = "org.unifiedpush.android.connector.";

    /** A message from the stream, for endpoint. */
    static void deliver(Context ctx, String endpoint, byte[] body, long seq) {
        SharedPreferences p = UnifiedPush.prefs(ctx);
        if (endpoint.equals(p.getString("self_id", null))) {
            ChangeJob.upload(ctx); // a calendar changed elsewhere: sync now
            return;
        }
        JSONObject app = apps(p).optJSONObject(endpoint);
        if (app == null) return; // an app since gone: its endpoint goes with the next unregister
        String pkg = app.optString("package");
        raise(ctx, pkg);
        ctx.sendBroadcast(new Intent(CONNECTOR + "MESSAGE").setPackage(pkg)
                .putExtra("token", app.optString("token"))
                .putExtra("bytesMessage", body)
                .putExtra("id", Long.toString(seq)));
    }

    /**
     * The spec's 5 seconds at foreground importance for the app that gets a
     * message: bound to its RAISE_TO_FOREGROUND service, when it has one.
     */
    private static void raise(Context ctx, String pkg) {
        Context app = ctx.getApplicationContext();
        ServiceConnection conn = new ServiceConnection() {
            @Override
            public void onServiceConnected(ComponentName name, IBinder service) {}

            @Override
            public void onServiceDisconnected(ComponentName name) {}
        };
        try {
            if (app.bindService(new Intent(CONNECTOR + "RAISE_TO_FOREGROUND").setPackage(pkg), conn, Context.BIND_AUTO_CREATE)) {
                new Handler(Looper.getMainLooper()).postDelayed(() -> {
                    try {
                        app.unbindService(conn);
                    } catch (RuntimeException ignored) {
                        // already gone
                    }
                }, 5_000);
            }
        } catch (RuntimeException e) {
            Log.i("agendouille", "push: raising " + pkg, e);
        }
    }

    static JSONObject apps(SharedPreferences p) {
        try {
            return new JSONObject(p.getString("apps", "{}"));
        } catch (JSONException e) {
            return new JSONObject();
        }
    }

    /** The labels of the apps that use the app as their distributor. */
    static java.util.List<String> users(Context ctx) {
        java.util.List<String> out = new java.util.ArrayList<>();
        JSONObject apps = apps(UnifiedPush.prefs(ctx));
        for (Iterator<String> it = apps.keys(); it.hasNext(); ) {
            String pkg = apps.optJSONObject(it.next()).optString("package");
            String label = pkg;
            try {
                label = String.valueOf(ctx.getPackageManager().getApplicationLabel(ctx.getPackageManager().getApplicationInfo(pkg, 0)));
            } catch (android.content.pm.PackageManager.NameNotFoundException ignored) {
                // uninstalled
            }
            if (!out.contains(label)) out.add(label);
        }
        return out;
    }

    /** What connector apps send: REGISTER, UNREGISTER, MESSAGE_ACK. */
    public static final class Receiver extends BroadcastReceiver {
        private static final String PREFIX = "org.unifiedpush.android.distributor.";

        @Override
        public void onReceive(Context ctx, Intent intent) {
            String action = intent.getAction();
            String token = intent.getStringExtra("token");
            if (action == null || token == null) return;
            String pkg = sender(this, intent);
            if (pkg == null || pkg.equals(ctx.getPackageName())) return;
            if ((PREFIX + "MESSAGE_ACK").equals(action)) return; // the server had our acknowledgement already
            PendingResult result = goAsync();
            new Thread(() -> {
                try {
                    if ((PREFIX + "REGISTER").equals(action)) register(ctx, pkg, token);
                    else if ((PREFIX + "UNREGISTER").equals(action)) unregister(ctx, pkg, token);
                } finally {
                    result.finish();
                }
            }, "agendouille-distributor").start();
        }

        /** Who sent it: the identity Android 14 shares, else the PendingIntent, else what it says. */
        @SuppressWarnings("deprecation")
        private static String sender(BroadcastReceiver r, Intent intent) {
            if (Build.VERSION.SDK_INT >= 34) {
                String s = r.getSentFromPackage();
                if (s != null) return s;
            }
            PendingIntent pi = Build.VERSION.SDK_INT >= 33 ? intent.getParcelableExtra("pi", PendingIntent.class) : intent.getParcelableExtra("pi");
            if (pi != null) return pi.getCreatorPackage();
            return intent.getStringExtra("application");
        }
    }

    private static void register(Context ctx, String pkg, String token) {
        Credentials cred = Credentials.load(ctx);
        SharedPreferences p = UnifiedPush.prefs(ctx);
        if (cred == null || !p.getBoolean("instant", false) || !p.getBoolean("service", false)) {
            // Instant sync is off in the app: the person turns it on there.
            failed(ctx, pkg, token, "ACTION_REQUIRED");
            return;
        }
        try {
            Http.Answer a = Http.call("POST", cred.server + "/api/app/push/endpoints", cred.username, cred.secret, null,
                    "{\"instance\":" + JSONObject.quote(pkg + ":" + token) + "}", "agendouille-android");
            if (a.status != 200) {
                failed(ctx, pkg, token, "INTERNAL_ERROR");
                return;
            }
            JSONObject o = new JSONObject(a.body);
            synchronized (Distributor.class) {
                JSONObject apps = apps(p);
                apps.put(o.getString("id"), new JSONObject().put("package", pkg).put("token", token));
                p.edit().putString("apps", apps.toString()).apply();
            }
            ctx.sendBroadcast(new Intent(CONNECTOR + "NEW_ENDPOINT").setPackage(pkg)
                    .putExtra("token", token).putExtra("endpoint", o.getString("endpoint")));
            PushService.update(ctx);
        } catch (java.io.IOException e) {
            failed(ctx, pkg, token, "NETWORK");
        } catch (JSONException e) {
            failed(ctx, pkg, token, "INTERNAL_ERROR");
        }
    }

    private static void unregister(Context ctx, String pkg, String token) {
        SharedPreferences p = UnifiedPush.prefs(ctx);
        Credentials cred = Credentials.load(ctx);
        String id = null;
        synchronized (Distributor.class) {
            JSONObject apps = apps(p);
            for (Iterator<String> it = apps.keys(); it.hasNext(); ) {
                String k = it.next();
                JSONObject a = apps.optJSONObject(k);
                if (a != null && pkg.equals(a.optString("package")) && token.equals(a.optString("token"))) {
                    id = k;
                    it.remove();
                    break;
                }
            }
            p.edit().putString("apps", apps.toString()).apply();
        }
        if (id != null && cred != null) {
            try {
                Http.call("DELETE", cred.server + "/api/app/push/endpoints/" + id, cred.username, cred.secret, null, null, "agendouille-android");
            } catch (java.io.IOException ignored) {
                // the endpoint stays unused on the server
            }
        }
        ctx.sendBroadcast(new Intent(CONNECTOR + "UNREGISTERED").setPackage(pkg).putExtra("token", token));
    }

    private static void failed(Context ctx, String pkg, String token, String reason) {
        ctx.sendBroadcast(new Intent(CONNECTOR + "REGISTRATION_FAILED").setPackage(pkg).putExtra("token", token).putExtra("reason", reason));
    }

    /** Signing out: every app is told its endpoint is gone. */
    static void unregisterAll(Context ctx) {
        JSONObject apps = apps(UnifiedPush.prefs(ctx));
        for (Iterator<String> it = apps.keys(); it.hasNext(); ) {
            JSONObject a = apps.optJSONObject(it.next());
            if (a != null) ctx.sendBroadcast(new Intent(CONNECTOR + "UNREGISTERED").setPackage(a.optString("package")).putExtra("token", a.optString("token")));
        }
    }
}
