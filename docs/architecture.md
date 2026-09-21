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

## Decisão: integração de voz com LiveKit — token assinado localmente, sem SDK oficial

**Contexto:** implementar o item de TODO "Integração com LiveKit: criar sala por canal de voz, emitir token de acesso, aplicar permissões" em `server-channel`.

**Alternativas consideradas:** usar `github.com/livekit/server-sdk-go` (SDK oficial, reexporta `github.com/livekit/protocol` para montar o `AccessToken`); assinar o JWT do access token diretamente com uma lib JWT genérica, sem depender do SDK; criar a sala explicitamente via `RoomServiceClient` (API HTTP do LiveKit) antes de devolver o token.

**Decisão:**
1. **Sem SDK oficial:** `github.com/livekit/protocol` (dependência do `server-sdk-go` só para montar um `AccessToken`) arrasta `pion/webrtc`, `redis` e `prometheus` de trânsito — peso incompatível com a decisão "server-channel é um binário Go único fácil de distribuir" (ver "Decisão: linguagem de server-central e server-channel — Go" abaixo). O formato do access token do LiveKit é um JWT HS256 documentado e estável, então `internal/livekit/token.go` assina com `github.com/golang-jwt/jwt/v5` (dependência já leve, sem transitivos pesados) em vez de importar o SDK inteiro.
2. **Sala não é criada explicitamente:** `POST /api/channels/{id}/voice/token` não chama a API do LiveKit — o próprio LiveKit cria a sala (nome = id do canal de voz) implicitamente no primeiro participante que entrar com um token válido para aquele nome. Evita depender de `RoomServiceClient` (que teria trazido de volta o SDK pesado) só para um `CreateRoom` que o LiveKit já faz sozinho.
3. **Permissões:** como o sistema de permissões/roles do FFCom (TODO.md, "Sistema de permissões/roles por servidor e por canal") ainda não existe, todo membro autenticado deste `server-channel` recebe o mesmo grant (`roomJoin`, `canPublish`, `canSubscribe`, `canPublishData`) para qualquer canal de voz — o único controle de acesso hoje é "é membro deste server-channel" (via `auth.Middleware`), o mesmo nível que já vale para canais de texto.
4. **Descoberta do endereço do LiveKit pelo client:** a conexão de voz (sinalização + mídia) vai direto do client para o LiveKit, sem passar pelo `server-channel` — então o client precisa saber o endereço público do LiveKit, que pode ser diferente do endereço do próprio `server-channel` (porta/subdomínio dedicados). `POST /api/channels/{id}/voice/token` devolve `{ token, roomName, url }`, onde `url` vem de uma nova variável `LIVEKIT_PUBLIC_URL` (ver `docker-compose.yml`/`.env.example`), obrigatória assim como `LIVEKIT_API_KEY`/`LIVEKIT_API_SECRET`.

**Razão:** mantém a mesma filosofia de dependências enxutas já aplicada em `server-channel` (pgx direto em vez de ORM, sem framework HTTP) — trocar ~15 pacotes transitivos por uma lib JWT de escopo único para uma operação que é, na prática, "assinar um JSON".

**Revisitar quando:** o sistema de permissões/roles for implementado (nesse ponto o grant do LiveKit passa a variar por role/canal, não mais fixo); ou se algum fluxo precisar de operações administrativas no LiveKit (kickar participante, listar quem está numa sala, forçar fechamento de sala) que exigem mesmo a API HTTP do LiveKit — nesse ponto pode valer a pena reavaliar o `server-sdk-go` só para esse uso específico, isolado do caminho de emissão de token.

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

**Extensão a server-central (2026-09-20):** ao implementar a API REST de diretório de servidores conhecidos (`GET/POST/DELETE /api/servers`), o mesmo problema surgiu em `server-central` — o client passou a chamá-lo de uma origem diferente pela primeira vez (antes, `server-central` não tinha nenhuma rota chamada pelo client). Aplicado exatamente o mesmo mecanismo (`internal/httpapi/cors.go` copiado de `server-channel`, variável `CORS_ALLOWED_ORIGINS`, vazia por padrão), sem WebSocket a considerar neste componente ainda.

**Bug recorrente — `Access-Control-Allow-Methods` desatualizado (2026-09-21):** `withCORS` em ambos os componentes fixava `Access-Control-Allow-Methods: GET, POST, OPTIONS` só com os métodos que existiam quando o mecanismo foi escrito. Toda rota nova com método diferente (ex. `PATCH /api/me`, usado por "editar apelido" no client) falha o preflight no navegador — CORS error mesmo com a origem liberada e o backend respondendo 200 na aba Network, porque o método da requisição real não bate com a allowlist do preflight. Corrigido trocando para uma lista fixa e abrangente (`GET, POST, PATCH, PUT, DELETE, OPTIONS`) nos dois `cors.go`, para não precisar lembrar de tocar nesse arquivo a cada rota nova. **Se aparecer erro de CORS de novo com a origem certa na lista de `CORS_ALLOWED_ORIGINS`:** primeiro suspeitar do método HTTP da rota nova faltando aqui (ou, mais raro, de um header customizado faltando em `Access-Control-Allow-Headers`), antes de investigar origem/proxy/DNS.

## Decisão: diretório de servidores conhecidos — `address` é a base URL completa

**Contexto:** ao implementar `GET/POST/DELETE /api/servers` em `server-central` e a tela "Adicionar servidor" no client (`client/src/components/AddServerDialog.tsx`), era preciso decidir o formato do campo `address` da tabela `known_servers` (`TEXT` livre, sem validação de formato definida no schema).

**Decisão:** `address` é a base URL completa do `server-channel` (ex. `http://localhost:8080`, `https://chat.minhacomunidade.com`), o mesmo valor usado diretamente em `KnownServer.baseUrl` no client para montar as chamadas REST/WebSocket (`client/src/lib/serverChannelApi.ts`, `serverCentralApi.ts`). O campo `name` (nome de exibição) é enviado separadamente pelo formulário, não derivado do endereço.

**Razão:** evita qualquer lógica de composição de URL (esquema, porta) no client ou no servidor — o self-hoster informa exatamente o endereço que os membros devem usar, e o client usa esse valor sem transformação. Consistente com a decisão já registrada de não ter descoberta automática: quem cadastra sabe o endereço completo (de um convite ou digitado manualmente).

**Revisitar quando:** um mecanismo de convite (TODO "Convites" em `server-channel`) precisar resolver o endereço automaticamente a partir de um código, em vez do usuário digitá-lo — nesse ponto vale considerar separar esquema/host/porta ou validar o formato no backend.

## Decisão: gateway de presença em server-central — hub em memória + WebSocket dedicado

**Alternativas consideradas:** reaproveitar uma conexão WebSocket já existente (não havia nenhuma em `server-central` antes deste item — DMs, o outro consumidor natural de WebSocket ali, ainda é TODO), coluna `last_seen_at`/heartbeat via REST (poll periódico), hub em memória com WebSocket dedicado (mesmo padrão de `server-channel`), Redis pub/sub para presença.

**Decisão:** WebSocket dedicado (`GET /api/presence/ws`) com um `realtime.Hub` em memória em `server-central/internal/realtime` (estrutura irmã da de `server-channel`, mas particionada por `account_id` em vez de por canal — uma conta pode ter várias conexões simultâneas, ex. várias abas, e só é considerada offline quando a última cai). `GET /api/presence` (REST) devolve o snapshot atual de quem, entre os amigos aceitos (`FriendshipStore.AcceptedFriendIDs`, novo), está online — usado pelo client para popular a lista ao abrir, antes do primeiro evento chegar pelo WebSocket. Ao conectar/desconectar (só na transição, não a cada conexão redundante), o servidor emite `presence.update` só para os amigos aceitos que estiverem online agora.

**Razão:** poll periódico via REST adicionaria latência perceptível (ficar "online" ou "offline" levaria até um ciclo de poll para refletir) e carga constante de requisições mesmo sem mudança de estado — errado para algo que o Discord e equivalentes tratam como praticamente instantâneo. Redis pub/sub resolveria o caso de múltiplas réplicas de `server-central`, mas hoje a instância central é um processo único (mesma razão já registrada no Hub de `server-channel`); adicionar Redis só para presença, sem outro consumidor, não paga a complexidade agora. Reaproveitar o mesmo mecanismo de autenticação via subprotocolo WebSocket (`wsAuthSubprotocol`, replicado de `server-channel` para `server-central/internal/auth/middleware.go`) evita reinventar como carregar o Bearer token num handshake que não aceita headers customizados.

**Escopo do broadcast:** só amigos com `status = 'accepted'` recebem o evento — desconhecidos e pedidos pendentes não veem presença de ninguém. Isso é o motivo de `handlePresenceWS` reconsultar `AcceptedFriendIDs` a cada conexão/desconexão em vez de cachear a lista de amigos: a lista pode ter mudado desde a última conexão da conta.

**Revisitar quando:** `server-central` precisar rodar em múltiplas réplicas (mesmo gatilho já registrado no Hub de `server-channel`) — nesse ponto Redis pub/sub (ou equivalente) passa a ser necessário para presença ser consistente entre réplicas.

## Decisão: adicionar amigos via convite (código de uso único), não username pesquisável

**Contexto:** implementar "Lista de amigos + presença" no client esbarrou num buraco não documentado: o modelo de dados de `friendships` e o gateway de presença já existiam, mas nenhuma API permitia criar a amizade em primeiro lugar — sem isso a lista ficaria sempre vazia.

**Alternativas consideradas:** adicionar por ID de conta (UUID) copiado/colado manualmente; adicionar por código/link de convite de uso único; adicionar um campo de username/handle único ao `Profile` com endpoint de busca.

**Decisão:** convite por código de uso único. `POST /api/friends/invites` gera um código (`friend_invites.code`, 10 caracteres base32 aleatórios, ver `internal/httpapi/friends.go`) associado à conta que o criou; `POST /api/friends/invites/{code}/redeem` é chamado pela outra conta e, atomicamente, marca o convite como resgatado (`FriendInviteStore.Redeem`, `UPDATE ... WHERE redeemed_at IS NULL`) e cria a amizade já com `status = 'accepted'` (`FriendshipStore.CreateAccepted`) — resgatar o código já é o consentimento mútuo, não há uma etapa extra de aprovação como em `Request`/`SetStatus` (que continuam existindo no store para um fluxo de pedido explícito futuro, hoje sem endpoint). `CreateAccepted` verifica as duas direções (`requester_id`/`addressee_id`) antes de inserir, para não duplicar uma amizade já existente por outro caminho.

**Razão:** mantém a mesma filosofia já registrada em "Decisão: descoberta de server-channel" — sem diretório pesquisável, só convite explícito compartilhado por fora. Um username pesquisável exigiria migration nova, validação de unicidade e um endpoint de busca que vira, na prática, um diretório de contas pesquisável — o tipo de superfície que o projeto já decidiu não ter para servidores, e que abriria a mesma discussão de moderação/opt-in registrada ali. UUID copiado/colado não exige infra nova, mas é pior UX sem ganhar nada em troca do convite (que também não exige infra além de uma tabela).

**Implicação de UI:** `client/src/components/AddFriendDialog.tsx` reúne as duas pontas — gerar um código pra compartilhar, ou colar um código recebido — no mesmo diálogo, reaproveitando `Dialog.css` (renomeado de `AddServerDialog.css`, que não tinha nada específico de servidor).

**Revisitar quando:** o volume de convites simultâneos por conta precisar de limite/expiração de fato (hoje `expires_at` existe na coluna mas nenhum endpoint o define — todo convite é "sem expiração, só de uso único") — mesmo padrão do TODO aberto "Convites (geração e validação)" em `server-channel`, que também não implementa expiração ainda.

## Decisão: DMs entregues no mesmo WebSocket de presença, restritas a amigos aceitos

**Contexto:** implementar "Implementar DMs: server-central como gateway de mensagens" (TODO.md) esbarrou em duas decisões não detalhadas pela decisão original ("Decisão: modelo de DMs — server-central medeia diretamente"): por qual conexão WebSocket a entrega em tempo real acontece, e quem pode mandar DM para quem.

**Alternativas consideradas (transporte):** WebSocket dedicado (`GET /api/dms/ws`), registrado no mesmo `realtime.Hub` já usado por presença; reaproveitar a própria conexão `GET /api/presence/ws` que o client já mantém aberta, aceitando também frames `dm.create` nela.

**Decisão:** reaproveitar a conexão de `GET /api/presence/ws`. `realtime.Client.ReadPump` (antes só descartava frames recebidos, já que presença não aceita nada do client) passou a receber um `onMessage func([]byte)`, mesmo formato já usado em `server-channel`. O único frame aceito é `dm.create` (`internal/httpapi/dms.go`, `handleIncomingDM`); o servidor responde `dm.created` (broadcast para destinatário e remetente, mesmo padrão de `message.created` em `server-channel` — o remetente recebe de volta para confirmar id/timestamp atribuídos pelo servidor) ou `error`.

**Razão:** o client já abre e mantém uma única conexão de `server-central` por sessão (a de presença); pedir uma segunda conexão WebSocket só para DMs duplicaria handshake/reconexão/heartbeat sem nenhum ganho, já que `realtime.Hub` já particiona por `account_id` e já suporta múltiplos clients por conta (múltiplas abas). Consequência prática: abrir a conexão de DM (que hoje é a mesma de presença) já conta como "online" para efeito de presença — não existe hoje um jeito de mandar/receber DM sem também aparecer online para os amigos, o que é o comportamento esperado (a pessoa está com o client aberto).

**Alternativas consideradas (autorização):** qualquer conta pode mandar DM para qualquer outra (bastando saber o `accountId`); restrita a amigos com amizade aceita (`FriendshipStore.AreFriends`, nova).

**Decisão:** só amigos com `status = 'accepted'` podem trocar DMs entre si — aplicado tanto no envio via WebSocket (`handleIncomingDM`) quanto na leitura de histórico (`GET /api/dms/{accountId}/messages`, `handleListDMs`).

**Razão:** consistente com a mesma filosofia já registrada em "Decisão: adicionar amigos via convite" e no escopo do broadcast de presença (só amigos aceitos se veem) — não há diretório de contas pesquisável, então não faz sentido permitir DM para alguém que a conta nem consegue descobrir por conta própria. Sem essa checagem, `accountId` na URL/no frame seria só um UUID adivinhável.

**Modelo de dados criado (migration `0003_direct_messages`):** tabela `direct_messages` (`sender_id`, `recipient_id`, `content`, `created_at`, `edited_at`), índice funcional em `(LEAST(sender_id, recipient_id), GREATEST(sender_id, recipient_id), created_at DESC)` para a leitura da conversa nos dois sentidos. `DirectMessageStore.ListConversation` faz keyset pagination por `created_at`, mesmo padrão de `MessageStore.ListForChannel` em `server-channel`.

**Revisitar quando:** um fluxo de pedido de amizade explícito (`Request`/`SetStatus`, hoje sem endpoint) entrar em uso — nesse ponto vale decidir se uma amizade `pending` já permite DM ou só `accepted` (hoje a resposta é só `accepted`, já que é o único status alcançável na prática via convite).

## Decisão: UI de DMs no client — um listener extra no mesmo WebSocket de `useFriends`, não uma segunda conexão

**Contexto:** implementar a UI de DMs no client (TODO.md, "DMs (API de `server-central` pronta... falta UI no client)") esbarrou numa consequência não detalhada da decisão de backend "DMs entregues no mesmo WebSocket de presença": `hooks/useFriends.ts` já abre e possui a única conexão a `GET /api/presence/ws` da sessão. Uma UI de DM ingênua abriria sua própria conexão a essa mesma rota, duplicando handshake/heartbeat e contando como uma segunda aba para efeito de presença — o que o backend explicitamente evita fazer no seu próprio design.

**Alternativas consideradas:** cada componente de DM abre sua própria `WebSocket` para `/api/presence/ws` (duplica conexão); mover a conexão para um React Context/Provider dedicado; `useFriends` expõe a instância de `WebSocket` já aberta, e quem mais precisar dela (`hooks/useDirectMessages.ts`) usa `addEventListener('message', ...)` na mesma instância em vez de `onmessage` (que só aceita um handler por vez).

**Decisão:** `useFriends` passou a guardar o socket em estado (`useState<WebSocket | null>`) e devolvê-lo no retorno do hook. `useDirectMessages(accessToken, peerId, socket)` recebe essa mesma instância como parâmetro — não abre conexão própria — e anexa um listener via `addEventListener('message', ...)` (em vez de sobrescrever `.onmessage`, que já está em uso por `useFriends` para `presence.update`); os dois listeners coexistem porque a API `WebSocket` do navegador suporta múltiplos listeners de `message` simultâneos. `App.tsx` é o único ponto que chama `useFriends` e repassa `socket` para `DirectMessageView`.

**Razão:** evita introduzir Context/Provider só para compartilhar uma referência (o projeto não usa nenhuma lib de estado/roteamento, mesmo argumento já registrado na decisão de login OIDC) — `App.tsx` já é a raiz única onde `useFriends` é chamado, então passar o socket como prop resolve o compartilhamento sem infraestrutura nova. Reaproveita exatamente o mesmo comportamento que o backend já decidiu ter (uma conexão por sessão faz dupla função).

**Implicação:** `serverCentralApi.ts` unificou `decodePresenceFrame` (só `presence.update`) em `decodePresenceSocketFrame`, que decodifica os três tipos de frame que trafegam nessa conexão (`presence.update`, `dm.created`, `error`) — qualquer novo tipo de frame que passe a usar essa conexão no futuro deve ser adicionado ali, não num decoder paralelo.

**Revisitar quando:** o client ganhar mais de um consumidor do socket de presença/DM que precise também *enviar* frames concorrentemente (hoje só `useDirectMessages.sendMessage` escreve nele) — nesse ponto vale considerar um pequeno hook "dono" dedicado (`usePresenceSocket`) em vez de `useFriends` continuar acumulando essa responsabilidade.

## Decisão: convites obrigatórios para entrar em server-channel

**Contexto:** implementar "Convites (geração e validação)" (TODO.md) esbarrou numa lacuna não documentada: `internal/store/invites.go` (tabela e repositório) já existia desde a migration `0001_init`, mas sem endpoint nenhum, e `auth.Middleware` criava um `member` automaticamente (upsert por `sub`) em **toda** requisição autenticada — ou seja, qualquer usuário com um token válido da mesma instância central de Authentik virava membro deste `server-channel` só por saber o endereço, sem qualquer convite. Isso contradizia o espírito de "descoberta de server-channel: nenhuma, apenas convite ou IP manual" (a decisão cobre como o *client* descobre o endereço, mas o *servidor* nunca de fato checava autorização de entrada).

**Alternativas consideradas:** manter o comportamento atual (convite vira só uma forma alternativa de divulgar o endereço, sem gatekeeping real); tornar o convite obrigatório, fechando a entrada a quem não resgatar um código válido.

**Decisão:** convite passou a ser obrigatório. `auth.Middleware` foi dividido em dois (`internal/auth/middleware.go`):
1. `VerifyToken` — só valida o Bearer token (JWT do Authentik), anexa o `sub` (`SubjectFromContext`) ao contexto, sem tocar em `members`.
2. `RequireMember` — encadeado depois de `VerifyToken`, busca o membro por `sub` (`MemberStore.GetByOIDCSubject`, sem criar) e devolve 403 se o `sub` ainda não for membro.

`POST /api/join` (`internal/httpapi/join.go`) é a única rota que usa só `VerifyToken` (sem `RequireMember`) — é o ponto de entrada:
- Se o `sub` já é membro, devolve 200 (idempotente), ignora `code`.
- Se `members` estiver vazia (bootstrap: ninguém ainda entrou), o primeiro `sub` a chamar vira **fundador** sem precisar de convite — é o próprio self-hoster subindo a instância pela primeira vez, e não existe ainda quem gerar um convite para ele.
- Caso contrário, exige `code` de um convite válido no corpo (`{"code": "..."}`); resgate é atômico (`InviteStore.Redeem`, `UPDATE ... WHERE (expires_at IS NULL OR expires_at > now()) AND (max_uses IS NULL OR uses < max_uses)`, devolve `ErrConflict` se 0 linhas afetadas) para não estourar `max_uses` sob concorrência.

Todas as outras rotas continuam atrás de `RequireMember` (renomeadas de "protected" para a mesma composição `VerifyToken` + `RequireMember` em `internal/httpapi/server.go`).

**Geração/revogação de convites:** `POST /api/invites` (qualquer membro pode gerar — mesmo nível de acesso "é membro" já registrado na integração de voz com LiveKit, até o sistema de permissões/roles existir), `GET /api/invites` (lista todos, visível a qualquer membro), `DELETE /api/invites/{id}` (só quem criou o convite pode revogar — `InviteStore.Delete` agora exige `created_by_member_id` na cláusula `WHERE`, devolvendo `ErrNotFound` tanto para convite inexistente quanto de outro membro, para não vazar existência). Código: mesmo formato do convite de amizade em `server-central` (base32 sem padding, 10 caracteres).

**Cliente:** `AddServerDialog` ganhou um campo opcional "Código de convite" — `useKnownServers.addServer` agora chama `POST /api/join` (`serverChannelApi.joinServer`) antes de registrar o endereço no diretório de `server-central`; se o join falhar (convite ausente/inválido), o servidor nem é adicionado ao diretório. `ChannelSidebar` ganhou um botão "Convidar" que abre `InviteServerDialog` (gera código via `POST /api/invites`) — UI de geração de convite é uma tela separada da de resgate porque acontecem em fluxos diferentes (quem já está no servidor gera; quem ainda não está resgata ao adicionar o endereço), ao contrário do convite de amizade em `server-central`, onde as duas pontas cabem no mesmo diálogo porque ambos os lados já compartilham o mesmo objeto (amizade). `AddFriendDialog.css` foi renomeado para `InviteCode.css` com classes genéricas (`invite-section`/`invite-hint`/`invite-code`, antes prefixadas `friend-`) para ser reaproveitado por `InviteServerDialog` sem duplicar CSS.

**Razão:** fechar a lacuna de segurança real (qualquer usuário do Authentik central conseguia entrar em qualquer `server-channel` cujo endereço descobrisse) e alinhar o comportamento ao modelo de servidor privado do Discord, que é a referência de produto do projeto. O bootstrap por "primeiro a entrar vira fundador" evita precisar de um conceito de "dono" pré-existente ou um passo de setup separado — resolve o problema do ovo e da galinha (preciso de um membro para criar um convite, mas preciso de um convite para virar membro) da mesma forma que instaladores self-hosted costumam fazer (ex. primeiro admin de um Wordpress/Nextcloud novo).

**Revisitar quando:** o sistema de permissões/roles (TODO.md) for implementado — nesse ponto criar/revogar convites deveria depender de uma permissão específica em vez de "ser membro" ou "ser o criador", e o fundador do bootstrap deveria virar uma role real ("Owner") em vez de só ser o primeiro registro em `members` sem nenhuma marcação especial (hoje não há coluna para isso — o fundador não é diferenciável dos demais membros depois que outros entram).

## Decisão: sistema de permissões/roles — bitmask por role + overwrite por canal, dono do bootstrap ignora tudo

**Contexto:** implementar "Sistema de permissões/roles por servidor e por canal" (TODO.md). O schema (`roles.permissions BIGINT`, `member_roles`) já existia desde a migration `0001_init`, mas sem nenhum bit definido nem endpoint de gerenciamento — toda rota tratava "é membro deste server-channel" como único nível de acesso (ver decisões de LiveKit e de convites acima, ambas com uma nota de "revisitar quando o sistema de permissões existir"). Faltava: (1) definir os bits, (2) resolver o problema do ovo e da galinha de quem administra roles/convites antes de existir qualquer role, (3) decidir o que "por canal" significa de fato.

**Alternativas consideradas (modelo geral):** permissões só por role, sem nada por canal (mais simples, mas não cobre o "por canal" do próprio nome do TODO); overwrite por canal e por role, no estilo Discord (allow/deny por role dentro de um canal, sobrescrevendo a permissão base); overwrite também por membro individual (Discord tem os dois níveis).

**Decisão:** bitmask por role (OR entre todas as roles do membro, incluindo uma role default implícita) como base, mais overwrite de allow/deny por canal **só por role** (sem overwrite por membro individual — corta a combinação mais rara para não dobrar a superfície de UI/API numa v1). Bits definidos em `internal/permissions/permissions.go`:

```
ViewChannels  = 1   SendMessages = 2   Voice = 4
ManageInvites = 8   ManageRoles  = 16  Administrator = 32
```

Há bastante espaço sobrando no `BIGINT` para bits futuros (canal forum, gerenciar mensagens/canais, kick/ban — nenhum tem endpoint ainda, então não ganhou bit; ver "Revisitar quando").

**Role default ("@everyone"):** seeded pela migration `0002_roles_permissions` (`roles.is_default`, índice único parcial garantindo só uma por servidor), com `ViewChannels|SendMessages|Voice` (soma 7) — preserva o comportamento anterior (qualquer membro tinha acesso total) para quem não tiver nenhuma role extra. É implícita a todo membro sem precisar de linha em `member_roles` (`internal/httpapi/permissions.go`, `memberBasePermission` sempre soma `RoleStore.GetDefault()` às roles atribuídas).

**Dono do servidor, não uma role:** `members.is_owner` (nova coluna), setado só para quem entra primeiro via `POST /api/join` (`MemberStore.CreateFounder`, ver decisão de convites acima). Dono ignora toda checagem de permissão e todo overwrite de canal (`permissions.Owner = -1`, todos os bits em 1) — resolve o ovo-e-a-galinha (precisa de alguém com `ManageRoles`/`ManageInvites` para o sistema funcionar, mas não existe ninguém com role nenhuma no primeiro boot) do mesmo jeito que instaladores self-hosted costumam fazer (primeiro admin de um Wordpress/Nextcloud novo). É deliberadamente **não** uma role: não pode ser removido/editado como uma, mesma distinção que o Discord faz entre "server owner" e a permissão `ADMINISTRATOR`.

**Overwrite de canal (`channel_role_overwrites`, `internal/store/channel_overwrites.go`):** `allow`/`deny` por `(channel_id, role_id)`. `internal/permissions.Effective(base, roleIDs, overwrites)` soma todos os `allow` das roles do membro que tiverem overwrite naquele canal, depois remove todos os `deny` — nessa ordem, então `deny` sempre vence quando a mesma role nega e libera o mesmo bit (não deveria acontecer numa única linha, mas evita ambiguidade entre roles diferentes). `Administrator` (ou o dono) ignora overwrites por completo. É isso que torna um canal privado: uma role sem `ViewChannels` na base pode ganhar acesso só a um canal via `allow`, ou uma role com acesso geral pode ser bloqueada só num canal via `deny`.

**Onde é aplicado:** `GET /api/categories`/`GET /api/channels` filtram por `ViewChannels` efetivo (categoria sem nenhum canal visível some da lista, para não vazar nem o nome dela); `GET /api/channels/{id}/messages` e `GET /api/channels/{id}/ws` exigem `ViewChannels`, e o WS também confere `SendMessages` — checado uma única vez na conexão (não por frame), então uma role revogada só produz efeito na próxima reconexão, mesmo tipo de corte já aceito no resto do sistema de tempo real; `POST /api/channels/{id}/voice/token` exige `Voice` (join e falar são um único bit — não há hoje um modo "só ouvir" na UI do client, então não valia diferenciar); `POST/GET/DELETE /api/invites` exigem `ManageInvites` (antes "qualquer membro", ver decisão de convites); `POST/PATCH/DELETE /api/roles`, `POST/DELETE /api/members/{memberId}/roles/{roleId}` e o CRUD de overwrite de canal exigem `ManageRoles`.

**`GET /api/members` (novo):** lista os membros com `isOwner` e `roleIds`, fechando a lacuna notada em `client/src/App.tsx` ("lista de membros... ainda não tem API real"). Não expõe `oidcSubject` a outros membros — não há necessidade de vazar o identificador do Authentik central.

**Cliente:** `ManageRolesDialog` (botão "Roles" na `ChannelSidebar`, visível só se `GET /api/me` disser `isOwner` ou permissão `ManageRoles`) cria/remove roles com checkboxes de permissão e atribui/remove role de membro. `MemberList` passou a mostrar membros reais (antes hardcoded vazio) com a cor da role de maior `position` e uma badge "dono". Overwrite de canal por role **não ganhou UI** nesta rodada — só a API (`GET/PUT/DELETE /api/channels/{id}/overwrites[/{roleId}]`) — editor de permissão por canal é bem mais superfície de UI (por canal × por role × allow/deny) e fica para quando alguém precisar de fato de canais privados na prática.

**Razão:** o modelo bitmask + overwrite por role é o mesmo que o Discord usa (adaptado, sem overwrite por membro) e já é familiar a quem vai administrar um server-channel. Resolver "dono" como atributo do membro em vez de uma role automática evita ter que decidir o que acontece se alguém apagar essa role "especial" por engano.

**Revisitar quando:** alguém pedir "não deixar X conceder uma permissão que X mesmo não tem" (hoje quem tem `ManageRoles` pode criar uma role com `Administrator` mesmo sem ser dono — escalonamento de privilégio já listado em TODO.md, "Segurança"); um endpoint de gerenciar mensagens/canais/kick existir (aí ganha bit próprio); overwrite por membro individual for pedido (hoje só por role); ou o editor de overwrite de canal ganhar UI no client.

## Decisão: canal forum — mesma rota de WebSocket do canal de texto, sem bit de permissão próprio

**Contexto:** implementar "Canal forum: threads/posts" em `server-channel` e "Canal forum (UI de threads)" no `client` (TODO.md). O modelo de dados (`threads`, `messages.thread_id`) e o repositório (`MessageStore.CreateThread`/`ListThreads`, `MessageStore.Create`/`ListForChannel` já aceitando `threadID`) já existiam desde a migration `0001_init` e a decisão original de canal de texto — faltava só a API e a UI.

**Alternativas consideradas (transporte):** REST puro para tudo (criar thread, postar resposta, listar) — mais simples, mas sem atualização ao vivo quando outro membro abre uma thread ou responde; WebSocket dedicado por thread (um hub particionado por `thread_id`, não por `channel_id`) — replica exatamente o granularidade do Discord (quem está numa thread só recebe eventos dela), mas exige um hub novo e uma conexão por thread aberta; reaproveitar a mesma rota `GET /api/channels/{id}/ws` e o mesmo `realtime.Hub` (particionado por canal, já existente) também para canais forum, com dois frames novos (`thread.create`, `post.create`) e dois eventos novos (`thread.created`, `post.created`) broadcast para todo mundo conectado ao canal.

**Decisão:** reaproveitar a rota e o Hub existentes. `handleChannelWS` (`internal/httpapi/channel_ws.go`) passou a aceitar `channel.Type` `text` **ou** `forum` (antes só `text`); no caso forum, o frame recebido é identificado pelo campo `type` (`realtime.FrameType`) e despachado para `thread.create` (cria a thread + o post inicial numa única operação lógica, `MessageStore.CreateThread` seguido de `MessageStore.Create` com o `threadID` retornado) ou `post.create` (responde numa thread existente, após confirmar que `thread.ChannelID` bate com o canal da conexão). Os dois broadcastam para **todo o canal** (`hub.Broadcast(channelID, ...)`), não só para quem estiver com aquela thread aberta — o client filtra localmente por `threadId` (ver `hooks/useForumChannel.ts`). REST cobre só leitura: `GET /api/channels/{id}/threads` (lista threads) e `GET /api/threads/{id}/messages` (histórico paginado de uma thread, mesmo keyset pagination por `created_at` já usado em canal de texto).

**Permissão:** sem bit novo — reaproveita `ViewChannels` (ver/listar threads) e `SendMessages` (abrir thread ou responder), checados do mesmo jeito que em canal de texto (`channelPermission`, overwrite de canal por role já cobre "canal forum privado" do mesmo jeito que cobre "canal de texto privado"). `docs/architecture.md` (decisão de permissões/roles) já deixava esse espaço em aberto ("canal forum" listado como candidato a bit futuro, mas nenhum motivo concreto apareceu para diferenciar "enviar mensagem" de "postar/responder no forum").

**Razão:** o volume de threads simultâneas abertas por diferentes membros num mesmo canal forum é baixo comparado ao de mensagens num canal de texto de alto tráfego — o custo de "todo mundo conectado ao canal recebe todo evento, cliente filtra" é aceitável nessa escala e evita introduzir um segundo eixo de particionamento no Hub (hoje só por `channel_id`) só para este caso. Mantém a mesma filosofia já registrada nas decisões de canal de texto e de permissões: reaproveitar infraestrutura existente em vez de generalizar cedo demais para um caso de uso que ainda não apareceu.

**Cliente:** `hooks/useForumChannel.ts` carrega a lista de threads via REST e abre a mesma conexão WebSocket do canal (`lib/serverChannelApi.ts`, `openChannelSocket` — sem mudança, já é genérica por `channelId`); posts de uma thread só são buscados (REST) quando o usuário abre essa thread (`openThread`), e novos posts chegam ao vivo pela mesma conexão enquanto a thread estiver aberta. `components/ForumChannelView.tsx` (ligado em `MainPanel.tsx`) mostra a lista de threads com um botão "Novo post" (título + conteúdo inicial) e, dentro de uma thread, a lista de posts com resposta — reaproveita as classes CSS de `TextChannelView.css` (`.message-list`/`.message`/`.message-form`) para a lista de posts.

**Revisitar quando:** o volume de atividade num canal forum justificar filtrar o broadcast por thread no servidor em vez de no client (mesmo gatilho de escala já registrado no Hub de canal de texto); ou um endpoint de editar/apagar thread/post existir (hoje só criação, mesmo escopo mínimo do TODO original).

## Decisão: primeira implantação de teste — infra do `a3s-network`, imagens no GHCR, deploy via GitHub Actions

**Contexto:** antes de decidir a infra "oficial" de longo prazo (domínio próprio `ffcom.a3sitsolutions.com`/`.com.br`, host dedicado), era preciso validar o fluxo ponta a ponta com um grupo maior de pessoas. O operador já mantém uma infra própria (`D:\Dev\a3s-network`) com Docker Swarm (rede overlay `a3s-services`, cluster entre `VMSUBS24OCI0102` e `SVRUBS24IPS0101`), Nginx Proxy Manager alcançando containers por nome via essa rede, um runner self-hosted de GitHub Actions (`[self-hosted, a3s-network]`) e um certificado wildcard cobrindo `*.a3sitsolutions.com.br` (inclusive subdomínios aninhados como `*.ffcom.a3sitsolutions.com.br`) — reaproveitar essa infra para o primeiro teste evita provisionar servidor/domínio/CI própria do zero só para validar o produto.

**Decisão:**
1. **Host:** `SVRUBS24IPS0101` (site Ipsep 01, worker do Swarm, sem IP público próprio — LAN `10.20.4.10`, Tailscale `100.64.0.2`). Responsabilidade dividida: este repositório cobre os containers (build, compose, nomes, portas); um agente separado cobre NPM (Proxy Hosts por nome de container) e DNS/port-forwarding no roteador do site — ver `D:\Dev\a3s-network`.
2. **Containers e portas** (nomes fixos via `container_name`, não os gerados pelo Compose):

   | Container | Porta interna | Rede | Alcançado via |
   |---|---|---|---|
   | `ffcom-client` | 8080 (nginx) | `a3s-services` | NPM, por nome |
   | `ffcom-central-app` | 8080 | `a3s-services` + `internal` | NPM, por nome |
   | `ffcom-central-db` | 5432 | `internal` | só `ffcom-central-app` |
   | `ffcom-channel-app` | 8080 | `a3s-services` + `internal` | NPM, por nome |
   | `ffcom-channel-db` | 5432 | `internal` | só `ffcom-channel-app` |
   | `ffcom-livekit` | 7880 (sinalização) | `a3s-services` | NPM, por nome |
   | `ffcom-livekit` | 7881/tcp + range RTC/udp | — | publicada direto no host, amarrada ao IP de LAN (`10.20.4.10`), sem passar pelo NPM |
   | `ffcom-coturn` | 3478 tcp/udp + range de relay/udp | — | idem, sem NPM nem rede overlay (não fala HTTP) |

   Os bancos Postgres ficam isolados numa rede `internal` própria de cada compose — nunca entram em `a3s-services`, mesmo padrão já usado nos composes de referência de self-host.
3. **Publicação de porta amarrada ao IP de LAN, não `0.0.0.0`:** mesmo padrão já usado em `D:\Dev\a3s-network\deploy\go2rtc\docker-compose.yml` (`"10.20.4.10:1984:1984"`) — evita publicar em todas as interfaces de um host com múltiplas redes.
4. **Compose de implantação separado do compose de referência de self-host:** `deploy/central/`, `deploy/channel/`, `deploy/client/` (novos, só para esta implantação) em vez de editar `server-central/docker-compose.yml`/`server-channel/docker-compose.yml` — esses últimos continuam sendo a referência genérica documentada no TODO ("Docker Compose de referência para X"), com `build: .` local e sem nenhuma suposição sobre rede overlay/GHCR/nomes de container fixos desta infra específica. Misturar os dois quebraria o caso de uso de terceiros self-hosteando a partir do repo público.
5. **CI/CD:** um workflow por componente (`.github/workflows/deploy-ffcom-{central,channel,client}.yml`), mesmo molde já usado por outros projetos do operador (`deploy-a3s-claude-relay.yml`): `workflow_dispatch` manual, builda e publica em `ghcr.io/apolope/ffcom-{central,channel,client}`, carrega o `.env` real de `/opt/ffcom/envs/ffcom-{central,channel}.env` no host (fora do git) e roda `docker compose up -d --pull always --remove-orphans`, com healthcheck via `docker inspect` no final.
6. **`GET /healthz` novo em `server-central` e `server-channel`:** endpoint sem autenticação (`internal/httpapi/healthz.go` nos dois), porque toda rota existente exigia Bearer token — sem isso o `HEALTHCHECK` do Docker não tinha como confirmar o processo de pé sem sempre falhar com 401.

**Razão:** reaproveitar infraestrutura e convenções já validadas em produção (naming, isolamento de rede, pipeline) é mais barato do que inventar um esquema novo só para o FFCom, e mantém a separação clara entre "como um terceiro self-hosteia isso" (composes de referência, inalterados) e "como esta implantação de teste específica roda" (novo `deploy/`).

**Correção descoberta ao rodar pela primeira vez (2026-09-20): `ffcom` é repositório pessoal (`apolope/ffcom`), não da organização `a3sitsolutions`.** Isso quebrou duas suposições da decisão original:
- **Runner:** `[self-hosted, a3s-network]` é registrado a nível de organização — não é visível a um repositório pessoal fora dela, mesmo com o runner ativo. Corrigido registrando um runner dedicado, escopado ao próprio repositório (`config.sh --url https://github.com/apolope/ffcom`, mesma máquina `SVRUBS24IPS0101`, serviço systemd próprio `actions.runner.apolope-ffcom.*`, label `a3s-network` mantida para os workflows não precisarem mudar `runs-on`).
- **GHCR:** o `GITHUB_TOKEN` automático de um workflow só publica pacotes no namespace do dono do repositório que o gerou — um repo pessoal não consegue publicar em `ghcr.io/a3sitsolutions/*` sem um PAT dedicado da organização (erro visto: `permission_denied: The requested installation does not exist`). Corrigido trocando o namespace das 3 imagens para `ghcr.io/apolope/ffcom-{central,channel,client}`, evitando gerenciar mais um segredo.

**Correção (2026-09-20): `hadolint` falhando em `server-central`/`server-channel` por `DL3018` (versão de pacote apk não fixada).** Pinar a versão do `ca-certificates` num `alpine:3.20` de tag flutuante é frágil (o índice do Alpine derruba versões antigas do repositório); corrigido com `# hadolint ignore=DL3018` acima do `RUN apk add` nos dois Dockerfiles, já que o workflow usa `failure-threshold: info` (barra até avisos leves).

**Checagem de portas (2026-09-20, via SSH em `apolo@10.20.4.10`):** confirmado que `3478/3479/5349/5350` já estão ocupados em `10.20.4.10` pelo coturn do stack VoIP/FreeSWITCH existente (`network_mode=host`, escuta em todas as interfaces do host). `TURN_LISTEN_PORT` do FFCom ajustado para `33478` (porta externa/host — o container continua escutando `3478` internamente via `--listening-port` fixo no compose). Faixas de relay do coturn (`49160-49200`) e RTC do LiveKit (`50000-50100`) não colidiram com nada em uso, mantidas no default. `.env` reais já criados em `/opt/ffcom/envs/ffcom-{central,channel}.env`.

**Correção (2026-09-20): exposição pública via `VMSUBS24OCI0102`, não pelo roteador do site IPS01.** O pedido inicial (DNS + port-forwarding direto no roteador do site, via WAN1/Nio ou WAN2/Tim) estava incorreto — o lado operacional (agente de infra) confirmou, checando a infra real, que nenhum dos dois links de WAN do site serve para isso: WAN1 (Nio) é CGNAT (nenhuma conexão de entrada é aceita, em nenhuma porta) e WAN2 (Tim) tem IP público dinâmico (já mudou pelo menos uma vez) com um aviso não resolvido de "router atrás de NAT". Além disso, o roteador do site não expõe nada à internet por política, e isso não muda para o FFCom.

**Desenho correto:** os containers continuam em `SVRUBS24IPS0101` (LAN `10.20.4.10`), mas toda a exposição pública — HTTP(S) via proxy reverso e a mídia RTC crua — entra por `VMSUBS24OCI0102` (IP público fixo, `137.131.249.145`), que encaminha internamente até `SVRUBS24IPS0101` por um túnel próprio da infra (fora do escopo deste repositório). `TURN_EXTERNAL_IP` é esse IP da VM (`137.131.249.145`), não um IP do site — é justamente o que esse encaminhamento evita expor. Os 4 hostnames públicos (`app.`, `central.`, `channel-test.`, `livekit-test.ffcom.a3sitsolutions.com.br`) apontam (DNS, do lado operacional) para esse mesmo IP.

**Validação ponta a ponta (2026-09-21):** proxy reverso/certificado/DNS em `VMSUBS24OCI0102` e `redirect_uris` do Authentik concluídos pelo lado operacional. Confirmado via `curl`/`openssl s_client`: os 4 hostnames resolvem para `137.131.249.145`, servem certificado Let's Encrypt válido, `/healthz` de `server-central`/`server-channel` responde `200 ok`, upgrade de WebSocket chega até os apps Go (erros de nível de aplicação, não de proxy), e login OIDC completo funciona a partir de `app.ffcom.a3sitsolutions.com.br`.

**Bugs encontrados testando com o client de verdade (2026-09-21), corrigidos neste repositório:**
- **Lista de membros mostrava UUID truncado em vez de nome:** `nickname` já existia no schema (`members.nickname`) e no store (`MemberStore.SetNickname`) desde o início, mas sem endpoint — `GET /api/members`/`useServerMembers.ts` caíam no fallback `remote.id.slice(0, 8)`. server-channel não tem acesso ao nome real da conta (só o `sub` do Authentik, ver "modelo de dados de server-channel" acima), então a solução foi apelido configurável, não nome real: `PATCH /api/me` novo (`internal/httpapi/me.go`, `handleUpdateMe`), `updateMyNickname` em `lib/serverChannelApi.ts`, `NicknameDialog.tsx` novo, botão "Apelido" em `ChannelSidebar`.
- **401 visível no console logo após navegar para `app.ffcom...`:** `App.tsx` chama `useKnownServers(accessToken ?? '')` e `useFriends(accessToken ?? '')` antes do early-return de `status === 'loading'` (regra dos hooks — não dá pra condicionar a chamada em si) — enquanto o login OIDC ainda não termina, `accessToken` é `undefined` e vira `''`, e os efeitos de fetch desses dois hooks disparavam sem essa guarda (diferente de `useMe`/`useServerMembers`/`useServerStructure`, que já checavam `!accessToken`). Corrigido adicionando a mesma guarda (`if (!accessToken) return`) em `useFriends.load`/`useKnownServers.load` — o efeito roda de novo sozinho quando o token real chega (já está nas dependências), então isso não muda o comportamento final, só evita a rajada de 401 durante a janela de loading.

**Revisitar quando:** essa instância de teste for promovida a "oficial" (domínio de produção definitivo, ver TODO "Provisionar `ffcom.a3sitsolutions.com`") — nesse ponto vale decidir se ela continua na infra do `a3s-network` ou migra para infra dedicada.

**3 runners dedicados, não 1 (2026-09-21):** rodar os 3 workflows juntos (fluxo normal quando várias mudanças tocam mais de um componente) serializava no único runner registrado — lint/scan de cada workflow rodam em paralelo entre si (sem dependência), mas as 3 etapas "Build, push e deploy" competiam pelo mesmo runner. Registrados mais dois (`SVRUBS24IPS0101-ffcom-02`, `-03`, mesma máquina, mesmo processo de `config.sh`/`svc.sh install`, mesma label `a3s-network`) — número escolhido para bater exatamente com os 3 workflows independentes que existem hoje (`deploy-ffcom-{central,channel,client}`), não mais que isso, já que não há um quarto workflow pra usar a capacidade extra. Host tinha folga de sobra pra isso (8 vCPU, ~16GB livres, load ~2-3 antes de adicionar).

## Decisão: compartilhamento de tela — `setScreenShareEnabled` do LiveKit, sem preview local

**Contexto:** implementar o item de TODO "Compartilhamento de tela" no `client`, sobre a integração de voz já existente (`useVoiceChannel.ts`/`VoiceChannelView.tsx`).

**Alternativas consideradas:** capturar `getDisplayMedia` à mão e publicar a track manualmente via `room.localParticipant.publishTrack`; usar `LocalParticipant.setScreenShareEnabled(enabled)` (helper nativo do `livekit-client`, já chama `getDisplayMedia` e cuida de source/publicação); renderizar as tracks de tela remotas como elementos de vídeo do React (state + `ref` por participante) vs. anexar/desanexar os elementos de forma imperativa (mesmo padrão já usado para os elementos de áudio, que são criados por `track.attach()` e inseridos direto no DOM).

**Decisão:**
1. `toggleScreenShare` (`client/src/hooks/useVoiceChannel.ts`) só chama `room.localParticipant.setScreenShareEnabled(next)` — sem `getDisplayMedia`/`publishTrack` manuais. O booleano `screenSharing` exposto pelo hook é derivado do participante local na lista de `participants` (`p.isScreenShareEnabled`), não é um `useState` paralelo — evita os dois ficarem dessincronizados se a track cair sozinha (ver ponto 3).
2. Renderização das tracks de tela remotas segue o mesmo padrão imperativo já usado para áudio: `RoomEvent.TrackSubscribed` com `track.source === Track.Source.ScreenShare` cria um `<video>` via `track.attach()` mais um rótulo com o nome do participante, embrulha os dois numa `<div class="screen-share-tile">` e insere no container apontado por `screenShareContainerRef` (um callback ref exposto pelo hook, preso pelo `VoiceChannelView` numa `<div>` vazia). `TrackUnsubscribed` remove a tile pelo `trackSid`. Alternativa descartada (state React por participante) exigiria sincronizar um `Map` de tracks com re-render a cada (un)subscribe só para montar `<video ref={...}>` — não ganha nada sobre continuar o padrão imperativo já estabelecido.
3. **Sem preview local da própria tela compartilhada:** ao contrário das tracks remotas, a track de tela do participante local nunca passa por `TrackSubscribed` (esse evento só dispara para tracks de outros participantes) — mostrar preview exigiria tratar `LocalTrackPublished` separadamente. Descartado por simplicidade nesta v1: o botão já troca de rótulo ("Compartilhar tela" ↔ "Parar compartilhamento") e o participante aparece com o ícone 🖥️ na lista, suficiente como confirmação de que a tela está sendo compartilhada.
4. Cancelar o seletor nativo do navegador (`getDisplayMedia`) rejeita a promise de `setScreenShareEnabled(true)` — `toggleScreenShare` engole esse erro (`try/catch` sem setar `status: 'error'`) porque desistir do seletor é uma ação normal do usuário, não uma falha do canal de voz.

**Razão:** o helper `setScreenShareEnabled` já existe no SDK exatamente para esse caso (fonte = `Track.Source.ScreenShare`, cuida de parar a track anterior se já houver uma, e de reagir ao evento nativo "Parar compartilhamento" da barra do navegador — que dispara o `ended` da `MediaStreamTrack` e o SDK já trata como unpublish, atualizando `LocalTrackUnpublished`/`isScreenShareEnabled` sozinho); reimplementar isso com `getDisplayMedia`/`publishTrack` manuais duplicaria lógica que o SDK já resolve.

**Revisitar quando:** o produto quiser preview local (mostrar pro usuário o que está sendo compartilhado) ou suportar múltiplas tracks de tela simultâneas por participante (hoje `setScreenShareEnabled` assume uma única track de `Track.Source.ScreenShare` por participante, suficiente pra v1).

## Decisão: convite auto-contido — link com endereço embutido, sem mudança de backend

**Contexto:** implementar o TODO "Registro do endereço do servidor (para o dono divulgar IP/DNS aos membros)" em `server-channel`. Até aqui, `InviteServerDialog` só gerava o código (`POST /api/invites`) e o próprio texto da UI avisava "quem entrar vai precisar também do endereço deste servidor" — o dono tinha que divulgar código e endereço por dois canais separados (ex. link do convite numa mensagem, IP/DNS em outra), e quem resgatava precisava colar os dois campos em `AddServerDialog` manualmente.

**Alternativas consideradas:** (1) `server-channel` ganhar uma variável de ambiente tipo `PUBLIC_ADDRESS` e devolver o endereço num endpoint/no corpo do convite, para o client não depender de já saber o endereço; (2) esquema de deep link customizado (`ffcom://join?...`) para abrir o client empacotado direto a partir do link; (3) montar o link inteiramente no client, já que quem gera o convite (`InviteServerDialog`) já está conectado ao `server-channel` e conhece `server.baseUrl` exatamente como o usuário o cadastrou — sem precisar que o backend "descubra" seu próprio endereço público.

**Decisão:** opção 3, sem nenhuma mudança de backend/API. `InviteServerDialog` (`client/src/components/InviteServerDialog.tsx`) monta `buildInviteLink(serverBaseUrl, code)` → `${baseUrl sem barra final}/?invite=${code}` e passa a copiar/exibir esse link em vez do código cru. Do outro lado, `AddServerDialog` (`client/src/components/AddServerDialog.tsx`) ganhou `parseInviteLink`, chamado a cada mudança no campo "Endereço": se o valor colado é uma URL válida com query `invite`, separa endereço e código de volta e preenche os dois campos automaticamente; se não for (endereço puro, como antes), o campo se comporta exatamente como antes. Os dois campos continuam existindo e aceitando entrada manual — colar só endereço, ou só digitar o código à parte, continua funcionando.

**Razão:** o backend não tem, e não precisa ganhar, noção do seu próprio endereço público — quem sabe esse endereço é sempre quem o digitou no client (seja o dono, ao rodar `server-channel` atrás de um proxy, seja quem cadastrou via `AddServerDialog`), consistente com a decisão já registrada em "diretório de servidores conhecidos: `address` é a base URL completa" (self-hoster informa o endereço, sem composição automática de esquema/host/porta). Uma variável `PUBLIC_ADDRESS` no servidor duplicaria essa informação e criaria uma segunda fonte de verdade para o mesmo dado (o endereço que o proxy reverso expõe, não necessariamente igual ao que o binário enxerga). Deep link customizado foi descartado por escopo: o build Electron (TODO separado) ainda não existe, não haveria como registrar o protocolo ainda, e o link `http(s)://` já resolve o problema real (divulgar os dois dados juntos) sem essa dependência.

**Revisitar quando:** o build Electron empacotado (TODO em aberto) existir e o produto quiser que clicar no link abra o app direto (em vez de só facilitar copiar/colar) — nesse ponto vale registrar um esquema customizado, com o link `http(s)://…?invite=` continuando como fallback pro client web/PWA.

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
