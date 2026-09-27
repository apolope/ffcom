-- Pedidos de cadastro no FFCom feitos pela home page. Cada pedido vai para
-- o tópico do FFCom no grupo "Rede" do Telegram com botões de aprovar e
-- reprovar; o clique chega ao a3s-network-monitor, que repassa a decisão ao
-- listener interno deste serviço. Aprovado, o server-central cria o usuário
-- no Authentik, põe no grupo ffcom-users e pede ao Authentik o e-mail de
-- definir senha. Ver docs/architecture.md, "Decisão: cadastro com
-- aprovação pelo Telegram".
--
-- status:
--   pending    aguardando decisão no Telegram
--   approving  aprovação em andamento (chamadas ao Authentik); volta a
--              pending se falhar, e um clique novo pode retomar depois de
--              alguns minutos se o processo morrer no meio
--   approved   usuário criado no Authentik
--   rejected   reprovado; a pessoa não é avisada
CREATE TABLE signup_requests (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name            TEXT NOT NULL CHECK (char_length(full_name) BETWEEN 2 AND 80),
    username             TEXT NOT NULL CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,29}$'),
    email                TEXT NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254 AND email = lower(email)),
    nickname             TEXT NOT NULL CHECK (char_length(nickname) BETWEEN 2 AND 32),
    reason               TEXT NOT NULL CHECK (char_length(reason) BETWEEN 10 AND 500),
    status               TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'approving', 'approved', 'rejected')),
    client_ip            TEXT NOT NULL DEFAULT '',
    telegram_message_id  BIGINT,
    decided_by           TEXT,
    decided_at           TIMESTAMPTZ,
    -- Última falha ao aprovar (ex. usuário já existe no Authentik) ou aviso
    -- de aprovação parcial (e-mail de senha não saiu).
    failure              TEXT,
    authentik_user_pk    BIGINT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Um pedido em aberto por e-mail e por nome de usuário.
CREATE UNIQUE INDEX signup_requests_open_email ON signup_requests (email)
    WHERE status IN ('pending', 'approving');
CREATE UNIQUE INDEX signup_requests_open_username ON signup_requests (username)
    WHERE status IN ('pending', 'approving');
CREATE INDEX signup_requests_email ON signup_requests (email, created_at DESC);
CREATE INDEX signup_requests_created ON signup_requests (created_at);
-- Pedidos ainda sem mensagem no Telegram (envio falhou), que o notificador
-- tenta de novo.
CREATE INDEX signup_requests_unsent ON signup_requests (created_at)
    WHERE telegram_message_id IS NULL AND status = 'pending';
