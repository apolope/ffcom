# server-central

Instância única oficial do FFCom, hospedada em `ffcom.a3sitsolutions.com`. Guarda login, perfil, lista de amigos e diretório de servidores (`server-channel`) de cada conta. Ver decisões em [`../docs/architecture.md`](../docs/architecture.md).

## Stack planejada

- Go
- PostgreSQL
- OIDC (Resource Server) contra a instância central de Authentik do
  `abs-3d-printer` (`authentik.abs.a3sitsolutions.com.br`) — não uma
  instância própria
- WebSocket para presença e DMs

## Responsabilidade

- Cadastro/login de conta: `server-central` é Resource Server puro (valida
  Bearer JWT via issuer/JWKS); quem faz o login de verdade (Authorization
  Code + PKCE) é o `client`, contra a instância central de Authentik.
- Lista de amigos e status de presença.
- Diretório de servidores conhecidos por cada conta (IP/DNS, nome, ícone).
- Mensagens diretas (DM): `server-central` medeia diretamente, com armazenamento em Postgres e entrega via WebSocket.

## Estado atual

Modelo de dados implementado (`internal/store`): `accounts`, `profiles`,
`friendships`, `known_servers` — migrations em `migrations/`, aplicadas
automaticamente na inicialização (embutidas no binário, sem passo manual).

`auth.Middleware` valida o Bearer JWT contra o JWKS do Authentik
(`internal/auth`, biblioteca `coreos/go-oidc/v3`) e garante a conta local
via `AccountStore.GetOrCreateBySubject` a cada requisição autenticada — não
há endpoint de "cadastro" separado, a primeira requisição de um `sub` novo
já cria a conta. Hoje já cobre `GET /api/me`, presença (WebSocket), lista de
amigos, diretório de servidores conhecidos e DMs (REST + WebSocket) — ver
[`../TODO.md`](../TODO.md), seção "server-central", para o detalhe item a
item.

## Rodando via Docker Compose

```
cp .env.example .env
# edite .env: defina POSTGRES_PASSWORD e OIDC_ISSUER_URL
docker compose up -d
```

Sobe dois serviços: `postgres` e `app` (o binário `server-central` em si,
buildado a partir do `Dockerfile` local). Não há Authentik neste compose —
ver [`../docs/architecture.md`](../docs/architecture.md), "Decisão:
autenticação em server-central — Authentik (OIDC) da instância central do
abs-3d-printer".

Antes de rodar em produção, o app "ffcom" precisa estar registrado na
instância central de Authentik (blueprint declarativo no repositório
`abs-3d-printer`, mesmo procedimento usado por outros projetos que reusam
essa instância — ver
`D:\Dev\a3s-network\docs\procedures\integrar-app-com-authentik.md`).

## Notificações push

O `server-central` é quem fala com o Firebase Cloud Messaging para avisar o
app Android de mensagem nova, DM e pedido ou aceite de amizade. Liga com
`FCM_SERVICE_ACCOUNT_JSON` no `.env`: a chave JSON da conta de serviço do
projeto Firebase, numa linha só (como o console baixa, sem quebrar as
linhas) ou o caminho de um arquivo montado no container. Sem ela, o log diz
"notificações push desligadas" e as rotas de push respondem normalmente,
só sem enviar. Para trocar a chave: gerar outra no console do Google Cloud,
trocar no `.env`, `docker compose up -d app` e revogar a antiga.

Os `server-channel` chamam `POST /api/push/notify` com os tokens que os
membros lhes deram; a rota não exige login e não entra no rate limit geral
(tem limites próprios). O texto das mensagens passa só em memória até o
Firebase: nada é gravado. Rotas e formato em
[`../docs/protocol.md`](../docs/protocol.md), "Notificações push"; decisão
em [`../docs/architecture.md`](../docs/architecture.md), "Decisão:
notificações push (fase 6)".

A notificação leva um link temporário (até 48 h) para a foto de quem
mandou, que o app baixa sem login em `GET /api/push/avatars/{token}`. O
link é montado com `CENTRAL_PUBLIC_URL`, o endereço público deste
servidor com `https://` (vazia: a instância oficial). Quem roda outra
instância precisa defini-la, ou as notificações apontam para a oficial e
saem sem foto.

## Backup / restore

Nenhum backup automático embutido — ver [`../docs/backup-restore.md`](../docs/backup-restore.md) para o procedimento de `pg_dump`/`pg_restore` do banco e dos avatares (`avatars_data`).

## TLS / HTTPS

Mesmo caso de `server-channel` (ver
[`../server-channel/README.md`](../server-channel/README.md#tls--https)):
o `app` fala HTTP puro, sem terminação TLS embutida — coloque um proxy
reverso (Caddy, Nginx Proxy Manager, Traefik) na frente para expor
publicamente. Com o proxy no ar, ligue `REQUIRE_TLS=true` no `.env` para o
`app` recusar (`426 Upgrade Required`) qualquer requisição sem
`X-Forwarded-Proto: https`. Padrão desligado, para não quebrar quem estiver
testando sem proxy.
