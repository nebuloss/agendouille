package io.github.nebuloss.agendouille;

import android.content.Context;
import android.content.SharedPreferences;
import android.util.Log;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.IOException;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.SecureRandom;
import java.security.interfaces.ECPublicKey;
import java.security.spec.ECGenParameterSpec;
import java.util.Base64;
import java.util.HashSet;
import java.util.Iterator;
import java.util.List;
import java.util.Set;

/**
 * At each sync, the server's side of push (WebDAV-Push, standard CalDAV as
 * DAVx5 does it): one registration per calendar the phone syncs, with the
 * app's own endpoint on the server's push service; renewed before they
 * expire, removed when a calendar stops syncing or instant sync is off.
 *
 * The keys given to the server are the app's own; it never decrypts what it
 * gets: any message for its token means "something changed, sync".
 */
final class PushSync {
    private PushSync() {}

    private static final long LIFETIME = 7L * 24 * 3600_000L;
    private static final long RENEW_BEFORE = 2L * 24 * 3600_000L;

    static void run(Context ctx, Credentials cred, String ua, List<CalendarSync.Synced> synced) {
        SharedPreferences p = UnifiedPush.prefs(ctx);
        try {
            // Whether the server offers push (WebDAV-Push), from a calendar's properties, once a day.
            if (!synced.isEmpty() && p.getString("dav_checked", "").compareTo(day()) < 0) {
                Http.Answer a = Http.dav("PROPFIND", synced.get(0).url, cred.username, cred.secret, "0", "application/xml; charset=utf-8", PushXml.PROPFIND, null, ua);
                p.edit().putString("dav_checked", day()).putBoolean("server_push", a.status == 207 && PushXml.vapidKey(a.body) != null).apply();
            }
            subscriptions(ctx, cred, ua, synced, UnifiedPush.selfEndpoint(ctx, cred, ua));
        } catch (IOException e) {
            Log.i("agendouille", "push: offline", e);
        } catch (Exception e) {
            Log.w("agendouille", "push", e);
        }
        PushService.update(ctx);
    }

    /** Brings the server's registrations in line: one per synced calendar while there is an endpoint. */
    static void subscriptions(Context ctx, Credentials cred, String ua, List<CalendarSync.Synced> synced, String endpoint) throws IOException, JSONException {
        SharedPreferences p = UnifiedPush.prefs(ctx);
        JSONObject regs = new JSONObject(p.getString("regs", "{}"));
        Set<String> want = new HashSet<>();
        if (endpoint != null && p.getBoolean("server_push", false)) for (CalendarSync.Synced c : synced) want.add(c.url);
        long now = System.currentTimeMillis();
        String[] keys = keys(p);
        for (String url : want) {
            JSONObject r = regs.optJSONObject(url);
            if (r != null && endpoint.equals(r.optString("endpoint")) && r.optLong("expires") - now > RENEW_BEFORE) continue;
            Http.Answer a = Http.dav("POST", url, cred.username, cred.secret, null, "application/xml; charset=utf-8",
                    PushXml.register(endpoint, keys[0], keys[1], PushXml.httpDate(now + LIFETIME)), null, ua);
            if (a.status / 100 != 2) {
                Log.i("agendouille", "push: registering " + url + ": " + a.status);
                continue;
            }
            String location = PushXml.resolve(url, a.headers.get("location"));
            long expires = PushXml.parseHttpDate(a.headers.get("expires"));
            // The old registration, for another endpoint, goes.
            if (r != null && !r.optString("location").isEmpty() && !r.optString("location").equals(location)) {
                delete(cred, ua, r.optString("location"));
            }
            regs.put(url, new JSONObject().put("location", location == null ? "" : location).put("endpoint", endpoint)
                    .put("expires", expires > 0 ? expires : now + 3L * 24 * 3600_000L));
        }
        for (Iterator<String> it = regs.keys(); it.hasNext(); ) {
            String url = it.next();
            if (want.contains(url)) continue;
            delete(cred, ua, regs.getJSONObject(url).optString("location"));
            it.remove();
        }
        p.edit().putString("regs", regs.toString()).apply();
    }

    /** Removes every registration of the app on the server (a sign-out). */
    static void clear(Context ctx, Credentials cred, String ua) {
        try {
            subscriptions(ctx, cred, ua, java.util.Collections.emptyList(), null);
        } catch (Exception e) {
            Log.i("agendouille", "push: clearing", e);
        }
        UnifiedPush.prefs(ctx).edit().remove("regs").remove("dav_checked").remove("server_push").apply();
    }

    private static void delete(Credentials cred, String ua, String location) {
        if (location == null || location.isEmpty()) return;
        try {
            Http.dav("DELETE", location, cred.username, cred.secret, null, null, null, null, ua);
        } catch (IOException ignored) {
            // it expires on its own
        }
    }

    /** The subscription's public key and auth secret (base64url), made once. */
    private static String[] keys(SharedPreferences p) {
        String pub = p.getString("p256dh", null), auth = p.getString("auth", null);
        if (pub != null && auth != null) return new String[]{pub, auth};
        try {
            KeyPairGenerator g = KeyPairGenerator.getInstance("EC");
            g.initialize(new ECGenParameterSpec("secp256r1"));
            KeyPair kp = g.generateKeyPair();
            ECPublicKey k = (ECPublicKey) kp.getPublic();
            pub = PushXml.uncompressed(k.getW().getAffineX(), k.getW().getAffineY());
        } catch (java.security.GeneralSecurityException e) {
            throw new IllegalStateException(e);
        }
        byte[] a = new byte[16];
        new SecureRandom().nextBytes(a);
        auth = Base64.getUrlEncoder().withoutPadding().encodeToString(a);
        p.edit().putString("p256dh", pub).putString("auth", auth).apply();
        return new String[]{pub, auth};
    }

    /** Today, to look at the server's key once a day. */
    private static String day() {
        return java.time.LocalDate.now().toString();
    }
}
