package io.github.nebuloss.agendouille;

import org.xmlpull.v1.XmlPullParser;
import org.xmlpull.v1.XmlPullParserException;
import org.xmlpull.v1.XmlPullParserFactory;

import java.io.IOException;
import java.io.StringReader;
import java.net.URI;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * The CalDAV the app speaks (PLAN.md 7.3): plain RFC 4918/4791/6578, what
 * DAVx5 sends too, nothing only agendouille understands. Requests go
 * through {@link Transport}; the multistatus answers are read with the
 * framework's XmlPullParser (kXML on the JVM tests).
 */
final class Dav {
    static final String NS_DAV = "DAV:";
    static final String NS_CALDAV = "urn:ietf:params:xml:ns:caldav";
    static final String NS_CS = "http://calendarserver.org/ns/";
    static final String NS_APPLE = "http://apple.com/ns/ical/";

    /** Sends one request; the app's is HttpURLConnection with the device's credential. */
    interface Transport {
        Answer send(String method, String url, String depth, String contentType, String body) throws IOException;
    }

    static final class Answer {
        final int status;
        final String body;
        final String etag;

        Answer(int status, String body, String etag) {
            this.status = status;
            this.body = body;
            this.etag = etag;
        }
    }

    /** One <response>: its href and the properties found (200), by "ns|name". */
    static final class Response {
        String href = "";
        final Map<String, Prop> props = new LinkedHashMap<>();
        int status = 200; // the response's own status, for a removed member in a sync report

        Prop get(String ns, String name) {
            return props.get(ns + "|" + name);
        }
    }

    /** A property's value: its text, the hrefs and the child elements inside it. */
    static final class Prop {
        final StringBuilder text = new StringBuilder();
        final List<String> hrefs = new ArrayList<>();
        final List<String> children = new ArrayList<>(); // "ns|name", at any depth

        String text() {
            return text.toString().trim();
        }

        boolean has(String ns, String name) {
            return children.contains(ns + "|" + name);
        }
    }

    /** A calendar collection as discovery finds it. */
    static final class Collection {
        String url;
        String name;
        String color; // #rrggbb, or "" when the server has none
        boolean readOnly;
        String ctag;
        String syncToken;
    }

    private final Transport transport;
    private final String base; // https://host[:port]

    Dav(Transport transport, String base) {
        this.transport = transport;
        this.base = base;
    }

    /** An href, made absolute against the server. */
    String absolute(String href) {
        return URI.create(base + "/").resolve(href).toString();
    }

    List<Response> propfind(String url, String depth, String props) throws IOException {
        String body = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<d:propfind xmlns:d=\"DAV:\" xmlns:c=\"" + NS_CALDAV
                + "\" xmlns:cs=\"" + NS_CS + "\" xmlns:a=\"" + NS_APPLE + "\"><d:prop>" + props + "</d:prop></d:propfind>";
        Answer a = transport.send("PROPFIND", url, depth, "application/xml; charset=utf-8", body);
        if (a.status == 401) throw new Unauthorized();
        if (a.status != 207) throw new IOException("PROPFIND " + url + ": HTTP " + a.status);
        return parseMultistatus(a.body);
    }

    /** Thrown when the server refuses the credential: the device is gone. */
    static final class Unauthorized extends IOException {
        Unauthorized() {
            super("unauthorized");
        }
    }

    /** The calendars of the signed-in user (RFC 4791 6.2.1, then a Depth 1 PROPFIND). */
    List<Collection> calendars() throws IOException {
        String principal = null;
        for (Response r : propfind(base + "/dav/", "0", "<d:current-user-principal/>")) {
            Prop p = r.get(NS_DAV, "current-user-principal");
            if (p != null && !p.hrefs.isEmpty()) principal = absolute(p.hrefs.get(0));
        }
        if (principal == null) throw new IOException("no current-user-principal");
        String home = null;
        for (Response r : propfind(principal, "0", "<c:calendar-home-set/>")) {
            Prop p = r.get(NS_CALDAV, "calendar-home-set");
            if (p != null && !p.hrefs.isEmpty()) home = absolute(p.hrefs.get(0));
        }
        if (home == null) throw new IOException("no calendar-home-set");
        List<Collection> out = new ArrayList<>();
        for (Response r : propfind(home, "1", "<d:resourcetype/><d:displayname/><a:calendar-color/><d:current-user-privilege-set/>"
                + "<c:supported-calendar-component-set/><cs:getctag/><d:sync-token/>")) {
            Prop type = r.get(NS_DAV, "resourcetype");
            if (type == null || !type.has(NS_CALDAV, "calendar")) continue;
            Prop comps = r.get(NS_CALDAV, "supported-calendar-component-set");
            if (comps != null && !comps.children.isEmpty() && !comps.children.contains(NS_CALDAV + "|comp")) continue;
            Collection c = new Collection();
            c.url = absolute(r.href);
            Prop name = r.get(NS_DAV, "displayname");
            c.name = name != null && !name.text().isEmpty() ? name.text() : lastSegment(c.url);
            Prop color = r.get(NS_APPLE, "calendar-color");
            c.color = color != null ? normalizeColor(color.text()) : "";
            Prop priv = r.get(NS_DAV, "current-user-privilege-set");
            c.readOnly = priv != null && !(priv.has(NS_DAV, "write") || priv.has(NS_DAV, "write-content") || priv.has(NS_DAV, "all"));
            Prop ctag = r.get(NS_CS, "getctag");
            c.ctag = ctag != null ? ctag.text() : "";
            Prop token = r.get(NS_DAV, "sync-token");
            c.syncToken = token != null ? token.text() : "";
            out.add(c);
        }
        return out;
    }

    static String lastSegment(String url) {
        String u = url.endsWith("/") ? url.substring(0, url.length() - 1) : url;
        return u.substring(u.lastIndexOf('/') + 1);
    }

    /** "#RRGGBBAA" or "#RRGGBB" to "#rrggbb"; anything else to "". */
    static String normalizeColor(String c) {
        c = c.trim().toLowerCase();
        if (c.matches("#[0-9a-f]{8}")) return c.substring(0, 7);
        if (c.matches("#[0-9a-f]{6}")) return c;
        return "";
    }

    /**
     * Reads a multistatus (RFC 4918 13): multistatus / response / (href,
     * status | propstat / (prop / property, status)). Each response keeps
     * the properties of its 2xx propstats; a response's own status (a
     * member gone, in a sync-collection report) is kept too.
     */
    static List<Response> parseMultistatus(String xml) throws IOException {
        List<Response> out = new ArrayList<>();
        try {
            XmlPullParserFactory f = XmlPullParserFactory.newInstance();
            f.setNamespaceAware(true);
            XmlPullParser p = f.newPullParser();
            p.setInput(new StringReader(xml));
            Response cur = null;
            boolean inProp = false;
            Prop prop = null;
            String propKey = null;
            Map<String, Prop> found = new LinkedHashMap<>();
            boolean okPropstat = true;
            StringBuilder text = new StringBuilder();
            for (int ev = p.getEventType(); ev != XmlPullParser.END_DOCUMENT; ev = p.next()) {
                int depth = p.getDepth();
                if (ev == XmlPullParser.START_TAG) {
                    String key = p.getNamespace() + "|" + p.getName();
                    text.setLength(0);
                    if (depth == 2 && key.equals(NS_DAV + "|response")) {
                        cur = new Response();
                    } else if (cur != null && depth == 3 && key.equals(NS_DAV + "|propstat")) {
                        okPropstat = true;
                        found.clear();
                    } else if (cur != null && depth == 4 && key.equals(NS_DAV + "|prop")) {
                        inProp = true;
                    } else if (inProp && depth == 5) {
                        prop = new Prop();
                        propKey = key;
                    } else if (prop != null) {
                        prop.children.add(key);
                    }
                } else if (ev == XmlPullParser.TEXT) {
                    text.append(p.getText());
                    if (prop != null && depth == 5) prop.text.append(p.getText());
                } else if (ev == XmlPullParser.END_TAG) {
                    String key = p.getNamespace() + "|" + p.getName();
                    String value = text.toString().trim();
                    if (prop != null && depth == 5) {
                        found.put(propKey, prop);
                        prop = null;
                    } else if (prop != null && key.equals(NS_DAV + "|href")) {
                        prop.hrefs.add(value);
                    } else if (inProp && depth == 4 && key.equals(NS_DAV + "|prop")) {
                        inProp = false;
                    } else if (cur != null && depth == 4 && key.equals(NS_DAV + "|status")) {
                        okPropstat = statusOf(value) / 100 == 2;
                    } else if (cur != null && depth == 3 && key.equals(NS_DAV + "|propstat")) {
                        if (okPropstat) cur.props.putAll(found);
                    } else if (cur != null && depth == 3 && key.equals(NS_DAV + "|href")) {
                        cur.href = value;
                    } else if (cur != null && depth == 3 && key.equals(NS_DAV + "|status")) {
                        cur.status = statusOf(value);
                    } else if (cur != null && depth == 2 && key.equals(NS_DAV + "|response")) {
                        out.add(cur);
                        cur = null;
                    }
                    text.setLength(0);
                }
            }
        } catch (XmlPullParserException e) {
            throw new IOException("a multistatus that is not XML: " + e.getMessage());
        }
        return out;
    }

    /** "HTTP/1.1 404 Not Found" to 404; 0 when it is not a status line. */
    static int statusOf(String line) {
        String[] parts = line.trim().split(" ");
        try {
            return parts.length > 1 ? Integer.parseInt(parts[1]) : 0;
        } catch (NumberFormatException e) {
            return 0;
        }
    }
}
