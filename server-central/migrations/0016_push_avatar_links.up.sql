-- Links do avatar de quem mandou uma notificação push (ver
-- docs/architecture.md, "Decisão: notificações push (fase 6)", "Avatar na
-- notificação"). O app Android baixa o avatar sem token de login, por um
-- link com um identificador aleatório de 256 bits. Só o SHA-256 do
-- identificador fica aqui. O link vale para várias leituras e só vence por
-- tempo (expires_at); as linhas vencidas são apagadas de tempos em tempos.
CREATE TABLE push_avatar_links (
    token_hash BYTEA PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX push_avatar_links_expires_idx ON push_avatar_links (expires_at);
