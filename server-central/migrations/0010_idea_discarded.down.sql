DELETE FROM ideas WHERE status = 'discarded';
DROP INDEX ideas_one_per_day;
CREATE UNIQUE INDEX ideas_one_per_day ON ideas (account_id, suggested_on);
ALTER TABLE ideas DROP COLUMN feedback;
ALTER TABLE ideas DROP CONSTRAINT ideas_status_check;
ALTER TABLE ideas ADD CONSTRAINT ideas_status_check
    CHECK (status IN ('checking', 'review', 'open', 'planned', 'implemented', 'rejected'));
