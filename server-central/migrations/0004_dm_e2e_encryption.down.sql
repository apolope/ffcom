ALTER TABLE direct_messages
    DROP COLUMN ciphertext,
    DROP COLUMN nonce,
    ADD COLUMN content TEXT NOT NULL DEFAULT '';
ALTER TABLE direct_messages ALTER COLUMN content DROP DEFAULT;

ALTER TABLE accounts DROP COLUMN e2e_public_key;
