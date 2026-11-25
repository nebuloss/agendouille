package io.github.nebuloss.agendouille;

import android.content.Context;
import android.content.pm.PackageManager;

/**
 * Whether the server's APK is an update for this phone, for the « Mise à
 * jour disponible » alert while the app is closed. The server says which
 * APK it carries at once on the push stream (instant sync: a deploy
 * restarts it, the stream reconnects, the alert follows within seconds),
 * and in /api/app/config at each sync otherwise. The app, opened, offers it
 * in its dialog as before.
 */
final class UpdateNotice {
    private UpdateNotice() {}

    static String installed(Context ctx) {
        try {
            return ctx.getPackageManager().getPackageInfo(ctx.getPackageName(), 0).versionName;
        } catch (PackageManager.NameNotFoundException e) {
            return "?";
        }
    }

    /** Réglages' « Versions de développement »: a dev build starts with it on, a release off. */
    static boolean devUpdates(Context ctx) {
        return ctx.getSharedPreferences("agendouille", Context.MODE_PRIVATE).getBoolean("dev_updates", !Versions.isRelease(installed(ctx)));
    }

    /** The server's APK (its version; whether it has one): the alert when it is one for this phone. */
    static void fromServer(Context ctx, String theirs, boolean apkThere) {
        if (theirs == null || theirs.isEmpty() || Credentials.load(ctx) == null) return;
        String ours = installed(ctx);
        boolean update = devUpdates(ctx)
                ? Versions.updateAvailable(ours, theirs, apkThere)
                : apkThere && Versions.isRelease(theirs) && Versions.newer(ours, theirs);
        if (update) Alerts.update(ctx, theirs);
    }
}
