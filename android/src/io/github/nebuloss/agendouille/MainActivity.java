package io.github.nebuloss.agendouille;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ContentResolver;
import android.content.ContentUris;
import android.content.ContentValues;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.CalendarContract;
import android.provider.CalendarContract.Calendars;
import android.webkit.MimeTypeMap;
import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import android.webkit.WebMessage;
import android.webkit.WebMessagePort;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.IOException;
import java.io.InputStream;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * The app's screens (DECISIONS: the app is a sync app): the TypeScript
 * pages of src/android, shipped in the APK's assets and served here at
 * https://appassets.androidplatform.net/, talking to this class over the
 * bridge (CONTRACTS.md 6, version 2). No calendar is shown: the phone's
 * calendar app shows what the sync adapter (SyncService) puts there.
 */
public final class MainActivity extends Activity {
    static final String ORIGIN = "https://appassets.androidplatform.net";
    private static final int REQ_CAMERA = 1;
    private static final int REQ_CALENDAR = 2;

    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler ui = new Handler(Looper.getMainLooper());
    private WebView web;
    private WebMessagePort port;
    private long pendingCamera = -1;
    private long pendingCalendar = -1;
    private Object syncObserver;
    private final SharedPreferences.OnSharedPreferenceChangeListener statusListener = (p, k) -> pushState();
    private String pendingLink;

    // --- lifecycle

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        CrashLog.install(this);
        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) WebView.setWebContentsDebuggingEnabled(true);
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        web.setWebViewClient(new WebViewClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest req) {
                return asset(req.getUrl());
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                if (ORIGIN.equals(origin(req.getUrl()))) return false;
                openOutside(req.getUrl());
                return true;
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                connectBridge();
            }

            // The page's renderer died (a crash in the WebView, or killed for
            // memory, as the camera can bring about): without this the app
            // dies with it. The screens are made again, and the report kept.
            @Override
            public boolean onRenderProcessGone(WebView view, android.webkit.RenderProcessGoneDetail detail) {
                CrashLog.write(MainActivity.this, "the page's renderer stopped: " + (detail.didCrash() ? "it crashed" : "the system ended it")
                        + ", priority " + detail.rendererPriorityAtExit());
                port = null;
                web = null;
                recreate();
                return true;
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onPermissionRequest(PermissionRequest r) {
                // The page's scanner: the camera, once Android gave it to the app.
                boolean video = false;
                for (String res : r.getResources()) if (PermissionRequest.RESOURCE_VIDEO_CAPTURE.equals(res)) video = true;
                if (video && ORIGIN.equals(origin(r.getOrigin())) && granted(Manifest.permission.CAMERA)) {
                    r.grant(new String[]{PermissionRequest.RESOURCE_VIDEO_CAPTURE});
                } else {
                    r.deny();
                }
            }
        });
        setContentView(web);
        web.loadUrl(ORIGIN + "/index.html");
        pendingLink = linkOf(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        String link = linkOf(intent);
        if (link != null) enrollFromIntent(link);
    }

    @Override
    protected void onResume() {
        super.onResume();
        syncObserver = ContentResolver.addStatusChangeListener(
                ContentResolver.SYNC_OBSERVER_TYPE_ACTIVE | ContentResolver.SYNC_OBSERVER_TYPE_PENDING, which -> ui.post(this::pushState));
        SyncStatus.prefs(this).registerOnSharedPreferenceChangeListener(statusListener);
        pushState();
        checkConfig();
    }

    @Override
    protected void onPause() {
        if (syncObserver != null) ContentResolver.removeStatusChangeListener(syncObserver);
        SyncStatus.prefs(this).unregisterOnSharedPreferenceChangeListener(statusListener);
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }

    private static String linkOf(Intent i) {
        Uri d = i != null ? i.getData() : null;
        return d != null ? d.toString() : null;
    }

    private static String origin(Uri u) {
        if (u == null || u.getScheme() == null || u.getHost() == null) return "";
        return u.getScheme() + "://" + u.getHost() + (u.getPort() > 0 ? ":" + u.getPort() : "");
    }

    private boolean granted(String permission) {
        return checkSelfPermission(permission) == PackageManager.PERMISSION_GRANTED;
    }

    String version() {
        try {
            return getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
        } catch (PackageManager.NameNotFoundException e) {
            return "?";
        }
    }

    private String userAgent() {
        return "agendouille-android/" + version() + " (Android " + Build.VERSION.RELEASE + "; " + Build.MODEL + ")";
    }

    // --- the screens, from the APK's assets

    private WebResourceResponse asset(Uri u) {
        if (!ORIGIN.equals(origin(u))) return null;
        String path = u.getPath() == null || u.getPath().equals("/") ? "/index.html" : u.getPath();
        if (path.contains("..")) return null;
        try {
            InputStream in = getAssets().open("web" + path);
            String ext = MimeTypeMap.getFileExtensionFromUrl(path);
            String mime = "js".equals(ext) ? "text/javascript" : "css".equals(ext) ? "text/css" : "svg".equals(ext) ? "image/svg+xml"
                    : "html".equals(ext) ? "text/html" : MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext);
            return new WebResourceResponse(mime != null ? mime : "application/octet-stream", "utf-8", in);
        } catch (IOException e) {
            WebResourceResponse r = new WebResourceResponse("text/plain", "utf-8", null);
            r.setStatusCodeAndReasonPhrase(404, "Not Found");
            return r;
        }
    }

    private void openOutside(Uri u) {
        if (u == null || !("https".equals(u.getScheme()) || "http".equals(u.getScheme()))) return;
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, u));
        } catch (ActivityNotFoundException ignored) {
            // nothing to open it with
        }
    }

    // --- the bridge (CONTRACTS.md 6, version 2)

    private void connectBridge() {
        WebMessagePort[] ch = web.createWebMessageChannel();
        port = ch[0];
        port.setWebMessageCallback(new WebMessagePort.WebMessageCallback() {
            @Override
            public void onMessage(WebMessagePort p, WebMessage m) {
                onBridge(m.getData());
            }
        });
        web.postWebMessage(new WebMessage("agendouille-bridge", new WebMessagePort[]{ch[1]}), Uri.parse(ORIGIN));
        if (pendingLink != null) {
            String l = pendingLink;
            pendingLink = null;
            ui.postDelayed(() -> enrollFromIntent(l), 300);
        }
    }

    private void send(String text) {
        ui.post(() -> {
            if (port != null) port.postMessage(new WebMessage(text));
        });
    }

    private void pushState() {
        if (port == null) return;
        io.execute(() -> send(BridgeCodec.event("state", state())));
    }

    private void onBridge(String text) {
        BridgeCodec.Request r = BridgeCodec.parse(text);
        if (r == null) return;
        switch (r.call) {
            case "hello":
                send(BridgeCodec.ok(r.id, BridgeCodec.hello(version(), Build.VERSION.SDK_INT)));
                break;
            case "state":
                io.execute(() -> send(BridgeCodec.ok(r.id, state())));
                break;
            case "camera.request":
                if (granted(Manifest.permission.CAMERA)) send(BridgeCodec.ok(r.id, null));
                else {
                    pendingCamera = r.id;
                    requestPermissions(new String[]{Manifest.permission.CAMERA}, REQ_CAMERA);
                }
                break;
            case "calendar.request":
                pendingCalendar = r.id;
                List<String> want = new ArrayList<>();
                want.add(Manifest.permission.READ_CALENDAR);
                want.add(Manifest.permission.WRITE_CALENDAR);
                if (Build.VERSION.SDK_INT >= 33) want.add("android.permission.POST_NOTIFICATIONS");
                requestPermissions(want.toArray(new String[0]), REQ_CALENDAR);
                break;
            case "enroll":
                enroll(r.id, r.args.optString("link"));
                break;
            case "manual":
                manual(r.id, r.args.optString("server"), r.args.optString("username"), r.args.optString("password"));
                break;
            case "sync.now":
                syncNow();
                io.execute(() -> send(BridgeCodec.ok(r.id, state())));
                break;
            case "calendar.toggle":
                io.execute(() -> {
                    toggle(r.args.optLong("id"), r.args.optBoolean("enabled"));
                    send(BridgeCodec.ok(r.id, state()));
                });
                break;
            case "account.remove":
                removeAccount(r.id);
                break;
            case "crash.clear":
                CrashLog.clear(this);
                io.execute(() -> send(BridgeCodec.ok(r.id, state())));
                break;
            case "crash.send":
                sendCrash(r.id);
                break;
            case "open":
                ui.post(() -> openOutside(Uri.parse(r.args.optString("url"))));
                send(BridgeCodec.ok(r.id, null));
                break;
            default:
                send(BridgeCodec.error(r.id, "unknown_call", null));
        }
    }

    @Override
    public void onRequestPermissionsResult(int request, String[] permissions, int[] results) {
        if (request == REQ_CAMERA && pendingCamera >= 0) {
            long id = pendingCamera;
            pendingCamera = -1;
            if (granted(Manifest.permission.CAMERA)) send(BridgeCodec.ok(id, null));
            else send(BridgeCodec.error(id, "camera_denied", getString(R.string.camera_needed)));
        } else if (request == REQ_CALENDAR && pendingCalendar >= 0) {
            long id = pendingCalendar;
            pendingCalendar = -1;
            if (granted(Manifest.permission.WRITE_CALENDAR)) {
                syncNow();
                io.execute(() -> send(BridgeCodec.ok(id, state())));
            } else {
                send(BridgeCodec.error(id, "calendar_denied", getString(R.string.calendar_needed)));
            }
        }
    }

    // --- what the screens show

    private JSONObject state() {
        try {
            JSONObject o = new JSONObject();
            Credentials c = Credentials.load(this);
            if (c != null) {
                o.put("account", new JSONObject().put("server", c.server).put("host", Uri.parse(c.server).getHost())
                        .put("username", c.username).put("mode", c.mode));
            } else {
                o.put("account", JSONObject.NULL);
            }
            boolean cal = granted(Manifest.permission.READ_CALENDAR) && granted(Manifest.permission.WRITE_CALENDAR);
            o.put("permissions", new JSONObject().put("calendar", cal).put("camera", granted(Manifest.permission.CAMERA)));
            SharedPreferences p = SyncStatus.prefs(this);
            boolean active = c != null && ContentResolver.isSyncActive(c.account, CalendarContract.AUTHORITY);
            JSONObject sync = new JSONObject().put("state", active ? "syncing" : p.getString("state", "never"))
                    .put("events", p.getInt("events", 0));
            long last = p.getLong("last_sync", 0);
            sync.put("last_sync", last > 0 ? iso(last) : JSONObject.NULL);
            sync.put("error", p.contains("error") ? p.getString("error", "") : JSONObject.NULL);
            o.put("sync", sync);
            JSONArray cals = new JSONArray();
            if (c != null && cal) {
                try (Cursor q = getContentResolver().query(Calendars.CONTENT_URI,
                        new String[]{Calendars._ID, Calendars.CALENDAR_DISPLAY_NAME, Calendars.CALENDAR_COLOR, Calendars.SYNC_EVENTS, Calendars.CALENDAR_ACCESS_LEVEL},
                        Calendars.ACCOUNT_NAME + "=? AND " + Calendars.ACCOUNT_TYPE + "=?", new String[]{c.account.name, c.account.type},
                        Calendars.CALENDAR_DISPLAY_NAME)) {
                    while (q != null && q.moveToNext()) {
                        cals.put(new JSONObject().put("id", q.getLong(0)).put("name", q.getString(1))
                                .put("color", String.format("#%06x", q.getInt(2) & 0xffffff)).put("sync", q.getInt(3) == 1)
                                .put("read_only", q.getInt(4) < Calendars.CAL_ACCESS_CONTRIBUTOR));
                    }
                }
            }
            o.put("calendars", cals);
            String crash = CrashLog.read(this);
            o.put("crash", crash != null ? crash : JSONObject.NULL);
            return o;
        } catch (Exception e) {
            return new JSONObject();
        }
    }

    private static String iso(long ms) {
        SimpleDateFormat f = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.ROOT);
        f.setTimeZone(TimeZone.getTimeZone("UTC"));
        return f.format(new Date(ms));
    }

    private void syncNow() {
        Credentials c = Credentials.load(this);
        if (c == null) return;
        Bundle b = new Bundle();
        b.putBoolean(ContentResolver.SYNC_EXTRAS_MANUAL, true);
        b.putBoolean(ContentResolver.SYNC_EXTRAS_EXPEDITED, true);
        ContentResolver.requestSync(c.account, CalendarContract.AUTHORITY, b);
    }

    private void toggle(long id, boolean enabled) {
        Credentials c = Credentials.load(this);
        if (c == null || !granted(Manifest.permission.WRITE_CALENDAR)) return;
        ContentValues v = new ContentValues();
        v.put(Calendars.SYNC_EVENTS, enabled ? 1 : 0);
        v.put(Calendars.VISIBLE, enabled ? 1 : 0);
        getContentResolver().update(ContentUris.withAppendedId(CalendarSync.asSyncAdapter(Calendars.CONTENT_URI, c.account), id), v, null, null);
        if (enabled) syncNow();
    }

    // --- signing in: an enrollment link (PLAN.md 7.2), or by hand

    private void enrollFromIntent(String link) {
        // An App Link or agendouille:// opened the app: the screens hear of it
        // as the state that follows.
        io.execute(() -> {
            String err = doEnroll(link);
            if (err != null) ui.post(() -> new AlertDialog.Builder(this).setMessage(err).setPositiveButton(android.R.string.ok, null).show());
            pushState();
        });
    }

    private void enroll(long id, String link) {
        io.execute(() -> {
            String err = doEnroll(link);
            if (err != null) send(BridgeCodec.error(id, "enroll", err));
            else send(BridgeCodec.ok(id, state()));
        });
    }

    /** Redeems an enrollment link; null, or the reason it failed, for a person. */
    private String doEnroll(String text) {
        EnrollLink link = EnrollLink.parse(text);
        if (link == null) return getString(R.string.bad_link);
        Credentials have = Credentials.load(this);
        if (have != null) return getString(R.string.enroll_other, have.username, Uri.parse(have.server).getHost());
        try {
            if (EnrollLink.PASSWORD.equals(link.kind)) return checkAndSave(link.server, link.username, link.password, "qr");
            if (EnrollLink.HANDOFF.equals(link.kind)) {
                // A DAVx5 device's QR code: the link its page holds.
                Http.Answer a = Http.call("POST", link.server + "/api/app/davx5", null, null, null,
                        new JSONObject().put("token", link.token).toString(), userAgent());
                if (a.status == 410) return getString(R.string.enroll_expired);
                if (a.status != 200) return getString(R.string.enroll_failed, "HTTP " + a.status);
                EnrollLink inner = EnrollLink.parse(new JSONObject(a.body).optString("link"));
                if (inner == null || !EnrollLink.PASSWORD.equals(inner.kind)) return getString(R.string.bad_link);
                return checkAndSave(link.server, inner.username, inner.password, "qr");
            }
            JSONObject body = new JSONObject().put("token", link.token).put("model", Build.MANUFACTURER + " " + Build.MODEL)
                    .put("android", Build.VERSION.SDK_INT).put("app_version", version());
            Http.Answer a = Http.call("POST", link.server + "/api/app/enroll", null, null, null, body.toString(), userAgent());
            JSONObject o = a.body.isEmpty() ? new JSONObject() : new JSONObject(a.body);
            if (a.status != 200) {
                String code = o.optString("error");
                return code.equals("used") ? getString(R.string.enroll_used)
                        : code.equals("expired") ? getString(R.string.enroll_expired)
                        : getString(R.string.enroll_failed, o.optString("message", "HTTP " + a.status));
            }
            Credentials.save(this, link.server, o.getString("username"), o.getString("secret"), o.getLong("device_id"), "enrolled");
            syncNow();
            return null;
        } catch (IOException e) {
            return getString(R.string.offline);
        } catch (Exception e) {
            return getString(R.string.enroll_failed, String.valueOf(e.getMessage()));
        }
    }

    /**
     * A device's password, checked on CalDAV, then the account: null, or the
     * reason it failed, for a person. mode is "manual" or "qr" (a DAVx5 code).
     */
    private String checkAndSave(String server, String username, String password, String mode) throws IOException {
        Http.Answer a = Http.dav("PROPFIND", server + "/dav/", username, password, "0", "application/xml; charset=utf-8",
                "<?xml version=\"1.0\"?><d:propfind xmlns:d=\"DAV:\"><d:prop><d:current-user-principal/></d:prop></d:propfind>", null, userAgent());
        if (a.status == 401) return getString(R.string.manual_refused);
        if (a.status != 207) return getString(R.string.manual_not_caldav, a.status);
        Credentials.save(this, server, username, password, 0, mode);
        syncNow();
        return null;
    }

    /** By hand: the server, the user and a device's password, checked on CalDAV. */
    private void manual(long id, String serverText, String username, String password) {
        io.execute(() -> {
            String server = ServerAddress.normalize(serverText);
            if (server == null) {
                send(BridgeCodec.error(id, "bad_server", getString(R.string.manual_bad_server)));
                return;
            }
            if (username.isEmpty() || password.isEmpty()) {
                send(BridgeCodec.error(id, "missing", getString(R.string.manual_missing)));
                return;
            }
            if (Credentials.load(this) != null) {
                send(BridgeCodec.error(id, "signed_in", getString(R.string.enroll_other, username, Uri.parse(server).getHost())));
                return;
            }
            try {
                String err = checkAndSave(server, username, password, "manual");
                if (err != null) send(BridgeCodec.error(id, "refused", err));
                else send(BridgeCodec.ok(id, state()));
            } catch (IOException e) {
                send(BridgeCodec.error(id, "offline", getString(R.string.offline)));
            } catch (Exception e) {
                send(BridgeCodec.error(id, "error", getString(R.string.enroll_failed, String.valueOf(e.getMessage()))));
            }
        });
    }

    /**
     * Removes the account: Android deletes its calendars with it. A phone
     * that was enrolled also removes its device on the server, through a
     * session its credential opens (PLAN.md 7.2).
     */
    private void removeAccount(long id) {
        io.execute(() -> {
            Credentials c = Credentials.load(this);
            if (c != null && "enrolled".equals(c.mode)) {
                try {
                    Http.Answer s = Http.call("POST", c.server + "/api/app/session", c.username, c.secret, null, null, userAgent());
                    StringBuilder cookie = new StringBuilder();
                    for (String sc : s.cookies) {
                        if (cookie.length() > 0) cookie.append("; ");
                        cookie.append(sc.split(";", 2)[0]);
                    }
                    if (s.status == 204 || s.status == 200) {
                        Http.call("DELETE", c.server + "/api/devices/" + c.deviceId, null, null, cookie.toString(), null, userAgent());
                    }
                } catch (IOException ignored) {
                    // Offline: the device stays listed in agendouille, where it can be removed.
                }
            }
            Credentials.remove(this);
            SyncStatus.clear(this);
            send(BridgeCodec.ok(id, state()));
        });
    }

    /**
     * The last crash report to the server's log (/api/app/crash): to the
     * account's server, else to the server the screens name (args.server).
     */
    private void sendCrash(long id) {
        io.execute(() -> {
            String report = CrashLog.read(this);
            Credentials c = Credentials.load(this);
            if (report == null || c == null) {
                send(BridgeCodec.error(id, "no_server", getString(R.string.crash_no_server)));
                return;
            }
            try {
                JSONObject body = new JSONObject().put("report", report).put("app_version", version()).put("model", Build.MANUFACTURER + " " + Build.MODEL);
                Http.Answer a = Http.call("POST", c.server + "/api/app/crash", null, null, null, body.toString(), userAgent());
                if (a.status / 100 != 2) throw new IOException("HTTP " + a.status);
                CrashLog.clear(this);
                send(BridgeCodec.ok(id, state()));
            } catch (Exception e) {
                send(BridgeCodec.error(id, "offline", getString(R.string.offline)));
            }
        });
    }

    /** At start and on return: whether the device still exists, and an update. */
    private void checkConfig() {
        Credentials c = Credentials.load(this);
        if (c == null) return;
        io.execute(() -> {
            try {
                Http.Answer a = Http.call("GET", c.server + "/api/app/config", c.username, c.secret, null, null, userAgent());
                if (a.status == 401) {
                    SyncStatus.failed(this, getString(R.string.revoked));
                    return;
                }
                if (a.status != 200) return;
                JSONObject o = new JSONObject(a.body);
                String theirs = o.optString("apk_version");
                if (Versions.updateAvailable(version(), theirs, !o.optString("apk_sha256").isEmpty())) ui.post(() -> offerUpdate(c.server, theirs));
            } catch (Exception ignored) {
                // Offline: the next start will tell.
            }
        });
    }

    private void offerUpdate(String server, String theirs) {
        SharedPreferences p = getSharedPreferences("agendouille", MODE_PRIVATE);
        if (theirs.equals(p.getString("update_dismissed", ""))) return;
        new AlertDialog.Builder(this)
                .setTitle(R.string.update_title)
                .setMessage(getString(R.string.update_body, theirs, version()))
                .setPositiveButton(R.string.update_install, (d, w) -> openOutside(Uri.parse(server + "/app/agendouille.apk")))
                .setNegativeButton(R.string.update_later, (d, w) -> p.edit().putString("update_dismissed", theirs).apply())
                .show();
    }
}
