DELETE FROM signup_requests WHERE username IS NULL OR nickname IS NULL;
ALTER TABLE signup_requests
    DROP CONSTRAINT signup_requests_new_account_fields,
    ALTER COLUMN username SET NOT NULL,
    ALTER COLUMN nickname SET NOT NULL,
    DROP COLUMN authentik_username,
    DROP COLUMN linked_existing,
    DROP COLUMN email_accounts,
    DROP COLUMN existing_account;
