-- Contas: uma linha por identidade autenticada via Authentik (OIDC).
-- server-central é Resource Server puro, então não guarda senha nenhuma,
-- só o vínculo com o "sub" (subject) emitido pelo IdP.
CREATE TABLE accounts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_subject  TEXT NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Perfil: dados editáveis pelo usuário, separados de accounts porque
-- accounts é só identidade/autenticação.
CREATE TABLE profiles (
    account_id    UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    display_name  TEXT NOT NULL,
    avatar_url    TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Amizades: uma linha por par, direção requester -> addressee.
-- status cobre o fluxo pedido -> aceito, e bloqueio.
CREATE TABLE friendships (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id   UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    addressee_id   UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'accepted', 'blocked')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (requester_id <> addressee_id),
    UNIQUE (requester_id, addressee_id)
);

CREATE INDEX friendships_addressee_idx ON friendships(addressee_id);

-- Diretório de servidores conhecidos por cada conta: não há descoberta
-- automática (ver docs/architecture.md), só convite/IP manual, então cada
-- conta guarda sua própria lista de server-channel conhecidos.
CREATE TABLE known_servers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    address     TEXT NOT NULL,
    name        TEXT NOT NULL,
    icon_url    TEXT,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, address)
);
