package io.github.nebuloss.agendouille;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import java.math.BigInteger;
import java.util.Base64;

import org.junit.Test;

/** WebDAV-Push as the app writes and reads it (PushXml). */
public class PushXmlTest {
    @Test
    public void readsTheVapidKeyOfAPropfind() {
        String ms = "<d:multistatus xmlns:d=\"DAV:\" xmlns:p=\"https://bitfire.at/webdav-push\"><d:response><d:propstat><d:prop>"
                + "<p:transports><p:web-push><p:vapid-public-key type=\"p256ecdsa\">BCVxsr7N_eNgVRqvHtD0zTZs</p:vapid-public-key></p:web-push></p:transports>"
                + "</d:prop></d:propstat></d:response></d:multistatus>";
        assertEquals("BCVxsr7N_eNgVRqvHtD0zTZs", PushXml.vapidKey(ms));
        assertNull("no push", PushXml.vapidKey("<d:multistatus xmlns:d=\"DAV:\"/>"));
        assertNull(PushXml.vapidKey(null));
    }

    @Test
    public void writesARegistration() {
        String x = PushXml.register("https://ntfy.example/upAB?up=1&x=2", "PUB", "AUTH", "Tue, 01 Dec 2026 10:00:00 GMT");
        assertTrue(x, x.contains("<push-register xmlns=\"https://bitfire.at/webdav-push\""));
        assertTrue(x, x.contains("<push-resource>https://ntfy.example/upAB?up=1&amp;x=2</push-resource>"));
        assertTrue(x, x.contains("<subscription-public-key type=\"p256dh\">PUB</subscription-public-key><auth-secret>AUTH</auth-secret>"));
        assertTrue(x, x.contains("<content-update><D:depth>1</D:depth></content-update>"));
        assertTrue(x, x.contains("<expires>Tue, 01 Dec 2026 10:00:00 GMT</expires>"));
    }

    @Test
    public void httpDatesBothWays() {
        long t = 1795860000000L; // 2026-11-28T10:00:00Z
        assertEquals("Sat, 28 Nov 2026 10:00:00 GMT", PushXml.httpDate(t));
        assertEquals(t, PushXml.parseHttpDate("Sat, 28 Nov 2026 10:00:00 GMT"));
        assertEquals(-1, PushXml.parseHttpDate("tomorrow"));
    }

    @Test
    public void resolvesTheLocation() {
        assertEquals("https://h/dav/push/abc", PushXml.resolve("https://h/dav/calendars/lea/lea~perso/", "/dav/push/abc"));
        assertEquals("https://other/x", PushXml.resolve("https://h/dav/", "https://other/x"));
        assertNull(PushXml.resolve("https://h/", null));
    }

    @Test
    public void uncompressedPointsAre65Bytes() {
        // A coordinate whose BigInteger has a sign byte, and a short one.
        BigInteger x = new BigInteger(1, new byte[]{(byte) 0xff, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31});
        BigInteger y = BigInteger.valueOf(5);
        byte[] b = Base64.getUrlDecoder().decode(PushXml.uncompressed(x, y));
        assertEquals(65, b.length);
        assertEquals(4, b[0]);
        assertEquals((byte) 0xff, b[1]);
        assertEquals(5, b[64]);
        assertEquals(0, b[33]);
    }
}
