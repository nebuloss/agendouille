package io.github.nebuloss.agendouille;

import java.io.BufferedInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;

import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLSocket;
import javax.net.ssl.SSLSocketFactory;

/**
 * One HTTP/1.1 request on its own connection, any method. CalDAV needs
 * PROPFIND and REPORT, which Android's HttpURLConnection refuses (it knows
 * GET to PATCH only), and reflection on it fails for https, where the
 * connection is a wrapper. TLS is the platform's, with its trust store and
 * hostname check; a client certificate can come from the socket factory.
 * Pure Java: tested on the JVM against a local server.
 */
final class RawHttp {
    private RawHttp() {}

    static final int MAX_BODY = 16 << 20;

    static final class Answer {
        final int status;
        final Map<String, String> headers; // lower-case names, the last value
        final byte[] body;

        Answer(int status, Map<String, String> headers, byte[] body) {
            this.status = status;
            this.headers = headers;
            this.body = body;
        }

        String text() {
            return new String(body, StandardCharsets.UTF_8);
        }
    }

    static Answer request(SSLSocketFactory tls, String method, String url, Map<String, String> headers, byte[] body) throws IOException {
        URI u = URI.create(url);
        boolean https = "https".equals(u.getScheme());
        if (!https && !"http".equals(u.getScheme())) throw new IOException("not http: " + url);
        String host = u.getHost();
        int port = u.getPort() > 0 ? u.getPort() : (https ? 443 : 80);
        Socket s = new Socket();
        try {
            s.connect(new InetSocketAddress(host, port), 15000);
            s.setSoTimeout(60000);
            if (https) {
                SSLSocket ssl = (SSLSocket) (tls != null ? tls : (SSLSocketFactory) SSLSocketFactory.getDefault()).createSocket(s, host, port, true);
                ssl.startHandshake();
                if (!HttpsURLConnection.getDefaultHostnameVerifier().verify(host, ssl.getSession())) {
                    throw new IOException("the certificate is not for " + host);
                }
                s = ssl;
            }
            String path = u.getRawPath() == null || u.getRawPath().isEmpty() ? "/" : u.getRawPath();
            if (u.getRawQuery() != null) path += "?" + u.getRawQuery();
            StringBuilder head = new StringBuilder();
            head.append(method).append(' ').append(path).append(" HTTP/1.1\r\n");
            head.append("Host: ").append(host).append(u.getPort() > 0 ? ":" + u.getPort() : "").append("\r\n");
            head.append("Connection: close\r\n");
            for (Map.Entry<String, String> h : headers.entrySet()) {
                if (h.getValue().indexOf('\r') >= 0 || h.getValue().indexOf('\n') >= 0) throw new IOException("a header with a line break");
                head.append(h.getKey()).append(": ").append(h.getValue()).append("\r\n");
            }
            if (body != null) head.append("Content-Length: ").append(body.length).append("\r\n");
            head.append("\r\n");
            OutputStream out = s.getOutputStream();
            out.write(head.toString().getBytes(StandardCharsets.UTF_8));
            if (body != null) out.write(body);
            out.flush();
            return read(new BufferedInputStream(s.getInputStream()), "HEAD".equals(method));
        } finally {
            s.close();
        }
    }

    static Answer read(InputStream in, boolean noBody) throws IOException {
        String status = line(in);
        String[] parts = status.split(" ", 3);
        if (parts.length < 2 || !parts[0].startsWith("HTTP/1.")) throw new IOException("not an HTTP answer: " + status);
        int code;
        try {
            code = Integer.parseInt(parts[1]);
        } catch (NumberFormatException e) {
            throw new IOException("not an HTTP status: " + status);
        }
        Map<String, String> headers = new LinkedHashMap<>();
        for (String l = line(in); !l.isEmpty(); l = line(in)) {
            int i = l.indexOf(':');
            if (i > 0) headers.put(l.substring(0, i).trim().toLowerCase(Locale.ROOT), l.substring(i + 1).trim());
        }
        if (code == 100) return read(in, noBody);
        if (noBody || code == 204 || code == 304) return new Answer(code, headers, new byte[0]);
        ByteArrayOutputStream body = new ByteArrayOutputStream();
        if ("chunked".equalsIgnoreCase(headers.get("transfer-encoding"))) {
            for (;;) {
                String size = line(in);
                int semi = size.indexOf(';');
                int n = Integer.parseInt((semi >= 0 ? size.substring(0, semi) : size).trim(), 16);
                if (n == 0) {
                    while (!line(in).isEmpty()) { /* trailers */ }
                    break;
                }
                copy(in, body, n);
                line(in);
            }
        } else if (headers.containsKey("content-length")) {
            copy(in, body, Integer.parseInt(headers.get("content-length")));
        } else {
            byte[] buf = new byte[8192];
            for (int n; (n = in.read(buf)) > 0; ) {
                body.write(buf, 0, n);
                if (body.size() > MAX_BODY) throw new IOException("answer too large");
            }
        }
        return new Answer(code, headers, body.toByteArray());
    }

    private static void copy(InputStream in, ByteArrayOutputStream out, int n) throws IOException {
        if (n < 0 || out.size() + n > MAX_BODY) throw new IOException("answer too large");
        byte[] buf = new byte[8192];
        while (n > 0) {
            int r = in.read(buf, 0, Math.min(buf.length, n));
            if (r < 0) throw new IOException("the answer ended early");
            out.write(buf, 0, r);
            n -= r;
        }
    }

    private static String line(InputStream in) throws IOException {
        ByteArrayOutputStream b = new ByteArrayOutputStream();
        for (int c; (c = in.read()) != '\n'; ) {
            if (c < 0) {
                if (b.size() == 0) throw new IOException("the connection closed");
                break;
            }
            if (c != '\r') b.write(c);
            if (b.size() > 16384) throw new IOException("a header line too long");
        }
        return new String(b.toByteArray(), StandardCharsets.ISO_8859_1);
    }
}
