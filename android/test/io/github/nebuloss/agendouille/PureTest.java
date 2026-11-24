package io.github.nebuloss.agendouille;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.json.JSONObject;
import org.junit.Test;

/** The pure-Java parts: enrollment links, the bridge codec, versions. */
public class PureTest {
    private static final String TOKEN = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcde";

    @Test
    public void enrollLinks() {
        EnrollLink l = EnrollLink.parse("https://agenda.example.org/enroll/" + TOKEN);
        assertNotNull(l);
        assertEquals("https://agenda.example.org", l.server);
        assertEquals(TOKEN, l.token);
        // The older spelling, a port, a trailing slash and spaces.
        l = EnrollLink.parse("  http://10.0.0.5:8791/enrol/" + TOKEN + "/ ");
        assertNotNull(l);
        assertEquals("http://10.0.0.5:8791", l.server);
        // The scheme the enrollment page offers, for any server.
        l = EnrollLink.parse("agendouille://enroll?u=" + java.net.URLEncoder.encode("https://agenda.example.org/enroll/" + TOKEN));
        assertNotNull(l);
        assertEquals("https://agenda.example.org", l.server);
        assertEquals(TOKEN, l.token);
    }

    @Test
    public void davx5Links() {
        // The QR code of a DAVx5 device: our handoff page.
        EnrollLink l = EnrollLink.parse("https://agenda.example.org/davx5/" + TOKEN);
        assertNotNull(l);
        assertEquals(EnrollLink.HANDOFF, l.kind);
        assertEquals("https://agenda.example.org", l.server);
        assertEquals(TOKEN, l.token);
        // The link the page holds, as the server writes it (QueryEscape).
        l = EnrollLink.parse("davx5://lea-pixel:s3cr%2Bt%3Ax@agenda.example.org/dav/");
        assertNotNull(l);
        assertEquals(EnrollLink.PASSWORD, l.kind);
        assertEquals("https://agenda.example.org", l.server);
        assertEquals("lea-pixel", l.username);
        assertEquals("s3cr+t:x", l.password);
        l = EnrollLink.parse("caldavs://u:p@10.0.0.5:8791/dav/");
        assertNotNull(l);
        assertEquals("https://10.0.0.5:8791", l.server);
        assertEquals(EnrollLink.ENROLL, EnrollLink.parse("https://agenda.example.org/enroll/" + TOKEN).kind);
        assertNull(EnrollLink.parse("davx5://agenda.example.org/dav/"));
        assertNull(EnrollLink.parse("davx5://u:@agenda.example.org/dav/"));
        assertNull(EnrollLink.parse("https://agenda.example.org/davx5/short"));
    }

    @Test
    public void apiPaths() {
        // Managing calendars and approving a browser.
        assertTrue(ApiPath.allowed("GET", "/api/calendars"));
        assertTrue(ApiPath.allowed("POST", "/api/calendars"));
        assertTrue(ApiPath.allowed("GET", "/api/people"));
        assertTrue(ApiPath.allowed("GET", "/api/me"));
        assertTrue(ApiPath.allowed("PATCH", "/api/calendars/lea%2Fecole"));
        assertTrue(ApiPath.allowed("DELETE", "/api/calendars/lea%2Fecole"));
        assertTrue(ApiPath.allowed("PUT", "/api/calendars/x/shares/g%3Afamille"));
        assertTrue(ApiPath.allowed("DELETE", "/api/calendars/x/shares/u%3Atom"));
        assertTrue(ApiPath.allowed("POST", "/api/calendars/x/transfer"));
        assertTrue(ApiPath.allowed("GET", "/api/approvals/PKW-6VY"));
        assertTrue(ApiPath.allowed("POST", "/api/approvals/PKW-6VY"));
        // Never events, exports, devices, the admin, or a way out of /api.
        assertFalse(ApiPath.allowed("GET", "/api/calendars/x/events/a.ics"));
        assertFalse(ApiPath.allowed("POST", "/api/calendars/x/events"));
        assertFalse(ApiPath.allowed("GET", "/api/calendars/x/export"));
        assertFalse(ApiPath.allowed("GET", "/api/calendars/x"));
        assertFalse(ApiPath.allowed("GET", "/api/events"));
        assertFalse(ApiPath.allowed("DELETE", "/api/devices/3"));
        assertFalse(ApiPath.allowed("GET", "/api/admin/users"));
        assertFalse(ApiPath.allowed("GET", "/api/calendars/../admin"));
        assertFalse(ApiPath.allowed("GET", "//evil.example/api/me"));
        assertFalse(ApiPath.allowed("GET", "https://evil.example/api/me"));
        assertFalse(ApiPath.allowed("TRACE", "/api/me"));
        assertFalse(ApiPath.allowed("GET", "/dav/"));
        assertFalse(ApiPath.allowed(null, null));
    }

    @Test
    public void notEnrollLinks() {
        assertNull(EnrollLink.parse(null));
        assertNull(EnrollLink.parse(""));
        assertNull(EnrollLink.parse("https://agenda.example.org/"));
        assertNull(EnrollLink.parse("https://agenda.example.org/enroll/short"));
        assertNull(EnrollLink.parse("https://agenda.example.org/enroll/" + TOKEN + "/../../x"));
        assertNull(EnrollLink.parse("javascript:alert(1)//enroll/" + TOKEN));
        assertNull(EnrollLink.parse("ftp://agenda.example.org/enroll/" + TOKEN));
        assertNull(EnrollLink.parse("agendouille://enroll?x=1"));
        assertNull(EnrollLink.parse("agendouille://other?u=https://a.b/enroll/" + TOKEN));
    }

    @Test
    public void bridgeRequests() throws Exception {
        BridgeCodec.Request r = BridgeCodec.parse("{\"id\": 3, \"call\": \"hello\", \"args\": {\"page_bridge\": 1}, \"extra\": true}");
        assertNotNull(r);
        assertEquals(3, r.id);
        assertEquals("hello", r.call);
        assertEquals(1, r.args.getInt("page_bridge"));
        r = BridgeCodec.parse("{\"id\": 4, \"call\": \"scan\"}");
        assertEquals(0, r.args.length());
        assertNull(BridgeCodec.parse("not json"));
        assertNull(BridgeCodec.parse("{\"call\": \"hello\"}"));
        assertNull(BridgeCodec.parse("{\"id\": 1}"));
        assertNull(BridgeCodec.parse("[1, 2]"));
    }

    @Test
    public void bridgeAnswers() throws Exception {
        JSONObject ok = new JSONObject(BridgeCodec.ok(1, BridgeCodec.hello("0.6.0", 34)));
        assertEquals(1, ok.getLong("id"));
        assertTrue(ok.getBoolean("ok"));
        assertEquals(BridgeCodec.VERSION, ok.getJSONObject("result").getInt("app_bridge"));
        assertEquals("0.6.0", ok.getJSONObject("result").getString("app_version"));
        assertEquals(34, ok.getJSONObject("result").getInt("android"));
        JSONObject err = new JSONObject(BridgeCodec.error(2, "unknown_call", null));
        assertFalse(err.getBoolean("ok"));
        assertEquals("unknown_call", err.getString("error"));
        assertFalse(err.has("message"));
        JSONObject told = new JSONObject(BridgeCodec.error(3, "refused", "Identifiants refusés."));
        assertEquals("Identifiants refusés.", told.getString("message"));
        JSONObject plain = new JSONObject(BridgeCodec.ok(5, null));
        assertFalse(plain.has("result"));
        JSONObject ev = new JSONObject(BridgeCodec.event("sync.changed", new JSONObject().put("state", "syncing")));
        assertEquals("syncing", ev.getJSONObject("data").getString("state"));
    }

    @Test
    public void serverAddresses() {
        assertEquals("https://agenda.example.org", ServerAddress.normalize("agenda.example.org"));
        assertEquals("https://agenda.example.org", ServerAddress.normalize(" https://Agenda.Example.org/dav/ "));
        assertEquals("http://10.0.0.5:8791", ServerAddress.normalize("http://10.0.0.5:8791/"));
        assertEquals("https://agenda.example.org:8443", ServerAddress.normalize("agenda.example.org:8443"));
        assertNull(ServerAddress.normalize(""));
        assertNull(ServerAddress.normalize("ftp://agenda.example.org"));
        assertNull(ServerAddress.normalize("agenda example"));
        assertNull(ServerAddress.normalize(null));
    }

    @Test
    public void versions() {
        assertFalse(Versions.updateAvailable("dev-1a2b3c4", "dev-1a2b3c4", true));
        assertFalse(Versions.updateAvailable("dev-1a2b3c4-dirty", "dev-1a2b3c4", true));
        assertTrue(Versions.updateAvailable("dev-1a2b3c4", "dev-5e6f7a8", true));
        assertFalse(Versions.updateAvailable("dev-1a2b3c4", "dev-5e6f7a8", false));
        assertFalse(Versions.updateAvailable("0.6.0", "v0.6.0", true));
        assertTrue(Versions.updateAvailable("0.6.0", "v0.7.0", true));
        assertFalse(Versions.updateAvailable("0.6.0", "", true));
    }
}
