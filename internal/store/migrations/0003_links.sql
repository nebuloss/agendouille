-- M4: read-only subscription links (PLAN.md 7): a secret URL serving the
-- chosen calendars as one .ics, for a TV or anything without CalDAV.

CREATE TABLE links (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label        TEXT NOT NULL,
  secret_hash  BLOB NOT NULL UNIQUE,
  created_at   TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at   TEXT
);
CREATE INDEX links_user ON links (user_id);

CREATE TABLE link_calendars (
  link_id     INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  PRIMARY KEY (link_id, calendar_id)
);
