-- Nome do perfil do Authentik (claim name, ou preferred_username sem ele),
-- gravado a cada requisição autenticada a partir do access token. É o nome
-- exibido de quem não escolheu um no perfil do FFCom (ver
-- docs/architecture.md, "Decisão: nome do Authentik no server-central").
ALTER TABLE accounts ADD COLUMN profile_name TEXT;
