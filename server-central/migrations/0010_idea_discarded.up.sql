-- Ideia descartada pela checagem final por não ser uma sugestão de melhoria
-- (ex. "aplicativo ruim", sem nada a mudar). Não aparece em lugar nenhum e
-- não gasta a sugestão do dia: o índice de uma por dia passa a ignorar as
-- descartadas. feedback guarda a dica do Claude do que faltou, mostrada
-- para quem escreveu. Ver docs/architecture.md, "Decisão: sugestões de
-- melhoria com varinha do Claude".
ALTER TABLE ideas DROP CONSTRAINT ideas_status_check;
ALTER TABLE ideas ADD CONSTRAINT ideas_status_check
    CHECK (status IN ('checking', 'review', 'open', 'planned', 'implemented', 'rejected', 'discarded'));
ALTER TABLE ideas ADD COLUMN feedback TEXT CHECK (feedback IS NULL OR char_length(feedback) <= 300);

DROP INDEX ideas_one_per_day;
CREATE UNIQUE INDEX ideas_one_per_day ON ideas (account_id, suggested_on) WHERE status <> 'discarded';
