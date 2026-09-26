-- Sugestões de melhoria (seção "Ideias" da home page). Ver
-- docs/architecture.md, "Decisão: sugestões de melhoria com varinha do
-- Claude".

-- Uma ideia por pessoa por dia (suggested_on é o dia no horário de
-- Brasília). status:
--   checking     aguardando a checagem final do Claude (via relay)
--   review       escondida até um admin (grupo ffcom-admins) aprovar:
--                checagem apontou conteúdo ofensivo ou não pôde rodar
--   open         publicada e aberta a votos
--   planned      publicada, marcada pelo admin como planejada
--   implemented  entregue; implemented_version diz em qual versão
--   rejected     recusada pelo admin (fica fora do site)
CREATE TABLE ideas (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id           UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    title                TEXT NOT NULL DEFAULT '' CHECK (char_length(title) <= 80),
    body                 TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    status               TEXT NOT NULL DEFAULT 'checking'
                         CHECK (status IN ('checking', 'review', 'open', 'planned', 'implemented', 'rejected')),
    review_reason        TEXT,
    implemented_version  TEXT,
    suggested_on         DATE NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ideas_one_per_day ON ideas (account_id, suggested_on);
CREATE INDEX ideas_status ON ideas (status);

-- Voto: like vale +2, dislike -1 na pontuação (calculada na leitura). Um
-- voto por pessoa por ideia, trocável.
CREATE TABLE idea_votes (
    idea_id     UUID NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    value       SMALLINT NOT NULL CHECK (value IN (1, -1)),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (idea_id, account_id)
);

-- Usos da varinha por pessoa por dia. Saldo = limite diário - wand_used -
-- wand_penalty (nunca abaixo de zero). wand_used é reservado ao pedir e
-- devolvido se o relay falhar; wand_penalty cresce quando a varinha acha a
-- ideia ofensiva.
CREATE TABLE idea_daily_usage (
    account_id    UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    day           DATE NOT NULL,
    wand_used     INT NOT NULL DEFAULT 0 CHECK (wand_used >= 0),
    wand_penalty  INT NOT NULL DEFAULT 0 CHECK (wand_penalty >= 0),
    PRIMARY KEY (account_id, day)
);

-- Pedidos ao a3s-claude-relay: kind 'improve' (varinha) ou 'check'
-- (checagem final de uma ideia enviada). O id é nosso e vai na URL do
-- callback; relay_job_id é o que o relay devolveu.
CREATE TABLE idea_assist_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind          TEXT NOT NULL CHECK (kind IN ('improve', 'check')),
    account_id    UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    day           DATE NOT NULL,
    idea_id       UUID REFERENCES ideas(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'failed')),
    relay_job_id  TEXT,
    result        JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);
CREATE INDEX idea_assist_jobs_pending ON idea_assist_jobs (status, created_at) WHERE status = 'pending';
CREATE INDEX idea_assist_jobs_account ON idea_assist_jobs (account_id, kind, status);
