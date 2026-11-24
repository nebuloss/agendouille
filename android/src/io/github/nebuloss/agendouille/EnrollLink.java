package io.github.nebuloss.agendouille;

import java.net.URI;
import java.net.URISyntaxException;

/**
 * An enrollment link (PLAN.md 7.2), as the app is given it: the admin's
 * https://host/enroll/TOKEN (an App Link, or pasted), the older
 * https://host/enrol/TOKEN, or agendouille://enroll?u=&lt;that link&gt; (the
 * enrollment page's "open in the app", for any server). Also what a DAVx5
 * device is given, so its QR code signs the app in too: the handoff page
 * https://host/davx5/TOKEN (kind HANDOFF: the server gives the link it
 * holds), and the link itself, davx5://user:password@host/dav/ or
 * caldavs://, with a device's password (kind PASSWORD). Pure Java: tested on
 * the JVM.
 */
public final class EnrollLink {
    public static final String ENROLL = "enroll", HANDOFF = "handoff", PASSWORD = "password";

    /** ENROLL, HANDOFF or PASSWORD. */
    public final String kind;
    /** The server's origin, "https://host[:port]", no trailing slash. */
    public final String server;
    /** The enrollment's or the handoff's token; null for PASSWORD. */
    public final String token;
    /** A device's credentials, for PASSWORD only. */
    public final String username, password;

    private EnrollLink(String kind, String server, String token, String username, String password) {
        this.kind = kind;
        this.server = server;
        this.token = token;
        this.username = username;
        this.password = password;
    }

    /** Parses a link, or returns null when it is not one. */
    public static EnrollLink parse(String text) {
        if (text == null) return null;
        text = text.trim();
        try {
            URI u = new URI(text);
            if ("agendouille".equals(u.getScheme()) && "enroll".equals(u.getHost())) {
                String q = u.getRawQuery();
                if (q == null) return null;
                for (String part : q.split("&")) {
                    if (part.startsWith("u=")) return parse(java.net.URLDecoder.decode(part.substring(2), "UTF-8"));
                }
                return null;
            }
            String scheme = u.getScheme();
            if ("davx5".equals(scheme) || "caldavs".equals(scheme)) return withPassword(u);
            if (!"https".equals(scheme) && !"http".equals(scheme)) return null;
            if (u.getHost() == null) return null;
            String path = u.getRawPath();
            if (path == null) return null;
            String token, kind = ENROLL;
            if (path.startsWith("/enroll/")) token = path.substring("/enroll/".length());
            else if (path.startsWith("/enrol/")) token = path.substring("/enrol/".length());
            else if (path.startsWith("/davx5/")) {
                token = path.substring("/davx5/".length());
                kind = HANDOFF;
            } else return null;
            if (token.endsWith("/")) token = token.substring(0, token.length() - 1);
            if (!token.matches("[A-Za-z0-9_-]{16,}")) return null;
            String origin = scheme + "://" + u.getHost() + (u.getPort() > 0 ? ":" + u.getPort() : "");
            return new EnrollLink(kind, origin, token, null, null);
        } catch (URISyntaxException | java.io.UnsupportedEncodingException e) {
            return null;
        }
    }

    /** davx5://user:password@host[:port]/path: always https, as DAVx5 reads it. */
    private static EnrollLink withPassword(URI u) throws java.io.UnsupportedEncodingException {
        String info = u.getRawUserInfo();
        if (u.getHost() == null || info == null) return null;
        int colon = info.indexOf(':');
        if (colon <= 0 || colon == info.length() - 1) return null;
        String user = java.net.URLDecoder.decode(info.substring(0, colon), "UTF-8");
        String pass = java.net.URLDecoder.decode(info.substring(colon + 1), "UTF-8");
        String origin = "https://" + u.getHost() + (u.getPort() > 0 ? ":" + u.getPort() : "");
        return new EnrollLink(PASSWORD, origin, null, user, pass);
    }
}
