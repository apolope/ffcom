-- Membros: um server-channel não guarda senha nem se comunica diretamente
-- com server-central (ver docs/architecture.md, "protocolo entre client,
-- server-central e server-channel"). A identidade vem do mesmo "sub" emitido
-- pela instância central de Authentik, validado localmente pelo server-channel
-- — por isso basta o vínculo com oidc_subject, sem depender de nenhuma
-- chamada a server-central. nickname é o apelido específico deste servidor,
-- distinto do display_name global guardado em server-central.
CREATE TABLE members (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_subject  TEXT NOT NULL UNIQUE,
    nickname      TEXT,
    joined_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Categorias agrupam canais. Cada server-channel representa uma única
-- comunidade, então não existe tabela "servers" — a instância é o servidor.
CREATE TABLE categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Canais: texto, voz ou forum (ver docs/architecture.md, decisão do LiveKit).
-- category_id é opcional porque um canal pode existir fora de qualquer
-- categoria (mesmo comportamento do Discord).
CREATE TABLE channels (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id  UUID REFERENCES categories(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('text', 'voice', 'forum')),
    position     INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX channels_category_idx ON channels(category_id);

-- Roles do servidor. O bitmask de permissions ainda não tem os bits
-- definidos — isso é o TODO separado "Sistema de permissões/roles por
-- servidor e por canal"; aqui só a coluna existe para guardá-lo.
CREATE TABLE roles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    color        TEXT,
    permissions  BIGINT NOT NULL DEFAULT 0,
    position     INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Atribuição de roles a membros (N:N).
CREATE TABLE member_roles (
    member_id  UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (member_id, role_id)
);

-- Threads de canais forum. Uma thread pertence a um único canal (que deve
-- ser do tipo 'forum' — não reforçado via CHECK porque cruzaria tabelas;
-- fica a cargo da camada de aplicação).
CREATE TABLE threads (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id        UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    title             TEXT NOT NULL,
    author_member_id  UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX threads_channel_idx ON threads(channel_id);

-- Mensagens: cobre tanto o histórico de um canal de texto (thread_id nulo)
-- quanto os posts dentro de uma thread de forum (thread_id preenchido).
CREATE TABLE messages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id        UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    thread_id         UUID REFERENCES threads(id) ON DELETE CASCADE,
    author_member_id  UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    content           TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at         TIMESTAMPTZ
);

CREATE INDEX messages_channel_created_idx ON messages(channel_id, created_at DESC);
CREATE INDEX messages_thread_created_idx ON messages(thread_id, created_at DESC);

-- Convites: código curto gerado pelo dono/admin do servidor (ver
-- docs/architecture.md, "descoberta de server-channel: nenhuma, apenas
-- convite ou IP manual"). max_uses e expires_at nulos significam "sem limite".
CREATE TABLE invites (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                   TEXT NOT NULL UNIQUE,
    created_by_member_id   UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    max_uses               INT,
    uses                   INT NOT NULL DEFAULT 0,
    expires_at             TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
