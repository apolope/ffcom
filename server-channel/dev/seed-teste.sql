-- Estrutura de exemplo do servidor local "Teste" (porta 8080), para testar
-- a ordenação de categorias e canais. Só para desenvolvimento: aplicado por
-- scripts/dev-local.ps1. Idempotente: só cria o que ainda não existe (por
-- nome), então rodar de novo não duplica nem desfaz a ordem que você
-- arrastou.

INSERT INTO categories (name, position)
SELECT v.name, (SELECT COALESCE(MAX(position), -1) FROM categories) + v.pos
FROM (VALUES ('Geral', 1), ('Jogos', 2), ('Projetos', 3), ('Música', 4)) AS v(name, pos)
WHERE NOT EXISTS (SELECT 1 FROM categories c WHERE c.name = v.name);

INSERT INTO channels (category_id, name, type, position)
SELECT c.id, v.name, v.type, v.pos
FROM (VALUES
    ('Geral', 'geral', 'text', 0),
    ('Geral', 'voz-geral', 'voice', 1),
    ('Geral', 'avisos', 'text', 2),
    ('Jogos', 'lol', 'text', 0),
    ('Jogos', 'valorant', 'text', 1),
    ('Jogos', 'Squad 1', 'voice', 2),
    ('Jogos', 'Squad 2', 'voice', 3),
    ('Projetos', 'ideias', 'text', 0),
    ('Projetos', 'bugs', 'text', 1),
    ('Projetos', 'sugestoes', 'forum', 2),
    ('Música', 'recomendacoes', 'text', 0),
    ('Música', 'Karaokê', 'voice', 1)
) AS v(category, name, type, pos)
JOIN categories c ON c.name = v.category
WHERE NOT EXISTS (SELECT 1 FROM channels ch WHERE ch.name = v.name);
