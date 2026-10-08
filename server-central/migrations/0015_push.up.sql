-- Notificações push do app Android (ver docs/architecture.md, "Decisão:
-- notificações push (fase 6)").
--
-- push_devices: tokens FCM dos aparelhos de cada conta. O token é do
-- aparelho, não da conta: se outra conta se registra com o mesmo token (troca
-- de login no mesmo celular), a linha passa para ela.
CREATE TABLE push_devices (
    token        TEXT PRIMARY KEY,
    account_id   UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    platform     TEXT NOT NULL CONSTRAINT push_devices_platform_valid CHECK (platform IN ('android')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX push_devices_account_idx ON push_devices (account_id);

-- push_grants: o token que a pessoa entrega a um server-channel para ele
-- poder pedir notificações em nome dela. Só o hash SHA-256 fica aqui; o
-- token em claro sai uma vez, na resposta de POST /api/push/grants. Preso
-- à linha de known_servers: tirar o servidor da lista apaga os grants dele
-- (ON DELETE CASCADE), e o endereço vem dali.
CREATE TABLE push_grants (
    token_hash      BYTEA PRIMARY KEY,
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    known_server_id UUID NOT NULL REFERENCES known_servers(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX push_grants_known_server_idx ON push_grants (known_server_id, created_at DESC);

-- push_mutes: servidor ou canal silenciado pela pessoa. channel_id vazio
-- silencia o servidor inteiro. Também presa a known_servers.
CREATE TABLE push_mutes (
    known_server_id UUID NOT NULL REFERENCES known_servers(id) ON DELETE CASCADE,
    channel_id      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (known_server_id, channel_id)
);
