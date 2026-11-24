-- How a user's devices sign in to CalDAV, when it differs from the
-- server's choice (DECISIONS: a sign-in policy per user): password, both or
-- keys; NULL follows the server's (switches.dav_auth).

ALTER TABLE users ADD COLUMN dav_auth TEXT;
