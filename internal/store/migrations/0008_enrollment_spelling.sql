-- "enrollment", with two l (DECISIONS: enroll). The table, its index, the
-- audit lines already written and a setting left in the old settings table
-- take the new spelling; the migrations before this one keep the old.

ALTER TABLE enrolments RENAME TO enrollments;
DROP INDEX IF EXISTS enrolments_user;
CREATE INDEX enrollments_user ON enrollments (user_id);
UPDATE audit SET kind = 'enrollment' WHERE kind = 'enrolment';
UPDATE settings SET key = 'enrollment_link_ttl_hours' WHERE key = 'enrolment_link_ttl_hours';
