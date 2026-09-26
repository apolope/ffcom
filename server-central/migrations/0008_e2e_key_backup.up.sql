-- Backup da chave privada de E2E da conta, cifrado no client com uma chave
-- derivada da frase de recuperação (scrypt + NaCl secretbox). server-central
-- guarda bytes opacos que não consegue abrir. Ver docs/architecture.md,
-- "Decisão: backup da chave de E2E com frase de recuperação".
ALTER TABLE accounts
    ADD COLUMN e2e_key_backup BYTEA
        CONSTRAINT accounts_e2e_key_backup_len CHECK (e2e_key_backup IS NULL OR octet_length(e2e_key_backup) <= 1024);
