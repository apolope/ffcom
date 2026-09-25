-- Ordem dos servidores no rail, escolhida pela pessoa arrastando os ícones
-- (ver docs/architecture.md, "Decisão: ordem dos servidores no rail"). As
-- linhas existentes ganham a ordem que o client já mostrava: mais recente
-- primeiro.
ALTER TABLE known_servers ADD COLUMN position INT NOT NULL DEFAULT 0;

UPDATE known_servers k
SET position = ordered.rn
FROM (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY account_id ORDER BY added_at DESC) - 1 AS rn
    FROM known_servers
) ordered
WHERE k.id = ordered.id;
