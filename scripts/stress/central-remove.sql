-- Desfaz central-seed.sql. Apagar as contas leva perfis e amizades junto
-- (ON DELETE CASCADE).
DELETE FROM known_servers WHERE address LIKE 'http://stress-%.invalid';
DELETE FROM accounts WHERE oidc_subject LIKE 'stress-%';
