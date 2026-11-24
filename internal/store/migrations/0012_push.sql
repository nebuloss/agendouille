-- WebDAV-Push (PLAN.md M9, DECISIONS: push): a device subscribes to a
-- calendar with a Web Push resource (its UnifiedPush endpoint) and keys;
-- each change sends it an encrypted message, which makes it sync.

CREATE TABLE push_subscriptions (
  id              TEXT PRIMARY KEY,   -- random: the registration URL's last segment
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id       INTEGER REFERENCES devices(id) ON DELETE CASCADE,
  calendar_id     INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  push_resource   TEXT NOT NULL,      -- where the push service takes messages
  public_key      TEXT NOT NULL,      -- the subscription's p256dh key, base64url
  auth_secret     TEXT NOT NULL,      -- base64url, 16 bytes
  content_update  INTEGER NOT NULL DEFAULT 1,
  property_update INTEGER NOT NULL DEFAULT 0,
  expires_at      TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  UNIQUE (calendar_id, push_resource)
);
CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id);

-- The server's VAPID key (RFC 8292), made on first use.
CREATE TABLE push_keys (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  private    TEXT NOT NULL,            -- the P-256 scalar, base64url
  created_at TEXT NOT NULL
);
