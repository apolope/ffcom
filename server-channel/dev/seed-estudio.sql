-- Estrutura de exemplo do segundo servidor local, "Estúdio" (porta 8083,
-- docker-compose.dev-second.yml). Só para desenvolvimento: aplicado por
-- scripts/dev-local.ps1. Idempotente, como seed-teste.sql.

INSERT INTO categories (name, position)
SELECT v.name, (SELECT COALESCE(MAX(position), -1) FROM categories) + v.pos
FROM (VALUES ('Boas-vindas', 1), ('Trabalho', 2), ('Off-topic', 3)) AS v(name, pos)
WHERE NOT EXISTS (SELECT 1 FROM categories c WHERE c.name = v.name);

INSERT INTO channels (category_id, name, type, position)
SELECT c.id, v.name, v.type, v.pos
FROM (VALUES
    ('Boas-vindas', 'regras', 'text', 0),
    ('Boas-vindas', 'apresentacoes', 'text', 1),
    ('Trabalho', 'frontend', 'text', 0),
    ('Trabalho', 'backend', 'text', 1),
    ('Trabalho', 'Daily', 'voice', 2),
    ('Trabalho', 'duvidas', 'forum', 3),
    ('Off-topic', 'memes', 'text', 0),
    ('Off-topic', 'Lounge', 'voice', 1)
) AS v(category, name, type, pos)
JOIN categories c ON c.name = v.category
WHERE NOT EXISTS (SELECT 1 FROM channels ch WHERE ch.name = v.name);
