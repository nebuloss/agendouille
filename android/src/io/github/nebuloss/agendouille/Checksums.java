package io.github.nebuloss.agendouille;

import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;

/** SHA256SUMS, as sha256sum writes it, and SHA-256 itself. Pure Java: tested on the JVM. */
public final class Checksums {
    private Checksums() {}

    /** The lower-case hex SHA-256 that sums gives for name ("<hex>  <name>" lines), or null. */
    public static String find(String sums, String name) {
        if (sums == null || name == null) return null;
        for (String line : sums.split("\n")) {
            line = line.trim();
            int sp = line.indexOf(' ');
            if (sp != 64) continue;
            String file = line.substring(sp).trim();
            if (file.startsWith("*")) file = file.substring(1); // binary mode
            String hex = line.substring(0, 64).toLowerCase();
            if (file.equals(name) && hex.matches("[0-9a-f]{64}")) return hex;
        }
        return null;
    }

    public static String sha256(byte[] data) {
        try {
            byte[] d = MessageDigest.getInstance("SHA-256").digest(data);
            StringBuilder sb = new StringBuilder(64);
            for (byte b : d) sb.append(String.format("%02x", b & 0xff));
            return sb.toString();
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException(e);
        }
    }
}
