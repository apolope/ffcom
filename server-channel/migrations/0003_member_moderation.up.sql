-- Kick/ban de membro (ver docs/architecture.md, "Decisão: kick/ban de
-- membro"). removed_at marca quem foi expulso sem apagar a linha de
-- members: mensagens, threads e convites já criados continuam
-- referenciando o mesmo author_member_id/created_by_member_id, então o
-- histórico permanece legível para quem ficou (mesmo critério do Discord:
-- remover alguém não apaga o que a pessoa já escreveu). Um membro com
-- removed_at preenchido perde acesso imediatamente, porque
-- MemberStore.GetByOIDCSubject só devolve linhas com removed_at nulo.
ALTER TABLE members ADD COLUMN removed_at TIMESTAMPTZ;

-- Banimento é indexado por oidc_subject, não por member_id: precisa
-- continuar bloqueando POST /api/join mesmo depois que a linha de members
-- correspondente for reaproveitada (removed_at limpo, ver
-- MemberStore.GetOrCreateByOIDCSubject) ou não existir mais.
CREATE TABLE member_bans (
    oidc_subject         TEXT PRIMARY KEY,
    banned_by_member_id  UUID REFERENCES members(id) ON DELETE SET NULL,
    reason               TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
