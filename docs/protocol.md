# Protocolo entre client, server-central e server-channel

Referência técnica dos endpoints REST e frames de WebSocket expostos por `server-central` e `server-channel`, e de como o `client` os consome. As decisões de design por trás de cada mecanismo (por quê REST vs. WebSocket, por quê DMs entram na conexão de presença, por quê convite é obrigatório, etc.) estão em [`docs/architecture.md`](architecture.md) — este documento não as repete, só referencia. Gerado a partir da leitura do código em 2026-09-21, atualizado em 2026-09-22 (criptografia ponta-a-ponta em DMs); se o comportamento e este doc divergirem no futuro, o código manda.

## Visão geral

Três componentes: `client` (SPA React, web/PWA e Electron), `server-central` (instância única oficial: contas, amigos, presença, DMs, diretório de servidores conhecidos) e `server-channel` (uma instância por comunidade self-hosted: categorias, canais, mensagens, permissões, convites, voz).

**Não existe comunicação direta `server-central` ↔ `server-channel`** — confirmado lendo `server.go` dos dois componentes: nenhum dos dois faz requisição HTTP para o outro. O `client` é o único que fala com ambos, cada um numa base URL própria (`SERVER_CENTRAL_URL`, fixa por build, e `KnownServer.baseUrl`, uma por servidor adicionado ao diretório — ver `docs/architecture.md`, "diretório de servidores conhecidos: `address` é a base URL completa"). Ver `docs/architecture.md`, "Decisão: protocolo entre client, server-central e server-channel".

## Autenticação

Os dois servidores validam o mesmo Bearer JWT emitido pela instância central de Authentik do `abs-3d-printer` (`OIDC_ISSUER_URL` idêntica nos dois, podendo listar mais de um issuer separado por vírgula porque o mesmo provider responde por `auth.ffcom` e `authentik.abs`; `coreos/go-oidc/v3`, `SkipClientIDCheck: true`) — não há chamada de rede entre eles para isso; cada um valida a assinatura/issuer/expiração de forma independente. Ver `docs/architecture.md`, "Decisão: autenticação em server-central" e "Decisão: modelo de dados de server-channel".

- **`server-central`:** qualquer token válido já basta — a conta local é criada implicitamente (upsert por `sub`) na primeira requisição autenticada (`auth.Middleware`, `AccountStore.GetOrCreateBySubject`).
- **`server-channel`:** validar o token só dá o "sub" (`auth.VerifyToken`). A maioria das rotas exige também que esse `sub` já seja **membro** daquela instância (`auth.RequireMember`, 403 se não for) — associação feita via `POST /api/join`, que exige convite exceto para o primeiro membro (fundador do self-host). Ver "Decisão: convites obrigatórios para entrar em server-channel".

**Header:** `Authorization: Bearer <token>` em toda chamada REST.

**WebSocket:** a API `WebSocket` do navegador não permite setar `Authorization` no handshake. O token vai como segundo elemento do subprotocolo: `new WebSocket(url, ["access_token", token])`, que vira o header `Sec-WebSocket-Protocol: access_token, <token>` — mesmo mecanismo nos dois servidores (`wsAuthSubprotocol = "access_token"`). O `Upgrader` de cada rota WS declara `Subprotocols: []string{"access_token"}` para aceitar/ecoar.

## Convenções gerais

- **Erros HTTP:** JSON `{"code", "message", "params"?}` nos dois servidores (ver "Erros da API HTTP" abaixo). Status codes usados: `400` (corpo/parâmetro inválido), `401` (token ausente/inválido), `403` (sem permissão), `404` (recurso não encontrado), `409` (conflito — ex. convite já usado), `410 Gone` (convite expirado), `426 Upgrade Required` (TLS obrigatório, só quando `REQUIRE_TLS=true`, ver abaixo), `429` (rate limit, ver abaixo).
- **Respostas de sucesso:** JSON, `Content-Type: application/json`, `camelCase` em todos os campos.
- **Paginação de histórico:** keyset pagination por `created_at`, sempre os mesmos dois query params — `before` (RFC3339, opcional) e `limit` (opcional, default 50, máximo 200). O servidor devolve mais recentes primeiro; o `client` inverte para ordem cronológica antes de renderizar (`fetchChannelHistory`, `fetchThreadMessages`, `fetchDirectMessages` em `client/src/lib/`).
- **Frames de WebSocket:** envelope JSON uniforme `{"type": "...", ...payload}` nos dois servidores. O client → servidor emite um tipo (`*.create`); o servidor → client responde com o `*.created` correspondente (broadcast, **incluindo o próprio autor**, para confirmar id/timestamp atribuídos pelo servidor) ou `"error"` (`{"type": "error", "code": "...", "message": "...", "params"?: {...}, "error": "..."}`, só para quem causou o erro; ver "Frame `error` do WebSocket" abaixo).
- **CORS:** variável de ambiente `CORS_ALLOWED_ORIGINS` (lista separada por vírgula, vazia por padrão) em ambos os servidores, mesmo mecanismo (`internal/httpapi/cors.go`, idêntico nos dois). Preflight `OPTIONS` respondido com `204`; origem liberada ecoa em `Access-Control-Allow-Origin` + `Vary: Origin`; `Access-Control-Allow-Methods: GET, POST, PATCH, PUT, DELETE, OPTIONS` (lista fixa e abrangente — **atualizar aqui ao adicionar uma rota com método novo**, é a causa mais comum de erro de CORS neste projeto apesar da origem estar liberada, ver `docs/architecture.md`). O `CheckOrigin` do upgrader de WebSocket usa a mesma allowlist, com fallback ao comportamento padrão do gorilla (sem header `Origin`, ou `Origin == Host`, sempre passa).
- **`GET /healthz`:** sem autenticação, nos dois servidores. `{"status":"ok","version":"..."}`.
- **TLS obrigatório (`REQUIRE_TLS`):** desligado por padrão nos dois servidores. Quando `true`, toda requisição (exceto `/healthz`) precisa chegar com o header `X-Forwarded-Proto: https` (setado pelo proxy reverso na frente — nenhum dos dois binários termina TLS) ou é rejeitada com `426`. Ver `docs/architecture.md`, "Criptografia em trânsito obrigatória".

## Erros da API HTTP

Formato das respostas de erro do `server-central` e do `server-channel` (o mesmo vale para o frame `error` do WebSocket, abaixo). Decisão em `docs/architecture.md`, "Decisão: internacionalização".

**Formato.** Todo erro HTTP responde `Content-Type: application/json; charset=utf-8` com:

```json
{"code": "friends.already_friends", "message": "vocês já são amigos"}
{"code": "ideas.text_length", "message": "o texto precisa ter entre 10 e 1000 caracteres", "params": {"min": 10, "max": 1000}}
```

- `code`: identificador estável do motivo. É contrato: renomear um código é mudança de protocolo (client antigo deixa de traduzir).
- `message`: o texto em português, só como reserva (código sem tradução, client antigo) e para logs. O client não compara esse texto.
- `params` (opcional): valores interpolados na tradução como `{{nome}}` (formato do i18next), ex. `"errors.ideas.text_length": "o texto precisa ter entre {{min}} e {{max}} caracteres"`. Pode levar também um dado útil ao client, como `params.id` em `ideas.wand_busy`.
- O status HTTP continua dizendo a classe do erro (`400`, `401`, `404`, `409`...), igual a antes; o `code` só detalha.

**Como o client usa.** Em resposta fora de 2xx, lê o JSON e mostra `t('errors.' + code, params)` a partir de `locales/<idioma>.json`. Se a chave não existir no idioma, mostra `message`. Se o corpo não for JSON (proxy na frente, `server-channel` ainda sem o formato, rota inexistente, que o `ServeMux` responde com `404 page not found` em texto), mostra o texto cru ou um erro genérico pelo status.

**Convenção de códigos.** `area.motivo`, os dois em `snake_case` minúsculo. Áreas em uso no `server-central`: `common`, `auth`, `accounts`, `profile`, `avatar`, `friends`, `dm`, `e2e`, `presence`, `servers`, `ideas`, `signup`; no `server-channel`: `common`, `auth`, `profile`, `permissions`, `channels`, `categories`, `messages`, `attachments`, `forum`, `invites`, `members`, `roles`, `overwrites`, `voice`; nos dois, `realtime` (frames de WebSocket mal formados). `common.*` vale para qualquer rota dos dois servidores (`common.invalid_body`, `common.account_missing`, `common.member_missing`, `common.rate_limited`, `common.tls_required`, `common.limit_invalid`, `common.before_invalid`, `common.name_required`...). Falta de permissão no `server-channel` é `permissions.required` (`params.permission`, o nome do bit, ex. `ManageRoles`) ou `permissions.required_either` (`params.permission` e `params.alternative`, ex. `ManageChannels` ou `CreateChannels`); o nome do bit não se traduz. Falha interna (`500`) é `<area>.<o_que>_failed` (ex. `servers.fetch_failed`). A mesma mensagem usa o mesmo código em todas as rotas. Cada código tem a chave `errors.<code>` em `locales/pt-BR.json` (com o texto da `message`, trocando os números por `{{params}}`) e em todos os outros idiomas. A chave é aninhada: `errors.friends.already_friends` é `{"errors": {"friends": {"already_friends": "..."}}}`.

**No Go.** Pacote `internal/apierr`, igual nos dois servidores (cada módulo Go tem a sua cópia, com o teste):

```go
apierr.Write(w, http.StatusConflict, "friends.already_friends", "vocês já são amigos")
apierr.WriteParams(w, http.StatusBadRequest, "ideas.text_length", "o texto precisa ter entre 10 e 1000 caracteres",
	apierr.Params{"min": ideaMinChars, "max": ideaMaxChars})

// Validação que só diz o motivo; o handler escolhe o status.
p := apierr.New("signup.email_invalid", "e-mail inválido") // ou NewParams(code, message, params)
apierr.WriteProblem(w, http.StatusBadRequest, p)
```

O código é sempre uma string literal: terceiro argumento de `Write`/`WriteParams`, primeiro de `New`/`NewParams`. Assim `scripts/check-locales.mjs` (no CI) e o teste `internal/apierr/apierr_test.go` (que lê `../../../locales/pt-BR.json`) acham todos os códigos por regex e falham se algum não tiver `errors.<code>`, ou se alguma chamada tiver o código numa variável. Código novo: criar a chave em `pt-BR.json` e `en.json` no mesmo commit.

### Frame `error` do WebSocket

Nos dois servidores, um frame que o servidor recusa (tipo desconhecido, payload inválido, conteúdo vazio ou longo demais, sem permissão, excesso de frames, falha interna) responde só a quem o mandou, com o mesmo `code`/`message`/`params` dos erros HTTP e as mesmas chaves `errors.<code>`:

```json
{"type": "error", "code": "messages.content_too_long", "message": "conteúdo excede o limite de 4000 caracteres", "params": {"max": 4000}, "error": "conteúdo excede o limite de 4000 caracteres"}
```

- `error` repete `message`. Fica só para os clients anteriores ao `code` (até a `client-v0.21.x`, que mostram `frame.error`); client novo lê `code` e cai em `message`. Pode sair quando nenhum client em uso depender dele.
- Frame mal formado usa a área `realtime`: `realtime.frame_invalid` (não é JSON), `realtime.frame_type_unknown` e `realtime.payload_invalid` (os dois com `params.type`, o tipo do frame), `realtime.rate_limited` (excesso de frames no canal). A `message` desses pode trazer o detalhe do decoder Go, só útil em log; a tradução não usa.
- No Go, `Client.SendError` recebe um `*apierr.Problem` (`client.SendError(apierr.New("messages.not_found", "mensagem não encontrada"))`), então o código também é literal e entra na mesma checagem. Os decoders de `internal/realtime` devolvem `*apierr.Problem` em vez de `error`.
- Nenhum outro frame dos dois servidores leva texto para o usuário: `presence.update`, `friend.*`, `dm.created`, `message.*`, `thread.created` e `post.created` só carregam dados.

## `server-central`

Base URL: `VITE_SERVER_CENTRAL_URL` no client (`http://localhost:8081` em dev).

### REST

| Método | Rota | Auth | Request | Response | Erros |
|---|---|---|---|---|---|
| GET | `/healthz` | não | — | `{status, version}` | — |
| GET | `/api/me` | Bearer | — | `{accountId, oidcSubject, createdAt, e2ePublicKey, hasE2EKeyBackup, status, language, displayName?, customDisplayName?, avatarUrl?}` — `e2ePublicKey` é base64 ou `null` (nunca omitido); `language` é o idioma da interface escolhido na conta (`pt-BR` ou `en`) ou `null` se nunca escolheu (nunca omitido); `displayName` é o nome escolhido no FFCom ou, sem ele, o do Authentik, e `customDisplayName` só vem quando há um escolhido | — |
| PATCH | `/api/me` | Bearer | `{language?}`: só os campos presentes mudam; `language` é `pt-BR`, `en` ou `null` (apaga a escolha); fica só no FFCom (o Authentik não é alterado) | mesmo formato de `GET /api/me` | `400` `profile.language_unsupported` (`params.supported`) ou `common.invalid_body` |
| PUT | `/api/me/display-name` | Bearer | `{displayName}` — até 64 bytes depois do trim, sem caractere de controle; vazio ou `null` volta ao nome do Authentik | mesmo formato de `GET /api/me` | `400` nome longo, com caractere de controle ou corpo inválido |
| PUT | `/api/me/e2e-public-key` | Bearer | `{publicKey}` (base64, 32 bytes) | `204` | `400` tamanho inválido; `409` se a conta já tem backup da chave (rota de clients antigos) |
| GET | `/api/me/e2e-key-backup` | Bearer | — | `{publicKey, backup}` (base64) — backup cifrado com a frase de recuperação, opaco para o servidor | `404` sem backup |
| PUT | `/api/me/e2e-key-backup` | Bearer | `{publicKey, backup, replace}` — `publicKey` 32 bytes, `backup` até 1024 bytes (base64) | `204` | `400` tamanho inválido; `409` se já houver backup e `replace` for falso |
| GET | `/api/servers` | Bearer | — | `{servers: [{id, address, name, iconUrl?, position, addedAt}]}`, na ordem do rail (`position`) | — |
| POST | `/api/servers` | Bearer | `{address, name, iconUrl?}` | `201` + `KnownServer`; servidor novo entra no topo do rail, readicionar mantém a posição | `400` address/name vazios |
| PUT | `/api/servers/order` | Bearer | `{ids: [string]}` — todos os servidores da conta, na ordem nova | `204` | `409` se `ids` não for exatamente a lista da conta |
| DELETE | `/api/servers/{id}` | Bearer | — | `204` | `404` |
| GET | `/api/presence` | Bearer | — | `{friends: [{accountId, online}]}` — snapshot dos amigos aceitos | — |
| GET | `/api/presence/ws` | Bearer (subprotocolo) | upgrade WS | ver abaixo | — |
| GET | `/api/friends` | Bearer | — | `{friends: [{accountId, displayName?, avatarUrl?, e2ePublicKey?, lastMessageAt?}]}` — `lastMessageAt` é a DM mais recente trocada com esse amigo em qualquer sentido, mesmo uso do `lastMessageAt` de canal em `server-channel` (ver `docs/architecture.md`, "Decisão: indicador de não lida") | — |
| DELETE | `/api/friends/{accountId}` | Bearer | — | `204` — desfaz a amizade aceita (qualquer um dos lados); a linha é apagada, então os dois podem pedir de novo, e as DMs ficam no banco mas inacessíveis até lá | `404` não são amigos |
| POST | `/api/friends/invites` | Bearer | — | `201` `{code, createdAt}` | — |
| POST | `/api/friends/invites/{code}/redeem` | Bearer | — | `201` `{friendAccountId}`; um pedido pendente entre os dois vira amizade aceita | `400` convite próprio, `404`, `409` já usado ou já amigos, `410` expirado |
| GET | `/api/friends/requests` | Bearer | — | `{incoming: [FriendRequest], outgoing: [FriendRequest]}` — pedidos pendentes, recebidos e enviados | — |
| POST | `/api/friends/requests` | Bearer | `{accountId}` | `201` `{status: "pending", request: FriendRequest}`; `200` `{status: "accepted", ...}` quando o outro lado já tinha pedido | `400` a si mesmo, `404` conta inexistente, `409` já amigos ou pedido já enviado |
| POST | `/api/friends/requests/{id}/accept` | Bearer | — | `200` `{status: "accepted", request: FriendRequest}` | `404` (inexistente, já respondido, ou quem chama não é o destinatário) |
| DELETE | `/api/friends/requests/{id}` | Bearer | — | `204` — recusa (destinatário) ou cancela (remetente) | `404` |
| GET | `/api/dms/{accountId}/messages?before=&limit=` | Bearer | — | `{messages: [DirectMessage]}` | `403` se não são amigos |
| GET | `/api/ideas?view=ranking\|implemented\|review` | opcional | — | `[Idea]` — `ranking` (padrão): publicadas e planejadas, maior pontuação primeiro, empate para a mais antiga; `implemented`: implementadas, mais recentes primeiro; `review`: fila de moderação | `403` `review` sem o grupo de admin |
| GET | `/api/ideas/me` | Bearer | — | `{assistEnabled, wandLimit, wandLeft, wandUsed, wandPenalty, pendingAssist?, suggestedToday, todayIdea?: Idea, lastDiscarded?: Idea, discardsLeft, isAdmin}` — `lastDiscarded` é a última ideia de hoje descartada por não ser sugestão (com a dica em `feedback`), só enquanto não houver ideia do dia | — |
| POST | `/api/ideas` | Bearer | `{text}` (10 a 1000 caracteres) | `202` + `Idea` em `checking`; vira `open`, `review` ou `discarded` (não é sugestão; não gasta o dia) quando a checagem final termina | `400` tamanho; `409` já enviou hoje; `429` já teve 3 textos descartados hoje |
| POST | `/api/ideas/assist` | Bearer | `{text}` (10 a 1000 caracteres) | `202` `{id, status: "pending", wandLeft}` — gasta um uso da varinha | `400` tamanho; `409` `ideas.wand_busy` já há um pedido em andamento (o id dele vem em `params.id`); `429` sem usos hoje; `502` relay fora do ar (uso devolvido); `503` varinha desligada |
| GET | `/api/ideas/assist/{id}` | Bearer | — | `{id, status: "pending"\|"done"\|"failed", wandLeft, result?: {offensive, notSuggestion, hint?, title, text, similar: [Idea]}}` — `notSuggestion` com `hint`: o texto não propõe nada a mudar, e o site mostra a dica sem trocar o texto | `404` inexistente ou de outra conta |
| PUT | `/api/ideas/{id}/vote` | Bearer | `{value: 1\|-1\|0}` (like, dislike, tirar o voto) | `Idea` atualizada | `400`; `403` na própria ideia; `404`; `409` ideia fora de votação |
| DELETE | `/api/ideas/{id}` | Bearer + grupo admin | — | `204` — apaga a ideia e os votos; se a ideia for de hoje, o autor pode enviar outra (diferente de `rejected`) | `403` sem o grupo; `404` |
| PATCH | `/api/ideas/{id}` | Bearer + grupo admin | `{status: "open"\|"planned"\|"implemented"\|"rejected"\|"review", implementedVersion?}` — versão no formato do CHANGELOG (`client v0.15.0`), obrigatória para `implemented` | `Idea` atualizada | `400`; `403` sem o grupo; `404` |

`FriendRequest`: `{id, accountId, displayName?, avatarUrl?, createdAt}` — `accountId`/`displayName`/`avatarUrl` são sempre do **outro** lado do pedido, do ponto de vista de quem recebe a resposta ou o frame.

`DirectMessage`: `{id, senderId, recipientId, ciphertext, nonce, createdAt, editedAt?}` — `ciphertext`/`nonce` são base64, opacos ao servidor (criptografia ponta-a-ponta, ver `docs/architecture.md`, "Decisão: criptografia ponta-a-ponta em DMs"). O client decifra localmente com a chave pública atual do outro lado da conversa (`GET /api/friends`) + a chave privada do dispositivo.

`Idea`: `{id, title, body, author, status, score, likes, dislikes, myVote, mine, implementedVersion?, reviewReason?, createdAt}` — `author` é só o primeiro nome do perfil; `score` = likes × 2 − dislikes; `myVote` (1, −1 ou 0) e `mine` dependem do token (0 e falso para visitante); `reviewReason` (`conteudo_ofensivo` ou `verificacao_indisponivel`) só aparece para o admin e para quem escreveu; `feedback` (a dica do que faltou, em ideia descartada) só para quem escreveu. "Auth opcional" em `GET /api/ideas`: sem token a lista é pública; com token vêm o voto e a marca de quem pede. O grupo de admin vem da claim `groups` (scope `ffcom-groups`, pedido só pela home page) e é `ffcom-admins` por padrão (`IDEAS_ADMIN_GROUP`).

| POST | `/api/signup-requests` | — (público) | `{fullName, username, email, nickname, reason, existingAccount, website, language?}` — `website` é isca para robô e deve ir vazio; `language` (`pt-BR` ou `en`, outro valor é ignorado) é o idioma do e-mail de definir senha mandado na aprovação e, em conta criada por ela, o `settings.locale` no Authentik; `username` 3 a 30 de `[a-z0-9._-]` começando por letra ou número, fora da lista de reservados (`admin`, `root`, `ffcom`, `suporte`...; ponto, hífen e sublinhado não contam na comparação); `nickname` 2 a 32; `reason` 10 a 500. Com `existingAccount: true` (a pessoa já tem conta no Authentik), `username` e `nickname` são ignorados | `201` `{status: "pending"}` — o pedido vai ao Telegram para aprovação, com a mesma resposta havendo ou não conta com aquele e-mail | `400` campo inválido (texto diz qual); `409` pedido em aberto ou conta já aprovada com o mesmo e-mail ou usuário, ou usuário já existe no Authentik (sem diferenciar maiúsculas); `429` 3 pedidos do mesmo IP em 24 h ou 20 no total na última hora; `503` cadastro desligado |
| GET | `/api/signup-requests/username-available?username=` | — (público) | — | `200` `{available, message, code?, params?}` — quando `available` é `false`, `code`, `message` e `params` dizem por que não (formato, reservado, pedido em aberto ou aprovado, já existe no Authentik), no mesmo formato dos erros (ver "Erros da API HTTP"). Só conforto para o formulário: o `POST` confere tudo de novo | `429` mais de 30 consultas por minuto do mesmo IP (burst 10); `503` cadastro desligado ou Authentik sem resposta |

Decisão de um pedido de cadastro, no listener interno (porta `CLAUDE_CALLBACK_PORT`, fora do proxy público), chamada pelo `a3s-network-monitor` quando alguém clica nos botões da mensagem no Telegram (`callback_data` `ffcom-signup:approve:<id>` ou `ffcom-signup:reject:<id>`): `POST /internal/signup-requests/{id}/decision`, header `X-FFCom-Secret: <SIGNUP_DECISION_SECRET>`, corpo `{decision: "approve"|"reject", by}` (`by` é quem clicou, até 64 caracteres). Responde `200` `{ok, message}` sempre que o pedido foi tratado, inclusive aprovação que falhou (`ok: false`); `message` cabe no `answerCallbackQuery`. `401` segredo errado, `400` corpo inválido, `404` id inexistente. Quem edita a mensagem do Telegram com a decisão (e tira os botões) é o `server-central`.

O resultado da varinha e da checagem chega do `a3s-claude-relay` por webhook, num listener interno separado (`POST /claude-callback?secret=&job=`, porta `CLAUDE_CALLBACK_PORT`, padrão 8090), fora do proxy público. Ver `docs/architecture.md`, "Decisão: sugestões de melhoria com varinha do Claude".

### WebSocket — `GET /api/presence/ws`

Uma conexão por sessão do client, mantida aberta enquanto online; serve **presença e DMs na mesma conexão** (ver `docs/architecture.md`, "Decisão: DMs entregues no mesmo WebSocket de presença"). Ao conectar (primeira conexão da conta) e desconectar (última conexão), o servidor emite `presence.update` para cada amigo aceito online.

- **client → servidor:** só `dm.create` — `{"type": "dm.create", "recipientId": "...", "ciphertext": "...", "nonce": "..."}` (`ciphertext`/`nonce` base64, cifrados no client antes de enviar). Rejeitado com `error` se: destinatário é o próprio remetente, `nonce` não tem 24 bytes decodificados, `ciphertext` vazio ou > 8192 bytes decodificados, ou remetente/destinatário não são amigos aceitos (`FriendshipStore.AreFriends`).
- **servidor → client:**
  - `presence.update` — `{"type": "presence.update", "accountId": "...", "online": bool}`, só para amigos aceitos.
  - `dm.created` — `{"type": "dm.created", "message": DirectMessage}`, broadcast para remetente e destinatário.
  - `friend.request` — `{"type": "friend.request", "request": FriendRequest}`, para quem recebeu um pedido de amizade novo.
  - `friend.request.removed` — `{"type": "friend.request.removed", "id": "..."}`, para os dois lados quando um pedido pendente é recusado ou cancelado.
  - `friend.accepted` — `{"type": "friend.accepted", "requestId": "...", "accountId": "...", "displayName"?: "..."}`, para os dois lados quando uma amizade passa a valer (pedido aceito ou convite resgatado); `accountId` é o novo amigo de quem recebe o frame.
  - `friend.removed` — `{"type": "friend.removed", "accountId": "..."}`, para os dois lados quando uma amizade é desfeita; `accountId` é o ex-amigo de quem recebe o frame.
  - `error` — `{"type": "error", "code": "...", "message": "...", "params"?: {...}, "error": "..."}` (ver "Frame `error` do WebSocket"). Códigos: `dm.send_self`, `dm.nonce_invalid`, `dm.ciphertext_invalid`, `dm.send_friends_only`, `dm.send_failed` e os `realtime.*`.

### Rate limiting

Token bucket em memória por usuário, aplicado a toda a API exceto `/healthz` (`RATE_LIMIT_RPM`, padrão 120; `RATE_LIMIT_BURST`, padrão 60; chave = `sub` do token verificado, ou IP via `X-Forwarded-For`/`RemoteAddr` quando não há token válido). Excesso responde `429 Too Many Requests` com header `Retry-After`. Ver `docs/architecture.md`, "Decisão: rate limiting em server-central".

## `server-channel`

Base URL: `KnownServer.baseUrl`, uma por servidor cadastrado no client (endereço completo informado pelo self-hoster ou embutido num link de convite).

### REST

| Método | Rota | Auth | Request | Response | Erros |
|---|---|---|---|---|---|
| GET | `/healthz` | não | — | `{status, version}` | — |
| POST | `/api/join` | Bearer (sem `RequireMember`) | `{code?}` | `200` (já membro) ou `201` `{memberId, founder?}` | `403` sem convite (exceto fundador) ou banido, `404`/`410`/`409` convite inválido |
| GET | `/api/me` | Bearer + membro | — | `{memberId, oidcSubject, nickname?, joinedAt, isOwner?, permissions, roleIds?}` | `403` só quando o `sub` não é membro (nunca entrou, expulso ou banido): a rota não exige bit, e o client usa esse 403 para mostrar o aviso de membro expulso |
| PATCH | `/api/me` | Bearer + membro | `{nickname: string \| null}` | `Me` (mesmo shape de GET) | `400` apelido > 64 chars |
| GET | `/api/members` | Bearer + membro | — | `{members: [{id, nickname?, joinedAt, isOwner?, roleIds?}]}` — só membros ativos, sem quem foi expulso | — |
| POST | `/api/members/{memberId}/kick` | Bearer + membro + `KickMembers` | — | `204` | `400` alvo é você mesmo, `403` alvo é o dono, `404` membro, `409` já expulso |
| POST | `/api/members/{memberId}/ban` | Bearer + membro + `BanMembers` | `{reason?}` | `204` | `400` alvo é você mesmo, `403` alvo é o dono, `404` membro |
| GET | `/api/bans` | Bearer + membro + `BanMembers` | — | `{bans: [{oidcSubject, bannedByMemberId?, reason?, createdAt, lastNickname?}]}` | — |
| DELETE | `/api/bans/{oidcSubject}` | Bearer + membro + `BanMembers` | — | `204` | `404` não banido |
| GET | `/api/categories` | Bearer + membro | — | `{categories: [{id, name, position, createdAt}]}` — só com canal visível; quem tem qualquer bit de estrutura (`ManageChannels` ou as fatias, ver `docs/permissions.md`) ou é dono vê todas, inclusive vazias | — |
| POST | `/api/categories` | Bearer + membro + `ManageChannels` ou `CreateCategories` | `{name, position?}` (sem `position` = fim da lista) | `201` `Category` | `400` nome vazio ou > 100 chars |
| PUT | `/api/categories/order` | Bearer + membro + `ManageChannels` ou `ReorderCategories` | `{ids: [string]}` — todas as categorias, na ordem nova | `204`; grava posições 0..n-1 numa transação | `409` se `ids` não for exatamente o conjunto atual (alguém criou/apagou no meio tempo) |
| PATCH | `/api/categories/{id}` | Bearer + membro; `name` exige `ManageChannels`, só `position` aceita também `ReorderCategories` | `{name?, position?}` (ausente = mantém) | `Category` | `400`, `403`, `404` |
| DELETE | `/api/categories/{id}` | Bearer + membro + `ManageChannels` ou `DeleteCategories` | — | `204`; os canais da categoria ficam sem categoria | `404` |
| GET | `/api/channels` | Bearer + membro | — | `{channels: [{id, categoryId?, name, type, position, createdAt, lastMessageAt?}]}` — só visíveis; `lastMessageAt` alimenta o indicador de não lida do client, ver `docs/architecture.md`, "Decisão: indicador de não lida" | — |
| POST | `/api/channels` | Bearer + membro + `ManageChannels` ou `CreateChannels` | `{name, type: "text"\|"voice"\|"forum", categoryId?, position?}` (sem `position` = fim da categoria) | `201` `Channel` | `400` nome vazio/> 100 chars, tipo inválido, `categoryId` inexistente |
| PUT | `/api/channels/order` | Bearer + membro + `ManageChannels` ou `ReorderChannels` | `{groups: [{categoryId: string \| null, ids: [string]}]}` — ordem nova de cada categoria afetada (mover entre categorias manda origem e destino) | `204`; canais listados vão para a categoria do grupo, na ordem; os da categoria que não vieram (ex. invisíveis a quem arrastou) vão para o fim, na ordem em que estavam | `400` sem grupos, `409` canal/categoria inexistente ou repetido |
| PATCH | `/api/channels/{id}` | Bearer + membro; `name` exige `ManageChannels`, só `categoryId`/`position` aceita também `ReorderChannels` | `{name?, categoryId?: string \| null, position?}` (ausente = mantém, `categoryId: null` = sem categoria) | `Channel` | `400` inclusive ao mandar `type` (não muda depois de criado), `404` |
| DELETE | `/api/channels/{id}` | Bearer + membro + `ManageChannels` ou `DeleteChannels` | — | `204`; apaga mensagens, threads, anexos (inclusive os arquivos em disco) e overwrites do canal | `404` |
| GET | `/api/channels/{id}/messages?before=&limit=` | Bearer + membro + `ViewChannels` | — | `{messages: [Message]}` | `400` canal não é texto, `403`, `404` |
| POST | `/api/channels/{id}/messages` | Bearer + membro + `SendMessages` | `multipart/form-data`: `content?`, `file?` (pelo menos um) | `201` `Message` | `400` sem conteúdo nem anexo, `413` anexo maior que `ATTACHMENT_MAX_MB`, `403`, `404` |
| GET | `/api/attachments/{id}` | Bearer + membro + `ViewChannels` (do canal da mensagem do anexo) | — | bytes do arquivo (`Content-Type`/`Content-Disposition` do anexo) | `403`, `404` |
| GET | `/api/channels/{id}/threads` | Bearer + membro + `ViewChannels` | — | `{threads: [Thread]}` | `400` canal não é forum |
| GET | `/api/threads/{id}/messages?before=&limit=` | Bearer + membro + `ViewChannels` | — | `{messages: [Message]}` | `404` thread |
| GET | `/api/channels/{id}/ws` | Bearer (subprotocolo) + membro + `ViewChannels` | upgrade WS | ver abaixo | `400` tipo de canal não suporta WS, `403` |
| POST | `/api/channels/{id}/voice/token` | Bearer + membro + `Voice` | — | `{token, roomName, url}` | `400` canal não é voz, `403` |
| POST | `/api/voice/move` | Bearer + membro + `MoveMembers` nas salas de origem e destino + `Voice` no destino | `{memberId, channelId}` | `204`; o client de quem é movido recebe pelo LiveKit (tópico `ffcom.voice.move`, mandado pelo servidor) `{channelId, channelName, token, url}` e troca de sala com esse token. Já estar no destino também dá `204` | `400`, `403`, `404`, `409` membro fora de qualquer sala de voz, `502` LiveKit indisponível |
| GET | `/api/voice/participants` | Bearer + membro | — | `{channels: {<channelId>: [{memberId, name}]}}` — só canais de voz com `ViewChannels` e com alguém dentro; foto do LiveKit guardada por 5s | `502` LiveKit indisponível |
| GET | `/api/channels/{id}/overwrites` | Bearer + membro + `ManageRoles` | — | `{overwrites: [{roleId, allow, deny}]}` | `404` canal |
| PUT | `/api/channels/{id}/overwrites/{roleId}` | Bearer + membro + `ManageRoles` | `{allow, deny}` | `Overwrite` | `403` allow excede permissão própria (`Grants`), `404` canal |
| DELETE | `/api/channels/{id}/overwrites/{roleId}` | Bearer + membro + `ManageRoles` | — | `204` | `404` |
| POST | `/api/invites` | Bearer + membro + `CreateInvites` ou `ManageInvites` | `{maxUses?, expiresAt?}` | `201` `Invite` | `400` maxUses ≤ 0 |
| GET | `/api/invites` | Bearer + membro + `ManageInvites` | — | `{invites: [Invite]}` | — |
| DELETE | `/api/invites/{id}` | Bearer + membro + `ManageInvites` | — | `204` | `404` |
| GET | `/api/roles` | Bearer + membro | — | `{roles: [Role]}` — aberto a todo membro | — |
| POST | `/api/roles` | Bearer + membro + `ManageRoles` | `{name, color?, permissions, position}` | `201` `Role` | `400` name vazio, `403` `Grants` |
| PATCH | `/api/roles/{id}` | Bearer + membro + `ManageRoles` | idem | `Role` | `403` `Grants`, `404` |
| DELETE | `/api/roles/{id}` | Bearer + membro + `ManageRoles` | — | `204` | `400` role default, `404` |
| POST | `/api/members/{memberId}/roles/{roleId}` | Bearer + membro + `ManageRoles` | — | `204` | `400` role default, `403` `Grants`, `404` membro/role |
| DELETE | `/api/members/{memberId}/roles/{roleId}` | Bearer + membro + `ManageRoles` | — | `204` | `400` role default, `404` |

`Message`: `{id, channelId, threadId?, authorMemberId, content, createdAt, editedAt?, attachments?: [Attachment]}` — `attachments` só em mensagem de canal de texto (canal forum fora do escopo, ver docs/architecture.md, "Decisão: upload de anexo em mensagem"). `Attachment`: `{id, filename, contentType, sizeBytes, url}` — `url` é relativo (`/api/attachments/{id}`) e exige o mesmo Bearer token de qualquer outra rota, não dá pra usar direto num `<img src>` ou link de download. `Thread`: `{id, channelId, title, authorMemberId, createdAt}`. `Role`: `{id, name, color?, permissions, position, isDefault, createdAt}`. `Invite`: `{id, code, createdByMemberId, maxUses?, uses, expiresAt?, createdAt}`. `Overwrite`: `{roleId, allow, deny}`.

**Bits de permissão** (`internal/permissions`, `int64`): `ViewChannels=1, SendMessages=2, Voice=4, ManageInvites=8, ManageRoles=16, Administrator=32, KickMembers=64, BanMembers=128, ManageChannels=256, CreateInvites=512, CreateChannels=1024, ReorderChannels=2048, DeleteChannels=4096, CreateCategories=8192, ReorderCategories=16384, DeleteCategories=32768, MoveMembers=65536`. Catálogo completo, com o que cada bit libera, em `docs/permissions.md`. `Owner=-1` (dono do bootstrap, ignora tudo). `Grants(base, target)` impede que `ManageRoles` sozinho conceda um bit que quem chama não possui — ver `docs/architecture.md`, "Decisão: ManageRoles não concede permissões além das próprias". Kick/ban não passam por `Grants` (não concedem bit nenhum a ninguém) e não têm checagem de hierarquia entre roles — só o bit e "não pode ser o dono/você mesmo", ver `docs/architecture.md`, "Decisão: kick/ban de membro".

### WebSocket — `GET /api/channels/{id}/ws`

Mesma rota para canal de **texto** e **forum** (`docs/architecture.md`, "Decisão: canal forum"); o tipo do canal decide quais frames são aceitos. `SendMessages` é checado uma vez na conexão, não por frame.

**Canal de texto:**
- client → servidor: `message.create` — `{"type": "message.create", "content": "..."}`; `message.update` — `{"type": "message.update", "id": "...", "content": "..."}` (só o autor); `message.delete` — `{"type": "message.delete", "id": "..."}` (autor ou `Administrator`).
- servidor → client: `message.created` — `{"type": "message.created", "message": Message}`; `message.updated` — `{"type": "message.updated", "message": Message}`; `message.deleted` — `{"type": "message.deleted", "id": "...", "channelId": "..."}`. Todos broadcast a todo o canal.

**Canal forum** (mesmo hub, particionado por `channel_id`, não por thread — todo mundo conectado ao canal recebe todo evento, o client filtra por `threadId`):
- client → servidor: `thread.create` — `{"type": "thread.create", "title": "...", "content": "..."}` (abre thread + post inicial); `post.create` — `{"type": "post.create", "threadId": "...", "content": "..."}`.
- servidor → client: `thread.created` — `{"type": "thread.created", "thread": Thread, "message": Message}`; `post.created` — `{"type": "post.created", "message": Message}`.

Em ambos os casos: `error` para conteúdo vazio, acima do limite (mensagem: 4000 chars, título de thread: 200 chars), sem `SendMessages`, thread de outro canal, ou tipo de frame desconhecido. `message.update`/`message.delete` também retornam `error` para mensagem não encontrada, mensagem de outro canal, ou sem autoria/`Administrator`. Formato e códigos em "Frame `error` do WebSocket" (`messages.*`, `forum.*`, `realtime.*`).

### Rate limiting

Dois token buckets em memória, chaves diferentes (ver `docs/architecture.md`, "Decisão: rate limiting em server-channel"):
- **REST** (toda a API exceto `/healthz`, incluindo o handshake de `GET /api/channels/{id}/ws`): por usuário (`sub` do token verificado; IP quando não há token válido), `RATE_LIMIT_RPM` (padrão 120) / `RATE_LIMIT_BURST` (padrão 60). Orçamento por tela em `docs/rate-limits.md`. Excesso responde `429 Too Many Requests` com header `Retry-After`.
- **Frames de WebSocket** (dentro de uma conexão de canal já aberta): por membro, `RATE_LIMIT_WS_RPM` (padrão 60) / `RATE_LIMIT_WS_BURST` (padrão 10). Excesso responde com um frame `error` de código `realtime.rate_limited` (conexão permanece aberta, frame é descartado).

## Não confirmado / fora do escopo deste documento

- Formato exato de erro de validação do Authentik (fora do código deste repo).
- Rotas administrativas do LiveKit/coturn (não expostas pela API do `server-channel` — a integração é só emissão de token, ver `docs/architecture.md`).
