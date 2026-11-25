package io.github.nebuloss.agendouille;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONObject;

import java.io.IOException;

/**
 * Instant sync's state, in the "push" preferences: whether the person
 * turned it on ("instant"), whether the server has its push service
 * ("service", from /api/app/config), the app's own endpoint on it, the
 * stream's last message ("last_seq") and whether it is connected, and the
 * apps the app distributes to (Distributor).
 */
final class UnifiedPush {
    private UnifiedPush() {}

    static SharedPreferences prefs(Context ctx) {
        return ctx.getSharedPreferences("push", Context.MODE_PRIVATE);
    }

    /**
     * The app's own endpoint on the server's push service, made once: what
     * it registers with each calendar (WebDAV-Push). Null while instant
     * sync is off, or the server has no push service.
     */
    static String selfEndpoint(Context ctx, Credentials cred, String ua) throws IOException {
        SharedPreferences p = prefs(ctx);
        if (!p.getBoolean("instant", false) || !p.getBoolean("service", false)) return null;
        String url = p.getString("self_endpoint", null);
        if (url != null && cred.server.equals(p.getString("self_server", null))) return url;
        Http.Answer a = Http.call("POST", cred.server + "/api/app/push/endpoints", cred.username, cred.secret, null,
                "{\"instance\":" + JSONObject.quote(ctx.getPackageName()) + "}", ua);
        if (a.status != 200) return null;
        try {
            JSONObject o = new JSONObject(a.body);
            p.edit().putString("self_id", o.getString("id")).putString("self_endpoint", o.getString("endpoint"))
                    .putString("self_server", cred.server).apply();
            return o.getString("endpoint");
        } catch (org.json.JSONException e) {
            throw new IOException("the push service's answer", e);
        }
    }

    /** Forgets everything of the account (a sign-out). */
    static void clear(Context ctx) {
        prefs(ctx).edit().clear().apply();
    }
}
