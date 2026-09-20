DROP TABLE channel_role_overwrites;
DELETE FROM roles WHERE is_default;
DROP INDEX roles_single_default_idx;
ALTER TABLE roles DROP COLUMN is_default;
ALTER TABLE members DROP COLUMN is_owner;
