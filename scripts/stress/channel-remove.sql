-- Desfaz channel-seed.sql. Apagar os canais leva junto mensagens, threads e
-- anexos (ON DELETE CASCADE); apagar os membros leva as roles atribuídas.
-- Não toca em nada criado fora do seed de stress.
DELETE FROM channels
WHERE category_id IN (SELECT id FROM categories WHERE name LIKE 'Stress %');
DELETE FROM categories WHERE name LIKE 'Stress %';
DELETE FROM roles WHERE name LIKE 'Stress %';
-- Mensagens de membros de stress em canais normais (se alguém moveu um canal
-- de stress para fora da categoria) caem junto por cascade.
DELETE FROM members WHERE oidc_subject LIKE 'stress-%';
