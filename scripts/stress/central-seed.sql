-- Contas, amigos e servidores no rail em massa no server-central local, para
-- teste de stress da interface. Só para desenvolvimento: aplicado por
-- scripts/stress-seed.ps1, que roda central-remove.sql antes.
--
-- As contas 'stress-NNNN' são as mesmas dos membros de channel-seed.sql, então
-- nome e status de presença aparecem nas listas de membros. Ninguém está
-- conectado de verdade, então a presença mostra offline mesmo com status
-- escolhido.
--
-- Parâmetros (psql -v): members, friends, rail_servers.

INSERT INTO accounts (oidc_subject, presence_status)
SELECT 'stress-' || lpad(i::text, 4, '0'),
       (ARRAY['online','busy','away','invisible'])[1 + i % 4]
FROM generate_series(1, :members) AS i
ON CONFLICT (oidc_subject) DO NOTHING;

INSERT INTO profiles (account_id, display_name)
SELECT a.id,
       (ARRAY['Ana','Bruno','Carla','Diego','Elisa','Felipe','Gabriela','Heitor','Isabela','João','Karina','Lucas','Marina','Nicolas','Olívia','Pedro','Quésia','Rafael','Sofia','Tiago','Úrsula','Vitor','Wesley','Yasmin','Zeca'])[1 + i % 25]
         || ' ' ||
       (ARRAY['Silva','Souza','Oliveira','Santos','Lima','Pereira','Costa','Ferreira','Almeida','Ribeiro','Carvalho','Gomes','Martins','Rocha','Barbosa','Araújo','Mendes','Cardoso','Teixeira','Moreira'])[1 + (i / 25) % 20]
         || ' ' || i
FROM generate_series(1, :members) AS i
JOIN accounts a ON a.oidc_subject = 'stress-' || lpad(i::text, 4, '0')
ON CONFLICT (account_id) DO NOTHING;

-- Toda conta real (não stress) vira amiga das primeiras N contas de stress.
INSERT INTO friendships (requester_id, addressee_id, status)
SELECT real.id, fake.id, 'accepted'
FROM accounts real
JOIN accounts fake ON fake.oidc_subject LIKE 'stress-%'
  AND substring(fake.oidc_subject from 8)::int <= :friends
WHERE real.oidc_subject NOT LIKE 'stress-%';

-- Servidores extras no rail, depois dos que já existem. O domínio .invalid
-- nunca resolve (RFC 2606), então o poll de não lido falha na hora, sem
-- esperar timeout; clicar num deles mostra o erro de conexão.
INSERT INTO known_servers (account_id, address, name, position)
SELECT a.id,
       'http://stress-' || lpad(n::text, 2, '0') || '.invalid',
       'Stress ' || n,
       (SELECT COALESCE(MAX(k.position), -1) FROM known_servers k WHERE k.account_id = a.id) + n
FROM accounts a
CROSS JOIN generate_series(1, :rail_servers) AS n
WHERE a.oidc_subject NOT LIKE 'stress-%'
ON CONFLICT (account_id, address) DO NOTHING;
