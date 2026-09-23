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

**Implicação de deploy:** cada `server-channel` roda (ou aponta para) um LiveKit self-hosted via Docker, mais um [coturn](https://github.com/coturn/coturn) para TURN/NAT traversal — necessário porque quem hospedar um `server-channel` em casa estará atrás de NAT. Port-forwarding e DNS dinâmico estão documentados em [`server-channel/README.md`](../server-channel/README.md#hospedando-atrás-de-nat-ex-em-casa).

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

**Revisitar quando:** ~~criptografia ponta-a-ponta em DMs for exigida~~ — feito, ver "Decisão: criptografia ponta-a-ponta em DMs" abaixo. O `server-central` continua mediando (roteia payloads cifrados sem conseguir lê-los).

## Decisão: criptografia ponta-a-ponta em DMs — NaCl box, chave por dispositivo

**Contexto:** `server-central` é a **instância única oficial** do FFCom (ao contrário de `server-channel`, self-hosted por comunidade) — antes desta decisão ela guardava o conteúdo de **todas** as DMs de **todos** os usuários da plataforma em texto puro, num único Postgres. Isso é um raio de exposição bem maior que mensagens de canal, que ficam isoladas por comunidade em cada `server-channel`. Se essa instância central fosse comprometida ou intimada judicialmente, todo o histórico de DM de todos os usuários ficaria exposto de uma vez. Item já previsto em Segurança/TODO.md ("Avaliar necessidade de criptografia ponta-a-ponta em DMs") e no "Revisitar quando" acima.

**Alternativas consideradas:**
- **Double Ratchet / Signal protocol:** forward secrecy e post-compromise security de verdade, padrão da indústria — descartado nesta v1 pelo custo de engenharia (X3DH, prekey bundles, estado de ratchet persistido por sessão, história de multi-dispositivo do próprio Signal) desproporcional ao ganho numa fase sem usuários reais fora do autor. Registrado como candidato futuro se o produto crescer.
- **`crypto.subtle` nativo (WebCrypto) com curvas X25519:** suporte a curvas seguras (X25519/Ed25519) ainda é recente e inconsistente entre versões de Chromium/Electron e navegadores de terceiros que um self-hoster do `client` web possa usar — descartado por apostar numa API de plataforma ainda não universal, ao contrário da filosofia de "não reinventar o que uma lib madura já cobre" (LiveKit, `oidc-client-ts`, `vite-plugin-pwa`, etc.) aplicada aqui trocando "nativo" por "lib pura JS madura".
- **NaCl `box` (X25519-XSalsa20-Poly1305) via `tweetnacl`:** escolhida — pura JS, ~7KB, auditada, décadas de uso em produção, funciona idêntico em qualquer ambiente (web, Electron, qualquer Chromium razoável), API mínima (`box`/`box.open`) suficiente para autenticação + confidencialidade de mensagem ponto-a-ponto sem reimplementar nada.

**Decisão:**
1. **Chave por dispositivo, não por conta:** cada perfil de navegador / instalação do Electron gera seu próprio par de chaves X25519 na primeira vez que a conta loga (`client/src/crypto/e2e.ts`, `nacl.box.keyPair()`), guardado só em `localStorage`, separado por conta desde a "Decisão: chave de E2E e cursores de não lida por conta" (mesmo padrão de risco já aceito para o `oidc-client-ts` em `auth/userManager.ts` — não é uma novidade de superfície de ataque deste código-base) e publicado em `server-central` via `PUT /api/me/e2e-public-key` (`client/src/hooks/useE2EKeys.ts`, instanciado em `App.tsx` assim que há login, não adiado até abrir uma DM).
2. **Diretório de chaves públicas:** `accounts.e2e_public_key` (nova coluna, `server-central/migrations/0004_dm_e2e_encryption.up.sql`), exposta em `GET /api/friends` (`e2ePublicKey`, ver `internal/httpapi/friends.go`) — só entre amigos aceitos, mesma regra de acesso que já vale para DMs.
3. **`direct_messages.content TEXT` vira `ciphertext BYTEA` + `nonce BYTEA(24)`:** `server-central` armazena e roteia bytes opacos, não consegue mais validar "conteúdo vazio" nem ler nada — só o tamanho bruto do ciphertext (`maxDMCiphertextLength`, `internal/httpapi/dms.go`). Mensagens de teste existentes na migração foram descartadas (`TRUNCATE`, sem usuários reais fora do autor ainda).
4. **Decifrar sempre com a chave pública ATUAL do outro lado da conversa** (resolvida via `GET /api/friends` no momento, não uma chave "pinada" por mensagem) — simplificação de v1, ver limitação de multi-dispositivo abaixo. Por isso o schema/wire não guarda a chave pública de quem enviou por mensagem, só `ciphertext`+`nonce`.
5. **Nunca cai para texto puro em silêncio:** se o amigo ainda não publicou uma chave (`peer.e2ePublicKey` ausente), o client bloqueia o envio com uma mensagem explícita (`useDirectMessages.ts`) em vez de mandar sem cifrar ou falhar sem explicação.

**Limitações de escopo, para não vender além do que entrega:**
- **Não protege contra um operador do `server-central` ativamente malicioso** substituindo a chave pública de alguém no diretório — não há verificação fora de banda ("números de segurança" tipo Signal) nesta v1. Protege contra acesso passivo aos dados em repouso (breach, dump, intimação), não contra um MITM ativo de quem opera a instância.
- **Sem forward secrecy / post-compromise security:** par de chaves estático por dispositivo, não um ratchet — se o dispositivo for comprometido depois, dá pra decifrar histórico antigo guardado ali.
- **Sem sincronização entre dispositivos:** trocar de navegador/reinstalar o Electron gera um par de chaves novo, perdendo acesso ao histórico antigo (mensagens antigas mostram "não foi possível decifrar neste dispositivo"). Conversas novas funcionam normalmente, porque a chave pública atual é sempre resolvida no momento do envio — só o histórico anterior à troca fica ilegível no dispositivo novo.

**Razão:** para uma instância central única que serve toda a plataforma, o ganho de "o operador não consegue ler DMs mesmo com acesso total ao banco" supera o custo de implementação de uma cifra de mensagem única (não é um protocolo de sessão completo) — mas um protocolo com ratchet/multi-dispositivo de verdade só se justifica com usuários reais e demanda por essas garantias adicionais, não antes.

**Revisitar quando:** houver demanda real por multi-dispositivo (nesse ponto avaliar um fluxo de "vincular dispositivo" que transfira a chave privada, ou migrar para um protocolo com prekeys) ou por proteção contra operador malicioso (nesse ponto avaliar verificação de chave fora de banda, tipo "números de segurança").

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

**Revisitar quando:** o app ganhar roteamento real (`react-router` ou similar) — nesse ponto vale avaliar se `react-oidc-context` (que já integra bem com rotas protegidas) passa a valer a pena. ~~Renovação silenciosa via refresh token pode ser revisitada junto do ajuste de `redirect_uris` de produção~~ — feito, ver "Decisão: renovação silenciosa de sessão no client" abaixo.

## Decisão: renovação silenciosa de sessão no client — refresh token (`offline_access`), não iframe

**Contexto:** item de TODO em aberto ("Ligar `automaticSilentRenew: true`") — sem renovação silenciosa, o access token expira (`access_token_validity: hours=1` no provider Authentik) e o usuário precisa logar de novo sem aviso.

**Alternativas consideradas:**
- **Iframe de silent renew (`prompt=none`)**, o padrão histórico do `oidc-client-ts`: exigiria um segundo `redirect_uri` (ex. `/auth/silent-renew.html`) cadastrado no blueprint do Authentik e uma página dedicada só para rodar `signinSilentCallback()` dentro do iframe oculto. Descartada: depende do navegador permitir ler o cookie de sessão do Authentik dentro de um iframe cross-origin — exatamente o que Chrome/Safari vêm bloqueando por padrão (third-party cookies) — e complica ainda mais no build Electron empacotado (esquema customizado `app://ffcom`, sem garantia de que um iframe oculto navegando para uma origem `https://` funcione da mesma forma dentro da `BrowserWindow`).
- **Refresh token (`offline_access`):** escolhida. `oidc-client-ts`, com `automaticSilentRenew: true`, usa o `refresh_token` do `User` (quando presente) para renovar via POST direto ao token endpoint — sem iframe, sem depender de cookie de terceiro, mesmo comportamento em web/PWA/Electron.

**Decisão:**
1. `client/src/auth/userManager.ts`: `scope` ganhou `offline_access` (`'openid profile email offline_access'`) e `automaticSilentRenew: true`.
2. **Authentik exige o scope mapping explícito, não só o client pedir o scope:** a partir da 2024.2, mesmo com `grant_types` incluindo `refresh_token` (já presente desde o registro inicial do provider `ffcom`) e o client requisitando `offline_access`, o Authentik só emite `refresh_token` se `offline_access` também estiver na lista `property_mappings` do provider. Adicionado em `abs-3d-printer/infra/authentik/blueprints/providers-ffcom.yaml` (`!Find [authentik_providers_oauth2.scopemapping, [scope_name, offline_access]]`), ao lado de `openid`/`email`/`profile` já existentes — precisa de `ak apply_blueprint` (ou restart do `authentik-server`) na instância central para valer, mesmo procedimento já usado nos registros anteriores do provider `ffcom`. **Aplicado em 2026-09-22:** push do commit `04a24dc` no `abs-3d-printer` (o deploy daquele repo atualiza o checkout montado em `/blueprints/custom` do worker, sem recriar os containers do Authentik) seguido de `docker exec infra-authentik-worker-1 ak apply_blueprint custom/providers-ffcom.yaml`; conferido via `ak shell` que o provider tem `offline_access` nos scope mappings e `refresh_token` nos grant types. O push é necessário, e não só aplicar um arquivo avulso: o Authentik reaplica periodicamente os blueprints a partir do arquivo montado, e uma versão aplicada só de `/tmp` seria revertida.
3. `client/src/auth/AuthProvider.tsx`: novo listener `userManager.events.addSilentRenewError` — se o refresh token expirar ou for revogado, a renovação falha silenciosamente por definição (não há iframe/redirect pra usuário ver); sem esse listener o estado React ficaria "signed-in" com um access token morto até a próxima chamada de API falhar. O listener força `applyUser(null)` (volta pra tela de login).

**Razão:** para um app que roda como PWA web e como Electron empacotado, refresh token evita a fragilidade do iframe (cookies de terceiro, comportamento inconsistente entre WebViews/Chromium embutido) sem exigir nenhum `redirect_uri` novo no Authentik — só um scope mapping adicional no provider já existente.

**Revisitar quando:** o Authentik desta instância passar a suportar rotação de refresh token de forma mais agressiva (hoje não configurado explicitamente) e valer a pena revisar o tempo de vida do refresh token para reduzir a janela de uso indevido caso um dispositivo seja comprometido (mesma categoria de risco já aceito para outros tokens guardados em `localStorage` neste client).

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

**Correção (2026-09-22): IP anunciado pelo LiveKit.** No primeiro teste real de voz, o client conectava a sinalização (sala criada, `starting RTC session`), mas o ICE nunca fechava: o LiveKit registrava `removing participant without connection` e o client desistia. Causa: `deploy/channel/docker-compose.yml` tinha `rtc.use_external_ip: true`, então o LiveKit descobria por STUN o IP de **saída** do host (`179.34.63.42`, WAN do site IPS01) e o anunciava nos candidatos ICE. Esse IP não aceita entrada (ver correção acima); a mídia só entra por `VMSUBS24OCI0102`. O comentário de `TURN_EXTERNAL_IP` no `.env.example` já dizia que o LiveKit usava esse IP, mas isso não era verdade: só o coturn usava. Antes de corrigir, validado que o caminho de entrada funciona: UDP enviado da LAN do site para `137.131.249.145:50099` chega em `10.20.4.10:50099` com origem `10.10.10.1` (o encaminhamento faz SNAT, então a resposta volta pelo mesmo caminho), e isso vale inclusive para cliente dentro do próprio site. Corrigido com `use_external_ip: false` + `node_ip: ${TURN_EXTERNAL_IP}`. O `server-channel/docker-compose.yml` de referência genérico continua com `use_external_ip: true`, que é o certo para o caso comum de self-hosting (port-forwarding no próprio roteador, IP de entrada igual ao de saída). **Achado relacionado, não corrigido:** o coturn do compose não é repassado aos clientes pelo LiveKit (não há `rtc.turn_servers` na config), então hoje nenhum client usa TURN; quem estiver atrás de NAT/firewall restritivo que bloqueie UDP 50000-50100 e o TCP 7881 fica sem voz. Registrado no TODO.

**3 runners dedicados, não 1 (2026-09-21):** rodar os 3 workflows juntos (fluxo normal quando várias mudanças tocam mais de um componente) serializava no único runner registrado — lint/scan de cada workflow rodam em paralelo entre si (sem dependência), mas as 3 etapas "Build, push e deploy" competiam pelo mesmo runner. Registrados mais dois (`SVRUBS24IPS0101-ffcom-02`, `-03`, mesma máquina, mesmo processo de `config.sh`/`svc.sh install`, mesma label `a3s-network`) — número escolhido para bater exatamente com os 3 workflows independentes que existem hoje (`deploy-ffcom-{central,channel,client}`), não mais que isso, já que não há um quarto workflow pra usar a capacidade extra. Host tinha folga de sobra pra isso (8 vCPU, ~16GB livres, load ~2-3 antes de adicionar).

## Decisão: compartilhamento de tela — `setScreenShareEnabled` do LiveKit, sem preview local

**Contexto:** implementar o item de TODO "Compartilhamento de tela" no `client`, sobre a integração de voz já existente (`useVoiceChannel.ts`/`VoiceChannelView.tsx`).

**Alternativas consideradas:** capturar `getDisplayMedia` à mão e publicar a track manualmente via `room.localParticipant.publishTrack`; usar `LocalParticipant.setScreenShareEnabled(enabled)` (helper nativo do `livekit-client`, já chama `getDisplayMedia` e cuida de source/publicação); renderizar as tracks de tela remotas como elementos de vídeo do React (state + `ref` por participante) vs. anexar/desanexar os elementos de forma imperativa (mesmo padrão já usado para os elementos de áudio, que são criados por `track.attach()` e inseridos direto no DOM).

**Decisão:**
1. `toggleScreenShare` (`client/src/hooks/useVoiceChannel.ts`) só chama `room.localParticipant.setScreenShareEnabled(next)` — sem `getDisplayMedia`/`publishTrack` manuais. O booleano `screenSharing` exposto pelo hook é derivado do participante local na lista de `participants` (`p.isScreenShareEnabled`), não é um `useState` paralelo — evita os dois ficarem dessincronizados se a track cair sozinha (ver ponto 3).
2. *(Nomes atualizados na decisão "câmera no canal de voz": `screenShareContainerRef` virou `videoContainerRef` e as classes `screen-share-*` viraram `video-*`, porque a mesma grade agora recebe câmera e tela.)* Renderização das tracks de tela remotas segue o mesmo padrão imperativo já usado para áudio: `RoomEvent.TrackSubscribed` com `track.source === Track.Source.ScreenShare` cria um `<video>` via `track.attach()` mais um rótulo com o nome do participante, embrulha os dois numa `<div class="screen-share-tile">` e insere no container apontado por `screenShareContainerRef` (um callback ref exposto pelo hook, preso pelo `VoiceChannelView` numa `<div>` vazia). `TrackUnsubscribed` remove a tile pelo `trackSid`. Alternativa descartada (state React por participante) exigiria sincronizar um `Map` de tracks com re-render a cada (un)subscribe só para montar `<video ref={...}>` — não ganha nada sobre continuar o padrão imperativo já estabelecido.
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

## Decisão: versionamento e release dos binários — semver independente por componente, disparado por git tag

**Contexto:** os três workflows de deploy (`deploy-ffcom-{central,channel,client}.yml`) só tinham `workflow_dispatch` manual, taggeando a imagem GHCR só com o sha curto do commit (`git rev-parse --short HEAD`) — nenhum conceito de versão, changelog ou release existia (TODO "Definir versionamento e forma de release dos binários").

**Alternativas consideradas:** versão única compartilhada pelos três componentes vs. versão independente por componente; disparo por push de git tag vs. input manual no `workflow_dispatch`.

**Decisão:**
1. **Versão independente por componente**, não uma versão única do repositório. Tags no formato `central-vX.Y.Z`, `channel-vX.Y.Z`, `client-vX.Y.Z`.
2. **Push de git tag** dispara o workflow do componente correspondente (`on.push.tags: ['central-v*']` etc.), além do `workflow_dispatch` manual que continua existindo para redeploy ad-hoc do commit atual.
3. Ao rodar por um push de tag, a imagem GHCR recebe **duas** tags: a de sempre (sha curto) e a versão semântica extraída da tag (`central-v0.1.0` → imagem `ghcr.io/apolope/ffcom-central:0.1.0`). Rodando via `workflow_dispatch` manual, só a tag de sha é publicada (comportamento inalterado). O passo de deploy continua usando o sha, não a versão — nenhuma mudança no mecanismo de deploy em si.
4. `server-central` e `server-channel` (binários Go) agora embutem a versão via `-ldflags "-X main.version=${VERSION}"` no build (`ARG VERSION=dev` no Dockerfile, `dev` fora de um build versionado) e expõem em `GET /healthz` (`{"status":"ok","version":"..."}`), útil para quem autohospeda `server-channel` confirmar qual versão está rodando. `client` (SPA estática servida por nginx) recebe a mesma tag de imagem por consistência, mas não expõe a versão em runtime hoje — não há uma tela "Sobre" ou endpoint equivalente para mostrá-la.

**Razão:** `server-channel` é autohospedado por terceiros e evolui fora de sincronia com a instância única oficial de `server-central` — faz sentido cada um ter seu próprio número de versão e seu próprio ritmo de release, em vez de forçar os três a subirem de versão juntos só porque vivem no mesmo repositório. Git tag como gatilho é o padrão mais comum para isso no ecossistema (versão fica registrada no histórico do git, `git tag`/`git describe` funcionam sem ferramenta extra) e evita ter que lembrar de digitar a versão certa toda vez que alguém aperta "Run workflow" na UI do Actions.

**Revisitar quando:** o build Electron/PWA (TODOs em aberto) existir — nesse ponto vale decidir como a versão do `client` aparece para o usuário final (tela "Sobre", rodapé, etc.) e se o instalador do Electron usa a mesma tag `client-vX.Y.Z` para nomear o artefato de release no GitHub Releases.

## Decisão: rate limiting em server-central — token bucket em memória por IP

**Contexto:** implementar o item de TODO "Rate limiting / proteção contra abuso em `server-central` (cadastro, login)". `server-central` não tem endpoint de "cadastro" ou "login" próprio — é Resource Server puro contra o Authentik central (ver "Decisão: autenticação em server-central"), e a conta local é criada implicitamente na primeira requisição autenticada válida (`auth.Middleware` → `AccountStore.GetOrCreateBySubject`, ver "Decisão: validação de JWT em server-central"). Não há, portanto, uma rota específica de auth para proteger isoladamente.

**Alternativas consideradas:** `golang.org/x/time/rate` (token bucket por IP, biblioteca padrão do ecossistema Go); rate limiting no proxy reverso (NPM) na frente da instância, fora do repositório; limite em memória implementado à mão, sem dependência nova.

**Decisão:** limite geral por IP sobre toda a API (exceto `/healthz`), com um token bucket em memória implementado à mão (`internal/httpapi/ratelimit.go`, tipo `rateLimiter`), sem depender de `golang.org/x/time/rate` nem de configuração no proxy reverso. Chave é o IP do cliente (primeiro valor de `X-Forwarded-For` quando presente — a implantação de referência roda atrás de proxy reverso, ver "Decisão: primeira implantação de teste" — senão `RemoteAddr`). Configurável via `RATE_LIMIT_RPM` (padrão 120) e `RATE_LIMIT_BURST` (padrão 20); excesso responde `429 Too Many Requests` com `Retry-After`. Buckets ociosos por mais de 10 minutos são varridos periodicamente para não crescer sem limite.

**Razão:** a lógica de um token bucket é pequena o suficiente (~60 linhas) para não justificar puxar `golang.org/x/time/rate` só por isso — mesma filosofia de dependências enxutas já registrada em várias decisões anteriores (token do LiveKit assinado à mão, pgx sem ORM). Fazer isso no proxy reverso moveria a decisão para fora deste repositório (infra de outro projeto, `a3s-network`) e deixaria de proteger quem roda `server-central` sem esse proxy na frente (ex. outro self-hoster futuro, se a instância única deixar de ser premissa). Limite geral por IP (em vez de só num endpoint de "login") é a escolha certa aqui precisamente porque não existe um endpoint de auth isolado para mirar.

**Revisitar quando:** `server-central` precisar rodar em múltiplas réplicas (mesmo gatilho já registrado no Hub de presença) — nesse ponto um bucket em memória por processo deixa de ser suficiente (cada réplica teria seu próprio orçamento) e valeria a pena mover para Redis ou para o proxy reverso compartilhado.

## Decisão: ManageRoles não concede permissões além das próprias

**Contexto:** revisão de segurança do item de TODO "Política de permissões/roles em `server-channel` revisada contra escalonamento de privilégio". `requireManageRoles` (`internal/httpapi/roles.go`) exigia só o bit `ManageRoles` para: criar/editar uma role com qualquer bitmask de `Permissions` arbitrário (inclusive `Administrator`), atribuir qualquer role (inclusive uma recém-criada com `Administrator`) a qualquer membro — inclusive a si mesmo — e criar overwrites de canal com `Allow` arbitrário. Um membro com `ManageRoles` mas sem `Administrator` (ex. papel de "moderador" comum) conseguia, portanto, se auto-conceder `Administrator` em 3 passos: `POST /api/roles` com `permissions: Administrator`, depois `POST /api/members/{self}/roles/{roleId}` — sem precisar de mais nada. Nenhum teste ou checagem cobria isso; foi encontrado só por leitura de código.

**Alternativas consideradas:** (1) checagem de hierarquia por `Position` (a role só pode ser gerenciada por quem tem uma role de posição mais alta, modelo do Discord) — o schema já tem a coluna `position`, mas hoje ela só ordena a listagem, nunca foi usada para autorização; (2) checagem de subconjunto de bits: quem tem `ManageRoles` só pode conceder (via criação/edição de role, atribuição de role ou `Allow` de overwrite) bits de permissão que ele mesmo já possui, com `Administrator`/dono sempre passando; (3) exigir `Administrator` (em vez de `ManageRoles`) para qualquer operação que toque o bitmask de permissões, reservando `ManageRoles` só para operações que não envolvam o próprio bitmask (ex. nenhuma, já que toda rota de `/api/roles` mexe em `permissions`).

**Decisão:** opção 2 — `permissions.Grants(base, target int64) bool` (`internal/permissions/permissions.go`) reporta se a permissão base do requisitante (`Administrator` ou `Owner` sempre concedem tudo; senão, `target &^ base == 0`, ou seja, todo bit de `target` já está em `base`) cobre o bitmask sendo concedido. Aplicado nos quatro pontos que efetivamente entregam poder a alguém:
- `POST /api/roles` e `PATCH /api/roles/{id}` — `body.Permissions` precisa passar em `Grants(base, body.Permissions)`.
- `POST /api/members/{memberId}/roles/{roleId}` — as permissões da role sendo atribuída (`target.Permissions`, buscada via `findRole`) precisam passar em `Grants(base, target.Permissions)`. Este é o passo que fecha o caminho de auto-escalonamento, mesmo que a role já existisse (criada por outra pessoa) em vez de ser criada na hora pelo próprio atacante.
- `PUT /api/channels/{id}/overwrites/{roleId}` — só o `Allow` (`body.Allow`) precisa passar em `Grants`; `Deny` não, porque negar um bit nunca concede poder a ninguém (só restringe, e overwrites nem chegam a ser aplicados contra quem já tem `Administrator`, ver `permissions.Effective`).

Deliberadamente **não** adicionada a mesma checagem em `DELETE /api/roles/{id}` (deletar) nem em `DELETE /api/members/{memberId}/roles/{roleId}` (desatribuir) — remover algo de alguém não concede nada a quem remove, é uma classe de problema diferente (sabotagem/abuso de moderação, não escalonamento de privilégio) e ficou fora do escopo desta revisão.

**Razão:** a checagem por subconjunto de bits (opção 2) resolve exatamente o vetor relatado (ganhar um bit que não se tinha) com uma função pura de ~5 linhas, sem exigir estado adicional nem mexer no modelo de dados. A opção 3 (exigir `Administrator` para tudo) tornaria `ManageRoles` inútil na prática — hoje ele existe precisamente para permitir um "moderador" gerenciar roles/overwrites sem ser dono/admin total, um caso de uso legítimo (ex. dar `Voice`/`SendMessages` para um cargo novo) que a opção 2 preserva. A hierarquia por `Position` (opção 1) é o modelo real do Discord e mais completa (também impediria um moderador de *editar/remover* uma role acima da sua, não só de se auto-conceder bits), mas exigiria decidir como calcular a "posição" efetiva de um membro com várias roles e não estava causando o problema relatado — fica registrada como possível endurecimento futuro.

**Revisitar quando:** o produto quiser impedir também que um `ManageRoles` sem `Administrator` edite/remova/desatribua uma role cujas permissões ele não teria como conceder (hoje isso continua permitido) — nesse ponto vale portar a mesma checagem `Grants` para `handleDeleteRole`/`handleRemoveRole`, ou adotar hierarquia por `Position` (opção 1) para cobrir também renomear/mover uma role sem tocar no bitmask.

## Decisão: empacotamento Electron — `electron-builder`, sem assinatura de código ainda

**Contexto:** implementar o TODO "Build Electron para Windows/macOS/Linux". O wrapper Electron já existia (`vite-plugin-electron`, ver "Decisão: wrapper Electron" acima), mas só rodava em modo dev/unpacked (`npm run electron`) — nenhum instalador distribuível.

**Alternativas consideradas:** `electron-builder` (padrão de facto, config declarativa via `package.json`, suporta NSIS/DMG/AppImage entre outros); `electron-forge` (mais opinativo, própria CLI de scaffolding, redundante já que o scaffolding via `vite-plugin-electron` já existe); scripts manuais chamando `@electron/packager` + ferramentas de instalador separadas por plataforma.

**Decisão:** `electron-builder`, configurado no bloco `"build"` de `client/package.json` (não em arquivo `electron-builder.yml` separado, mesma filosofia de manter config num único lugar já usada para o restante do projeto). Alvo por plataforma: NSIS (`.exe`) no Windows, DMG no macOS, AppImage no Linux — os três formatos mais comuns/de menor fricção para instalação manual em cada SO, sem exigir loja de aplicativos. Scripts novos em `package.json`: `package` (plataforma do host atual), `package:win`/`package:mac`/`package:linux` (força uma plataforma). Saída em `release/` (já estava no `.gitignore`).

**Verificado nesta sessão (2026-09-21):** build completo (`npm run build:electron` + `electron-builder`) e execução do artefato final para Windows (nativo, neste host) e Linux (AppImage, via container `electronuserland/builder:22`, já que builds Linux exigem toolchain nativo — `@electron/rebuild`/gcc/python — não presente por padrão num host Windows). macOS **não verificado**: `electron-builder` exige rodar em host macOS de verdade para o alvo `mac` (assinatura/notarização dependem de ferramentas da Apple — `codesign`/`iconutil` — que não existem em Linux/Windows, nem mesmo via Docker); a config para `mac` está em `package.json` mas fica sem teste real até haver acesso a uma máquina macOS.

**Deliberadamente fora de escopo desta rodada:**
- **Ícone customizado:** hoje usa o ícone padrão do Electron (só existe `client/public/favicon.svg`, usado no `<link rel="icon">` do web/PWA — `electron-builder` precisa de `.ico`/`.icns`/PNGs em resoluções específicas, não do SVG direto). Ícone segue o padrão do Electron até existir arte própria.
- **Assinatura de código:** sem certificado Authenticode (Windows) nem Apple Developer ID (macOS) — instalador Windows vai disparar aviso do SmartScreen, e o app macOS (quando existir) vai exigir bypass do Gatekeeper. Aceitável para uso pessoal/self-host nesta fase; certificados custam dinheiro/processo de verificação, não faz sentido adquirir antes de haver usuários reais fora do autor.
- **CI/GitHub Releases:** nenhum workflow novo publica os instaladores automaticamente. O workflow `deploy-ffcom-client.yml` existente só builda a imagem Docker (web/PWA) — construir os 3 instaladores desktop em CI exigiria runners macOS (não existe no `a3s-network`, que é só `SVRUBS24IPS0101`, Linux) e ficou fora do escopo deste item. Empacotar continua manual (`npm run package:*`) até haver demanda real de distribuição para terceiros.

**Razão:** `electron-builder` é a escolha padrão do ecossistema (documentação extensa, suporta os 3 alvos com uma única config declarativa) e evita reimplementar orquestração de instalador que ele já resolve — mesma filosofia de "não reinventar o que uma lib madura já cobre" aplicada em outras decisões deste projeto (LiveKit para SFU, `oidc-client-ts` para OIDC). Testar via Docker (`electronuserland/builder`) em vez de só confiar na config, mesmo sem CI dedicado, porque a alternativa (declarar o TODO concluído sem rodar o build nenhuma vez) teria risco real de a config estar quebrada e só ser descoberta na hora de precisar de um instalador de verdade.

**Revisitar quando:** houver usuários reais fora do autor pedindo instaladores assinados (nesse ponto, comprar certificado Authenticode/Apple Developer ID) e/ou builds automatizados via tag `client-vX.Y.Z` publicando em GitHub Releases (precisaria de um runner macOS, hospedado ou GitHub-hosted, para o alvo `mac` — os alvos `win`/`linux` já podem migrar para o `deploy-ffcom-client.yml` existente ou um workflow irmão a qualquer momento, sem essa dependência).

## Decisão: build web/PWA — `vite-plugin-pwa`, ícones gerados a partir do favicon.svg, desligado no build Electron

**Contexto:** implementar o TODO "Build web/PWA" em `client`. O `Dockerfile`/`nginx.conf` já servem a SPA como web app comum (ver "Decisão: primeira implantação de teste"), mas sem manifest nem service worker — não instalável, sem ícone além do favicon do navegador.

**Alternativas consideradas:** escrever manifest.json e um service worker à mão (controle total, mas reimplementa cache/versionamento que já é problema resolvido); `vite-plugin-pwa` (plugin de Vite, gera manifest + service worker via Workbox a partir de config declarativa, mesmo padrão de "reaproveitar a config do Vite já existente" já usado na decisão do wrapper Electron); para os ícones — exportar PNGs manualmente do design (`favicon.svg`) em cada resolução exigida, ou gerar via `@vite-pwa/assets-generator` (ferramenta irmã do plugin, produz o `favicon.ico`/PNGs/ícone maskable a partir de uma única imagem fonte).

**Decisão:**
1. `vite-plugin-pwa` (`registerType: 'autoUpdate'`) adicionado a `vite.config.ts`, condicionado a `mode !== 'electron'` (mesmo padrão de flag condicional já usado pelo plugin do Electron) — o shell desktop já é o "app instalado" e não deve ganhar service worker/manifest.
2. Ícones gerados uma vez com `@vite-pwa/assets-generator` (`client/pwa-assets.config.ts`, preset `minimal2023Preset`) a partir de `public/favicon.svg`: `pwa-64/192/512.png` (transparente), `maskable-icon-512x512.png` (fundo branco + padding 30%, safe zone para launcher Android), `apple-touch-icon-180x180.png` e `favicon.ico`. Gerados uma vez e commitados em `public/` (não gerados no build) para não depender de `resvg`/`sharp` no pipeline de CI/Docker; script `npm run generate:pwa-assets` reroda se `favicon.svg` mudar.
3. `workbox.navigateFallbackDenylist: [/^\/api\//, /^\/auth\//]` — o fallback de SPA do service worker (servir `index.html` para rotas sem arquivo, necessário para `/auth/callback`) não deve interceptar chamadas de API real; sem isso, uma rota `/api/...` sem conectividade cairia silenciosamente no `index.html` em vez de falhar visivelmente.
4. `registerType: 'autoUpdate'` (troca silenciosa para a versão nova assim que baixada, sem prompt) em vez de `prompt` — consistente com a UX de um SPA que já não tem histórico/estado de navegação persistente entre versões; o chat em si é stateless o bastante (histórico vem sempre do backend) para não haver risco real de perder algo no meio de uma troca de versão.
5. `theme_color`/`background_color` do manifest usam os tokens já existentes em `src/index.css` (`--accent: #aa3bff` do tema claro, `--bg: #fff`) em vez de escolher uma cor nova — mantém o manifest consistente com o CSS sem introduzir uma segunda fonte de cor de marca.

**Razão:** `vite-plugin-pwa` evita reimplementar cache/versionamento de service worker (problema já resolvido pelo Workbox) com uma dependência que já se integra à mesma config do Vite usada pelo resto do projeto — mesma filosofia já registrada nas decisões de LiveKit, `oidc-client-ts` e `electron-builder` ("não reinventar o que uma lib madura já cobre"). Gerar os ícones uma vez e commitar os PNGs (em vez de gerar no build) segue a mesma razão já registrada para não trazer `resvg`/`sharp` como dependência de build de produção — o Dockerfile do client já não tem esse tipo de ferramenta de imagem.

**Verificado nesta sessão (2026-09-21):** `npm run build` gera `manifest.webmanifest`/`sw.js`/`registerSW.js` e os ícones apontam para arquivos existentes; `npm run build:electron` confirmadamente **não** injeta manifest/service worker no `dist/index.html` (checado via grep no HTML gerado). Servido via `vite preview`, `manifest.webmanifest` responde com `Content-Type: application/manifest+json`, `sw.js` com `text/javascript`, e o `navigateFallbackDenylist` aparece no `sw.js` compilado. Não testado num navegador de verdade (extensão Claude in Chrome indisponível nesta sessão) — instalabilidade completa (banner "Instalar app", ícone na tela inicial) fica como verificação pendente da próxima vez que alguém abrir `app.ffcom.a3sitsolutions.com.br` num Chrome real.

**Revisitar quando:** o produto quiser notificações push (Web Push exige um passo de configuração separado, `vite-plugin-pwa` não cobre isso sozinho) ou suporte offline real de mensagens (hoje o cache do Workbox só cobre os assets estáticos da SPA, não o histórico de chat — abrir o app sem rede mostra o shell, não mensagens antigas).

## Decisão: guia de self-hosting de `server-channel` — TLS via proxy reverso, Caddy como recomendação default

**Contexto:** implementar o TODO "Guia de self-hosting de `server-channel`". O compose de referência já cobria Docker Compose, TURN e DNS dinâmico (`server-channel/README.md`); faltava TLS — o `app` só fala HTTP puro na porta 8080, sem terminação TLS embutida no binário — e os READMEs (raiz, `server-central`, `server-channel`) estavam desatualizados, ainda descrevendo o projeto como "fase de planejamento/scaffolding, nenhum componente funcional" quando na verdade já há uma implantação de teste completa no ar (ver "Decisão: primeira implantação de teste").

**Alternativas consideradas para TLS:** implementar terminação TLS dentro do próprio binário Go (`server-channel`/`server-central` servindo HTTPS direto, com certificado próprio); recomendar um proxy reverso específico na documentação; deixar em aberto, sem recomendação, já que cada self-hoster pode ter preferência própria.

**Decisão:** TLS fica fora do binário, delegado a um proxy reverso na frente — mesmo padrão já usado na implantação de referência (Nginx Proxy Manager no `a3s-network`, ver "Decisão: primeira implantação de teste"). Para quem autohospeda sem infra própria já montada, o guia recomenda [Caddy](https://caddyserver.com/) como default (emite/renova Let's Encrypt sozinho, `Caddyfile` de 3 linhas, sem passo manual de certificado), mas documenta que Nginx Proxy Manager/Traefik funcionam igual desde que repassem o upgrade de WebSocket usado por `GET /api/channels/{id}/ws`.

**Razão:** terminação TLS embutida no binário Go duplicaria um problema já resolvido por qualquer proxy reverso maduro (renovação automática de certificado, redirecionamento HTTP→HTTPS, etc.) — mesma filosofia de "não reinventar o que uma lib/ferramenta madura já cobre" já registrada em várias decisões anteriores (LiveKit, `oidc-client-ts`, `vite-plugin-pwa`). Caddy como recomendação (não obrigação) segue a mesma lógica já registrada para o self-hoster leigo: menor caminho para "funcionando" sem exigir conhecimento prévio de configuração de proxy, mas sem travar quem já usa outra coisa.

**Revisitar quando:** o TODO "Criptografia em trânsito (TLS) obrigatória entre client e ambos os tipos de servidor" for atacado — hoje TLS é recomendado no guia mas não é tecnicamente exigido (nada no `client` ou nos servidores recusa conexão HTTP pura); tornar obrigatório exigiria decidir onde essa checagem vive (client recusa endereço `http://`? servidor recusa iniciar sem certificado configurado?), o que é escopo daquele TODO separado, não deste guia.

## Decisão: criptografia em trânsito obrigatória — client recusa `http://` fora de localhost, servidor exige `X-Forwarded-Proto: https`

**Contexto:** implementar o TODO "Criptografia em trânsito (TLS) obrigatória entre client e ambos os tipos de servidor", deixado em aberto na decisão anterior (guia de self-hosting). Nenhum dos dois binários Go termina TLS (delegado a um proxy reverso, ver decisão acima) — "obrigatório" não pode significar "o binário recusa iniciar sem certificado", porque ele nunca tem um certificado para checar.

**Alternativas consideradas:** (a) só o client recusar `http://` na UI; (b) só o servidor exigir `X-Forwarded-Proto: https`; (c) as duas coisas; (d) adiar, sem mudança de código agora.

**Decisão:** as duas camadas, escolhidas pelo usuário (Apolonio) quando questionado sobre onde a checagem deveria viver:

1. **Client** (`AddServerDialog.tsx`, `isAddressSecure`): recusa registrar um `server-central`/`server-channel` cujo endereço não seja `https://`, exceto `localhost`/`127.0.0.1`/`::1` (dev local). Cobre o caso de alguém colar um endereço `http://` de produção por engano.
2. **Servidor** (`internal/httpapi/requiretls.go`, idêntico nos dois — mesmo padrão de duplicação já usado em `cors.go`): middleware `withRequireTLS`, controlado por `REQUIRE_TLS` (env var booleana, **padrão desligado**), que rejeita com `426 Upgrade Required` qualquer requisição sem `X-Forwarded-Proto: https`, exceto `/healthz` (mesma exceção já aplicada a `withRateLimit`, porque o `HEALTHCHECK` do Docker chama `http://127.0.0.1:8080/healthz` de dentro do próprio container, sem passar pelo proxy).

**Por que `REQUIRE_TLS` tem padrão desligado, e não ligado:** os `docker-compose.yml` de referência (`server-central/`, `server-channel/`) são usados tanto para produção (atrás de um proxy externo) quanto para dev local direto — sem a flag desligada por padrão, `docker compose up` local (fluxo documentado nos dois `README.md`) quebraria imediatamente, já que nada envia `X-Forwarded-Proto` sem um proxy na frente. A instância de teste real (`a3s-network`) já tem TLS+proxy validados ponta a ponta (ver "Validação ponta a ponta" no TODO), mas ligar `REQUIRE_TLS=true` lá é uma mudança de infra fora deste repositório (o `.env` real vive em `/opt/ffcom/envs/`, fora do git) — documentado como próximo passo em `deploy/{central,channel}/.env.example`, condicionado a confirmar antes que o Nginx Proxy Manager em `VMSUBS24OCI0102` realmente sete esse header (não verificado nesta sessão).

**Razão:** mesma filosofia de "não reinventar o que o proxy já resolve" das decisões anteriores — o servidor não tenta validar certificado nem terminar TLS, só desconfia da ausência do sinal que só um proxy TLS-terminating envia. Duas camadas (client + servidor) porque cobrem falhas diferentes: o client evita o erro humano de digitar `http://`; o servidor cobre o caso de alguém expor a porta diretamente (sem proxy nenhum, ou um client de terceiros que ignore a checagem da UI).

**Verificado nesta sessão (2026-09-21):** `go build ./...` limpo em `server-central` e `server-channel` após a mudança de assinatura de `NewRouter`. Não testado contra um proxy real setando `X-Forwarded-Proto` nem contra a instância de produção — `REQUIRE_TLS` segue desligado em todo `.env`/`.env.example` deste repositório, incluindo os de `deploy/`.

**Ligado na instância oficial (2026-09-22):** `REQUIRE_TLS=true` adicionado aos dois `.env` reais em `/opt/ffcom/envs/` (cópia `.bak-<timestamp>` ao lado). Antes foi preciso fazer o primeiro deploy versionado (`central-v0.1.0`/`channel-v0.1.0`): as imagens em produção eram de 20-21/09 e não tinham o middleware, então a flag teria sido ignorada. Validação: pelos hostnames públicos (passando pelo NPM de `VMSUBS24OCI0102`) `GET /api/me` e `GET /api/channels` respondem `401`, o upgrade de WebSocket nos dois servidores responde `401` e o preflight CORS responde `204`, ou seja, nenhum `426`; de dentro do container, sem proxy e sem o header, a mesma rota responde `426 Upgrade Required`. Isso confirma que o NPM seta `X-Forwarded-Proto: https` nos hostnames `central.` e `channel-test.`. `deploy/{central,channel}/.env.example` passaram a trazer `REQUIRE_TLS=true` descomentado; os `docker-compose.yml`/`.env.example` de referência genéricos (`server-central/`, `server-channel/`) continuam com o padrão desligado, pelo motivo acima.

**Revisitar quando:** surgir um self-hoster usando um proxy que não sete `X-Forwarded-Proto` (exigiria documentar exceção ou um header alternativo), ou se a instância oficial trocar de proxy reverso.

## Decisão: domínio oficial — `ffcom.a3sitsolutions.com.br`, instância de teste vira a oficial

**Contexto:** o TODO "Provisionar `ffcom.a3sitsolutions.com`" listava a extensão (`.com` vs `.com.br`) como a decidir. A "primeira implantação de teste" (ver seção acima) já usa `*.ffcom.a3sitsolutions.com.br` na infra do `a3s-network`, com DNS/TLS/OIDC validados ponta a ponta (2026-09-21).

**Decisão (Apolonio, 2026-09-21):** `.com.br`, não `.com` — decisão já tomada antes desta sessão, só não estava registrada aqui. Como a extensão coincide com a já usada na implantação de teste, essa instância deixa de ser "provisória" e passa a ser a oficial; não há domínio novo a provisionar. Os hostnames mantêm o sufixo `-test` onde já existia (`channel-test.`, `livekit-test.ffcom.a3sitsolutions.com.br`, ao lado de `app.`/`central.` sem sufixo) — renomear exigiria novo trabalho do agente de infra (DNS + certificado) em `VMSUBS24OCI0102`, sem ganho que justifique isso agora.

**Razão:** evita provisionar uma segunda instância/domínio só para "ser a oficial" quando a de teste já está validada ponta a ponta e em uso.

## Decisão: callback OIDC no Electron empacotado — esquema customizado `app://ffcom`

**Contexto:** o build Electron empacotado (`client/electron/main.ts`) carregava o bundle via `win.loadFile('../dist/index.html')`, isto é, `file://`. Isso quebrava o login OIDC de duas formas: (1) `redirect_uri` em `userManager.ts` é `window.location.origin + '/auth/callback'` — sob `file://` cada documento tem uma origin opaca, sem um valor estável para cadastrar no Authentik; (2) os assets do Vite usam caminho raiz-absoluta (`/assets/...`), que sob `file://` resolve contra a raiz do sistema de arquivos, não contra `dist/` (bug latente, não pego antes porque o smoke test do TODO "Build Electron" só confirmou que o instalador roda, não que a UI renderiza).

**Alternativas consideradas:** (a) `loadURL` apontando pro domínio de produção do client web (`https://app.ffcom.a3sitsolutions.com.br`) em vez do bundle local — mais simples, mas transforma o app "instalado" num wrapper fino que sempre depende desse domínio estar no ar, e amarra o build empacotado a um domínio fixo (ruim para quem faz build próprio noutro domínio, dado o caráter self-host do projeto); (b) esquema customizado (`app://`) via `protocol.handle`, servindo os arquivos de `dist/` e dando uma origin fixa e estável independente de domínio.

**Decisão:** (b). `client/electron/main.ts` registra o esquema `app` como privileged (`standard`, `secure`, `supportFetchAPI`, `corsEnabled`) e implementa `protocol.handle('app', ...)` resolvendo `pathname` contra `dist/` (com fallback para `index.html` em qualquer rota sem arquivo correspondente — mesmo padrão de `try_files` do `client/nginx.conf` — necessário para `/auth/callback` funcionar). A janela carrega `app://ffcom/index.html`. Origin resultante: `app://ffcom`, igual em toda execução do app empacotado, em qualquer host/domínio de build.

**Verificado nesta sessão (2026-09-21):** `npm run build:electron` limpo; smoke test do binário (`electron .` sobre o build empacotado, fora do modo dev) confirmou o bundle JS carregando via `app://ffcom` (fetch 200, tamanho batendo com o bundle) e o React montando (root com conteúdo, tela de login). `app://ffcom/auth/callback` já foi cadastrado como `redirect_uri` no blueprint `providers-ffcom.yaml` (repo `abs-3d-printer`, fora deste repositório). Ainda faltam, antes do fluxo completo de login funcionar de ponta a ponta no app empacotado: adicionar `app://ffcom` a `CORS_ALLOWED_ORIGINS` de `server-central`/`server-channel` (ops, fora do git — ver `TODO.md`) e testar o redirect real pro Authentik e volta (não testado nesta sessão, só o carregamento local do bundle).

**Razão:** uma origin fixa e independente de domínio combina melhor com o resto do projeto (self-host, sem assumir uma instância "oficial" única de `server-central`/`server-channel` — só o Authentik central é de fato compartilhado) do que amarrar o app empacotado a um domínio de produção específico. Resolve os dois problemas (asset loading e origin do redirect_uri) com uma única mudança.

**Revisitar quando:** `post_logout_redirect_uri` (também `window.location.origin`, hoje sem uso ativo de logout redirect testado) precisar de cadastro equivalente no Authentik; ou se o esquema de deep link para convites (`docs/architecture.md`, "Revisitar quando" da decisão de convites) for implementado — os dois podem reaproveitar o mesmo `protocol.handle`.

## Decisão: CI de testes/lint — workflow novo separado do deploy, disparado em `push`/`pull_request`

**Contexto:** implementar o item de TODO "CI rodar testes/lint de código". Os três workflows existentes (`deploy-ffcom-{central,channel,client}.yml`) só disparam em `workflow_dispatch` ou push de tag (`central-v*`/`channel-v*`/`client-v*`, ver "Decisão: versionamento e release dos binários"), e cada um só faz Hadolint (Dockerfile) + Gitleaks (segredo) antes do build/push/deploy — nenhum roda `go vet`/`go test` nem lint/build do frontend. `CONTRIBUTING.md` já documentava esses comandos como algo a rodar manualmente antes de commitar (`go vet ./...`, `go test ./...` nos dois servidores; `npm run lint`, `npm run build` no client), mas nada os executava automaticamente.

**Alternativas consideradas:** (a) adicionar um job de teste/lint dentro de cada workflow de deploy existente, antes do build; (b) um workflow novo (`ci.yml`), disparado em `push`/`pull_request` em vez de tag.

**Decisão:** (b). `.github/workflows/ci.yml`, com três jobs independentes (`test-central`, `test-channel`, `lint-build-client`), cada um rodando os mesmos comandos já documentados em `CONTRIBUTING.md`. Roda nos mesmos runners self-hosted (`[self-hosted, a3s-network]`) dos outros workflows.

**Razão:** os workflows de deploy só disparam em tag — se o job de teste ficasse lá dentro, um `push` comum (o caso comum durante o desenvolvimento, antes de cortar uma release) nunca rodaria `go test`/lint, só o Hadolint/Gitleaks já existentes. Um workflow separado disparado em `push`/`pull_request` cobre esse caso sem tocar nos workflows de deploy (que continuam só com a checagem de Dockerfile/segredo antes do build, já suficiente ali porque o código já devia ter passado pelo CI antes de virar tag).

**Revisitar quando:** o repositório ganhar teste de integração contra Postgres real (hoje os únicos 2 arquivos `_test.go` são unitários, sem dependência externa) — nesse ponto o job de teste precisa subir um serviço de Postgres (`services:` do GitHub Actions) em vez de só `go test ./...`.

## Decisão: editar/apagar mensagem de texto — frames WS, autor apaga ou edita a própria, Administrator apaga qualquer uma

**Contexto:** implementar o item de TODO "Endpoint para editar/apagar mensagem de texto" em `server-channel` (e a UI correspondente no `client`). `MessageStore.Edit`/`Delete` (`internal/store/messages.go`) já existiam desde a decisão original de canal de texto, mas nenhuma rota expunha os dois — mensagem enviada era permanente na prática.

**Alternativas consideradas (transporte):** REST (`PATCH`/`DELETE /api/messages/{id}`) — mais simples de testar isoladamente, mas introduz uma segunda forma de mutar o mesmo recurso (mensagem) fora do WebSocket, sem propagação ao vivo para outros clients conectados ao canal (precisaria de um broadcast manual do handler REST para o Hub, misturando os dois transportes); frames novos no mesmo WebSocket de canal (`message.update`, `message.delete`), mesmo padrão já usado por `message.create`/`thread.create`/`post.create`, com o servidor respondendo `message.updated`/`message.deleted` via broadcast do `realtime.Hub`.

**Decisão:** frames WS. `handleChannelWS` (`internal/httpapi/channel_ws.go`) passou a despachar o texto recebido num canal de texto por `type` (`realtime.FrameType`, mesmo mecanismo já usado pelo canal forum) em vez de assumir sempre `message.create` — `handleIncomingTextFrame` decide entre `handleIncomingMessage` (cria, exige `SendMessages`), `handleIncomingMessageUpdate` e `handleIncomingMessageDelete`. Canal forum **não** ganhou os frames novos (fora do escopo do item de TODO, que fala só de canal de texto); revisitar junto da decisão de forum se um dia fizer sentido.

**Autorização (por mensagem, não pelo bit `SendMessages` checado uma vez por conexão):** editar exige ser o autor (`MessageStore.GetByID` confirma `AuthorMemberID` e `ChannelID` antes de `Edit`) — sem edição por moderação nesta v1. Apagar aceita autor **ou** `Administrator` (`permissions.Has(effective, permissions.Administrator)`) — não há bit de permissão dedicado a mensagens (mesma lacuna já registrada no TODO para kick/ban), então reaproveita o bit que já ignora qualquer outra checagem no resto do sistema.

**Decisão (protocolo):** `message.update` `{id, content}` → broadcast `message.updated` `{message}` (mesmo `MessageView`, com `editedAt` preenchido). `message.delete` `{id}` → broadcast `message.deleted` `{id, channelId}` (payload mínimo — quem recebe só precisa remover da lista local, não do `MessageView` inteiro). Ver `docs/protocol.md`.

**Cliente:** `lib/serverChannelApi.ts` ganhou `sendUpdateMessage`/`sendDeleteMessage` e os dois frames novos em `ChannelSocketFrame`; `hooks/useChannelChat.ts` aplica `message.updated` (substitui a mensagem na lista pelo id) e `message.deleted` (remove pelo id) e expõe `editMessage`/`deleteMessage`. `components/TextChannelView.tsx` mostra "editar"/"apagar" só na própria mensagem (`authorMemberId === selfMemberId`), ao passar o mouse — apagar é direto, editar troca o conteúdo por um form inline reaproveitando o mesmo frame de saída; sem UI para Administrator apagar mensagem de outro membro ainda (a permissão já existe no servidor, só falta o botão condicionado a `Administrator` no client).

**Razão:** manter tudo no mesmo WebSocket evita duplicar a lógica de propagação em tempo real (REST teria que chamar o Hub manualmente) e segue exatamente o padrão já estabelecido por `message.create`/`thread.create`/`post.create` — o mesmo motivo já registrado na decisão de canal forum ("reaproveitar infraestrutura existente em vez de generalizar cedo demais").

**Revisitar quando:** um bit de permissão dedicado a moderação de mensagens for criado (ver TODO, kick/ban); ou quando canal forum precisar dos mesmos frames (hoje só citado como possibilidade, sem caso de uso concreto).

## Decisão: rate limiting em server-channel — dois limiters, por IP na API REST e por membro nos frames de WebSocket

**Contexto:** implementar o item de TODO "Rate limiting em `server-channel`". `server-central` já tinha proteção por IP (ver "Decisão: rate limiting em server-central"), mas `server-channel` não — apesar de ser exposto publicamente por cada self-hoster (REST + WebSocket de chat) e não ter nenhuma proteção contra spam/abuso.

**Diferença em relação a server-central:** ali, o único tráfego de escrita repetido é REST — um limiter por IP sobre toda a API cobre o cenário inteiro. Aqui, o vetor mais óbvio de spam é enviar muitos frames `message.create` (ou `message.update`/`message.delete`/`thread.create`/`post.create`) numa única conexão WebSocket já estabelecida — um limiter que só olha a requisição HTTP de upgrade não enxerga nada do que acontece depois disso, então reaplicar exatamente o mesmo desenho de server-central deixaria o canal de chat, que é o alvo mais provável de abuso, sem proteção nenhuma.

**Alternativas consideradas:** (1) um único limiter por IP cobrindo REST + o handshake de WS, igual a server-central — simples, mas não protege contra flood dentro de uma conexão já aberta; (2) limitar por IP também dentro do WS — mais simples de implementar (reaproveita a mesma chave), mas member ID é a identidade que já resolvemos no handshake (`auth.RequireMember`) e é o que o resto do sistema usa para autorização por mensagem (ver decisão de editar/apagar mensagem), então é a chave mais correta para atribuir "de quem" é o spam, além de não punir todo mundo atrás do mesmo IP/NAT por um único membro abusivo; (3) dois limiters (adotada).

**Decisão:** dois `rateLimiter` (mesmo token bucket em memória de `server-central/internal/httpapi/ratelimit.go`, duplicado aqui — módulos Go separados, mesmo padrão de duplicação já usado por `requiretls.go`/`cors.go`):
1. Um por IP (`clientIP`, mesma extração via `X-Forwarded-For`/`RemoteAddr`), aplicado via `withRateLimit` a toda a API REST exceto `/healthz` — cobre inclusive o handshake HTTP de `GET /api/channels/{id}/ws`. Configurável via `RATE_LIMIT_RPM`/`RATE_LIMIT_BURST` (mesmos nomes e mesmos padrões de `server-central`: 120/20).
2. Um por `member.ID`, checado dentro do callback de `Client.ReadPump` em `handleChannelWS` (`internal/httpapi/channel_ws.go`), antes de despachar qualquer frame — tanto em canal de texto quanto forum. Excesso responde com um frame `error` (`"muitas mensagens, aguarde um instante"`) e descarta o frame, sem fechar a conexão. Configurável via `RATE_LIMIT_WS_RPM`/`RATE_LIMIT_WS_BURST`, padrão 60/10 — mais permissivo por minuto que o limite de REST, mas pensado para o ritmo de uma conversa (1 msg/s sustentado, rajada de 10), bem abaixo do que um cliente legítimo de chat de texto jamais precisaria.

**Razão:** a mesma implementação de token bucket à mão (sem `golang.org/x/time/rate`) já vinha da decisão equivalente em `server-central` — sem motivo para trocar de abordagem aqui, só de reaproveitar a chave certa para cada superfície. Responder com frame `error` em vez de fechar a conexão no limiter de WS evita que um cliente legítimo que só mandou rajada maior que o normal (ex. colar várias linhas coladas rápido) perca a conexão e precise reconectar; ele só precisa esperar o bucket recarregar.

**Revisitar quando:** os mesmos gatilhos já registrados na decisão de `server-central` (múltiplas réplicas de `server-channel` tornariam um bucket em memória por processo insuficiente); ou se um self-hoster relatar falso positivo em uso legítimo (ajustar via env, sem mudança de código).

## Decisão: kick/ban de membro — remoção lógica (`removed_at`), banimento por `oidc_subject`

**Contexto:** implementar o item de TODO "Kick/ban de membro" — até aqui não havia nenhuma forma de remover um membro problemático de um `server-channel`, só o sistema de bits de permissão sem bit dedicado a isso.

**Alternativas consideradas (remoção do membro):** apagar a linha de `members` (`DELETE`), o caminho óbvio à primeira vista; marcar como removido sem apagar (`removed_at`).

**Decisão:** não apagar. `members` ganhou a coluna `removed_at` (migration `0003_member_moderation`); kick define `removed_at = now()` e limpa `member_roles` do membro. `MemberStore.GetByOIDCSubject`/`List` passam a ignorar quem tem `removed_at` preenchido, então a perda de acesso é imediata (próxima requisição já nega, mesmo critério de corte "muda só na próxima conexão/requisição" já aceito em outros lugares deste sistema).

**Razão:** `messages.author_member_id`, `threads.author_member_id` e `invites.created_by_member_id` são `ON DELETE CASCADE` para `members(id)` desde a migration `0001_init` (nunca exercitado até agora — não havia nenhuma forma de apagar um membro). Um `DELETE` de verdade apagaria em cascata todo o histórico de mensagens/threads/convites da pessoa expulsa, destruindo a conversa pra quem ficou — foi decisão consciente **não** fazer isso, mesmo critério do Discord (kick/ban não apaga o que a pessoa já escreveu). A remoção lógica evita mexer nessas três FKs (e no formato de `Message`/`Thread`/`MessageView`, hoje `AuthorMemberID string` sem ponteiro) e resolve o requisito com uma mudança bem mais contida.

**Reentrada:** `GetOrCreateByOIDCSubject` (chamado por `POST /api/join` depois de resgatar um convite) ganhou `removed_at = NULL` no `ON CONFLICT DO UPDATE` — quem foi expulso e consegue um convite novo reaproveita a mesma linha/`id` de antes (nickname antigo se perde porque não é reafirmado, mas o `id` e portanto a autoria das mensagens antigas continuam batendo).

**Banimento:** tabela nova `member_bans`, chave primária `oidc_subject` (não `member_id` — precisa continuar bloqueando mesmo depois que a linha de `members` for reaproveitada por uma reentrada, ou não existir mais). `POST /api/members/{id}/ban` cria o registro e expulsa (chama o mesmo `Kick`); `POST /api/join` confere `member_bans` antes de qualquer outra coisa, inclusive antes do bootstrap. `GET /api/bans`/`DELETE /api/bans/{oidcSubject}` existem para a UI poder revisar/revogar — banir sem nenhuma forma de reverter pela UI (só via acesso direto ao Postgres) foi julgado uma lacuna ruim o suficiente pra não ficar de fora desta v1.

**Bits novos:** `KickMembers=64`, `BanMembers=128` — acrescentados **depois** de `Administrator` no bloco de `const` (não reordenados no meio), porque os bits existentes já valem como bitmask persistido em `roles.permissions` de instâncias reais; reordenar mudaria o significado de roles já criadas. Dois bits separados (não um só "ModerateMembers") pelo mesmo motivo que `ManageInvites` é separado de `ManageRoles` neste sistema: severidade diferente, alguém pode ter um sem o outro.

**Alvo inválido:** nenhuma checagem de hierarquia entre roles (ex. "só pode kickar quem tem role abaixo da sua") — este sistema nunca teve isso em lugar nenhum, nem para atribuição de role (`Grants` só olha bits concedidos, não posição). Só duas recusas fixas: o dono do servidor (`IsOwner`, nunca é alvo de kick/ban) e o próprio requisitante (sair do servidor não é uma feature desta v1, então usar a rota em si mesmo é rejeitado em vez de virar um "leave" disfarçado).

**Cliente:** `ManageRolesDialog.tsx` virou o painel de administração geral de membro (renomeado de "Gerenciar roles" para "Gerenciar membros"), aberto por quem tem `ManageRoles`, `KickMembers` ou `BanMembers` (antes só `ManageRoles` abria); cada seção (roles, atribuição de role, expulsar/banir, lista de banidos) é condicionada à permissão específica de quem está vendo. Botão de banir na v1 não coleta motivo pela UI (o campo existe na API, só não tem input ainda). `GET /api/bans` só é chamado quando `canBan` é verdadeiro — um 403 nessa chamada pra quem não é moderador derrubaria o `Promise.all` inteiro e quebraria a lista de membros pra todo mundo (`hooks/useServerMembers.ts`).

**Revisitar quando:** existir demanda por banimento com expiração automática (hoje é permanente até um unban manual); ou por hierarquia entre roles para kick/ban, se algum self-hoster relatar moderador expulsando outro moderador de igual ou maior nível.

## Decisão: upload de anexo em mensagem — disco local, mensagem via REST separada do WebSocket

**Contexto:** implementar o item de TODO "Upload de anexo/imagem em mensagem" — `messages.content` era só texto, sem campo, endpoint ou storage para arquivo. Escopo limitado a canal de texto (canal forum fica de fora, mesmo critério já usado na decisão de editar/apagar mensagem).

**Alternativas consideradas (onde guardar o arquivo):** objeto storage compatível com S3 (MinIO self-hosted, ou um bucket de nuvem); disco local do host, atrás de um volume Docker.

**Decisão:** disco local, via `internal/storage.FileStore` — um diretório configurável (`ATTACHMENTS_DIR`, padrão `/data/attachments`) montado como volume Docker (`attachments_data` na referência de self-host, `ffcom_channel_attachments` na implantação de teste), mesmo padrão já usado pelo Postgres.

**Razão:** MinIO (ou equivalente) adicionaria mais um serviço ao Docker Compose de cada self-hoster, na mesma categoria de custo que já pesou contra SFU próprio sobre Pion (ver decisão do LiveKit) e contra o SDK oficial do LiveKit (ver decisão de integração de voz) — mais uma dependência pesada para uma operação que, na escala de uma comunidade self-hosted, não precisa dela. Um bucket de nuvem trocaria a filosofia "self-hosted, sem dependência de terceiro" por uma conta externa obrigatória. Disco local é o que já vale para Postgres; o único custo é não escalar horizontalmente (`server-channel` continua sendo um processo único, mesmo limite já aceito em outras decisões deste sistema).

**Chave de armazenamento:** `storage_key` é gerada pelo servidor (16 bytes aleatórios, hex) — nunca o filename original enviado pelo client, para não abrir path traversal a partir de um nome de arquivo controlado pelo usuário. `filename`/`content_type` originais ficam só como metadado em `attachments` (migration `0004_message_attachments`), usados para exibição e `Content-Disposition` no download.

**Mensagem com anexo não vai pelo WebSocket:** o handshake de `GET /api/channels/{id}/ws` (e o frame `message.create` depois de aberto) não tem como carregar um `multipart/form-data`. Em vez de inventar um jeito de mandar bytes de arquivo por um frame JSON (base64 infla ~33% o payload e o limite de frame do WebSocket já é apertado, ver `maxMessageSize` em `internal/realtime`), mensagem com anexo ganhou uma rota REST própria: `POST /api/channels/{id}/messages` (multipart, campos `content` opcional e `file` opcional — pelo menos um dos dois obrigatório). O broadcast (`message.created`) usa o mesmo `realtime.Hub` do fluxo via WebSocket, então quem está com o canal aberto recebe a mensagem em tempo real por qualquer um dos dois caminhos, inclusive quem enviou.

**Tamanho máximo:** `ATTACHMENT_MAX_MB` (padrão 8 MB), aplicado via `http.MaxBytesReader` sobre o corpo da requisição antes de `ParseMultipartForm`. Self-hoster com proxy reverso na frente (produção sempre tem) precisa confirmar que o `client_max_body_size` (ou equivalente) do proxy acomoda esse valor — documentado no `.env.example` e no `.env` da implantação de teste, não automatizado (mesmo critério já usado para `REQUIRE_TLS`: a checagem existe no código, a config do proxy é responsabilidade de quem hospeda).

**Download exige Bearer token:** `GET /api/attachments/{id}` está atrás do mesmo `auth.RequireMember` + checagem de `ViewChannels` no canal da mensagem do resto da API — não existe rota pública para servir o arquivo direto. Consequência aceita: um `<img src="...">` ou clique direto num link não funciona (a API de fetch do navegador não deixa setar `Authorization` nesses casos), então o client sempre busca via `fetch()` com o Bearer token e monta uma `Blob`/object URL local (`fetchAttachmentBlob` em `client/src/lib/serverChannelApi.ts`, usado por `components/MessageAttachment.tsx`) — mesmo tipo de contorno que o WebSocket de canal já precisa (subprotocolo `access_token`) pela mesma limitação de navegador.

**Exclusão de mensagem limpa o arquivo:** `attachments.message_id` é `ON DELETE CASCADE`, então apagar uma mensagem já limpa a linha em Postgres sozinho — mas não o arquivo em disco. `handleIncomingMessageDelete` busca os anexos antes de apagar a mensagem e chama `FileStore.Delete` depois, best-effort (erro ao apagar o arquivo fica só logado, não desfaz a exclusão da mensagem nem bloqueia a resposta ao client). Editar mensagem (`message.update`) não mexe em anexo — é imutável nesta v1 — mas o broadcast da edição precisa reanexar o que já existia, ou o client perderia a exibição do anexo ao aplicar o `message.updated` (que substitui a mensagem inteira no estado local).

**Revisitar quando:** existir demanda por mais de um anexo por mensagem (schema já é 1:N, só a v1 que assume um só); por anexo em canal forum; ou por um self-hoster relatar que 8 MB é baixo demais para o uso real (ajustar via `ATTACHMENT_MAX_MB`, sem mudança de código).

## Decisão: upload de avatar de conta — disco local com chave fixa por conta, `display_name` provisório no primeiro upload

**Contexto:** implementar o item de TODO "Upload de avatar" (`server-central`) — `avatar_url` já existia no schema (`profiles`, migration `0001_init`) e era devolvido por `GET /api/me`/`GET /api/friends`, mas nunca era gravado: não havia endpoint de upload nem chamada a `ProfileStore.Upsert` em lugar nenhum do código. Campo morto desde a v1. Diferente do apelido por servidor (`PATCH /api/me` em `server-channel`, ver decisão de UUID truncado), avatar é um dado de conta, então mora em `server-central`.

**Alternativas consideradas (chave de armazenamento):** mesmo padrão de `internal/storage.FileStore` do upload de anexo em `server-channel` (chave aleatória por upload, rastreada numa coluna); ou chave determinística = `accountId`, sobrescrevendo o arquivo a cada novo upload.

**Decisão:** chave determinística. Um avatar é sempre 1:1 com a conta e o upload novo sempre substitui o anterior — nunca existe motivo pra manter os dois nem pra "esquecer qual é o atual", então rastrear uma chave aleatória à parte (e ter que apagar o arquivo anterior a cada troca) seria estado redundante. `internal/storage.AvatarStore` (novo pacote em `server-central`, mesma filosofia de `server-channel/internal/storage` mas API própria — `Save`/`Open`/`Delete` recebem `accountId` direto) grava/lê sob um diretório configurável (`AVATARS_DIR`, padrão `/data/avatars`), montado como volume Docker (`avatars_data` na referência de self-host, `ffcom_central_avatars` na implantação de teste) — mesmo padrão já usado por `ATTACHMENTS_DIR`. `Save` escreve num arquivo temporário (`os.CreateTemp` no mesmo diretório) e troca via `os.Rename` atômico, pra nunca deixar um avatar parcialmente escrito servível em caso de falha no meio do upload.

**Content-Type não é gravado à parte:** `GET /api/avatars/{id}` serve o arquivo via `http.ServeContent`, que sniffa o tipo a partir dos próprios bytes (`net/http.DetectContentType`) em vez de uma coluna/sidecar dedicado — só os tipos em `allowedAvatarContentTypes` (PNG/JPEG/WebP/GIF) chegam a ser salvos, então o sniff nunca precisa lidar com um tipo fora dessa lista.

**`display_name` provisório:** `profiles.display_name` é `NOT NULL`, mas não existe (ainda) endpoint para editá-lo — nenhuma linha de `profiles` chegava a ser criada antes desta feature. No primeiro upload de avatar de uma conta sem perfil, `POST /api/me/avatar` cria a linha com `display_name = oidcSubject` (o mesmo fallback que o client já aplica no lugar de `displayName` ausente, ver `client/src/hooks/useFriends.ts`), só que agora persistido. Uploads seguintes preservam o `display_name` já gravado (editado ou não). Não ativa o resto da feature de perfil (editar nome de exibição continua fora do escopo, ver `TODO.md`).

**Download exige Bearer token, sem checagem de amizade:** `GET /api/avatars/{id}` está atrás do `auth.Middleware` de qualquer outra rota deste servidor, mas sem nenhuma checagem extra de relação (amizade, servidor em comum) — um avatar não é dado sensível, e o `accountId` só chega a quem já tem algum vínculo com a conta (amigo, ou visualizando lista de membros de um `server-channel` em comum). Consequência aceita, mesmo padrão de `GET /api/attachments/{id}` em `server-channel`: um `<img src="...">` direto não funciona, o client busca via `fetch()`+Blob e monta uma object URL local (`components/UserAvatar.tsx`, `fetchAvatarBlob` em `client/src/lib/serverCentralApi.ts`).

**Tamanho máximo:** `AVATAR_MAX_MB` (padrão 2 MB, bem menor que os 8 MB de anexo — avatar não precisa do mesmo espaço), aplicado via `http.MaxBytesReader` antes de `ParseMultipartForm`, mesmo mecanismo do upload de anexo.

**Revisitar quando:** a feature de editar `display_name` for implementada de verdade (nesse ponto, o fallback pro `oidcSubject` aqui deixa de ser necessário); ou por demanda de redimensionar/normalizar a imagem no servidor (hoje o arquivo enviado é gravado como está, sem reencode).

## Decisão: indicador de não lida — `lastMessageAt` do servidor + cursor local no client, sem contagem nem WebSocket dedicado

**Contexto:** implementar o item de TODO (seção `client`) "Notificação/contador de não lidas" — hoje nada indica atividade num canal ou numa DM fechada, só abrindo cada um manualmente.

**Problema de base:** o `realtime.Hub` de `server-channel` é particionado por canal (ver "Decisão: canal de texto em server-channel") — um client só recebe broadcast de mensagens do canal que tem aberto agora, não dos demais canais do mesmo servidor. Não existe (e esta decisão não cria) um feed de atividade cruzando canais.

**Alternativas consideradas:**
- **WebSocket de atividade por servidor:** nova conexão/hub em `server-channel` que qualquer membro mantém aberta e recebe um ping mínimo (`{channelId, lastMessageAt}`) a cada mensagem nova em qualquer canal que ele possa ver — tempo real de verdade, mas introduz um segundo tipo de hub (por servidor, não por canal) e uma segunda conexão WS por client só para isso.
- **Contagem de não lidas com estado por membro no servidor** (tabela `read_cursors` em `server-channel`/`server-central`, um endpoint para marcar como lido) — replica o modelo do Discord (contagem exata, sincronizada entre dispositivos), mas exige schema novo, endpoint novo e mais uma escrita a cada canal aberto, em ambos os servidores.
- **`lastMessageAt` no servidor + cursor "última leitura" só local ao client (localStorage), sem contagem:** escolhida.

**Decisão:**
1. **Servidor expõe só o timestamp da mensagem mais recente, sem contar quantas:** `GET /api/channels` (`server-channel`) ganha `lastMessageAt` por canal (`MessageStore.LastMessageAtByChannel`, `MAX(created_at) GROUP BY channel_id` — cobre canal de texto e posts de thread de forum, que também têm `channel_id`). `GET /api/friends` (`server-central`) ganha `lastMessageAt` por amigo (`DirectMessageStore.LastMessageAtByPeer`, `MAX(created_at)` da conversa em qualquer sentido). Nenhum dos dois servidores sabe o que cada membro/conta já leu — só client sabe, ver próximo ponto.
2. **Cursor "última leitura" é local ao dispositivo, em `localStorage`** (`client/src/lib/unread.ts`, chave `ffcom:lastRead:<channel|dm>:<id>`) — mesmo padrão de persistência/risco já aceito para a sessão OIDC (`auth/userManager.ts`) e as chaves de E2E (`crypto/e2e.ts`). Não sincroniza entre dispositivos: um canal lido no desktop pode aparecer como não lido no navegador até ser aberto lá também. Aceito pela mesma razão do NaCl box por dispositivo — engenharia de sincronização entre dispositivos só se justifica com demanda real (ver "Revisitar quando" abaixo).
3. **`hooks/useUnread.ts` deriva o conjunto de ids não lidos** comparando `lastMessageAt` (do servidor) contra o cursor local, excluindo sempre o canal/DM selecionado no momento (nunca aparece como não lido enquanto está aberto). `App.tsx` marca como lido (grava cursor = agora) ao entrar e de novo ao sair de um canal/DM selecionado.
4. **Primeira vez que um id é visto (sem cursor local ainda) conta como já lido, não como atrasado:** evita acender a bolinha em tudo que já existia antes desta feature (rollout) ou num servidor/amigo recém-adicionado — o cursor é semeado com o `lastMessageAt` atual na primeira leitura em vez de tratado como "nunca lido".
5. **Atualização em canais fora do selecionado:**
   - **DMs:** o `dm.created` do WebSocket de presença já chega para qualquer amigo, aberto ou não (ver "Decisão: DMs entregues no mesmo WebSocket de presença") — `hooks/useFriends.ts` atualiza `lastMessageAt` do amigo correspondente em memória a cada evento, sem polling.
   - **Canais de `server-channel`:** sem um feed cruzando canais (ver "Problema de base"), `hooks/useServerStructure.ts` repassa a lista de categorias/canais a cada 20s (`STRUCTURE_POLL_INTERVAL_MS`) enquanto o servidor está selecionado, além da carga inicial — não é tempo real, mas é suficiente para uma bolinha de "tem mensagem nova".

**Escopo aceito, para não vender além do que entrega:**
- **Sem contagem** — só bolinha binária (tem/não tem mensagem nova), não "3 não lidas".
- **Sem indicador agregado no `ServerRail`** (ex. bolinha no ícone do servidor quando há canal não lido em qualquer lugar dele) — exigiria buscar a estrutura de **todos** os servidores conhecidos, não só o selecionado (`useServerStructure` hoje só busca o servidor ativo). Fora do escopo desta v1; a bolinha aparece no `ChannelSidebar` do servidor aberto e na lista de amigos.
- **Atraso de até ~20s para atividade em canal de texto/forum fora do selecionado** (intervalo de poll), e nenhuma atualização enquanto nenhum servidor está selecionado (tela de Amigos) — poll só roda com um `serverBaseUrl` ativo.

**Razão:** o servidor já grava tudo que esse recurso precisa (timestamp de mensagem) sem schema novo; o que falta é só "o que este dispositivo específico já viu", que é inerentemente um dado de cliente — guardá-lo no servidor implicaria sincronizar estado entre N dispositivos por conta, problema que o NaCl box por dispositivo já decidiu não resolver nesta v1 pelo mesmo motivo (ver "Decisão: criptografia ponta-a-ponta em DMs"). Reaproveita a infraestrutura de tempo real que já existe (WebSocket de presença já entrega todo `dm.created`) em vez de abrir uma conexão nova, e usa polling só onde de fato não existe alternativa (canais de `server-channel`), evitando o custo de projetar um hub por servidor para um indicador que não precisa ser instantâneo.

**Bug introduzido por esta decisão, corrigido em 2026-09-22:** o repoll de 20s de `useServerStructure` faz `categories` mudar de identidade a cada ciclo, e `App.tsx` tinha um efeito que resetava `selectedChannelId` para o primeiro canal sempre que `categories` mudava (escrito quando `categories` só mudava ao trocar de servidor). Resultado: a cada 20s a pessoa era jogada de volta para o primeiro canal, e num canal de voz isso desmontava `VoiceChannelView` e derrubava a chamada. Corrigido mantendo a seleção quando o canal ainda existe na lista nova. **Lição:** ao transformar uma carga única em poll, procurar efeitos que dependem da identidade do resultado.

**Revisitar quando:** houver demanda por contagem exata ou por sincronizar "lido" entre dispositivos da mesma conta (nesse ponto avaliar mover o cursor para o servidor, com um endpoint de "marcar como lido" — mesmo gatilho, em espírito, do "Revisitar quando" de multi-dispositivo da criptografia de DM); ou por um indicador agregado no `ServerRail` (nesse ponto `useServerStructure` precisaria buscar todos os servidores conhecidos, não só o ativo, ou `server-channel` precisaria de um endpoint mais barato que `GET /api/channels` completo só para isso).

## Decisão: câmera no canal de voz — `setCameraEnabled` do LiveKit, mesma grade da tela compartilhada, com preview local

**Contexto:** o canal de voz só publicava áudio e tela; a integração com LiveKit já tinha vídeo no nome e o token de `server-channel` já concede `canPublish` sem restringir fonte (`internal/livekit/token.go`), então faltava só o client.

**Alternativas consideradas:** grade separada para câmeras vs. a mesma grade da tela compartilhada; remover a tile ao desligar a câmera vs. escondê-la; com ou sem preview da própria câmera; engolir o erro de permissão (como no seletor de tela) vs. mostrá-lo.

**Decisão:**
1. `toggleCamera` (`client/src/hooks/useVoiceChannel.ts`) chama `room.localParticipant.setCameraEnabled(next)`, mesmo raciocínio de `setScreenShareEnabled` na decisão anterior. `cameraEnabled` é derivado de `participants` (`p.isCameraEnabled`), não um `useState` paralelo.
2. **Mesma grade para câmera e tela:** o `TrackSubscribed` agora cria tile para qualquer track de vídeo remota (`track.kind === Video`), com rótulo `nome` para câmera e `nome (tela)` para tela. Por isso o callback ref virou `videoContainerRef` e as classes CSS viraram `video-grid`/`video-tile`/`video-tile-label`. Grade separada foi descartada: duplicaria o padrão imperativo sem ganho numa v1 com poucos participantes.
3. **Desligar esconde a tile em vez de removê-la:** `setCameraEnabled(false)` não despublica a track, só a silencia (mute), e religar só tira o mute. Sem tratar isso, a tile ficaria congelada no último quadro. `RoomEvent.TrackMuted`/`TrackUnmuted` (disparam para a câmera local e para as remotas) põem/tiram a classe `muted` (`display: none`); uma track que já chega silenciada (alguém entra com a câmera desligada) nasce com a classe. A grade inteira some quando não sobra tile visível (`:has`).
4. **Com preview local da câmera**, ao contrário da tela: `LocalTrackPublished` com fonte `Camera` cria uma tile `video-tile local`, espelhada via CSS (`scaleX(-1)`, só no preview; os outros recebem o vídeo sem espelhar). Ver a si mesmo é parte esperada de ligar a câmera (enquadramento, luz), diferente da tela, em que a pessoa já está vendo o que compartilha.
5. **Erro de câmera é mostrado, não engolido:** permissão negada (`NotAllowedError`), nenhuma câmera (`NotFoundError`/`OverconstrainedError`) ou câmera em uso (`NotReadableError`) viram `cameraError`, exibido acima dos controles, sem mudar `status` (a voz continua conectada). Cancelar o seletor de tela é desistência; negar a câmera costuma ser engano ou configuração do sistema, e a pessoa precisa saber por que nada aconteceu.

**Razão:** mínimo de código novo reaproveitando o que o SDK e o padrão imperativo já resolvem; a única sutileza real (mute em vez de unpublish) é tratada explicitamente.

**Verificado nesta sessão (2026-09-22):** `npm run lint` sem avisos novos e `npm run build` limpos. Não testado num browser com câmera nem com dois participantes (TODO em aberto na seção de implantação de teste).

**Revisitar quando:** houver mais participantes com câmera do que a grade comporta bem (layout de palco/falante ativo, simulcast), ou se o produto quiser escolher o dispositivo de câmera/microfone (hoje usa o padrão do sistema).

## Decisão: backup/restore — `pg_dump`/`pg_restore` via `docker compose exec`, sem WAL archiving

**Contexto:** item de TODO em aberto ("Guia de backup/restore do Postgres") — `server-central` e `server-channel` são a única fonte de dados de suas respectivas comunidades (cada `server-channel` é isolado, sem banco compartilhado — ver "Decisão: dados e transporte"), mas nenhum guia de self-hosting cobria como fazer backup.

**Alternativas consideradas:** `pg_basebackup`/WAL archiving (PITR — point-in-time recovery), backup a nível de volume (snapshot do volume Docker/disco via LVM ou ferramenta do provedor de VPS), `pg_dump`/`pg_restore` lógico via `docker compose exec`.

**Decisão:** `pg_dump -Fc` / `pg_restore`, disparado de fora do container via `docker compose exec postgres pg_dump ...`, documentado em [`backup-restore.md`](backup-restore.md).

**Razão:** o público-alvo de self-hosting (`server-channel`, mesmo argumento já registrado nas decisões de linguagem e de config via `.env`) não vai operar WAL archiving/PITR — exige um repositório de WAL contínuo (S3 ou equivalente) e ferramentas extras (`pgbackrest`/`wal-g`) desproporcionais ao volume de dados de uma comunidade self-hosted. Snapshot a nível de volume depende do backend de storage de quem hospeda (nem todo VPS/host doméstico tem LVM ou snapshot de disco disponível), enquanto `pg_dump`/`pg_restore` funciona igual em qualquer host que já rode o `docker compose` do projeto, sem infraestrutura adicional — só precisa do próprio container `postgres` já no ar.

**Escopo aceito:** só backup lógico pontual (RPO = intervalo entre backups agendados via cron, ver o guia), sem recovery para um timestamp exato entre dois backups. Aceitável para o volume de dados de uma comunidade self-hosted; não é o mesmo padrão de disponibilidade de `server-central` (instância única oficial), que continua fora do escopo deste guia — backup da instância de teste/produção em `a3s-network` é responsabilidade de infra separada, ainda não automatizada.

**Revisitar quando:** `server-central` (instância única, sem isolamento por comunidade) justificar RPO menor que "uma vez por dia" — nesse ponto PITR via WAL archiving passa a valer o custo de operação adicional, especificamente para esse componente.

## Decisão: gerenciar categorias e canais — bit `ManageChannels` no nível do servidor, apagar canal leva o conteúdo junto

**Contexto:** até aqui não existia endpoint nem tela para criar categoria ou canal (`CategoryStore.Create`/`ChannelStore.Create` existiam sem nenhum chamador). Um `server-channel` recém-instalado ficava sem nenhum canal, e na instância oficial a estrutura "Geral / geral / Voz / forum" foi inserida à mão via `psql`. Isso quebrava a proposta central do projeto (qualquer comunidade sobe o próprio servidor e usa).

**Alternativas consideradas:**
- **Reusar `Administrator`:** sem bit novo, só admin mexe na estrutura. Descartada porque obriga a dar tudo a quem só precisa organizar canais, o mesmo motivo que já separou `ManageInvites`, `ManageRoles`, `KickMembers` e `BanMembers`.
- **Bit novo que também valesse em overwrite de canal** (como o `MANAGE_CHANNELS` do Discord, que pode ser concedido num canal só): mais flexível, mas criar canal e mexer em categoria não pertencem a nenhum canal específico, e o caso "pode editar só este canal" não apareceu. Adiado.
- **Bit novo `ManageChannels` checado só na permissão base:** escolhida.
- **Apagar categoria:** recusar enquanto houver canal dentro (`409`) vs. apagar os canais em cascata vs. deixar os canais sem categoria. Escolhida a última, que já é o que o schema faz (`channels.category_id ... ON DELETE SET NULL`, migration `0001_init`) e é o comportamento do Discord; nenhuma mensagem se perde por apagar uma categoria.
- **Apagar canal:** apagar só logicamente (`deleted_at`, recuperável) vs. apagar de verdade. Escolhido apagar de verdade, com confirmação em dois cliques na UI avisando que não dá para desfazer; as FKs já estavam em `ON DELETE CASCADE` para mensagens, threads, anexos e overwrites.

**Decisão:**
1. **Bit `ManageChannels = 256`** em `internal/permissions`, no fim do bloco (mesmo cuidado de não reordenar bits persistidos, ver "Decisão: kick/ban de membro"). Checado só na permissão base (`requireManageChannels`, `internal/httpapi/channels_admin.go`); dono e `Administrator` passam sempre. Conceder o bit a uma role passa pela regra de `permissions.Grants` já existente, então `ManageRoles` sozinho não consegue se dar `ManageChannels`.
2. **Rotas:** `POST/PATCH/DELETE /api/categories[/{id}]` e `POST/PATCH/DELETE /api/channels[/{id}]` (ver `docs/protocol.md`). Nome com espaço nas pontas cortado, obrigatório, até 100 caracteres. Sem `position` na criação, o item vai para o fim (categoria: fim da lista; canal: fim da própria categoria, ou dos sem categoria). O `PATCH` de canal lê o corpo como mapa para distinguir `categoryId` ausente (mantém) de `null` (tira da categoria). O tipo do canal não muda depois de criado: histórico de texto não faz sentido num canal de voz.
3. **Apagar canal apaga os arquivos de anexo do disco:** as linhas vão por CASCADE, mas os arquivos em `ATTACHMENTS_DIR` não. O handler busca as `storage_key` do canal antes (`AttachmentStore.StorageKeysForChannel`) e apaga os arquivos depois do `DELETE`, igual a apagar uma mensagem (ver "Decisão: upload de anexo em mensagem"). Falha ao apagar um arquivo só é logada: o canal já sumiu e o arquivo órfão não é servido por nenhuma rota.
4. **`GET /api/categories` mostra categoria vazia para quem tem `ManageChannels`:** a regra "categoria sem canal visível não aparece" (para não vazar nome de categoria privada) deixaria uma categoria recém-criada invisível justamente para quem precisa pôr o primeiro canal nela. Quem não gerencia continua vendo só categorias com canal visível.
5. **Client:** botão "+ Categoria" no topo da `ChannelSidebar`; ao passar o mouse, "+" (novo canal) e "✎" (editar) no cabeçalho de cada categoria, e "✎" em cada canal. Diálogos em `components/StructureDialogs.tsx` (renomear, mover de categoria, apagar com confirmação em dois cliques, sem `confirm()` do navegador). Tudo só aparece com `ManageChannels` ou dono. Servidor sem nenhum canal mostra uma dica para quem pode criar. Depois de cada mudança, `useServerStructure` recarrega na hora (`refresh()`), sem esperar o poll de 20s; canal recém-criado já abre selecionado.

**Escopo aceito:**
- **Sem reordenar pela UI** (arrastar e soltar): a ordem muda só pelo `position` do `PATCH`, que a UI ainda não expõe.
- **Sem aviso em tempo real a quem está no canal apagado:** o `realtime.Hub` não é avisado; quem está com o canal aberto continua conectado até o próximo poll de estrutura (até 20s) tirar o canal da lista e o client trocar de canal. Numa sala de voz, a chamada no LiveKit continua até a pessoa sair, porque o token já emitido não é revogado.
- **Outros membros veem a mudança em até 20s** (mesmo poll do indicador de não lida), não na hora.

**Razão:** fecha a lacuna que impedia um self-host novo de funcionar sem `psql`, reaproveitando o que já existia (stores, FKs em cascata, `Grants`, poll de estrutura) em vez de criar mecanismo novo. Manter o bit só no nível do servidor evita decidir agora a semântica de overwrite para uma permissão que, na prática, é sobre a estrutura do servidor inteiro.

**Revisitar quando:** alguém precisar delegar a edição de um canal só (nesse ponto, deixar `ManageChannels` valer também em overwrite, checando o canal no `PATCH`/`DELETE`); ou a demora de até 20s para outros membros verem canal novo/apagado incomodar (nesse ponto, um feed de estrutura por servidor, o mesmo que o indicador de não lida adiou).

## Decisão: chave de E2E e cursores de não lida por conta, logout sem apagar nada

**Contexto:** o botão de sair não existia (`signOut` em `client/src/auth/AuthProvider.tsx` não era chamado por ninguém), e o `localStorage` do client guardava estado de conta como se fosse do navegador: o par de chaves de E2E (`ffcom.e2e.keypair.v1`), a flag "chave já publicada" (`ffcom.e2e.publishedKey.v1`) e os cursores de não lida (`ffcom:lastRead:*`). Quem logasse depois de outra pessoa no mesmo navegador herdava a chave privada dela e, como a flag já batia, nunca publicava a própria chave em `server-central`; DM para essa conta cifrava para a chave errada. Isso bloqueava os testes com duas contas (`teste-ffcom01`/`teste-ffcom02`).

**Alternativas consideradas:**
- **Limpar tudo no logout:** mais simples, mas a pessoa perde o histórico de DM deste dispositivo a cada logout (a chave privada só existe aqui, ver "Decisão: criptografia ponta-a-ponta em DMs"). Também não resolve o caso de fechar a aba sem sair e outra pessoa logar depois.
- **Chavear pelo `sub` do OIDC, logout sem apagar nada:** escolhida.

**Decisão:**
1. Par de chaves em `ffcom.e2e.keypair.v2:<sub>`, flag em `ffcom.e2e.publishedKey.v2:<sub>`, cursores em `ffcom:lastRead:v2:<sub>:<tipo>:<id>` (`client/src/crypto/e2e.ts`, `hooks/useE2EKeys.ts`, `lib/unread.ts`). O `sub` vem de `user.profile.sub` do `oidc-client-ts`, o mesmo valor que `server-central` guarda em `accounts.oidc_subject`.
2. **Migração do par antigo:** ele só é adotado por uma conta se a chave pública dele for igual à que a conta já publicou. Para isso `GET /api/me` de `server-central` passou a devolver `e2ePublicKey`, **sempre presente** (`null` sem chave), para que "campo ausente" signifique "servidor anterior a esta mudança". Nesse caso o client não decide nada e tenta de novo depois, em vez de gerar um par novo e deixar o antigo ilegível para o dono. Se não bater, o par antigo fica onde está (a conta dona pode adotá-lo depois) e a conta atual ganha um par novo.
3. Cursores de não lida antigos não são migrados: sem dono conhecido, e um cursor ausente já é tratado como "tudo lido" (`hooks/useUnread.ts`). As chaves `ffcom:lastRead:*` antigas ficam órfãs no `localStorage`, sem efeito.
4. Enquanto o par não está resolvido (só acontece durante a migração, que espera `GET /api/me`), a DM mostra "Preparando a chave de criptografia…" em vez de montar `DirectMessageView` sem chave.
5. Botão de sair no rodapé do `ServerRail`, logo abaixo do avatar; chama `userManager.signoutRedirect()` (encerra também a sessão no Authentik, senão o próximo login entraria direto na mesma conta).

**Ordem de deploy:** `central-v*` antes de `client-v*`. Na ordem inversa nada se perde (o client só adia a migração), mas quem tiver o par antigo fica sem DM até o `server-central` novo subir.

**Limitação, para não vender além do que entrega:** isso separa contas, não protege uma da outra. Qualquer código rodando na origem do app lê o `localStorage` inteiro, inclusive o par de outra conta que usou o mesmo navegador. É o mesmo nível de risco já aceito para a sessão OIDC.

**Verificado nesta sessão (2026-09-22):** 6 cenários da migração rodados em Node contra o hook real (React e `fetch` simulados): dono adota o par antigo sem republicar; segunda conta gera e publica o próprio; conta volta com o mesmo par; conta alheia não herda e preserva o par antigo; servidor sem o campo adia; navegador limpo cria e publica. `npm run lint` sem avisos novos, `npm run build`, `go build`/`go test` de `server-central`. Não testado num browser real com Authentik (logout e troca de conta ficam para o teste de DM com duas contas).

**Revisitar quando:** houver "vincular dispositivo" / multi-dispositivo (ver o "Revisitar quando" da decisão de E2E), porque aí o par deixa de ser só local.

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
