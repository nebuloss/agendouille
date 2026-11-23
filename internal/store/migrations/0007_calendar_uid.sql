-- A calendar's own id, for the admin UI's addresses (DECISIONS: calendar
-- ids). owner~slug names a calendar for its owner and their devices, but it
-- changes on a transfer and two owners can have the same slug: the admin
-- addresses a calendar by this random id, which never changes.

ALTER TABLE calendars ADD COLUMN uid TEXT;

-- A random UUID (version 4) in SQL, for the rows there are and the new ones.
UPDATE calendars SET uid =
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
  substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6)))
WHERE uid IS NULL;

CREATE UNIQUE INDEX calendars_uid ON calendars (uid);

-- Every way a calendar is made (the service, an import, a test) gets one.
CREATE TRIGGER calendars_uid_new AFTER INSERT ON calendars WHEN NEW.uid IS NULL
BEGIN
  UPDATE calendars SET uid =
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
    substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
  WHERE id = NEW.id;
END;
