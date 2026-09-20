-- Sistema de permissões/roles (ver docs/architecture.md, "Sistema de
-- permissões/roles por servidor e por canal"). Bits definidos em
-- internal/permissions/permissions.go: ViewChannels=1, SendMessages=2,
-- Voice=4, ManageInvites=8, ManageRoles=16, Administrator=32.

-- Dono do servidor: quem entrou primeiro via POST /api/join (bootstrap do
-- self-host, ver docs/architecture.md, "Convites obrigatórios para entrar em
-- server-channel"). Ignora toda checagem de permissão, roles e overwrites de
-- canal — não é uma role para não poder ser removido/editado como uma
-- (mesmo comportamento do "server owner" do Discord, distinto de
-- Administrator).
ALTER TABLE members ADD COLUMN is_owner BOOLEAN NOT NULL DEFAULT false;

-- Role "@everyone": implícita a todo membro sem precisar de linha em
-- member_roles (mesmo padrão do Discord). Só pode existir uma por servidor.
ALTER TABLE roles ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX roles_single_default_idx ON roles ((is_default)) WHERE is_default;

-- ViewChannels(1) | SendMessages(2) | Voice(4) = 7 — preserva o
-- comportamento anterior (qualquer membro tinha acesso total) até que
-- alguém restrinja explicitamente via role/overwrite de canal.
INSERT INTO roles (name, permissions, position, is_default)
VALUES ('@everyone', 7, -1, true);

-- Overwrites de permissão por canal e role: allow/deny sobrescrevem bits
-- específicos da permissão base (roles do membro) só dentro deste canal —
-- é o que torna um canal privado/restrito a quem não tiver a role liberada.
CREATE TABLE channel_role_overwrites (
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    allow       BIGINT NOT NULL DEFAULT 0,
    deny        BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (channel_id, role_id)
);
