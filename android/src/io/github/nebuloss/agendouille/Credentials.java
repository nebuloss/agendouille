package io.github.nebuloss.agendouille;

import android.accounts.Account;
import android.accounts.AccountManager;
import android.content.Context;
import android.os.Bundle;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;

import java.nio.charset.StandardCharsets;
import java.security.KeyStore;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/**
 * The device credential (PLAN.md 7.2): an Android account
 * "username@host" of our type, its user data the server, the username and
 * the device id, and the secret encrypted with an AES key that never
 * leaves the Android Keystore. One account at a time.
 */
final class Credentials {
    static final String TYPE = "io.github.nebuloss.agendouille";
    private static final String KEY_ALIAS = "agendouille-device-secret";

    final Account account;
    final String server;
    final String username;
    final String secret;
    final long deviceId;

    private Credentials(Account account, String server, String username, String secret, long deviceId) {
        this.account = account;
        this.server = server;
        this.username = username;
        this.secret = secret;
        this.deviceId = deviceId;
    }

    /** The account there is, or null. */
    static Credentials load(Context ctx) {
        AccountManager am = AccountManager.get(ctx);
        Account[] all = am.getAccountsByType(TYPE);
        if (all.length == 0) return null;
        Account a = all[0];
        try {
            String server = am.getUserData(a, "server");
            String username = am.getUserData(a, "username");
            long id = Long.parseLong(am.getUserData(a, "device_id"));
            String secret = decrypt(am.getUserData(a, "secret"));
            if (server == null || username == null || secret == null) return null;
            return new Credentials(a, server, username, secret, id);
        } catch (Exception e) {
            // A key lost with a reset of the lock screen, or damaged data: the
            // account is useless, sign in again.
            remove(ctx);
            return null;
        }
    }

    /** Keeps a new credential, replacing any other account. */
    static Credentials save(Context ctx, String server, String username, String secret, long deviceId) throws Exception {
        remove(ctx);
        String host = android.net.Uri.parse(server).getHost();
        Account a = new Account(username + "@" + host, TYPE);
        Bundle data = new Bundle();
        data.putString("server", server);
        data.putString("username", username);
        data.putString("device_id", Long.toString(deviceId));
        data.putString("secret", encrypt(secret));
        if (!AccountManager.get(ctx).addAccountExplicitly(a, null, data)) throw new IllegalStateException("the account could not be added");
        return new Credentials(a, server, username, secret, deviceId);
    }

    /** Removes the account (the server side is the caller's). */
    static void remove(Context ctx) {
        AccountManager am = AccountManager.get(ctx);
        for (Account a : am.getAccountsByType(TYPE)) am.removeAccountExplicitly(a);
    }

    private static SecretKey key() throws Exception {
        KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
        ks.load(null);
        if (ks.containsAlias(KEY_ALIAS)) return ((KeyStore.SecretKeyEntry) ks.getEntry(KEY_ALIAS, null)).getSecretKey();
        KeyGenerator g = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        g.init(new KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build());
        return g.generateKey();
    }

    private static String encrypt(String plain) throws Exception {
        Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
        c.init(Cipher.ENCRYPT_MODE, key());
        byte[] iv = c.getIV();
        byte[] ct = c.doFinal(plain.getBytes(StandardCharsets.UTF_8));
        return Base64.encodeToString(iv, Base64.NO_WRAP) + ":" + Base64.encodeToString(ct, Base64.NO_WRAP);
    }

    private static String decrypt(String stored) throws Exception {
        if (stored == null) return null;
        int i = stored.indexOf(':');
        byte[] iv = Base64.decode(stored.substring(0, i), Base64.NO_WRAP);
        byte[] ct = Base64.decode(stored.substring(i + 1), Base64.NO_WRAP);
        Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
        c.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, iv));
        return new String(c.doFinal(ct), StandardCharsets.UTF_8);
    }
}
