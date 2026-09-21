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
