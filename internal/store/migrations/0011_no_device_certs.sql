-- Device certificates are gone (DECISIONS 2026-11-24): CalDAV signs a
-- device in with its password only. Their fingerprints, the users' sign-in
-- policies and the switches table (which only held the server's mode) go.
DROP INDEX IF EXISTS devices_cert;
ALTER TABLE devices DROP COLUMN cert_fingerprint;
ALTER TABLE devices DROP COLUMN cert_expires_at;
ALTER TABLE users DROP COLUMN dav_auth;
DROP TABLE switches;
