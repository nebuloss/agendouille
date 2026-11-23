package io.github.nebuloss.agendouille;

import android.content.Context;
import android.content.SharedPreferences;

/** The last sync, as the screens show it: kept by the sync adapter. */
final class SyncStatus {
    private SyncStatus() {}

    static SharedPreferences prefs(Context ctx) {
        return ctx.getSharedPreferences("sync", Context.MODE_PRIVATE);
    }

    static void syncing(Context ctx) {
        prefs(ctx).edit().putString("state", "syncing").apply();
    }

    static void done(Context ctx, int events) {
        prefs(ctx).edit().putString("state", "idle").putLong("last_sync", System.currentTimeMillis())
                .putInt("events", events).remove("error").apply();
    }

    static void failed(Context ctx, String error) {
        prefs(ctx).edit().putString("state", "error").putString("error", error).apply();
    }

    static void clear(Context ctx) {
        prefs(ctx).edit().clear().apply();
    }
}
