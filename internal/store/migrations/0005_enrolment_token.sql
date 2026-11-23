-- A sign-in link the admin can show again while nobody used it: its token
-- is kept, readable, while the link is pending, and cleared when it is
-- used, cancelled or seen expired (DECISIONS: links shown again). The
-- hash stays what redeem looks up. kind: 'phone' (a QR for a phone's
-- camera) or 'browser' (a short link to sign a browser in, for testing).

ALTER TABLE enrolments ADD COLUMN token TEXT;
ALTER TABLE enrolments ADD COLUMN kind TEXT NOT NULL DEFAULT 'phone';
