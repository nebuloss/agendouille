package io.github.nebuloss.agendouille;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.io.IOException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

import org.junit.Test;

/** The CalDAV the app speaks, against canned answers shaped like agendouille's. */
public class DavTest {
    /** Answers by method and URL, and records what was sent. */
    static final class Fake implements Dav.Transport {
        final List<String> sent = new ArrayList<>();
        final java.util.Map<String, Dav.Answer> answers = new java.util.HashMap<>();

        Fake on(String method, String url, int status, String body) {
            answers.put(method + " " + url, new Dav.Answer(status, body, null));
            return this;
        }

        @Override
        public Dav.Answer send(String method, String url, String depth, String type, String body) throws IOException {
            sent.add(method + " " + url + " depth=" + depth + " " + body);
            Dav.Answer a = answers.get(method + " " + url);
            if (a == null) throw new IOException("unexpected " + method + " " + url);
            return a;
        }
    }

    private static final String B = "https://agenda.example.org";

    @Test
    public void discovery() throws Exception {
        Fake f = new Fake()
                .on("PROPFIND", B + "/dav/", 207, "<?xml version=\"1.0\"?><d:multistatus xmlns:d=\"DAV:\"><d:response><d:href>/dav/</d:href>"
                        + "<d:propstat><d:prop><d:current-user-principal><d:href>/dav/principals/lea/</d:href></d:current-user-principal></d:prop>"
                        + "<d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>")
                .on("PROPFIND", B + "/dav/principals/lea/", 207, "<d:multistatus xmlns:d=\"DAV:\" xmlns:c=\"urn:ietf:params:xml:ns:caldav\"><d:response>"
                        + "<d:href>/dav/principals/lea/</d:href><d:propstat><d:prop><c:calendar-home-set><d:href>/dav/calendars/lea/</d:href>"
                        + "</c:calendar-home-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>")
                .on("PROPFIND", B + "/dav/calendars/lea/", 207, "<d:multistatus xmlns:d=\"DAV:\" xmlns:c=\"urn:ietf:params:xml:ns:caldav\" "
                        + "xmlns:a=\"http://apple.com/ns/ical/\" xmlns:cs=\"http://calendarserver.org/ns/\">"
                        + "<d:response><d:href>/dav/calendars/lea/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>"
                        + "<d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>"
                        + "<d:response><d:href>/dav/calendars/lea/lea~perso/</d:href><d:propstat><d:prop>"
                        + "<d:resourcetype><d:collection/><c:calendar/></d:resourcetype><d:displayname>Perso &amp; famille</d:displayname>"
                        + "<a:calendar-color>#7B1FA2FF</a:calendar-color><d:current-user-privilege-set><d:privilege><d:read/></d:privilege>"
                        + "<d:privilege><d:write/></d:privilege></d:current-user-privilege-set><cs:getctag>c1</cs:getctag><d:sync-token>t1</d:sync-token>"
                        + "</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat><d:propstat><d:prop><c:calendar-timezone/></d:prop>"
                        + "<d:status>HTTP/1.1 404 Not Found</d:status></d:propstat></d:response>"
                        + "<d:response><d:href>/dav/calendars/lea/maman~ecole/</d:href><d:propstat><d:prop>"
                        + "<d:resourcetype><d:collection/><c:calendar/></d:resourcetype><d:displayname>École</d:displayname>"
                        + "<d:current-user-privilege-set><d:privilege><d:read/></d:privilege></d:current-user-privilege-set>"
                        + "</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>");
        List<Dav.Collection> cals = new Dav(f, B).calendars();
        assertEquals(2, cals.size());
        Dav.Collection perso = cals.get(0);
        assertEquals(B + "/dav/calendars/lea/lea~perso/", perso.url);
        assertEquals("Perso & famille", perso.name);
        assertEquals("#7b1fa2", perso.color);
        assertFalse(perso.readOnly);
        assertEquals("t1", perso.syncToken);
        Dav.Collection ecole = cals.get(1);
        assertTrue(ecole.readOnly);
        assertEquals("", ecole.color);
        assertTrue(f.sent.get(0).startsWith("PROPFIND " + B + "/dav/ depth=0"));
    }

    @Test
    public void syncAndMultiget() throws Exception {
        String cal = B + "/dav/calendars/lea/lea~perso/";
        Fake f = new Fake()
                .on("REPORT", cal, 207, "<d:multistatus xmlns:d=\"DAV:\">"
                        + "<d:response><d:href>/dav/calendars/lea/lea~perso/judo.ics</d:href><d:propstat><d:prop><d:getetag>\"e1\"</d:getetag></d:prop>"
                        + "<d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>"
                        + "<d:response><d:href>/dav/calendars/lea/lea~perso/old.ics</d:href><d:status>HTTP/1.1 404 Not Found</d:status></d:response>"
                        + "<d:sync-token>t2</d:sync-token></d:multistatus>");
        Dav dav = new Dav(f, B);
        Dav.Changes ch = dav.changes(cal, "t1");
        assertEquals("t2", ch.token);
        assertEquals(2, ch.members.size());
        assertEquals("\"e1\"", ch.members.get(0).etag);
        assertFalse(ch.members.get(0).gone);
        assertTrue(ch.members.get(1).gone);
        // The same request DAVx5 sends (internal/dav/testdata/clients/davx5/04-sync-initial.http), with a token.
        assertTrue(f.sent.get(0).contains("depth=0 <?xml version='1.0' encoding='UTF-8' ?><sync-collection xmlns=\"DAV:\"><sync-token>t1</sync-token>"
                + "<sync-level>1</sync-level><prop><getetag /></prop></sync-collection>"));

        f.on("REPORT", cal, 207, "<d:multistatus xmlns:d=\"DAV:\" xmlns:c=\"urn:ietf:params:xml:ns:caldav\"><d:response>"
                + "<d:href>/dav/calendars/lea/lea~perso/judo.ics</d:href><d:propstat><d:prop><d:getetag>\"e1\"</d:getetag>"
                + "<c:calendar-data>BEGIN:VCALENDAR&#13;\nEND:VCALENDAR&#13;\n</c:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>"
                + "</d:response></d:multistatus>");
        List<Dav.Member> got = dav.multiget(cal, Arrays.asList("/dav/calendars/lea/lea~perso/judo.ics"));
        assertEquals(1, got.size());
        assertEquals("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", got.get(0).data);
        assertTrue(f.sent.get(1).contains("<CAL:calendar-multiget xmlns=\"DAV:\" xmlns:CAL=\"urn:ietf:params:xml:ns:caldav\"><prop><getcontenttype />"
                + "<getetag /><CAL:calendar-data /></prop><href>/dav/calendars/lea/lea~perso/judo.ics</href></CAL:calendar-multiget>"));
    }

    @Test(expected = Dav.TokenExpired.class)
    public void expiredToken() throws Exception {
        String cal = B + "/dav/calendars/lea/lea~perso/";
        new Dav(new Fake().on("REPORT", cal, 403, "<d:error xmlns:d=\"DAV:\"><d:valid-sync-token/></d:error>"), B).changes(cal, "old");
    }

    @Test(expected = Dav.Unauthorized.class)
    public void refused() throws Exception {
        new Dav(new Fake().on("PROPFIND", B + "/dav/", 401, ""), B).calendars();
    }
}
