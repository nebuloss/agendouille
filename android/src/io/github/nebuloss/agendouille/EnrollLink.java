package io.github.nebuloss.agendouille;

import java.net.URI;
import java.net.URISyntaxException;

/**
 * An enrollment link (PLAN.md 7.2), as the app is given it: the admin's
 * https://host/enroll/TOKEN (an App Link, or pasted), the older
 * https://host/enrol/TOKEN, or agendouille://enroll?u=&lt;that link&gt; (the
 * enrollment page's "open in the app", for any server). Pure Java: tested on
 * the JVM.
 */
public final class EnrollLink {
    /** The server's origin, "https://host[:port]", no trailing slash. */
    public final String server;
    public final String token;

    private EnrollLink(String server, String token) {
        this.server = server;
        this.token = token;
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
            if (!"https".equals(scheme) && !"http".equals(scheme)) return null;
            if (u.getHost() == null) return null;
            String path = u.getRawPath();
            if (path == null) return null;
            String token;
            if (path.startsWith("/enroll/")) token = path.substring("/enroll/".length());
            else if (path.startsWith("/enrol/")) token = path.substring("/enrol/".length());
            else return null;
            if (token.endsWith("/")) token = token.substring(0, token.length() - 1);
            if (!token.matches("[A-Za-z0-9_-]{16,}")) return null;
            String origin = scheme + "://" + u.getHost() + (u.getPort() > 0 ? ":" + u.getPort() : "");
            return new EnrollLink(origin, token);
        } catch (URISyntaxException | java.io.UnsupportedEncodingException e) {
            return null;
        }
    }
}
