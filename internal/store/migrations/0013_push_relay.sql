-- The push service in agendouille itself (DECISIONS: push without ntfy):
-- each endpoint is a Web Push resource (RFC 8030) of one device, for one
-- app on it (the agendouille app, or DAVx5 through the app as UnifiedPush
-- distributor); messages wait here until the device's stream takes them.

CREATE TABLE push_endpoints (
  id         TEXT PRIMARY KEY,          -- random: /push/<id>, a capability URL
  device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  instance   TEXT NOT NULL,             -- the app and its token on the phone
  created_at TEXT NOT NULL,
  UNIQUE (device_id, instance)
);

CREATE TABLE push_messages (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT, -- the stream's event id
  endpoint_id TEXT NOT NULL REFERENCES push_endpoints(id) ON DELETE CASCADE,
  device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  topic       TEXT,                     -- RFC 8030 5.4: a newer one replaces it
  urgency     TEXT NOT NULL,
  body        BLOB NOT NULL,            -- as received: encrypted for the app
  expires_at  TEXT NOT NULL,
  created_at  TEXT NOT NULL
);
CREATE INDEX push_messages_device ON push_messages (device_id, seq);
