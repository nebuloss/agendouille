-- M2: calendars, their shares, each user's view of them, the objects and
-- the change log that sync tokens are made of (CONTRACTS.md 5).

CREATE TABLE calendars (
  id          INTEGER PRIMARY KEY,
  owner_id    INTEGER NOT NULL REFERENCES users(id),
  slug        TEXT NOT NULL,             -- fixed: it is in every phone's URL
  name        TEXT NOT NULL,
  color       TEXT NOT NULL,
  timezone    TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  seq         INTEGER NOT NULL DEFAULT 0, -- the last change in this calendar
  pruned_seq  INTEGER NOT NULL DEFAULT 0, -- changes at or below it are gone: older tokens are invalid
  created_at  TEXT NOT NULL,
  deleted_at  TEXT,                       -- the row stays so the slug is never reused
  UNIQUE (owner_id, slug)
);

CREATE TABLE shares (
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  user_id     INTEGER REFERENCES users(id) ON DELETE CASCADE,
  group_id    INTEGER REFERENCES groups(id) ON DELETE CASCADE,
  access      TEXT NOT NULL CHECK (access IN ('read','readwrite')),
  created_at  TEXT NOT NULL,
  CHECK ((user_id IS NULL) <> (group_id IS NULL)),
  UNIQUE (calendar_id, user_id),
  UNIQUE (calendar_id, group_id)
);
CREATE INDEX shares_user ON shares (user_id);
CREATE INDEX shares_group ON shares (group_id);

CREATE TABLE calendar_views (
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  shown       INTEGER NOT NULL DEFAULT 1, -- the web app's toggle
  hidden      INTEGER NOT NULL DEFAULT 0, -- off every one of the user's devices
  displayname TEXT,                       -- a sharee's own PROPPATCH
  color       TEXT,
  sort_order  INTEGER,
  PRIMARY KEY (calendar_id, user_id)
);

CREATE TABLE objects (
  id           INTEGER PRIMARY KEY,
  calendar_id  INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  uid          TEXT NOT NULL,
  etag         TEXT NOT NULL,
  data         BLOB NOT NULL,
  summary      TEXT NOT NULL DEFAULT '',
  start_utc    TEXT NOT NULL,
  end_utc      TEXT,                      -- NULL: an open-ended recurrence
  recurring    INTEGER NOT NULL,
  all_day      INTEGER NOT NULL,
  created_by   INTEGER NOT NULL REFERENCES users(id),
  modified_by  INTEGER NOT NULL REFERENCES users(id),
  created_via  TEXT NOT NULL,
  modified_via TEXT NOT NULL,
  device_id    INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  created_at   TEXT NOT NULL,
  modified_at  TEXT NOT NULL,
  UNIQUE (calendar_id, name),
  UNIQUE (calendar_id, uid)
);
CREATE INDEX objects_range ON objects (calendar_id, start_utc, end_utc);

-- AUTOINCREMENT: a sequence number is never reused, even after pruning.
CREATE TABLE changes (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  name        TEXT NOT NULL DEFAULT '',   -- '' for a change of the calendar itself
  summary     TEXT NOT NULL DEFAULT '',   -- never shown to the admin
  action      TEXT NOT NULL,
  user_id     INTEGER REFERENCES users(id),
  device_id   INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  via         TEXT NOT NULL,
  at          TEXT NOT NULL
);
CREATE INDEX changes_cal ON changes (calendar_id, seq);
CREATE INDEX changes_at ON changes (at);
