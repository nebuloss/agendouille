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

    // "v0.6.0" and "0.6.0" are the same version; "-dirty" builds are their commit.
    private static String strip(String v) {
        v = v.trim();
        if (v.startsWith("v")) v = v.substring(1);
        if (v.endsWith("-dirty")) v = v.substring(0, v.length() - "-dirty".length());
        return v;
    }
}
