# TODO — FFCom

Backlog unificado por tema. Ver [`docs/architecture.md`](docs/architecture.md) para o porquê de cada decisão técnica citada aqui.

## Decisões de design pendentes (bloqueiam implementação)

Todas as decisões abaixo foram tomadas — ver `docs/architecture.md` para o detalhe de cada uma.

- [x] Definir modelo de DMs: `server-central` medeia diretamente ou existe algum outro mecanismo?
- [x] Definir estratégia de autenticação (usuário/senha simples vs. Keycloak/OAuth)
- [x] Definir modelo de descoberta de `server-channel` (manual via IP/DNS na v1, confirmar)
- [x] Desenhar o protocolo/API entre `client` ↔ `server-central` e `client` ↔ `server-channel` (REST? WebSocket only? gRPC entre server-central e server-channel?)

## Infraestrutura / DevOps

- [x] Inicializar módulo Go em `server-central/`
- [x] Inicializar módulo Go em `server-channel/`
- [x] Inicializar projeto Vite + React + TypeScript em `client/`
- [x] Adicionar wrapper Electron ao `client/`
- [x] Docker Compose de referência para `server-channel` (app Go + LiveKit + coturn + Postgres)
- [x] Docker Compose de referência para `server-central` (app Go + Postgres — Authentik é a instância central do `abs-3d-printer`, não roda aqui)
- [ ] Documentar port-forwarding / DNS dinâmico para quem for self-hostear `server-channel` atrás de NAT
- [ ] Pipeline de CI (lint + testes) para os três componentes
- [ ] Definir versionamento e forma de release dos binários (`server-central`, `server-channel`, `client`)
- [ ] Provisionar `ffcom.a3sitsolutions.com` (DNS + certificado) para a instância oficial do `server-central`

## server-central

- [x] Modelo de dados: conta, perfil, lista de amigos, diretório de servidores (IP/DNS + nome + ícone por servidor)
- [x] Registrar o app "ffcom" (provider + application) na instância central de Authentik do `abs-3d-printer`, via blueprint declarativo (`infra/authentik/blueprints/providers-ffcom.yaml` naquele repo, commit `03f726d`) — feito com `redirect_uris` provisórios (só `http://localhost:5173/auth/callback`); ver TODO abaixo para o ajuste final
- [ ] Atualizar `redirect_uris` em `providers-ffcom.yaml` (repo `abs-3d-printer`) quando o domínio de produção do `client` (web/PWA) e o esquema de callback do build Electron empacotado forem decididos — usar um prompt dedicado para o agente daquele repo, mesmo molde do que registrou o app (ver `docs/architecture.md`)
- [x] Cadastro/login de conta via Authentik
- [x] Endpoint/gateway de presença (quem está online)
- [x] Implementar DMs: `server-central` como gateway de mensagens (armazenamento em Postgres + entrega via WebSocket)
- [x] API para o client listar/adicionar/remover servidores conhecidos (adição manual via IP/DNS ou convite — sem discovery automático)

## server-channel

- [x] Modelo de dados: categorias, canais (texto/voz/forum), mensagens, permissões/roles, convites
- [ ] Integração com LiveKit: criar sala por canal de voz, emitir token de acesso, aplicar permissões
- [x] Canal de texto: envio/histórico de mensagens via WebSocket
- [x] CORS configurável (`CORS_ALLOWED_ORIGINS`) para o client chamar de outra origem (REST + WebSocket)
- [ ] Canal forum: threads/posts
- [ ] Sistema de permissões/roles por servidor e por canal
- [ ] Convites (geração e validação)
- [ ] Registro do endereço do servidor (para o dono divulgar IP/DNS aos membros)

## client

- [x] Layout base: rail de servidores → categorias → canais → lista de membros (estilo Discord)
- [x] Login OIDC (Authorization Code + PKCE, `oidc-client-ts`) contra o Authentik central — tela de login antes do shell principal
- [x] Tela de adicionar servidor via IP/DNS (`client/src/components/AddServerDialog.tsx`, via API de `server-central`; ver `client/src/hooks/useKnownServers.ts`)
- [x] API REST em `server-channel` para o client listar categorias/canais reais
- [ ] Integração de voz/vídeo via `livekit-client`
- [ ] Compartilhamento de tela
- [x] Chat de texto em tempo real (histórico via REST + WebSocket, ver `client/src/hooks/useChannelChat.ts`)
- [ ] Canal forum (UI de threads)
- [x] Lista de amigos + presença (via `server-central`)
- [x] DMs (API de `server-central` pronta — `GET/POST` via WebSocket de presença e `GET /api/dms/{accountId}/messages`, ver `docs/architecture.md`; UI no client via `client/src/components/DirectMessageView.tsx`)
- [ ] Build Electron para Windows/macOS/Linux
- [ ] Build web/PWA

## Segurança

- [ ] Criptografia em trânsito (TLS) obrigatória entre client e ambos os tipos de servidor
- [ ] Rate limiting / proteção contra abuso em `server-central` (cadastro, login)
- [ ] Política de permissões/roles em `server-channel` revisada contra escalonamento de privilégio
- [ ] Avaliar necessidade de criptografia ponta-a-ponta em DMs

## Documentação

- [ ] Guia de self-hosting de `server-channel` (Docker Compose, TURN, DNS dinâmico)
- [ ] Guia de contribuição (CONTRIBUTING.md)
- [ ] Documentar protocolo/API entre os três componentes assim que definido

## Fora de escopo da v1 (registrado para não esquecer)

- [ ] Cliente mobile
- [ ] Federação entre instâncias de `server-central`
