package io.github.nebuloss.agendouille;

/**
 * Whether the server holds an app other than this one (PLAN.md 7.6). The
 * server embeds the APK of its own version, so any difference is an update:
 * "dev-1a2b3c4" builds have no order, and a release is never older than
 * the app a server of that release made. Pure Java: tested on the JVM.
 */
public final class Versions {
    private Versions() {}

    /** True when the server offers an APK of another version than ours. */
    public static boolean updateAvailable(String ours, String servers, boolean apkThere) {
        if (!apkThere || servers == null || servers.isEmpty() || ours == null) return false;
        return !strip(ours).equals(strip(servers));
    }

    /** A release's version ("0.7.0", "v0.7.0"), not a dev build ("dev-1a2b3c4"). */
    public static boolean isRelease(String v) {
        return v != null && strip(v).matches("[0-9]+\\.[0-9]+\\.[0-9]+");
    }

    /** True when theirs is a release later than ours (GitHub's latest against this app). */
    public static boolean newer(String ours, String theirs) {
        if (!isRelease(ours) || !isRelease(theirs)) return false;
        String[] a = strip(ours).split("\\."), b = strip(theirs).split("\\.");
        for (int i = 0; i < 3; i++) {
            long x = Long.parseLong(a[i]), y = Long.parseLong(b[i]);
            if (x != y) return y > x;
        }
        return false;
    }

    // "v0.6.0" and "0.6.0" are the same version; "-dirty" builds are their commit.
    private static String strip(String v) {
        v = v.trim();
        if (v.startsWith("v")) v = v.substring(1);
        if (v.endsWith("-dirty")) v = v.substring(0, v.length() - "-dirty".length());
        return v;
    }
}
