-- Convites de amizade: código de uso único gerado por uma conta e resgatado
-- por outra para criar a amizade (já aceita, sem etapa extra de aprovação —
-- resgatar o código já é o consentimento mútuo). Ver docs/architecture.md,
-- "Decisão: adicionar amigos via convite" — mesma filosofia de
-- known_servers (sem descoberta automática/pesquisável, só convite).
CREATE TABLE friend_invites (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                    TEXT NOT NULL UNIQUE,
    created_by_account_id   UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    expires_at              TIMESTAMPTZ,
    redeemed_by_account_id  UUID REFERENCES accounts(id) ON DELETE CASCADE,
    redeemed_at             TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX friend_invites_created_by_idx ON friend_invites(created_by_account_id);
