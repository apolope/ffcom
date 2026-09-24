-- Nome do perfil do Authentik (claim name, ou preferred_username sem ele),
-- gravado pelo próprio client: é o nome exibido de quem não escolheu apelido
-- neste servidor. Ver docs/architecture.md, "Decisão: nome exibido do membro".
ALTER TABLE members ADD COLUMN profile_name TEXT;
