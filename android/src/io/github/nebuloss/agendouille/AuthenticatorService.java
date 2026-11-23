/*
 * Shaped after Google's SampleSyncAdapter (Apache License 2.0,
 * Copyright (C) 2010 The Android Open Source Project): the account
 * authenticator a sync adapter needs, reduced to what agendouille uses.
 */
package io.github.nebuloss.agendouille;

import android.accounts.AbstractAccountAuthenticator;
import android.accounts.Account;
import android.accounts.AccountAuthenticatorResponse;
import android.accounts.AccountManager;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import android.os.IBinder;

/**
 * The agendouille account type. Adding an account from the system settings
 * opens the app (an account comes only from an enrollment link); the
 * credential is never handed out as an auth token: only the app uses it.
 */
public final class AuthenticatorService extends Service {
    private Authenticator authenticator;

    @Override
    public void onCreate() {
        authenticator = new Authenticator(this);
    }

    @Override
    public IBinder onBind(Intent intent) {
        return authenticator.getIBinder();
    }

    static final class Authenticator extends AbstractAccountAuthenticator {
        private final Context ctx;

        Authenticator(Context ctx) {
            super(ctx);
            this.ctx = ctx;
        }

        @Override
        public Bundle addAccount(AccountAuthenticatorResponse response, String accountType, String authTokenType, String[] features, Bundle options) {
            Intent i = new Intent(ctx, MainActivity.class);
            i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            Bundle b = new Bundle();
            b.putParcelable(AccountManager.KEY_INTENT, i);
            return b;
        }

        @Override
        public Bundle editProperties(AccountAuthenticatorResponse response, String accountType) {
            throw new UnsupportedOperationException();
        }

        @Override
        public Bundle confirmCredentials(AccountAuthenticatorResponse response, Account account, Bundle options) {
            return null;
        }

        @Override
        public Bundle getAuthToken(AccountAuthenticatorResponse response, Account account, String authTokenType, Bundle options) {
            Bundle b = new Bundle();
            b.putInt(AccountManager.KEY_ERROR_CODE, AccountManager.ERROR_CODE_UNSUPPORTED_OPERATION);
            b.putString(AccountManager.KEY_ERROR_MESSAGE, "agendouille keeps its credential to itself");
            return b;
        }

        @Override
        public String getAuthTokenLabel(String authTokenType) {
            return null;
        }

        @Override
        public Bundle updateCredentials(AccountAuthenticatorResponse response, Account account, String authTokenType, Bundle options) {
            return null;
        }

        @Override
        public Bundle hasFeatures(AccountAuthenticatorResponse response, Account account, String[] features) {
            Bundle b = new Bundle();
            b.putBoolean(AccountManager.KEY_BOOLEAN_RESULT, false);
            return b;
        }
    }
}
