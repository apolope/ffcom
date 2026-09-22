ALTER TABLE accounts
    ADD COLUMN e2e_public_key BYTEA
        CONSTRAINT accounts_e2e_public_key_len CHECK (e2e_public_key IS NULL OR octet_length(e2e_public_key) = 32);

-- Mensagens antigas (texto puro) não têm como virar ciphertext válido --
-- descartadas (só há dados de teste nesta fase, sem usuários reais fora
-- do autor, ver docs/architecture.md).
TRUNCATE direct_messages;

ALTER TABLE direct_messages
    DROP COLUMN content,
    ADD COLUMN ciphertext BYTEA NOT NULL,
    ADD COLUMN nonce BYTEA NOT NULL CHECK (octet_length(nonce) = 24);
