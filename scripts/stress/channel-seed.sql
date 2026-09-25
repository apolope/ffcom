-- Dados em massa para teste de stress da interface num server-channel local
-- (lista de membros, barra de canais, histórico e fórum com scroll). Só para
-- desenvolvimento: aplicado por scripts/stress-seed.ps1, que roda
-- channel-remove.sql antes, então cada execução recria tudo do zero.
--
-- Marcadores usados para remover depois: membros com oidc_subject
-- 'stress-NNN', categorias e roles com nome começando por 'Stress '. Os
-- subjects batem com as contas criadas por central-seed.sql, então os nomes
-- e o status de presença vêm de lá.
--
-- Parâmetros (psql -v): members, categories, channels_per_category,
-- chat_messages, forum_threads, thread_messages.

-- Membros: nome de perfil igual ao display_name de central-seed.sql; um em
-- cada sete com apelido próprio, para exercitar os dois caminhos de nome.
INSERT INTO members (oidc_subject, profile_name, nickname, joined_at)
SELECT 'stress-' || lpad(i::text, 4, '0'),
       (ARRAY['Ana','Bruno','Carla','Diego','Elisa','Felipe','Gabriela','Heitor','Isabela','João','Karina','Lucas','Marina','Nicolas','Olívia','Pedro','Quésia','Rafael','Sofia','Tiago','Úrsula','Vitor','Wesley','Yasmin','Zeca'])[1 + i % 25]
         || ' ' ||
       (ARRAY['Silva','Souza','Oliveira','Santos','Lima','Pereira','Costa','Ferreira','Almeida','Ribeiro','Carvalho','Gomes','Martins','Rocha','Barbosa','Araújo','Mendes','Cardoso','Teixeira','Moreira'])[1 + (i / 25) % 20]
         || ' ' || i,
       CASE WHEN i % 7 = 0 THEN 'apelido_bem_comprido_para_testar_quebra_' || i END,
       now() - (i || ' hours')::interval
FROM generate_series(1, :members) AS i
ON CONFLICT (oidc_subject) DO NOTHING;

-- Roles coloridas, para a lista de membros e o diálogo de roles.
INSERT INTO roles (name, color, permissions, position)
SELECT 'Stress ' || v.name, v.color, 0, 100 + v.pos
FROM (VALUES ('Moderação', '#e67e22', 1), ('Veteranos', '#3498db', 2), ('Convidados', '#95a5a6', 3), ('Bots', '#9b59b6', 4), ('Apoiadores', '#2ecc71', 5)) AS v(name, color, pos);

INSERT INTO member_roles (member_id, role_id)
SELECT m.id, r.id
FROM members m
JOIN roles r ON r.name LIKE 'Stress %'
WHERE m.oidc_subject LIKE 'stress-%'
  AND (substring(m.oidc_subject from 8)::int + r.position) % 4 = 0;

-- Categorias e canais para a barra lateral. Um de cada seis canais é de voz,
-- um de cada onze é fórum.
INSERT INTO categories (name, position)
SELECT 'Stress ' || lpad(c::text, 2, '0'), (SELECT COALESCE(MAX(position), -1) FROM categories) + c
FROM generate_series(1, :categories) AS c;

INSERT INTO channels (category_id, name, type, position)
SELECT cat.id,
       CASE WHEN n % 6 = 0 THEN 'Sala ' || n
            WHEN n % 11 = 0 THEN 'forum-' || n
            ELSE 'canal-com-nome-razoavelmente-longo-' || n END,
       CASE WHEN n % 6 = 0 THEN 'voice' WHEN n % 11 = 0 THEN 'forum' ELSE 'text' END,
       n
FROM categories cat
CROSS JOIN generate_series(1, :channels_per_category) AS n
WHERE cat.name LIKE 'Stress %';

-- Canal de texto e fórum cheios, no topo da primeira categoria de stress.
INSERT INTO channels (category_id, name, type, position)
SELECT cat.id, v.name, v.type, v.pos
FROM categories cat, (VALUES ('stress-chat', 'text', -2), ('stress-forum', 'forum', -1)) AS v(name, type, pos)
WHERE cat.name = 'Stress 01';

-- Frases de exemplo; mensagens longas juntam várias, algumas com quebra de
-- linha, para variar a altura das linhas no scroll.
CREATE TEMP TABLE stress_phrases (idx int, txt text);
INSERT INTO stress_phrases
SELECT row_number() OVER (), t FROM unnest(ARRAY[
  'bom dia, pessoal',
  'alguém on hoje à noite?',
  'acabei de subir a versão nova no servidor de teste',
  'kkkkkkkk',
  'isso aí ficou muito bom',
  'não consegui reproduzir aqui, qual navegador você está usando?',
  'vou olhar isso depois do almoço',
  'o áudio cortou de novo quando entrei na sala',
  'alguém tem o link daquela documentação?',
  'https://example.com/um/link/bem/comprido/para/testar/quebra/de/linha/sem/espaco/nenhum/no/meio',
  'concordo',
  'Revisei o PR e deixei alguns comentários; o principal é sobre o tratamento de erro quando o servidor devolve 429, acho que vale tentar de novo depois do Retry-After em vez de mostrar erro direto.',
  'partida às 21h?',
  '👍',
  'Lista do mercado: arroz, feijão, café, pão, manteiga, queijo, presunto, tomate, cebola, alho, azeite, sal, açúcar, leite, ovos, frutas, verduras e papel toalha.',
  'testando uma mensagem com `código inline` e *ênfase*',
  'palavraenormesemespacosparaverseoquebradelinhaestafuncionandocorretamentenalistademensagens'
]) AS t;

INSERT INTO messages (channel_id, author_member_id, content, created_at)
SELECT ch.id,
       (SELECT id FROM members WHERE oidc_subject = 'stress-' || lpad((1 + (i * 7919) % :members)::text, 4, '0')),
       CASE
         WHEN i % 13 = 0 THEN (SELECT string_agg(txt, E'\n') FROM stress_phrases WHERE idx % 3 = i % 3)
         WHEN i % 5 = 0 THEN (SELECT txt FROM stress_phrases WHERE idx = 1 + i % 17) || ' ' || (SELECT txt FROM stress_phrases WHERE idx = 1 + (i + 5) % 17)
         ELSE (SELECT txt FROM stress_phrases WHERE idx = 1 + i % 17)
       END || CASE WHEN i % 50 = 0 THEN ' (#' || i || ')' ELSE '' END,
       now() - ((:chat_messages - i) * interval '3 minutes')
FROM channels ch, generate_series(1, :chat_messages) AS i
WHERE ch.name = 'stress-chat';

-- Fórum: threads com título longo de vez em quando, cada uma com mensagens.
INSERT INTO threads (channel_id, title, author_member_id, created_at)
SELECT ch.id,
       CASE WHEN t % 4 = 0 THEN 'Tópico ' || t || ': um título bem mais comprido que o normal para ver como a lista de threads lida com isso'
            ELSE 'Tópico ' || t END,
       (SELECT id FROM members WHERE oidc_subject = 'stress-' || lpad((1 + (t * 31) % :members)::text, 4, '0')),
       now() - ((:forum_threads - t) * interval '2 hours')
FROM channels ch, generate_series(1, :forum_threads) AS t
WHERE ch.name = 'stress-forum';

INSERT INTO messages (channel_id, thread_id, author_member_id, content, created_at)
SELECT th.channel_id, th.id,
       (SELECT id FROM members WHERE oidc_subject = 'stress-' || lpad((1 + (k * 104729 + length(th.title)) % :members)::text, 4, '0')),
       (SELECT txt FROM stress_phrases WHERE idx = 1 + k % 17),
       th.created_at + k * interval '1 minute'
FROM threads th
JOIN channels ch ON ch.id = th.channel_id AND ch.name = 'stress-forum'
CROSS JOIN generate_series(1, :thread_messages) AS k;
