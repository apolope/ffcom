# Decisões de arquitetura — FFCom

Este documento registra as decisões técnicas do projeto no formato "decisão + alternativas consideradas + razão", para servir de referência viva conforme o projeto evolui. Data da pesquisa inicial: 2026-09-19.

## Contexto

O Discord está enfrentando problemas jurídicos que deixaram uma lacuna no mercado: não existe hoje uma alternativa madura com a mesma organização (servidores, categorias, canais de texto/voz/forum) que possa ser hospedada pela própria pessoa. O FFCom resolve isso com três componentes independentes: `client`, `server-central` (instância única oficial) e `server-channel` (self-hosted por cada comunidade).

## Prior art

[Gryt](https://github.com/Gryt-chat/gryt) é um projeto de código aberto que já resolve uma forma muito próxima deste problema: cliente desktop (Electron) + web, servidor de sinalização, SFU próprio em Go sobre [Pion](https://github.com/pion/webrtc), autenticação via Keycloak, self-hosted via Docker Compose ("um compose file pode hospedar quantos servidores você quiser compartilhando o mesmo SFU"). Não será copiado ou usado como dependência, mas validou que a forma geral da arquitetura proposta aqui é viável na prática e serve de referência de padrões (ex.: bundle de Docker Compose por operador, SFU compartilhado entre vários "servidores" no mesmo host).

Outros pontos de referência no mesmo espaço: [Spacebar](https://github.com/spacebarchat) (reimplementa a API do Discord no backend, permitindo clients/bots existentes apontarem para uma instância própria), [Backspace](https://github.com/Cirzear/backspace) (TypeScript, federação servidor-a-servidor, AGPL-3.0).

## Decisão: SFU de voz/vídeo — LiveKit

**Alternativas consideradas:** LiveKit, mediasoup, Janus, SFU próprio sobre Pion (caminho do Gryt).

**Decisão:** [LiveKit](https://github.com/livekit/livekit) (Go, construído sobre Pion).

**Razão:** a escala alvo é média/grande (centenas de participantes simultâneos por canal de voz), o que exige um SFU dedicado — malha P2P não resolve nesse patamar. Entre as opções maduras, LiveKit entrega mais pronto: SDKs de cliente para web/Electron, arquitetura de malha distribuída (vários nós coordenados via Redis, cada participante conecta no nó mais próximo, uma sala pode span múltiplos servidores físicos), gravação e webhooks nativos, self-host como binário Go único em Docker. mediasoup dá mais controle de baixo nível (rota que times maiores escolhem quando já sabem exatamente o que customizar, ex. spatial audio, jogos), mas exige muito mais engenharia própria — é o caminho que o Gryt tomou construindo o SFU do zero sobre Pion, viável mas caro em tempo de engenharia para um time pequeno. Janus é mais focado em interoperar com SIP/telefonia (PBX, Asterisk/FreeSWITCH), o que não é o caso de uso aqui.

**Implicação de deploy:** cada `server-channel` roda (ou aponta para) um LiveKit self-hosted via Docker, mais um [coturn](https://github.com/coturn/coturn) para TURN/NAT traversal — necessário porque quem hospedar um `server-channel` em casa estará atrás de NAT. Documentar port-forwarding/DNS dinâmico é parte do TODO de infraestrutura.

**Revisitar quando:** se algum canal precisar de controle de mídia muito específico (ex. áudio espacial, mixagem custom) que o LiveKit não expõe.

## Decisão: linguagem de server-central e server-channel — Go

**Alternativas consideradas:** Java/Spring Boot (stack já usado em outros projetos do autor), Node.js/TypeScript, Elixir/Phoenix, Go.

**Decisão:** Go para os dois componentes de servidor.

**Razão:**
1. `server-channel` será baixado e rodado por terceiros, muitas vezes leigos em deploy — um binário Go único + Docker Compose é ordens de grandeza mais simples de distribuir do que uma JVM ou o runtime do BEAM.
2. LiveKit já é Go; `server-channel` integra com o SDK/webhooks dele nativamente, sem trocar de linguagem na borda entre controle e mídia.
3. Go tem concorrência nativa (goroutines) suficiente para o fanout de WebSocket (texto, presença, sinalização) na escala alvo, sem precisar da infraestrutura de Elixir/Phoenix.

**Contraponto registrado:** o próprio Discord rodou anos em Elixir justamente porque o modelo de processos do BEAM é arquiteturalmente superior para presença/WebSocket em massa (Phoenix Presence + Channels resolve isso de forma nativa), e só reescreveu partes quentes em Rust depois, em escala de dezenas de milhões de conexões. Essa vantagem não paga o custo de manter um segundo runtime no stack enquanto o `server-central` não estiver nessa escala. **Revisitar quando:** lista de amigos/presença do `server-central` virar gargalo real — Elixir/Phoenix é o candidato natural para essa peça especificamente, não para o `server-channel`.

## Decisão: client desktop — Electron (não Tauri)

**Alternativas consideradas:** Electron, Tauri.

**Decisão:** Electron, envolvendo a mesma SPA React+TypeScript usada no caminho web (PWA).

**Razão:** Tauri usa o webview nativo do sistema operacional (WebView2 no Windows, WKWebView no macOS, WebKitGTK no Linux) — apesar do bundle até ~96% menor, esses webviews têm suporte inconsistente a APIs de WebRTC como captura de tela (`getDisplayMedia`) entre plataformas. Electron embute o próprio Chromium, garantindo que `getUserMedia`/`getDisplayMedia` se comportem igual em todo lugar. Como o core do produto é voz/vídeo/compartilhamento de tela, essa consistência pesa mais que o tamanho do instalador na v1.

**Revisitar quando:** o produto estiver estável e o peso do instalador virar reclamação real de usuários — nesse ponto vale reavaliar o estado do WebRTC em Tauri.

## Decisão: wrapper Electron — vite-plugin-electron, sem empacotamento ainda

**Alternativas consideradas:** `electron-vite` (build tool separado, próprio CLI/config), scripts manuais com `concurrently` + `wait-on` + `tsc`, `vite-plugin-electron`.

**Decisão:** `vite-plugin-electron` (variante `/simple`), plugado condicionalmente no `vite.config.ts` existente via `mode === 'electron'`. Processos `main` e `preload` ficam em `client/electron/*.ts`, buildados pelo próprio esbuild do plugin (sem `tsc` para emitir esses arquivos; `tsc -b` roda só para type-check).

**Razão:** reaproveita a configuração/plugins do Vite que já existem para a SPA web em vez de introduzir uma segunda ferramenta de build (`electron-vite`) ou orquestração manual de processos. `npm run dev` (web/PWA) continua idêntico; `npm run dev:electron` liga o mesmo dev server dentro de uma janela Electron. Empacotamento em instalador (electron-builder ou similar) fica fora de escopo deste item — é o TODO separado "Build Electron para Windows/macOS/Linux".

**Implicação:** `package.json` do `client` ganhou `"main": "dist-electron/main.js"` e os scripts `dev:electron`, `build:electron`, `electron`. `preload.mjs` é gerado com extensão `.mjs` pelo plugin (compatível com `"type": "module"` do projeto); `main.js` sai como ESM válido apesar da extensão `.js`, porque `"type": "module"` já é o padrão do pacote.

**Revisitar quando:** o wrapper precisar de recursos que o `vite-plugin-electron` não cobre bem (ex. multi-processo complexo, hot-reload do main process mais sofisticado) — `electron-vite` seria o próximo candidato.

## Decisão: dados e transporte

- **PostgreSQL** em `server-central` (contas, amigos, diretório de servidores) e em cada `server-channel` (canais, mensagens, permissões, forum) — bancos relacionais separados por componente, sem banco compartilhado entre instâncias de `server-channel` diferentes.
- **WebSocket** para texto, presença e sinalização de conexão ao LiveKit.
- A mídia de voz/vídeo em si trafega direto pelo LiveKit (SRTP/UDP), não pelo WebSocket da aplicação.

## Decisão: autenticação em server-central — Authentik (OIDC) da instância central do abs-3d-printer

**Alternativas consideradas:** usuário/senha simples implementado à mão, Keycloak, Authentik self-hosted dedicado ao FFCom, reusar a instância central de Authentik já operada pelo `abs-3d-printer` (`authentik.abs.a3sitsolutions.com.br`).

**Decisão:** reusar a instância central de Authentik do `abs-3d-printer`. O FFCom **não** roda seu próprio Authentik — `server-central` se registra nela como mais um app (Resource Server), seguindo o mesmo mecanismo de Infrastructure-as-Code (blueprints YAML declarativos) já usado por outros projetos que reusam essa instância (ex. `a3s-network`, `hms-financeiro`). Procedimento de referência (mantido no repo irmão): `D:\Dev\a3s-network\docs\procedures\integrar-app-com-authentik.md`.

**Razão:** essa instância já existe, é operada e mantida pela mesma pessoa/equipe, e já serve de IdP para vários projetos não relacionados entre si (ver o padrão `hms-financeiro` na mesma instância) — subir um Authentik dedicado só para o FFCom duplicaria infraestrutura (Postgres extra ou banco extra, Redis, upgrade/manutenção do Authentik) sem nenhum ganho, já que não há necessidade de isolar usuários entre instâncias.

**Como funciona na prática:**
- `server-central` é **Resource Server puro**: só valida Bearer JWT via `issuer`/JWKS (`OIDC_ISSUER_URL` apontando para `https://authentik.abs.a3sitsolutions.com.br/application/o/ffcom/`), sem `client_id`/`client_secret` próprio.
- Quem faz o login de verdade (Authorization Code + PKCE, client público, sem secret) é o `client` (SPA React/Electron), contra a mesma instância central — mesmo padrão usado pelas outras SPAs que reusam esse Authentik (`abs3d-academy`, `a3s-ops-portal`, `hms-financeiro`).
- O app "ffcom" (grupo + provider + application) precisa ser cadastrado via blueprint declarativo no repositório `abs-3d-printer` (`infra/authentik/blueprints/`), não pela UI — seguindo o passo a passo do procedimento citado acima. Isso ainda está pendente (ver TODO.md); os `redirect_uris` de produção dependem do domínio final do `client` web, ainda não decidido.
- CORS: o Authentik dessa instância não libera `Access-Control-Allow-Origin` automaticamente — a origem do `client` precisa ser adicionada manualmente na allowlist do proxy reverso (NPM) na frente do Authentik antes do login funcionar no navegador. **Feito em 2026-09-20 para `http://localhost:5173`** (origem de dev do `client`) — ver `D:\Dev\a3s-network\docs\services\nginx-proxy-manager.md` para o procedimento e o registro da mudança (repo/host fora do `ffcom`). Precisa ser revisto/trocado quando o domínio de produção do `client` for decidido (mesmo gatilho do ajuste de `redirect_uris` — ver TODO.md).

**Implicação de deploy:** o Docker Compose de referência de `server-central` só precisa do app Go e do Postgres — nenhum serviço de Authentik/Redis local.

**Revisitar quando:** o FFCom precisar de isolamento total de usuários/áudias em relação aos outros projetos que usam essa instância central (hoje não é o caso — todo mundo compartilha o mesmo diretório de contas do Authentik, só com apps/grupos diferentes).

**Status do cadastro do app "ffcom":** feito — `infra/authentik/blueprints/providers-ffcom.yaml` no repo `abs-3d-printer` (commit `03f726d`, aplicado via `ak apply_blueprint`, confirmado na UI). Client público `ffcom`, sem grupo dedicado nem claim `groups` (mesmo padrão do `hms-financeiro` — o FFCom não usa grupos do Authentik para autorização; permissões de servidor/canal são geridas pelo próprio `server-channel`). `redirect_uris` ainda são **provisórios**, só cobrindo o `client` web em desenvolvimento local (`http://localhost:5173/auth/callback`), porque o domínio de produção do `client` (web/PWA) e o esquema de callback do build Electron empacotado ainda não foram decididos — ver TODO.md para o ajuste pendente (um segundo prompt, mesmo molde do primeiro, atualiza `redirect_uris` quando essa decisão existir).

## Decisão: modelo de DMs — server-central medeia diretamente

**Alternativas consideradas:** `server-central` mediar diretamente, DM efêmera via WebRTC direto entre clients, `server-channel` "pessoal" auto-provisionado por par de amigos.

**Decisão:** `server-central` medeia diretamente as DMs.

**Razão:** não existe um `server-channel` fixo compartilhado entre dois amigos — como o `server-central` já é o diretório de contas/amigos, ele é o único componente que naturalmente conhece ambos os lados de uma DM. Isso expande seu escopo de "só diretório" para "também gateway de mensagens diretas", mas evita provisionar infraestrutura de channel por par de usuários.

**Implicação:** `server-central` precisa de armazenamento de mensagens de DM (Postgres) e entrega em tempo real via WebSocket, reaproveitando o mesmo transporte já decidido para presença.

**Revisitar quando:** criptografia ponta-a-ponta em DMs for exigida (já listado em Segurança) — o `server-central` passaria a rotear payloads cifrados sem conseguir lê-los, mas a mediação continua.

## Decisão: descoberta de server-channel — nenhuma, apenas convite ou IP manual

**Alternativas consideradas:** diretório público pesquisável no `server-central`, descoberta manual via IP/DNS, convite explícito.

**Decisão:** não existe mecanismo de descoberta. O client só conhece um `server-channel` por convite (link/código gerado pelo dono do servidor) ou por adição manual direta do endereço.

**Razão:** manter o `server-channel` privado por padrão é consistente com o modelo de comunidade self-hosted — o dono decide quem entra, sem exigir que a instância fique listada em algum lugar. Evita todo o trabalho de moderação/opt-in de um diretório público na v1.

**Revisitar quando:** houver demanda por comunidades públicas descobríveis (ex. um "diretório oficial opt-in"). Isso viraria uma feature nova de opt-in explícito, não uma mudança no modelo padrão.

## Decisão: protocolo entre client, server-central e server-channel

**Alternativas consideradas:** REST + WebSocket sem canal direto central↔channel; REST + WebSocket + gRPC central↔channel; GraphQL.

**Decisão:** REST para operações CRUD/stateless (autenticação via Authentik/OIDC, diretório de servidores, permissões, histórico paginado, convites) e WebSocket para tempo real (chat, presença, sinalização de conexão ao LiveKit), tanto em client↔server-central quanto em client↔server-channel. Não existe canal direto `server-central`↔`server-channel`; os dois só se relacionam através do client.

**Razão:** já era a direção apontada em "Decisão: dados e transporte"; formaliza aqui que não há necessidade de gRPC entre central e channel, já que a descoberta é manual/por convite (não um índice que o central precisa consultar nos channels).

**Revisitar quando:** o `server-central` precisar puxar ativamente status/contagem de membros de um `server-channel` (hoje desnecessário, pois não há diretório público).

## Decisão: injeção de config no Docker Compose de `server-channel` — variáveis de ambiente, não arquivos montados

**Alternativas consideradas:** arquivo `livekit.yaml`/`turnserver.conf` montado como volume (com placeholders que o self-hoster edita), variáveis de ambiente para tudo.

**Decisão:** um único `.env` concentra todos os segredos/parâmetros; o LiveKit recebe a config completa via `LIVEKIT_CONFIG` (variável de ambiente suportada nativamente pelo binário `livekit-server`, contendo o YAML como string) e o coturn recebe os parâmetros via flags de linha de comando (`command:` no compose), ambos com substituição de `${VAR}` feita pelo próprio Docker Compose a partir do `.env`.

**Razão:** o público-alvo de `server-channel` é gente leiga em deploy (mesmo argumento já registrado na decisão de linguagem). Um único arquivo `.env` para editar é mais simples do que pedir para editar `.env` + `livekit.yaml` + `turnserver.conf` mantendo os mesmos segredos sincronizados em três lugares. A substituição de variáveis do Compose funciona em qualquer parte do arquivo `docker-compose.yml`, incluindo dentro de block scalars YAML (o `LIVEKIT_CONFIG: |`), então não há perda de expressividade frente a um arquivo de config separado.

**Revisitar quando:** a config do LiveKit ou do coturn precisar de opções avançadas demais para caber confortavelmente em flags/env vars (ex. redis para LiveKit multi-nó) — nesse ponto um arquivo montado volta a fazer sentido.

## Decisão: acesso a Postgres e migrations em server-central — pgx + golang-migrate embutido

**Alternativas consideradas:** `database/sql` + `lib/pq`, `pgx` puro, ORM (GORM/ent), `sqlc`; para migrations: golang-migrate com arquivos em disco, goose, migrations rodadas manualmente via script externo, golang-migrate com SQL embutido no binário (`embed.FS`).

**Decisão:** `github.com/jackc/pgx/v5` (via `pgxpool`) como driver, sem ORM — queries SQL escritas à mão em `internal/store`, uma struct de repositório por entidade (`AccountStore`, `ProfileStore`, `FriendshipStore`, `KnownServerStore`). Migrations versionadas (`migrations/0001_init.{up,down}.sql`) aplicadas com `golang-migrate/migrate/v4`, cujos arquivos `.sql` são embutidos no binário via `//go:embed` (pacote `migrations`) e aplicadas automaticamente (`migrate.Up()`) na inicialização do processo, antes do servidor aceitar requisições.

**Razão:** pgx é o driver Postgres mais idiomático/performático do ecossistema Go atual e já citado como padrão razoável dado que o resto do stack é Go puro; um ORM adicionaria uma camada de abstração desnecessária para o volume de queries esperado nesta fase. Embutir as migrations no binário (em vez de exigir um diretório `migrations/` ao lado do executável ou um passo manual de `migrate up`) mantém a mesma filosofia de simplicidade de deploy já registrada nas decisões de linguagem e de config via `.env`: subir o container já deixa o schema no estado certo, sem passo extra.

**Modelo de dados criado (migration `0001_init`):** `accounts` (id + `oidc_subject` único, vínculo com o "sub" do Authentik — server-central não guarda senha), `profiles` (1:1 com `accounts`, dados editáveis: nome de exibição, avatar), `friendships` (relação direcionada `requester_id`/`addressee_id` com `status` pending/accepted/blocked), `known_servers` (diretório de `server-channel` conhecidos por conta: endereço, nome, ícone — reflete a decisão "descoberta de server-channel: nenhuma, apenas convite ou IP manual").

**Revisitar quando:** o volume/complexidade de queries justificar `sqlc` (geração de código a partir de SQL, mantém controle total sobre a query mas com type-safety) — hoje o número de entidades é pequeno demais para pagar esse custo de tooling.

## Decisão: validação de JWT em server-central — coreos/go-oidc/v3, sem cadastro separado

**Alternativas consideradas:** parsear o JWT à mão (`golang-jwt/jwt` + fetch manual do JWKS), `coreos/go-oidc/v3`; para o momento de criar a conta local: endpoint de "cadastro" explícito (`POST /api/accounts`) chamado uma vez pelo client após o primeiro login, vs. criar a conta implicitamente na primeira requisição autenticada.

**Decisão:** `github.com/coreos/go-oidc/v3` — `oidc.NewProvider(ctx, issuerURL)` busca o discovery document (JWKS incluso) e `provider.Verifier(&oidc.Config{SkipClientIDCheck: true})` valida assinatura/issuer/expiração de qualquer JWT emitido por ele (`internal/auth/verifier.go`). `SkipClientIDCheck: true` porque a validação de audiência não é feita pelos outros backends da organização que reusam esse mesmo Authentik (ver `D:\Dev\a3s-network\docs\procedures\integrar-app-com-authentik.md` — os backends Spring configuram só `issuer-uri`, que valida `iss`/`exp`/assinatura, não `aud`); manter o mesmo comportamento evita depender de uma suposição não confirmada sobre o formato de `aud` no access token do Authentik.

Não existe endpoint de cadastro separado: `internal/auth/middleware.go` (`auth.Middleware`) extrai o `sub` do token e chama `AccountStore.GetOrCreateBySubject` (upsert idempotente) em toda requisição autenticada, antes de chamar o handler. A primeira requisição de um `sub` novo — qualquer rota protegida, não só um endpoint específico de "login" — já cria a conta.

**Razão:** biblioteca é a forma idiomática em Go de fazer o que o Spring Boot Resource Server faz automaticamente (auto-config a partir de `issuer-uri`), evitando reimplementar fetch/cache de JWKS e validação de claims à mão. Criar a conta dentro do próprio middleware (em vez de um passo de cadastro explícito) elimina uma corrida possível entre "logou mas ainda não chamou /cadastro" e reflete que `server-central` nunca precisa de um estado "usuário autenticado mas sem conta" — a Account é 1:1 com o `sub` do Authentik e não tem nenhum dado obrigatório além disso (perfil é preenchido depois, separadamente, via `ProfileStore.Upsert`).

**Revisitar quando:** o Authentik desta instância passar a emitir `aud` de forma confiável e granular por client — nesse ponto vale reativar `ClientID` no `oidc.Config` para reforçar que um token de outro app não é aceito aqui (hoje isso já é implicitamente impossível, já que cada app tem seu próprio `issuer` por slug — `/application/o/<slug>/` — mas a checagem explícita de `aud` seria defesa em profundidade).

## Decisão: modelo de dados de server-channel — mesmo padrão de server-central, membro ligado direto ao Authentik central

**Alternativas consideradas (identidade do membro):** `server-channel` chamar `server-central` para resolver conta/perfil a cada requisição; `server-channel` replicar/cachear contas via alguma sincronização; `server-channel` validar o JWT localmente contra a mesma instância central de Authentik, sem nenhuma chamada a `server-central`.

**Decisão:** `server-channel` valida o token localmente contra a mesma instância central de Authentik (`OIDC_ISSUER_URL` igual à de `server-central`), do mesmo jeito que `server-central` já faz (`coreos/go-oidc/v3`, `SkipClientIDCheck: true`). Uma tabela `members` guarda só `oidc_subject` + `nickname` (apelido específico deste servidor) — não há linha "conta" duplicada nem chamada a `server-central` para resolver quem é o usuário.

**Razão:** já estava implícito em "Decisão: protocolo entre client, server-central e server-channel" (a autenticação via Authentik/OIDC vale tanto para client↔server-central quanto client↔server-channel), mas não estava detalhado. Como não existe canal direto `server-central`↔`server-channel` (decisão já registrada acima), a única forma de `server-channel` saber quem é o usuário sem introduzir esse canal é validar o mesmo token de forma independente — o que funciona porque o issuer da aplicação "ffcom" é único e compartilhado, sem precisar de registro por instância de `server-channel` na Authentik.

**Modelo de dados criado (migration `0001_init` de `server-channel`):** `members` (identidade local, `oidc_subject` único), `categories` (agrupam canais; não há tabela "servers" — a instância é o servidor), `channels` (tipo `text`/`voice`/`forum`, categoria opcional), `roles` (nome, cor, `permissions` como bitmask `BIGINT` ainda sem os bits definidos — isso é o TODO separado "Sistema de permissões/roles por servidor e por canal"), `member_roles` (N:N), `threads` (discussões dentro de um canal forum), `messages` (cobre tanto o histórico de um canal de texto quanto os posts de uma thread, via `thread_id` opcional), `invites` (código único, limite de usos e expiração opcionais). Segue o mesmo padrão de acesso a dados já usado em `server-central`: `pgx`/`pgxpool` sem ORM, um repositório por entidade em `internal/store`, migrations embutidas no binário via `//go:embed` e aplicadas automaticamente no boot.

**Revisitar quando:** o Authentik desta instância passar a emitir `aud` granular por client (mesmo gatilho já registrado na decisão de validação de JWT em `server-central` — nesse ponto reativar `ClientID` faria sentido nos dois componentes).

## Decisão: canal de texto em server-channel — gorilla/websocket, REST para histórico

**Alternativas consideradas (biblioteca WebSocket):** `gorilla/websocket`, `coder/websocket` (ex-`nhooyr.io/websocket`), `net/http` puro (sem lib, não cobre o handshake).

**Decisão:** `github.com/gorilla/websocket`, com o padrão hub/client (goroutine `ReadPump`/`WritePump` por conexão, ping/pong a cada 54s, `send` bufferizado por client) descrito no próprio exemplo oficial da lib.

**Razão:** é a biblioteca WebSocket mais madura e documentada do ecossistema Go, e o padrão hub/client do exemplo oficial resolve diretamente o requisito de fanout (uma mensagem nova precisa chegar a todos os membros conectados ao mesmo canal) sem reinventar controle de concorrência sobre `*websocket.Conn` (que não é seguro para escritas concorrentes).

**Split REST/WebSocket:** já estava definido em "Decisão: protocolo entre client, server-central e server-channel" (REST para CRUD/stateless, WebSocket para tempo real). Aplicado concretamente: `GET /api/channels/{id}/messages` (REST, paginação por `before`/`limit`, keyset em `created_at`) devolve histórico; `GET /api/channels/{id}/ws` (WebSocket) recebe frames `message.create` do client e distribui `message.created` (broadcast, inclusive para o autor, para confirmar id/timestamp atribuídos pelo servidor) via `internal/realtime.Hub`, um hub por canal mantido em memória pelo processo de `server-channel`. Escopo desta decisão é só canal de texto (`ThreadID` sempre nulo); posts de thread de forum ficam para o TODO separado "Canal forum: threads/posts".

**Formato de frame:** envelope JSON `{"type": "...", ...}`; hoje só `message.create` (client → servidor), `message.created` e `error` (servidor → client). Mensagem vazia ou maior que 4000 caracteres é rejeitada com `error` antes de tocar o Postgres.

**Autenticação no handshake de WebSocket:** a API `WebSocket` do navegador não permite setar headers customizados na abertura da conexão, então o client não consegue mandar `Authorization: Bearer <token>` como faz nas chamadas REST (`fetch`). `internal/auth.Middleware` (compartilhado entre REST e WebSocket) agora aceita o token por dois caminhos: header `Authorization` (rotas REST) ou subprotocolo `Sec-WebSocket-Protocol: access_token, <token>` (rota de WebSocket, setado via `new WebSocket(url, ["access_token", token])` no client), que é a forma padrão de carregar credenciais num handshake de WS sem colocar o token na URL/query string (evita vazamento em access log/histórico/referrer, relevante já que TLS obrigatório ainda é TODO em aberto). O `Upgrader` de `internal/httpapi/channel_ws.go` declara `Subprotocols: []string{"access_token"}` para aceitar e ecoar esse subprotocolo no handshake.

**Política de origem do Upgrade:** mantido o `CheckOrigin` padrão do gorilla (exige `Origin` igual a `Host` quando o header vem presente). Isso é suficiente para localhost/dev com client e server-channel no mesmo host, mas **vai bloquear** o client (origem própria, ex. PWA em outro domínio, ou build Electron) assim que a integração real começar — nenhuma rota de `server-channel` libera CORS/Origin cruzado hoje, nem as REST existentes. Revisitar junto do TODO "Layout base" do client, quando o client de fato passar a chamar `server-channel` de uma origem diferente.

**Revisitar quando:** o volume de conexões simultâneas por canal justificar mover o hub para fora do processo (ex. Redis pub/sub), caso `server-channel` algum dia precise rodar em múltiplas réplicas — hoje é um único processo por comunidade, então hub em memória é suficiente.

## Decisão: login OIDC no client — `oidc-client-ts` direto, sem framework de auth adicional

**Alternativas consideradas:** `react-oidc-context` (wrapper de contexto React sobre `oidc-client-ts`), `@axa-fr/react-oidc`, `oidc-client-ts` puro com um `AuthProvider` próprio.

**Decisão:** `oidc-client-ts` puro (`UserManager`), envolvido por um `AuthProvider`/`useAuth` próprios em `client/src/auth/`, sem lib de wrapper React adicional.

**Razão:** o app já não usa nenhuma lib de estado/roteamento (nem `react-router`); adicionar `react-oidc-context` só para reexpor o mesmo `UserManager` via um Context um pouco diferente do que escrever esse Context à mão (~60 linhas) não paga a dependência extra. `oidc-client-ts` é a lib de referência do ecossistema (mesma base usada por `react-oidc-context`/`@axa-fr/react-oidc`), com `WebStorageStateStore` para persistir sessão em `localStorage`.

**Fluxo implementado:** Authorization Code + PKCE, client público `ffcom` (mesmo cadastrado em `abs-3d-printer/infra/authentik/blueprints/providers-ffcom.yaml`), `redirect_uri` fixo em `${origin}/auth/callback` (hoje só `http://localhost:5173/auth/callback` está registrado — ver TODO.md sobre o ajuste de `redirect_uris` de produção). `client/src/auth/AuthProvider.tsx` detecta `pathname === '/auth/callback'` no mount, chama `signinRedirectCallback()` e limpa a URL; fora do callback, tenta restaurar sessão existente via `getUser()`. Um `useRef` evita processar o callback duas vezes sob o double-invoke de efeitos do `StrictMode` em dev (o `code` do Authorization Code é de uso único).

**Sem renovação silenciosa automática (`automaticSilentRenew: false`):** exigiria um segundo `redirect_uri` (iframe de silent renew) cadastrado no blueprint do Authentik, que hoje só tem o callback principal. Quando o access token expira, o usuário loga de novo — aceitável nesta fase.

**Revisitar quando:** o app ganhar roteamento real (`react-router` ou similar) — nesse ponto vale avaliar se `react-oidc-context` (que já integra bem com rotas protegidas) passa a valer a pena. Renovação silenciosa via refresh token pode ser revisitada junto do ajuste de `redirect_uris` de produção.

## Decisão: CORS em server-channel — origens liberadas via `CORS_ALLOWED_ORIGINS`

**Contexto:** já estava registrado como limitação conhecida na decisão de canal de texto acima ("vai bloquear o client... nenhuma rota de server-channel libera CORS/Origin cruzado hoje") — o gatilho previsto ali ("quando o client de fato passar a chamar server-channel de uma origem diferente") aconteceu ao implementar o chat de texto real no client (`http://localhost:5173` chamando `server-channel` em `http://localhost:8080`).

**Alternativas consideradas:** liberar CORS para qualquer origem (`*`), lista fixa hardcoded no binário, lista configurável via variável de ambiente.

**Decisão:** variável de ambiente `CORS_ALLOWED_ORIGINS` (lista separada por vírgula), vazia por padrão (nenhuma origem cruzada liberada — mantém o comportamento restrito anterior). `internal/httpapi/cors.go` (`withCORS`, aplicado a todo o mux) responde o preflight `OPTIONS` e ecoa `Access-Control-Allow-Origin` só para origens na lista; `internal/httpapi/channel_ws.go` (`newUpgrader`) aplica a mesma lista ao `CheckOrigin` do upgrader de WebSocket, mantendo como fallback o comportamento padrão do gorilla (sem header `Origin`, ou `Origin` igual a `Host`, sempre passa).

**Razão:** `*` abriria a API para qualquer site read/write com o token de um usuário logado em outra aba — inaceitável já que a API aceita Bearer token. Hardcoded no binário exigiria rebuild por self-hoster toda vez que o domínio do client mudar; variável de ambiente segue a mesma filosofia já registrada na decisão de config via `.env` (self-hoster leigo edita um arquivo, não recompila).

**Revisitar quando:** o domínio de produção do client (web/PWA) e o esquema de callback do Electron empacotado forem decididos (mesmo gatilho do ajuste de `redirect_uris` do Authentik) — nesse ponto o `CORS_ALLOWED_ORIGINS` de produção precisa incluir esse domínio.

## Questões em aberto (não resolvidas pela pesquisa, viram TODO)

- **Mobile:** fora do escopo da v1 (cliente é web + desktop); entra como tema separado no TODO.

## Fontes consultadas (2026-09-19)

- [mediasoup, Janus, LiveKit, Jitsi Videobridge, Pion: Choosing an SFU](https://www.forasoft.com/learn/video-streaming/articles-streaming/sfu-comparison-mediasoup-janus-livekit-jitsi-pion)
- [LiveKit vs Mediasoup vs Janus: Best WebRTC SFU (2026)](https://trembit.com/blog/choosing-the-right-sfu-janus-vs-mediasoup-vs-livekit-for-telemedicine-platforms/)
- [LiveKit self-hosted deployments — docs oficiais](https://docs.livekit.io/deploy/custom/deployments/)
- [GitHub — livekit/livekit](https://github.com/livekit/livekit)
- [GitHub — Gryt-chat/gryt](https://github.com/Gryt-chat/gryt) e [Gryt-chat/sfu](https://github.com/Gryt-chat/sfu)
- [Best Open Source Self-Hosted Alternatives to Slack and Discord in 2026 (Pinggy)](https://pinggy.io/blog/best_open_source_alternatives_to_slack_and_discord/)
- [Real time communication at scale with Elixir at Discord](https://elixir-lang.org/blog/2020/10/08/real-time-communication-at-scale-with-elixir-at-discord/)
- [Elixir vs Go 2026: Critical Backend Performance Comparison](https://bytepulse.io/elixir-vs-go-2026-2026/)
- [Tauri vs Electron [2026]: 96% Smaller Apps](https://tech-insider.org/tauri-vs-electron-2026/)
- [Tauri vs. Electron discussion — SpacingBat3/WebCord](https://github.com/SpacingBat3/WebCord/discussions/181)
