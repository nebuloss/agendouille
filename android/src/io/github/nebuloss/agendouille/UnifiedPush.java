package io.github.nebuloss.agendouille;

import android.app.BroadcastOptions;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.os.Build;

import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

/**
 * The UnifiedPush connector, written on its intents (spec AND_3, no
 * library; PLAN.md M9). A distributor app on the phone (ntfy, for one)
 * holds the connection to the family's push server and hands the app an
 * endpoint; the app gives that endpoint to the server per calendar
 * (PushSync, WebDAV-Push), and each message it gets back makes it sync.
 *
 * What it keeps, in the "push" preferences: the distributor chosen, the
 * token of the registration, the endpoint, the server's VAPID key, whether
 * the person turned it off, and the last failure.
 */
final class UnifiedPush {
    private UnifiedPush() {}

    static final String REGISTER = "org.unifiedpush.android.distributor.REGISTER";
    static final String UNREGISTER = "org.unifiedpush.android.distributor.UNREGISTER";
    static final String MESSAGE_ACK = "org.unifiedpush.android.distributor.MESSAGE_ACK";

    static SharedPreferences prefs(Context ctx) {
        return ctx.getSharedPreferences("push", Context.MODE_PRIVATE);
    }

    /** The distributors installed (their packages), ours never among them. */
    static List<String> distributors(Context ctx) {
        List<String> out = new ArrayList<>();
        for (ResolveInfo r : receivers(ctx, new Intent(REGISTER))) {
            String pkg = r.activityInfo == null ? null : r.activityInfo.packageName;
            if (pkg != null && !pkg.equals(ctx.getPackageName()) && !out.contains(pkg)) out.add(pkg);
        }
        return out;
    }

    @SuppressWarnings("deprecation")
    private static List<ResolveInfo> receivers(Context ctx, Intent i) {
        PackageManager pm = ctx.getPackageManager();
        if (Build.VERSION.SDK_INT >= 33) return pm.queryBroadcastReceivers(i, PackageManager.ResolveInfoFlags.of(0));
        return pm.queryBroadcastReceivers(i, 0);
    }

    /** The distributor in use: the one chosen while it is installed, else the first there is. */
    static String distributor(Context ctx) {
        List<String> all = distributors(ctx);
        String chosen = prefs(ctx).getString("distributor", null);
        if (chosen != null && all.contains(chosen)) return chosen;
        return all.isEmpty() ? null : all.get(0);
    }

    static boolean enabled(Context ctx) {
        return prefs(ctx).getBoolean("enabled", true);
    }

    /** The endpoint to give the server; null while there is none (off, waiting, failed). */
    static String endpoint(Context ctx) {
        return enabled(ctx) ? prefs(ctx).getString("endpoint", null) : null;
    }

    /**
     * Registers with the distributor, again with the same token when there
     * is one (the spec asks it at each start). Needs the server's VAPID key:
     * some push servers refuse a registration without it.
     */
    static void register(Context ctx) {
        SharedPreferences p = prefs(ctx);
        String dist = distributor(ctx);
        String vapid = p.getString("vapid", null);
        if (!enabled(ctx) || dist == null || vapid == null || Credentials.load(ctx) == null) return;
        String token = p.getString("token", null);
        if (token == null || !dist.equals(p.getString("token_for", null))) {
            if (token != null) unregisterFrom(ctx, p.getString("token_for", null), token);
            token = UUID.randomUUID().toString();
            p.edit().putString("token", token).putString("token_for", dist).remove("endpoint").apply();
        }
        Intent i = new Intent(REGISTER).setPackage(dist)
                .putExtra("token", token)
                .putExtra("application", ctx.getPackageName()) // read by distributors of older specs
                .putExtra("message", "agendouille")
                .putExtra("vapid", vapid);
        send(ctx, i);
    }

    /** Leaves the distributor: off, a sign-out, another distributor. */
    static void unregister(Context ctx) {
        SharedPreferences p = prefs(ctx);
        String token = p.getString("token", null);
        if (token != null) unregisterFrom(ctx, p.getString("token_for", null), token);
        p.edit().remove("token").remove("token_for").remove("endpoint").apply();
    }

    private static void unregisterFrom(Context ctx, String dist, String token) {
        if (dist == null) return;
        send(ctx, new Intent(UNREGISTER).setPackage(dist).putExtra("token", token));
    }

    /** Acknowledges a NEW_ENDPOINT or MESSAGE that carried an id. */
    static void ack(Context ctx, String token, String id) {
        String dist = prefs(ctx).getString("token_for", null);
        if (id == null || dist == null) return;
        send(ctx, new Intent(MESSAGE_ACK).setPackage(dist).putExtra("token", token).putExtra("id", id));
    }

    /**
     * Sends with the app's identity: shared with the broadcast on Android
     * 14 and later, and a PendingIntent of ours before (the distributor reads
     * the package from it).
     */
    private static void send(Context ctx, Intent i) {
        i.putExtra("pi", PendingIntent.getBroadcast(ctx, 0, new Intent().setPackage("org.unifiedpush.dummy_app"), PendingIntent.FLAG_IMMUTABLE));
        if (Build.VERSION.SDK_INT >= 34) {
            ctx.sendBroadcast(i, null, BroadcastOptions.makeBasic().setShareIdentityEnabled(true).toBundle());
        } else {
            ctx.sendBroadcast(i);
        }
    }

    /** What the distributor sends the app. */
    public static final class Receiver extends BroadcastReceiver {
        private static final String PREFIX = "org.unifiedpush.android.connector.";

        @Override
        public void onReceive(Context ctx, Intent intent) {
            String action = intent.getAction();
            String token = intent.getStringExtra("token");
            SharedPreferences p = prefs(ctx);
            if (action == null || token == null || !token.equals(p.getString("token", null))) return;
            String id = intent.getStringExtra("id");
            switch (action) {
                case PREFIX + "NEW_ENDPOINT": {
                    String endpoint = intent.getStringExtra("endpoint");
                    ack(ctx, token, id);
                    if (endpoint == null) return;
                    p.edit().putString("endpoint", endpoint).remove("error").apply();
                    ChangeJob.upload(ctx); // the sync gives it to the server
                    break;
                }
                case PREFIX + "MESSAGE":
                    // A calendar changed on the server: sync now.
                    ack(ctx, token, id);
                    ChangeJob.upload(ctx);
                    break;
                case PREFIX + "UNREGISTERED":
                    p.edit().remove("token").remove("token_for").remove("endpoint").apply();
                    ChangeJob.upload(ctx); // the server's subscriptions go
                    break;
                case PREFIX + "REGISTRATION_FAILED":
                    // A new token for the next try; a failure after an endpoint is ignored.
                    if (p.getString("endpoint", null) != null) return;
                    String reason = intent.getStringExtra("reason");
                    p.edit().remove("token").remove("token_for").putString("error", reason == null ? "INTERNAL_ERROR" : reason).apply();
                    break;
                case PREFIX + "TEMP_UNAVAILABLE":
                    p.edit().putString("error", "TEMP_UNAVAILABLE").apply();
                    break;
                default:
                    break;
            }
        }
    }
}
