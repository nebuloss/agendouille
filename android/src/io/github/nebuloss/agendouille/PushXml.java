package io.github.nebuloss.agendouille;

import java.math.BigInteger;
import java.net.URI;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;
import java.util.Base64;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * WebDAV-Push's requests and answers as the app needs them
 * (draft-bitfire-webdav-push-00): what a calendar's PROPFIND says of push,
 * the push-register body, the headers of the answer. Pure Java: tested on
 * the JVM.
 */
public final class PushXml {
    private PushXml() {}

    public static final String NS = "https://bitfire.at/webdav-push";

    /** A calendar's push properties, asked by name (they are not in allprop). */
    public static final String PROPFIND = "<?xml version=\"1.0\" encoding=\"utf-8\"?>"
            + "<d:propfind xmlns:d=\"DAV:\" xmlns:P=\"" + NS + "\"><d:prop><P:transports/><P:topic/></d:prop></d:propfind>";

    private static final Pattern VAPID = Pattern.compile("vapid-public-key[^>]*>\\s*([A-Za-z0-9_\\-]+=*)\\s*<");

    /** The server's VAPID key from a PROPFIND answer; null when it offers no Web Push. */
    public static String vapidKey(String multistatus) {
        if (multistatus == null) return null;
        Matcher m = VAPID.matcher(multistatus);
        return m.find() ? m.group(1) : null;
    }

    /** A push-register: this push resource and keys, for changes to the calendar and its properties. */
    public static String register(String resource, String publicKey, String authSecret, String expires) {
        return "<?xml version=\"1.0\" encoding=\"utf-8\"?>"
                + "<push-register xmlns=\"" + NS + "\" xmlns:D=\"DAV:\">"
                + "<subscription><web-push-subscription>"
                + "<push-resource>" + escape(resource) + "</push-resource>"
                + "<content-encoding>aes128gcm</content-encoding>"
                + "<subscription-public-key type=\"p256dh\">" + escape(publicKey) + "</subscription-public-key>"
                + "<auth-secret>" + escape(authSecret) + "</auth-secret>"
                + "</web-push-subscription></subscription>"
                + "<trigger><content-update><D:depth>1</D:depth></content-update>"
                + "<property-update><D:depth>0</D:depth></property-update></trigger>"
                + (expires == null ? "" : "<expires>" + expires + "</expires>")
                + "</push-register>";
    }

    /** An HTTP date (IMF-fixdate). */
    public static String httpDate(long millis) {
        return DateTimeFormatter.RFC_1123_DATE_TIME.format(ZonedDateTime.ofInstant(Instant.ofEpochMilli(millis), ZoneOffset.UTC));
    }

    /** Reads an HTTP date; -1 when it is not one. */
    public static long parseHttpDate(String s) {
        if (s == null) return -1;
        try {
            return ZonedDateTime.parse(s.trim(), DateTimeFormatter.RFC_1123_DATE_TIME).toInstant().toEpochMilli();
        } catch (RuntimeException e) {
            return -1;
        }
    }

    /** location (absolute, or relative to the request's url) as an absolute URL. */
    public static String resolve(String url, String location) {
        if (location == null || location.isEmpty()) return null;
        try {
            return URI.create(url).resolve(location).toString();
        } catch (IllegalArgumentException e) {
            return null;
        }
    }

    /** A P-256 public point as Web Push wants it: uncompressed (65 bytes), base64url. */
    public static String uncompressed(BigInteger x, BigInteger y) {
        byte[] out = new byte[65];
        out[0] = 4;
        put(x, out, 1);
        put(y, out, 33);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(out);
    }

    private static void put(BigInteger v, byte[] out, int at) {
        byte[] b = v.toByteArray();
        int n = Math.min(b.length, 32);
        System.arraycopy(b, b.length - n, out, at + 32 - n, n);
    }

    private static String escape(String s) {
        return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;");
    }
}
