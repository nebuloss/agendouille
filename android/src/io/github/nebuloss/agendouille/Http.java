package io.github.nebuloss.agendouille;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * The few HTTP calls the Java side makes (/api/app/*, and two on the
 * WebView's behalf), with HttpURLConnection: in the framework, no library.
 * Called off the main thread.
 */
final class Http {
    private Http() {}

    static final class Answer {
        final int status;
        final String body;
        final List<String> cookies;
        String etag;

        Answer(int status, String body, List<String> cookies) {
            this.status = status;
            this.body = body;
            this.cookies = cookies;
        }
    }

    /** A CalDAV request (RawHttp: PROPFIND and REPORT too), with the device's Basic credential. */
    static Answer dav(String method, String url, String user, String pass, String depth, String contentType, String body, String ifMatch, String userAgent) throws IOException {
        java.util.Map<String, String> h = new java.util.LinkedHashMap<>();
        h.put("User-Agent", userAgent);
        String raw = user + ":" + pass;
        h.put("Authorization", "Basic " + android.util.Base64.encodeToString(raw.getBytes(StandardCharsets.UTF_8), android.util.Base64.NO_WRAP));
        h.put("Accept-Language", Locale.getDefault().toLanguageTag());
        if (depth != null) h.put("Depth", depth);
        if (ifMatch != null) h.put("If-Match", ifMatch);
        if (body != null) h.put("Content-Type", contentType);
        RawHttp.Answer r = RawHttp.request(null, method, url, h, body == null ? null : body.getBytes(StandardCharsets.UTF_8));
        Answer a = new Answer(r.status, r.text(), new ArrayList<>());
        a.etag = r.headers.get("etag");
        return a;
    }

    /**
     * One request. user and pass, when set, are Basic credentials (the
     * device's); cookie, when set, is the WebView's session; json is the
     * body. The answer carries the Set-Cookie headers.
     */
    static Answer call(String method, String url, String user, String pass, String cookie, String json, String userAgent) throws IOException {
        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
        try {
            c.setRequestMethod(method);
            c.setConnectTimeout(15000);
            c.setReadTimeout(30000);
            c.setInstanceFollowRedirects(false);
            c.setRequestProperty("User-Agent", userAgent);
            c.setRequestProperty("Accept", "application/json");
            // The server writes its error messages in this language.
            c.setRequestProperty("Accept-Language", Locale.getDefault().toLanguageTag());
            if (user != null) {
                String raw = user + ":" + pass;
                c.setRequestProperty("Authorization", "Basic " + android.util.Base64.encodeToString(raw.getBytes(StandardCharsets.UTF_8), android.util.Base64.NO_WRAP));
            }
            if (cookie != null) c.setRequestProperty("Cookie", cookie);
            if (json != null) {
                byte[] b = json.getBytes(StandardCharsets.UTF_8);
                c.setDoOutput(true);
                c.setRequestProperty("Content-Type", "application/json");
                c.setFixedLengthStreamingMode(b.length);
                try (OutputStream out = c.getOutputStream()) {
                    out.write(b);
                }
            }
            int status = c.getResponseCode();
            InputStream in = status >= 400 ? c.getErrorStream() : c.getInputStream();
            String body = in == null ? "" : read(in);
            List<String> cookies = new ArrayList<>();
            for (Map.Entry<String, List<String>> h : c.getHeaderFields().entrySet()) {
                if (h.getKey() != null && h.getKey().equalsIgnoreCase("Set-Cookie")) cookies.addAll(h.getValue());
            }
            return new Answer(status, body, cookies);
        } finally {
            c.disconnect();
        }
    }

    private static String read(InputStream in) throws IOException {
        try (InputStream i = in) {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            int n;
            while ((n = i.read(buf)) > 0) {
                out.write(buf, 0, n);
                if (out.size() > 1 << 20) throw new IOException("answer too large");
            }
            return new String(out.toByteArray(), StandardCharsets.UTF_8);
        }
    }
}
