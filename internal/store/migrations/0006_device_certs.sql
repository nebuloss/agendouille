-- Device certificates for CalDAV (DECISIONS: device keys). A device may
-- hold a client certificate issued by agendouille's own CA; its SHA-256
-- fingerprint is kept here, never its private key. switches holds the few
-- choices the admin UI makes itself rather than the configuration file:
-- dav_auth, how a device signs in to CalDAV (password, both, keys).

ALTER TABLE devices ADD COLUMN cert_fingerprint TEXT;
ALTER TABLE devices ADD COLUMN cert_expires_at TEXT;
CREATE UNIQUE INDEX devices_cert ON devices (cert_fingerprint) WHERE cert_fingerprint IS NOT NULL;

CREATE TABLE switches (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
