-- M1: settings, the admin, users and groups, and everything that signs
-- someone in. CONTRACTS.md section 5 is the end state; the calendar tables
-- arrive with M2.

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,            -- JSON
  updated_at TEXT NOT NULL
);

CREATE TABLE admin (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  password_hash TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE users (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  color        TEXT NOT NULL,
  disabled     INTEGER NOT NULL DEFAULT 0,
  removed_at   TEXT,
  created_at   TEXT NOT NULL,
  last_seen_at TEXT
);

CREATE TABLE groups (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  color        TEXT NOT NULL,
  created_at   TEXT NOT NULL
);

CREATE TABLE group_members (
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user ON group_members (user_id);

CREATE TABLE devices (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind         TEXT NOT NULL CHECK (kind IN ('app','davx5','ios','thunderbird','other')),
  label        TEXT NOT NULL,
  cap          TEXT NOT NULL CHECK (cap IN ('readwrite','read')),
  secret_hash  BLOB NOT NULL UNIQUE,
  client       TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  last_used_at TEXT,
  last_ip      TEXT,
  revoked_at   TEXT
);
CREATE INDEX devices_user ON devices (user_id);

CREATE TABLE sessions (
  id         INTEGER PRIMARY KEY,
  surface    TEXT NOT NULL CHECK (surface IN ('app','admin')),
  user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,       -- NULL for the admin
  device_id  INTEGER REFERENCES devices(id) ON DELETE CASCADE,     -- set for the app's WebView
  token_hash BLOB NOT NULL UNIQUE,
  user_agent TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  seen_at    TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  CHECK ((surface = 'admin') = (user_id IS NULL))
);
CREATE INDEX sessions_user ON sessions (user_id);

CREATE TABLE enrolments (
  id             INTEGER PRIMARY KEY,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash     BLOB NOT NULL UNIQUE,
  made_by        TEXT NOT NULL,          -- 'admin' or the user's own name
  created_at     TEXT NOT NULL,
  expires_at     TEXT NOT NULL,
  used_at        TEXT,
  used_by_device INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  revoked_at     TEXT
);
CREATE INDEX enrolments_user ON enrolments (user_id);

CREATE TABLE approvals (
  id              TEXT PRIMARY KEY,
  code_hash       BLOB NOT NULL UNIQUE,
  short_code_hash BLOB NOT NULL UNIQUE,
  number          INTEGER NOT NULL,
  user_agent      TEXT NOT NULL,
  ip              TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  expires_at      TEXT NOT NULL,
  approved_by     INTEGER REFERENCES users(id) ON DELETE CASCADE,
  failed          INTEGER NOT NULL DEFAULT 0,
  used_at         TEXT
);

CREATE TABLE audit (
  id          INTEGER PRIMARY KEY,
  at          TEXT NOT NULL,
  actor       TEXT NOT NULL,             -- 'admin', 'cli' or a user name
  kind        TEXT NOT NULL,
  action      TEXT NOT NULL,
  calendar_id INTEGER,                   -- references calendars(id) from M2
  detail      TEXT NOT NULL
);
CREATE INDEX audit_at ON audit (at);
