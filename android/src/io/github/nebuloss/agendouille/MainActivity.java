package io.github.nebuloss.agendouille;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.graphics.Typeface;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.CookieManager;
import android.webkit.PermissionRequest;
import android.webkit.URLUtil;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebMessage;
import android.webkit.WebMessagePort;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.IOException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * The app's one screen (PLAN.md 7): the welcome page until the phone is
 * enrolled, then the family web app in a WebView, signed in with the
 * device's credential, and the bridge to it (CONTRACTS.md 6).
 */
public final class MainActivity extends Activity {
    private static final int REQ_CAMERA = 1;
    private static final int REQ_FILE = 2;
    private static final long CHECK_EVERY_MS = 5 * 60 * 1000;

    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final Handler ui = new Handler(Looper.getMainLooper());
    private Credentials cred;
    private WebView web;
    private WebMessagePort port;
    private long pendingScan = -1;
    private ValueCallback<Uri[]> pendingFiles;
    private long lastCheck;
    private boolean checkingSession;

    // --- lifecycle

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        cred = Credentials.load(this);
        handle(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handle(intent);
    }

    @Override
    protected void onResume() {
        super.onResume();
        // A device removed in the web app, on another phone or by the admin:
        // checked again when the app comes back, at most every five minutes.
        if (web != null && cred != null && System.currentTimeMillis() - lastCheck > CHECK_EVERY_MS) checkConfig(false);
    }

    @Override
    protected void onDestroy() {
        io.shutdownNow();
        super.onDestroy();
    }

    // Still how back works without opting in to the predictive back gesture
    // (android:enableOnBackInvokedCallback), which the app does not.
    @Override
    @SuppressWarnings("deprecation")
    public void onBackPressed() {
        if (web != null && web.getParent() != null && web.canGoBack()) web.goBack();
        else super.onBackPressed();
    }

    private void handle(Intent intent) {
        Uri data = intent != null ? intent.getData() : null;
        EnrollLink link = data != null ? EnrollLink.parse(data.toString()) : null;
        if (link != null) {
            if (cred != null && !cred.server.equals(link.server)) {
                showWelcome(getString(R.string.enroll_other, cred.username, Uri.parse(cred.server).getHost()));
                return;
            }
            enroll(link);
        } else if (cred != null) {
            openWeb();
        } else {
            showWelcome(null);
        }
    }

    private String version() {
        try {
            return getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
        } catch (PackageManager.NameNotFoundException e) {
            return "?";
        }
    }

    private String userAgent() {
        return "agendouille-android/" + version() + " (Android " + Build.VERSION.RELEASE + "; " + Build.MODEL + ")";
    }

    // --- the native screens: welcome, and a status with an optional retry

    private int dp(int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, getResources().getDisplayMetrics());
    }

    private TextView text(String s, int sp, boolean bold, int colour) {
        TextView t = new TextView(this);
        t.setText(s);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        t.setTextColor(getColor(colour));
        if (bold) t.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        t.setLineSpacing(0, 1.2f);
        return t;
    }

    private Button button(String label, View.OnClickListener l) {
        Button b = new Button(this);
        b.setText(label);
        b.setAllCaps(false);
        b.setTextColor(getColor(R.color.on_primary));
        b.setBackgroundTintList(android.content.res.ColorStateList.valueOf(getColor(R.color.primary)));
        b.setOnClickListener(l);
        return b;
    }

    private LinearLayout column() {
        LinearLayout col = new LinearLayout(this);
        col.setOrientation(LinearLayout.VERTICAL);
        col.setPadding(dp(28), dp(48), dp(28), dp(28));
        return col;
    }

    private void showWelcome(String message) {
        web = null;
        port = null;
        LinearLayout col = column();
        ImageView icon = new ImageView(this);
        icon.setImageResource(R.mipmap.ic_launcher);
        col.addView(icon, new LinearLayout.LayoutParams(dp(72), dp(72)));
        TextView title = text(getString(R.string.welcome_title), 26, false, R.color.text);
        title.setPadding(0, dp(20), 0, dp(12));
        col.addView(title);
        if (message != null) {
            TextView m = text(message, 15, true, R.color.primary);
            m.setPadding(0, 0, 0, dp(16));
            col.addView(m);
        }
        col.addView(text(getString(R.string.welcome_body), 16, false, R.color.muted));
        TextView paste = text(getString(R.string.welcome_paste), 14, false, R.color.muted);
        paste.setPadding(0, dp(28), 0, dp(6));
        col.addView(paste);
        EditText field = new EditText(this);
        field.setHint(R.string.welcome_paste_hint);
        field.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        field.setSingleLine(true);
        col.addView(field);
        Button go = button(getString(R.string.welcome_continue), v -> {
            EnrollLink link = EnrollLink.parse(field.getText().toString());
            if (link == null) Toast.makeText(this, R.string.bad_link, Toast.LENGTH_LONG).show();
            else enroll(link);
        });
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.gravity = Gravity.END;
        lp.topMargin = dp(12);
        col.addView(go, lp);
        ScrollView scroll = new ScrollView(this);
        scroll.addView(col);
        setContentView(scroll);
    }

    private void showStatus(String message, Runnable retry) {
        LinearLayout col = column();
        col.setGravity(Gravity.CENTER_HORIZONTAL);
        if (retry == null) col.addView(new ProgressBar(this));
        TextView t = text(message, 16, false, R.color.text);
        t.setGravity(Gravity.CENTER);
        t.setPadding(0, dp(20), 0, dp(20));
        col.addView(t);
        if (retry != null) col.addView(button(getString(R.string.retry), v -> retry.run()));
        setContentView(col);
    }

    // --- enrollment (PLAN.md 7.2)

    private void enroll(EnrollLink link) {
        showStatus(getString(R.string.enrolling), null);
        io.execute(() -> {
            try {
                JSONObject body = new JSONObject().put("token", link.token).put("model", Build.MANUFACTURER + " " + Build.MODEL)
                        .put("android", Build.VERSION.SDK_INT).put("app_version", version());
                Http.Answer a = Http.call("POST", link.server + "/api/app/enroll", null, null, null, body.toString(), userAgent());
                JSONObject o = a.body.isEmpty() ? new JSONObject() : new JSONObject(a.body);
                if (a.status != 200) {
                    String code = o.optString("error");
                    String msg = code.equals("used") ? getString(R.string.enroll_used)
                            : code.equals("expired") ? getString(R.string.enroll_expired)
                            : getString(R.string.enroll_failed, o.optString("message", "HTTP " + a.status));
                    ui.post(() -> showWelcome(msg));
                    return;
                }
                Credentials c = Credentials.save(this, link.server, o.getString("username"), o.getString("secret"), o.getLong("device_id"));
                ui.post(() -> {
                    cred = c;
                    openWeb();
                });
            } catch (IOException e) {
                ui.post(() -> showStatus(getString(R.string.offline), () -> enroll(link)));
            } catch (Exception e) {
                ui.post(() -> showWelcome(getString(R.string.enroll_failed, String.valueOf(e.getMessage()))));
            }
        });
    }

    // --- the web app, signed in

    private void openWeb() {
        showStatus(getString(R.string.enrolling), null);
        Credentials c = cred;
        io.execute(() -> {
            try {
                Http.Answer a = Http.call("POST", c.server + "/api/app/session", c.username, c.secret, null, null, userAgent());
                if (a.status == 401 || a.status == 403) {
                    ui.post(this::revoked);
                    return;
                }
                if (a.status != 204 && a.status != 200) throw new IOException("HTTP " + a.status);
                ui.post(() -> {
                    CookieManager cm = CookieManager.getInstance();
                    for (String sc : a.cookies) cm.setCookie(c.server, sc);
                    cm.flush();
                    showWeb(c.server + "/");
                    checkConfig(true);
                });
            } catch (IOException e) {
                ui.post(() -> showStatus(getString(R.string.offline), this::openWeb));
            }
        });
    }

    private boolean ours(Uri u) {
        Uri s = Uri.parse(cred.server);
        return u != null && s.getScheme().equals(u.getScheme()) && s.getHost().equals(u.getHost()) && s.getPort() == u.getPort();
    }

    private void showWeb(String url) {
        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) WebView.setWebContentsDebuggingEnabled(true);
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        s.setUserAgentString(s.getUserAgentString() + " agendouille-android/" + version());
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                Uri u = req.getUrl();
                if (ours(u)) {
                    // Sent to the sign-in page: the session is gone (the device
                    // removed, or the session expired). The app signs in again,
                    // or signs out if the device is no more.
                    if ("/signin".equals(u.getPath())) {
                        sessionLost();
                        return true;
                    }
                    return false;
                }
                openOutside(u);
                return true;
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                if (ours(Uri.parse(url))) {
                    if ("/signin".equals(Uri.parse(url).getPath())) sessionLost();
                    else connectBridge();
                }
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onPermissionRequest(PermissionRequest r) {
                // The page's scanner: the camera, for our origin, once Android
                // has given it to the app (the bridge's scan asks).
                boolean video = false;
                for (String res : r.getResources()) if (PermissionRequest.RESOURCE_VIDEO_CAPTURE.equals(res)) video = true;
                if (video && ours(r.getOrigin()) && checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
                    r.grant(new String[]{PermissionRequest.RESOURCE_VIDEO_CAPTURE});
                } else {
                    r.deny();
                }
            }

            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                // The .ics import's file field.
                if (pendingFiles != null) pendingFiles.onReceiveValue(null);
                pendingFiles = callback;
                try {
                    startActivityForResult(params.createIntent(), REQ_FILE);
                } catch (ActivityNotFoundException e) {
                    pendingFiles = null;
                    return false;
                }
                return true;
            }
        });
        web.setDownloadListener((link, ua, disposition, mime, length) -> download(link, disposition, mime));
        setContentView(web);
        web.loadUrl(url);
    }

    private void openOutside(Uri u) {
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, u));
        } catch (ActivityNotFoundException e) {
            Toast.makeText(this, R.string.no_browser, Toast.LENGTH_LONG).show();
        }
    }

    /** An export (.ics, a calendar's zip): Android's downloads, with the session. */
    private void download(String url, String disposition, String mime) {
        Uri u = Uri.parse(url);
        if (!ours(u)) {
            openOutside(u);
            return;
        }
        DownloadManager.Request r = new DownloadManager.Request(u);
        r.addRequestHeader("Cookie", CookieManager.getInstance().getCookie(url));
        r.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
        r.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, URLUtil.guessFileName(url, disposition, mime));
        ((DownloadManager) getSystemService(DOWNLOAD_SERVICE)).enqueue(r);
        Toast.makeText(this, R.string.download_started, Toast.LENGTH_SHORT).show();
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == REQ_FILE && pendingFiles != null) {
            pendingFiles.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(result, data));
            pendingFiles = null;
        }
    }

    /** The WebView's session is gone: a new one, or signed out. */
    private void sessionLost() {
        if (checkingSession) return;
        checkingSession = true;
        openWeb();
        ui.postDelayed(() -> checkingSession = false, 2000);
    }

    private void revoked() {
        Credentials.remove(this);
        cred = null;
        CookieManager.getInstance().removeAllCookies(null);
        showWelcome(getString(R.string.revoked));
    }

    /**
     * /api/app/config: whether the device still exists (401 when removed),
     * and whether the server holds another version of the app.
     */
    private void checkConfig(boolean offerUpdate) {
        Credentials c = cred;
        if (c == null) return;
        lastCheck = System.currentTimeMillis();
        io.execute(() -> {
            try {
                Http.Answer a = Http.call("GET", c.server + "/api/app/config", c.username, c.secret, null, null, userAgent());
                if (a.status == 401) {
                    ui.post(this::revoked);
                    return;
                }
                if (a.status != 200 || !offerUpdate) return;
                JSONObject o = new JSONObject(a.body);
                String theirs = o.optString("apk_version");
                boolean apk = !o.optString("apk_sha256").isEmpty();
                if (Versions.updateAvailable(version(), theirs, apk)) ui.post(() -> offerUpdate(c.server, theirs));
            } catch (Exception ignored) {
                // Offline: the next check will tell.
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

    // --- the bridge (CONTRACTS.md 6)

    private void connectBridge() {
        if (web == null) return;
        WebMessagePort[] ch = web.createWebMessageChannel();
        port = ch[0];
        port.setWebMessageCallback(new WebMessagePort.WebMessageCallback() {
            @Override
            public void onMessage(WebMessagePort p, WebMessage m) {
                onBridge(m.getData());
            }
        });
        web.postWebMessage(new WebMessage("agendouille-bridge", new WebMessagePort[]{ch[1]}), Uri.parse(cred.server));
    }

    private void send(String text) {
        ui.post(() -> {
            if (port != null) port.postMessage(new WebMessage(text));
        });
    }

    private void onBridge(String text) {
        BridgeCodec.Request r = BridgeCodec.parse(text);
        if (r == null) return;
        switch (r.call) {
            case "hello":
                send(BridgeCodec.ok(r.id, BridgeCodec.hello(version(), Build.VERSION.SDK_INT)));
                break;
            case "scan":
                if (checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
                    send(BridgeCodec.ok(r.id, null));
                } else {
                    pendingScan = r.id;
                    requestPermissions(new String[]{Manifest.permission.CAMERA}, REQ_CAMERA);
                }
                break;
            case "sync.status":
                // The sync adapter comes with M7.
                send(BridgeCodec.ok(r.id, new JSONObject()));
                break;
            case "sync.now":
                send(BridgeCodec.error(r.id, "not_available"));
                break;
            case "davx5.handoff":
                davx5(r.id);
                break;
            case "account.remove":
                removeAccount(r.id);
                break;
            default:
                send(BridgeCodec.error(r.id, "unknown_call"));
        }
    }

    @Override
    public void onRequestPermissionsResult(int request, String[] permissions, int[] results) {
        if (request != REQ_CAMERA || pendingScan < 0) return;
        long id = pendingScan;
        pendingScan = -1;
        if (results.length > 0 && results[0] == PackageManager.PERMISSION_GRANTED) send(BridgeCodec.ok(id, null));
        else send(BridgeCodec.error(id, "camera_denied"));
    }

    /** A DAVx5 device for this user, and DAVx5 opened with it. */
    private void davx5(long id) {
        try {
            getPackageManager().getPackageInfo("at.bitfire.davdroid", 0);
        } catch (PackageManager.NameNotFoundException e) {
            send(BridgeCodec.error(id, "davx5_missing"));
            return;
        }
        Credentials c = cred;
        String cookie = CookieManager.getInstance().getCookie(c.server);
        io.execute(() -> {
            try {
                JSONObject body = new JSONObject().put("kind", "davx5").put("label", Build.MODEL + " · DAVx5");
                Http.Answer a = Http.call("POST", c.server + "/api/devices", null, null, cookie, body.toString(), userAgent());
                if (a.status != 201) {
                    send(BridgeCodec.error(id, "server"));
                    return;
                }
                String uri = new JSONObject(a.body).optString("davx5_uri");
                ui.post(() -> openOutside(Uri.parse(uri)));
                send(BridgeCodec.ok(id, null));
            } catch (Exception e) {
                send(BridgeCodec.error(id, "offline"));
            }
        });
    }

    /** Removes this phone: its device on the server, then the account here. */
    private void removeAccount(long id) {
        Credentials c = cred;
        String cookie = CookieManager.getInstance().getCookie(c.server);
        send(BridgeCodec.ok(id, null));
        io.execute(() -> {
            try {
                Http.call("DELETE", c.server + "/api/devices/" + c.deviceId, null, null, cookie, null, userAgent());
            } catch (IOException ignored) {
                // Offline: the device stays listed in the web app, where it can be removed.
            }
            ui.post(() -> {
                Credentials.remove(this);
                cred = null;
                CookieManager.getInstance().removeAllCookies(null);
                showWelcome(null);
            });
        });
    }
}
