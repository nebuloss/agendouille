-- The admin UI's notification centre: which notices the admin marked read.
-- A notice is worked out from the state on every read; its mark is kept
-- while it lasts and forgotten when it goes, so the same problem coming
-- back is news again.

CREATE TABLE notice_reads (
  id      TEXT PRIMARY KEY,
  read_at TEXT NOT NULL
);
