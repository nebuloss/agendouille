package io.github.nebuloss.agendouille;

import java.net.URI;
import java.net.URISyntaxException;

/**
 * A server address as a person types it ("agenda.example.org",
 * "https://agenda.example.org/dav/", "http://10.0.0.5:8791") made into an
 * origin: https unless http is written, no path. Pure Java: tested on the
 * JVM.
 */
final class ServerAddress {
    private ServerAddress() {}

    static String normalize(String text) {
        if (text == null) return null;
        String t = text.trim();
        if (t.isEmpty() || t.contains(" ")) return null;
        if (!t.contains("://")) t = "https://" + t;
        try {
            URI u = new URI(t);
            String scheme = u.getScheme().toLowerCase();
            if (!scheme.equals("https") && !scheme.equals("http")) return null;
            String host = u.getHost();
            if (host == null || host.isEmpty()) return null;
            return scheme + "://" + host.toLowerCase() + (u.getPort() > 0 ? ":" + u.getPort() : "");
        } catch (URISyntaxException e) {
            return null;
        }
    }
}
