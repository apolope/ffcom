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
- [x] Documentar port-forwarding / DNS dinâmico para quem for self-hostear `server-channel` atrás de NAT — `server-channel/README.md`, seção "Hospedando atrás de NAT (ex. em casa)": tabela de portas a encaminhar, serviço de DDNS recomendado para `LIVEKIT_PUBLIC_URL`/endereço divulgado, e a limitação de `TURN_EXTERNAL_IP` exigir IP literal (sem hostname)
- [x] Pipeline de deploy (build + push GHCR + deploy) para os três componentes — `.github/workflows/deploy-ffcom-{central,channel,client}.yml`, ver `docs/architecture.md` ("Decisão: primeira implantação de teste"); ainda sem etapa de testes automatizados (só lint de Dockerfile via Hadolint e scan de segredo via Gitleaks)
- [x] CI rodar testes/lint de código — `.github/workflows/ci.yml` (novo, roda em `push`/`pull_request`, separado dos workflows de deploy que só disparam em tag): `go vet`/`go test` para `server-central` e `server-channel`, `npm run lint`/`npm run build` para `client`. Ver `docs/architecture.md`
- [x] Definir versionamento e forma de release dos binários (`server-central`, `server-channel`, `client`) — semver independente por componente, disparado por push de git tag (`central-v*`/`channel-v*`/`client-v*`); imagem GHCR ganha a tag de versão além do sha; `server-central`/`server-channel` expõem a versão em `GET /healthz`. Ver `docs/architecture.md`
- [x] Provisionar `ffcom.a3sitsolutions.com` (DNS + certificado) para a instância oficial — domínio confirmado como `.com.br` (2026-09-21); coincide com o já usado na implantação de teste (`*.ffcom.a3sitsolutions.com.br`), que passa a ser a instância oficial, sem domínio novo a provisionar. Hostnames mantêm o sufixo `-test` onde já existia. Ver `docs/architecture.md`

## Primeira implantação de teste (infra `a3s-network`, `SVRUBS24IPS0101`)

Ver `docs/architecture.md`, "Decisão: primeira implantação de teste" e a correção logo abaixo ("exposição pública via `VMSUBS24OCI0102`"), para o detalhe completo. Responsabilidade dividida entre este repositório (containers em `SVRUBS24IPS0101`) e um agente de infra separado (proxy reverso/DNS/certificado em `VMSUBS24OCI0102` + túnel até `SVRUBS24IPS0101` — **não** o roteador do site IPS01, que não expõe nada à internet).

- [x] Compose de implantação (`deploy/central/`, `deploy/channel/`, `deploy/client/`), nomes de container fixos, rede overlay `a3s-services` para tráfego HTTP/WebSocket, Postgres isolado em rede interna
- [x] `Dockerfile`/`nginx.conf` do `client` (build da SPA + serve estático) — não existia até esta rodada
- [x] `GET /healthz` sem autenticação em `server-central` e `server-channel`, para o `HEALTHCHECK` do Docker
- [x] Workflows `deploy-ffcom-{central,channel,client}.yml`
- [x] Corrigido ao rodar pela primeira vez (2026-09-20): `ffcom` é repositório pessoal (`apolope/ffcom`), não da organização `a3sitsolutions` — runner de organização `[self-hosted, a3s-network]` não é visível a ele (corrigido com runner dedicado ao repo, mesma máquina) e `GITHUB_TOKEN` não publica em `ghcr.io/a3sitsolutions/*` (corrigido trocando as 3 imagens para `ghcr.io/apolope/ffcom-*`). Ver `docs/architecture.md`.
- [x] Corrigido: `hadolint` barrando o pipeline por `DL3018` (apk sem versão fixa) em `server-central`/`server-channel` — `# hadolint ignore=DL3018` adicionado nos dois Dockerfiles
- [x] Checar portas livres em `SVRUBS24IPS0101` (via SSH, `ss -tuln`, 2026-09-20) — `3478/3479/5349/5350` já ocupados em `10.20.4.10` pelo coturn do stack VoIP/FreeSWITCH existente; `TURN_LISTEN_PORT` ajustado para `33478` em `deploy/channel/.env.example` (o container continua escutando `3478` internamente). Faixas de relay do coturn (`49160-49200`) e RTC do LiveKit (`50000-50100`) livres, sem mudança.
- [x] `.env` reais criados em `/opt/ffcom/envs/ffcom-{central,channel}.env` no host, com segredos gerados (Postgres, LiveKit, TURN)
- [x] `TURN_EXTERNAL_IP` confirmado pelo agente de infra: `137.131.249.145` (IP público fixo de `VMSUBS24OCI0102` — não um IP do site IPS01, ver correção em `docs/architecture.md`), preenchido em `/opt/ffcom/envs/ffcom-channel.env`
- [x] Agente de infra: proxy reverso/certificado/DNS em `VMSUBS24OCI0102` para os 4 hostnames (`app.`, `central.`, `channel-test.`, `livekit-test.ffcom.a3sitsolutions.com.br` → `137.131.249.145`) + encaminhamento das portas de mídia (RTC do LiveKit, STUN/TURN do coturn) até `SVRUBS24IPS0101`
- [x] Atualizar `redirect_uris` do blueprint Authentik (`abs-3d-printer/infra/authentik/blueprints/providers-ffcom.yaml`) para incluir `https://app.ffcom.a3sitsolutions.com.br/auth/callback`
- [x] Subir os containers em `SVRUBS24IPS0101` — todos `healthy`
- [x] Validação ponta a ponta (2026-09-21): DNS, TLS (Let's Encrypt), `/healthz`, upgrade de WebSocket e login OIDC completo, todos confirmados via `curl`/`openssl` e teste manual no client
- [x] Bug: lista de membros mostrava UUID truncado — apelido configurável adicionado (`PATCH /api/me` em `server-channel`, `NicknameDialog.tsx` no client). Ver `docs/architecture.md`
- [x] Bug: 401 visível no console logo após navegar pro client — `useFriends`/`useKnownServers` disparavam fetch com token vazio antes do login OIDC terminar; corrigido com a mesma guarda `if (!accessToken) return` já usada em outros hooks. Ver `docs/architecture.md`
- [x] 3 runners dedicados (`SVRUBS24IPS0101-ffcom{,-02,-03}`) em vez de 1 — os 3 workflows agora rodam de verdade em paralelo. Ver `docs/architecture.md`
- [ ] Testar canal de voz com pelo menos 2 pessoas em redes diferentes (validação real do TURN em `33478`/relay `49160-49200`)
- [ ] Testar convite/entrada de um segundo membro no `channel-test` de ponta a ponta (não só o fundador)
- [ ] Testar roles/permissões negando `ViewChannels` a um membro de teste
- [ ] Testar DM cifrada ponta a ponta com duas contas reais (dois logins/navegadores): confirmar que `PUT /api/me/e2e-public-key` é chamado, que o envio funciona nos dois sentidos, e que o payload do frame `dm.create`/`dm.created` (DevTools) é ciphertext opaco — ver `docs/architecture.md`, "Decisão: criptografia ponta-a-ponta em DMs". Verificado nesta sessão só via teste automatizado direto no store (Postgres real, sem UI)

## server-central

- [x] Modelo de dados: conta, perfil, lista de amigos, diretório de servidores (IP/DNS + nome + ícone por servidor)
- [x] Registrar o app "ffcom" (provider + application) na instância central de Authentik do `abs-3d-printer`, via blueprint declarativo (`infra/authentik/blueprints/providers-ffcom.yaml` naquele repo, commit `03f726d`) — feito com `redirect_uris` provisórios (só `http://localhost:5173/auth/callback`); ver TODO abaixo para o ajuste final
- [x] Atualizar `redirect_uris` em `providers-ffcom.yaml` (repo `abs-3d-printer`) — `app://ffcom/auth/callback` (callback do Electron empacotado) adicionado ao provider "ffcom", ao lado dos dois já existentes (dev local e web/PWA de produção)
- [ ] Adicionar `app://ffcom` a `CORS_ALLOWED_ORIGINS` nos `.env` reais de `server-central`/`server-channel` (`/opt/ffcom/envs/`, fora do git) quando o build Electron empacotado for distribuído de verdade — sem isso, o app desktop instalado não consegue chamar as APIs (ver `docs/architecture.md`, "Decisão: callback OIDC no Electron empacotado")
- [x] Cadastro/login de conta via Authentik
- [x] Endpoint/gateway de presença (quem está online)
- [x] Implementar DMs: `server-central` como gateway de mensagens (armazenamento em Postgres + entrega via WebSocket)
- [x] API para o client listar/adicionar/remover servidores conhecidos (adição manual via IP/DNS ou convite — sem discovery automático)
- [ ] Upload de avatar — `avatar_url` já existe no schema e em `GET/PUT /api/me`, mas não há endpoint de upload nem UI que leia/edite o campo; hoje é campo morto

## server-channel

- [x] Modelo de dados: categorias, canais (texto/voz/forum), mensagens, permissões/roles, convites
- [x] Integração com LiveKit: criar sala por canal de voz, emitir token de acesso (`POST /api/channels/{id}/voice/token`, sala criada implicitamente pelo LiveKit); agora exige o bit `Voice` da permissão efetiva do canal em vez de só "é membro", ver `docs/architecture.md`
- [x] Canal de texto: envio/histórico de mensagens via WebSocket
- [x] CORS configurável (`CORS_ALLOWED_ORIGINS`) para o client chamar de outra origem (REST + WebSocket)
- [x] Canal forum: threads/posts — mesma rota `GET /api/channels/{id}/ws` do canal de texto, frames `thread.create`/`post.create`, REST só para listagem (`GET /api/channels/{id}/threads`, `GET /api/threads/{id}/messages`); reaproveita `ViewChannels`/`SendMessages`, sem bit de permissão próprio; ver `docs/architecture.md`
- [x] Sistema de permissões/roles por servidor e por canal — bits em `internal/permissions` (ViewChannels/SendMessages/Voice/ManageInvites/ManageRoles/Administrator), role default "@everyone" implícita, dono do bootstrap ignora tudo, overwrites de canal por role (`internal/store/channel_overwrites.go`); API: `GET/POST/PATCH/DELETE /api/roles`, `POST/DELETE /api/members/{memberId}/roles/{roleId}`, `GET/PUT/DELETE /api/channels/{id}/overwrites[/{roleId}]`, `GET /api/members`; ver `docs/architecture.md`
- [x] Convites (geração e validação) — `POST/GET /api/invites`, `DELETE /api/invites/{id}`; entrar no servidor (`POST /api/join`) passou a exigir um convite válido, exceto o primeiro membro (fundador/bootstrap do self-host); agora exige a permissão `ManageInvites` em vez de "é membro"; ver `docs/architecture.md`
- [x] Registro do endereço do servidor (para o dono divulgar IP/DNS aos membros) — sem mudança de backend: `InviteServerDialog` gera um link com endereço embutido (`<baseUrl>/?invite=<code>`), `AddServerDialog` reconhece e separa endereço/código de volta; ver `docs/architecture.md`
- [x] Endpoint para editar/apagar mensagem de texto — frames WS `message.update`/`message.delete` em `handleChannelWS` (`internal/httpapi/channel_ws.go`), autor edita/apaga a própria, `Administrator` também apaga qualquer uma; canal forum fora do escopo. Ver `docs/architecture.md`, "Decisão: editar/apagar mensagem de texto"
- [x] Rate limiting em `server-channel` — dois token buckets em memória (`internal/httpapi/ratelimit.go`): por IP em toda a API REST (`RATE_LIMIT_RPM`/`RATE_LIMIT_BURST`, mesmo padrão de `server-central`) e por membro nos frames recebidos numa conexão de WebSocket de canal já aberta (`RATE_LIMIT_WS_RPM`/`RATE_LIMIT_WS_BURST`). Ver `docs/architecture.md`, "Decisão: rate limiting em server-channel"
- [x] Kick/ban de membro — bits `KickMembers`/`BanMembers` em `internal/permissions`; `POST /api/members/{id}/kick` remove logicamente (`removed_at`, preserva mensagens/threads já escritas); `POST /api/members/{id}/ban` idem + `member_bans` (por `oidc_subject`) bloqueia reentrada em `POST /api/join`; `GET/DELETE /api/bans[/{oidcSubject}]` para revisar/revogar. UI em `ManageRolesDialog.tsx`. Ver `docs/architecture.md`, "Decisão: kick/ban de membro"
- [ ] Upload de anexo/imagem em mensagem — `messages.content` é só texto; sem campo, endpoint ou storage para arquivo

## client

- [x] Layout base: rail de servidores → categorias → canais → lista de membros (estilo Discord)
- [x] Login OIDC (Authorization Code + PKCE, `oidc-client-ts`) contra o Authentik central — tela de login antes do shell principal
- [x] Tela de adicionar servidor via IP/DNS (`client/src/components/AddServerDialog.tsx`, via API de `server-central`; ver `client/src/hooks/useKnownServers.ts`)
- [x] API REST em `server-channel` para o client listar categorias/canais reais
- [x] Integração de voz/vídeo via `livekit-client` (`client/src/components/VoiceChannelView.tsx` + `client/src/hooks/useVoiceChannel.ts`; vídeo em si — publicar câmera — ainda não tem controle na UI, só áudio)
- [x] Compartilhamento de tela — `toggleScreenShare`/`screenShareContainerRef` em `client/src/hooks/useVoiceChannel.ts`, botão "Compartilhar tela" em `VoiceChannelView.tsx`; ver `docs/architecture.md`
- [x] Chat de texto em tempo real (histórico via REST + WebSocket, ver `client/src/hooks/useChannelChat.ts`)
- [x] Canal forum (UI de threads) — `client/src/components/ForumChannelView.tsx` + `client/src/hooks/useForumChannel.ts`, ligado em `MainPanel.tsx`; ver `docs/architecture.md`
- [x] Lista de amigos + presença (via `server-central`)
- [x] DMs (API de `server-central` pronta — `GET/POST` via WebSocket de presença e `GET /api/dms/{accountId}/messages`, ver `docs/architecture.md`; UI no client via `client/src/components/DirectMessageView.tsx`)
- [x] Convites de server-channel na UI: `AddServerDialog` ganhou campo opcional de código (chama `POST /api/join` antes de registrar o servidor no diretório) e `ChannelSidebar` ganhou botão "Convidar" (`InviteServerDialog`, gera código via `POST /api/invites`)
- [x] Lista de membros real (`MemberList` via `hooks/useServerMembers.ts`, `GET /api/members`) e painel de administração de roles (`ManageRolesDialog`, botão "Roles" na `ChannelSidebar`, visível só com `ManageRoles`/dono) — overwrite de canal por role ainda não tem UI, só a API (ver server-channel acima)
- [x] Build Electron para Windows/macOS/Linux — `electron-builder` (`client/package.json`, campo `"build"`), scripts `package`/`package:win`/`package:mac`/`package:linux`; sem ícone customizado nem assinatura de código ainda. Ver `docs/architecture.md`
- [x] Build web/PWA — `vite-plugin-pwa` (manifest + service worker via Workbox), ícones gerados por `@vite-pwa/assets-generator` a partir de `favicon.svg`, desligado no build Electron; ver `docs/architecture.md`
- [x] UI para editar/apagar mensagem própria — `TextChannelView.tsx`, botões "editar"/"apagar" visíveis ao passar o mouse na própria mensagem; editar troca por um form inline. Sem botão para Administrator apagar mensagem de outro (permissão já existe no servidor). Ver `docs/architecture.md`
- [ ] Notificação/contador de não lidas — nenhum indicador de atividade em canal/DM fechada; usuário só percebe mensagem nova abrindo cada canal manualmente

## Segurança

- [x] Criptografia em trânsito (TLS) obrigatória entre client e ambos os tipos de servidor — client recusa endereço `http://` fora de localhost (`AddServerDialog.tsx`), servidores recusam requisição sem `X-Forwarded-Proto: https` quando `REQUIRE_TLS=true` (`internal/httpapi/requiretls.go`, padrão desligado). Falta ligar `REQUIRE_TLS=true` no `.env` real da instância de teste (`/opt/ffcom/envs/`, fora do git) depois de confirmar que o NPM em `VMSUBS24OCI0102` seta o header — ver `docs/architecture.md`
- [x] Rate limiting / proteção contra abuso em `server-central` (cadastro, login) — limite geral por IP (token bucket em memória, `internal/httpapi/ratelimit.go`), já que não há endpoint de cadastro/login próprio (conta é criada implicitamente no `auth.Middleware`); configurável via `RATE_LIMIT_RPM`/`RATE_LIMIT_BURST`, padrão 120 req/min e burst 20; `/healthz` isento. Ver `docs/architecture.md`
- [x] Política de permissões/roles em `server-channel` revisada contra escalonamento de privilégio — achado: `ManageRoles` sozinho permitia auto-conceder `Administrator` (criar role com esse bit + se auto-atribuir); corrigido com `permissions.Grants` (bits concedidos via criação/edição de role, atribuição de role e `Allow` de overwrite de canal não podem exceder a permissão base de quem chama). Ver `docs/architecture.md`
- [x] Avaliar necessidade de criptografia ponta-a-ponta em DMs — implementado: NaCl box (X25519-XSalsa20-Poly1305) via `tweetnacl`, chave por dispositivo em `localStorage`, `server-central` armazena/roteia só ciphertext opaco. Ver `docs/architecture.md`, "Decisão: criptografia ponta-a-ponta em DMs", para o desenho completo e as limitações de escopo (sem multi-dispositivo, sem forward secrecy, sem proteção contra operador malicioso)
- [ ] Ligar `automaticSilentRenew: true` (ou mitigar de outra forma) — hoje está desligado (`docs/architecture.md`, decisão consciente na v1), token expira e força novo login sem aviso; fricção real já registrada como "revisitar quando" mas sem item de TODO correspondente
- [ ] Ligar `REQUIRE_TLS=true` no `.env` real da instância de teste (`/opt/ffcom/envs/`, fora do git) — a checagem já existe no código (`internal/httpapi/requiretls.go`), falta só confirmar que o NPM em `VMSUBS24OCI0102` seta `X-Forwarded-Proto` e ativar a flag

## Documentação

- [x] Guia de self-hosting de `server-channel` (Docker Compose, TURN, DNS dinâmico, TLS via proxy reverso) — `server-channel/README.md`; READMEs de `server-central`/`client`/raiz também atualizados (estavam desatualizados, ainda descreviam o projeto como scaffolding sem código funcional). Ver `docs/architecture.md`
- [x] Guia de contribuição (`CONTRIBUTING.md`) — estrutura do repo, ambiente de dev (Docker Compose por componente / `go run .` local), convenções de código (aponta para as decisões já registradas em `docs/architecture.md` em vez de duplicá-las), como rodar testes/lint localmente (CI ainda não roda nenhum dos dois — ver `TODO.md` acima), estilo de commit observado no `git log`, e o processo de marcar item do TODO + registrar decisão em `docs/architecture.md`
- [x] Documentar protocolo/API entre os três componentes assim que definido — `docs/protocol.md`: endpoints REST, frames de WebSocket, autenticação, CORS e convenções dos dois servidores
- [ ] Guia de backup/restore do Postgres — nenhum guia de self-hosting cobre como fazer backup/restore dos bancos de `server-central`/`server-channel`, que são a única fonte de dados do sistema

## Fora de escopo da v1 (registrado para não esquecer)

- [ ] Cliente mobile
- [ ] Federação entre instâncias de `server-central`
