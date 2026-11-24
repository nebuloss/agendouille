package io.github.nebuloss.agendouille;

import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageInstaller;
import android.os.Build;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;

/**
 * Updates from GitHub (DECISIONS): the latest release of the repository,
 * its APK checked against the release's SHA256SUMS, then installed by
 * Android's own PackageInstaller, which asks the person and refuses an APK
 * signed with another key than the one installed. No store, no Google
 * Play services.
 */
final class Updater {
    static final String REPO = "nebuloss/agendouille";
    static final String ACTION_STATUS = "io.github.nebuloss.agendouille.INSTALL_STATUS";
    private static final int MAX_APK = 64 << 20;

    /** A release with an APK: its version ("0.7.0"), the APK's and the sums' addresses. */
    static final class Release {
        final String version, apkName, apkUrl, sumsUrl;

        Release(String version, String apkName, String apkUrl, String sumsUrl) {
            this.version = version;
            this.apkName = apkName;
            this.apkUrl = apkUrl;
            this.sumsUrl = sumsUrl;
        }
    }

    private Updater() {}

    /** The latest release that carries an APK and its checksums, or null. */
    static Release latest(String userAgent) throws IOException {
        byte[] body = get("https://api.github.com/repos/" + REPO + "/releases/latest", userAgent, 1 << 20);
        try {
            JSONObject o = new JSONObject(new String(body, StandardCharsets.UTF_8));
            String tag = o.optString("tag_name");
            String version = tag.startsWith("v") ? tag.substring(1) : tag;
            JSONArray assets = o.optJSONArray("assets");
            if (assets == null || version.isEmpty()) return null;
            String apk = null, apkUrl = null, sums = null;
            for (int i = 0; i < assets.length(); i++) {
                JSONObject a = assets.getJSONObject(i);
                String name = a.optString("name"), url = a.optString("browser_download_url");
                if (name.equals("agendouille-" + version + ".apk")) {
                    apk = name;
                    apkUrl = url;
                } else if (name.equals("SHA256SUMS")) sums = url;
            }
            return apk == null || sums == null ? null : new Release(version, apk, apkUrl, sums);
        } catch (org.json.JSONException e) {
            throw new IOException("GitHub's answer: " + e.getMessage());
        }
    }

    /** Downloads the APK into the cache and checks it against SHA256SUMS; the file, or an IOException. */
    static File download(Context ctx, Release r, String userAgent) throws IOException {
        String sums = new String(get(r.sumsUrl, userAgent, 64 << 10), StandardCharsets.UTF_8);
        String want = Checksums.find(sums, r.apkName);
        if (want == null) throw new IOException("SHA256SUMS does not list " + r.apkName);
        byte[] apk = get(r.apkUrl, userAgent, MAX_APK);
        String got = Checksums.sha256(apk);
        if (!got.equals(want)) throw new IOException("the APK's SHA-256 is " + got + ", the release says " + want);
        return save(ctx, apk);
    }

    /**
     * The APK of the account's server (a dev build: scripts/devdeploy.sh
     * embeds it), checked against the SHA-256 the server announces
     * (/api/app/config); the file, or an IOException.
     */
    static File downloadFromServer(Context ctx, String server, String sha256, String userAgent) throws IOException {
        if (sha256 == null || !sha256.matches("[0-9a-f]{64}")) throw new IOException("the server announces no SHA-256 for its app");
        byte[] apk = get(server + "/app/agendouille.apk", userAgent, MAX_APK);
        String got = Checksums.sha256(apk);
        if (!got.equals(sha256)) throw new IOException("the APK's SHA-256 is " + got + ", the server says " + sha256);
        return save(ctx, apk);
    }

    private static File save(Context ctx, byte[] apk) throws IOException {
        File f = new File(ctx.getCacheDir(), "update.apk");
        try (OutputStream out = new FileOutputStream(f)) {
            out.write(apk);
        }
        return f;
    }

    /**
     * Hands the APK to Android: it shows its own confirmation (and, the
     * first time, the switch "allow from this source"), then answers on
     * ACTION_STATUS to the activity.
     */
    static void install(Context ctx, File apk) throws IOException {
        // Android refuses a lower version code: say so before it does.
        long theirs = versionCode(ctx, apk);
        if (theirs >= 0 && theirs < installedCode(ctx)) {
            //noinspection ResultOfMethodCallIgnored
            apk.delete();
            throw new IOException(ctx.getString(R.string.update_older));
        }
        PackageInstaller pi = ctx.getPackageManager().getPackageInstaller();
        PackageInstaller.SessionParams params = new PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL);
        params.setAppPackageName(ctx.getPackageName());
        int id = pi.createSession(params);
        try (PackageInstaller.Session s = pi.openSession(id)) {
            try (InputStream in = new FileInputStream(apk); OutputStream out = s.openWrite("agendouille.apk", 0, apk.length())) {
                byte[] buf = new byte[64 << 10];
                for (int n; (n = in.read(buf)) > 0; ) out.write(buf, 0, n);
                s.fsync(out);
            }
            Intent status = new Intent(ctx, MainActivity.class).setAction(ACTION_STATUS);
            int flags = PendingIntent.FLAG_UPDATE_CURRENT | (Build.VERSION.SDK_INT >= 31 ? 0x02000000 /* FLAG_MUTABLE */ : 0);
            s.commit(PendingIntent.getActivity(ctx, id, status, flags).getIntentSender());
        } catch (IOException | RuntimeException e) {
            // Half written: Android drops the session's copy with it.
            try {
                pi.abandonSession(id);
            } catch (RuntimeException ignored) {
                // already gone
            }
            throw e;
        } finally {
            // Android has its own copy now (or nothing): ours goes.
            //noinspection ResultOfMethodCallIgnored
            apk.delete();
        }
    }

    /**
     * At start: what an update left, an APK in the cache or an install
     * session of ours never committed (the app stopped while writing it).
     * Only one APK is ever kept, the one being installed; old versions are
     * never stored.
     */
    /** An APK file's version code (the commit count), or -1. */
    static long versionCode(Context ctx, File apk) {
        android.content.pm.PackageInfo pi = ctx.getPackageManager().getPackageArchiveInfo(apk.getPath(), 0);
        return pi == null ? -1 : code(pi);
    }

    // getLongVersionCode is Android 9's; before, the int field (min SDK 26).
    @SuppressWarnings("deprecation")
    private static long code(android.content.pm.PackageInfo pi) {
        return Build.VERSION.SDK_INT >= 28 ? pi.getLongVersionCode() : pi.versionCode;
    }

    /** This app's version code. */
    static long installedCode(Context ctx) {
        try {
            android.content.pm.PackageInfo pi = ctx.getPackageManager().getPackageInfo(ctx.getPackageName(), 0);
            return code(pi);
        } catch (android.content.pm.PackageManager.NameNotFoundException e) {
            return 0;
        }
    }

    static void cleanup(Context ctx) {
        //noinspection ResultOfMethodCallIgnored
        new File(ctx.getCacheDir(), "update.apk").delete();
        if (Build.VERSION.SDK_INT < 29) return; // isCommitted() is Android 10's
        PackageInstaller pi = ctx.getPackageManager().getPackageInstaller();
        for (PackageInstaller.SessionInfo s : pi.getMySessions()) {
            if (s.isCommitted()) continue; // waiting for the person's yes: theirs to answer
            try {
                pi.abandonSession(s.getSessionId());
            } catch (RuntimeException ignored) {
                // gone meanwhile
            }
        }
    }

    /** A GET that follows GitHub's redirects to its file storage, at most max bytes. */
    private static byte[] get(String address, String userAgent, int max) throws IOException {
        for (int hop = 0; hop < 5; hop++) {
            HttpURLConnection c = (HttpURLConnection) new URL(address).openConnection();
            try {
                c.setConnectTimeout(15000);
                c.setReadTimeout(60000);
                c.setInstanceFollowRedirects(false);
                c.setRequestProperty("User-Agent", userAgent);
                c.setRequestProperty("Accept", address.contains("api.github.com") ? "application/vnd.github+json" : "application/octet-stream");
                int status = c.getResponseCode();
                if (status / 100 == 3) {
                    String next = c.getHeaderField("Location");
                    if (next == null) throw new IOException("a redirect without Location");
                    URL to = new URL(new URL(address), next);
                    if ("https".equals(new URL(address).getProtocol()) && !"https".equals(to.getProtocol())) throw new IOException("a redirect off https: " + to);
                    address = to.toString();
                    continue;
                }
                if (status != 200) throw new IOException("HTTP " + status + " from " + new URL(address).getHost());
                try (InputStream in = c.getInputStream()) {
                    ByteArrayOutputStream out = new ByteArrayOutputStream();
                    byte[] buf = new byte[64 << 10];
                    for (int n; (n = in.read(buf)) > 0; ) {
                        if (out.size() + n > max) throw new IOException("too large: more than " + max + " bytes");
                        out.write(buf, 0, n);
                    }
                    return out.toByteArray();
                }
            } finally {
                c.disconnect();
            }
        }
        throw new IOException("too many redirects");
    }
}
