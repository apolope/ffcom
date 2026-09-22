# Protocolo entre client, server-central e server-channel

Referência técnica dos endpoints REST e frames de WebSocket expostos por `server-central` e `server-channel`, e de como o `client` os consome. As decisões de design por trás de cada mecanismo (por quê REST vs. WebSocket, por quê DMs entram na conexão de presença, por quê convite é obrigatório, etc.) estão em [`docs/architecture.md`](architecture.md) — este documento não as repete, só referencia. Gerado a partir da leitura do código em 2026-09-21, atualizado em 2026-09-22 (criptografia ponta-a-ponta em DMs); se o comportamento e este doc divergirem no futuro, o código manda.

## Visão geral

Três componentes: `client` (SPA React, web/PWA e Electron), `server-central` (instância única oficial: contas, amigos, presença, DMs, diretório de servidores conhecidos) e `server-channel` (uma instância por comunidade self-hosted: categorias, canais, mensagens, permissões, convites, voz).

**Não existe comunicação direta `server-central` ↔ `server-channel`** — confirmado lendo `server.go` dos dois componentes: nenhum dos dois faz requisição HTTP para o outro. O `client` é o único que fala com ambos, cada um numa base URL própria (`SERVER_CENTRAL_URL`, fixa por build, e `KnownServer.baseUrl`, uma por servidor adicionado ao diretório — ver `docs/architecture.md`, "diretório de servidores conhecidos: `address` é a base URL completa"). Ver `docs/architecture.md`, "Decisão: protocolo entre client, server-central e server-channel".

## Autenticação

Os dois servidores validam o mesmo Bearer JWT emitido pela instância central de Authentik do `abs-3d-printer` (`OIDC_ISSUER_URL` idêntica nos dois, `coreos/go-oidc/v3`, `SkipClientIDCheck: true`) — não há chamada de rede entre eles para isso; cada um valida a assinatura/issuer/expiração de forma independente. Ver `docs/architecture.md`, "Decisão: autenticação em server-central" e "Decisão: modelo de dados de server-channel".

- **`server-central`:** qualquer token válido já basta — a conta local é criada implicitamente (upsert por `sub`) na primeira requisição autenticada (`auth.Middleware`, `AccountStore.GetOrCreateBySubject`).
- **`server-channel`:** validar o token só dá o "sub" (`auth.VerifyToken`). A maioria das rotas exige também que esse `sub` já seja **membro** daquela instância (`auth.RequireMember`, 403 se não for) — associação feita via `POST /api/join`, que exige convite exceto para o primeiro membro (fundador do self-host). Ver "Decisão: convites obrigatórios para entrar em server-channel".

**Header:** `Authorization: Bearer <token>` em toda chamada REST.

**WebSocket:** a API `WebSocket` do navegador não permite setar `Authorization` no handshake. O token vai como segundo elemento do subprotocolo: `new WebSocket(url, ["access_token", token])`, que vira o header `Sec-WebSocket-Protocol: access_token, <token>` — mesmo mecanismo nos dois servidores (`wsAuthSubprotocol = "access_token"`). O `Upgrader` de cada rota WS declara `Subprotocols: []string{"access_token"}` para aceitar/ecoar.

## Convenções gerais

- **Erros HTTP:** texto simples via `http.Error` (não JSON) — `Content-Type: text/plain`, corpo é a mensagem de erro em português. Status codes usados: `400` (corpo/parâmetro inválido), `401` (token ausente/inválido), `403` (sem permissão), `404` (recurso não encontrado), `409` (conflito — ex. convite já usado), `410 Gone` (convite expirado), `426 Upgrade Required` (TLS obrigatório, só quando `REQUIRE_TLS=true`, ver abaixo), `429` (rate limit, ver abaixo).
- **Respostas de sucesso:** JSON, `Content-Type: application/json`, `camelCase` em todos os campos.
- **Paginação de histórico:** keyset pagination por `created_at`, sempre os mesmos dois query params — `before` (RFC3339, opcional) e `limit` (opcional, default 50, máximo 200). O servidor devolve mais recentes primeiro; o `client` inverte para ordem cronológica antes de renderizar (`fetchChannelHistory`, `fetchThreadMessages`, `fetchDirectMessages` em `client/src/lib/`).
- **Frames de WebSocket:** envelope JSON uniforme `{"type": "...", ...payload}` nos dois servidores. O client → servidor emite um tipo (`*.create`); o servidor → client responde com o `*.created` correspondente (broadcast, **incluindo o próprio autor**, para confirmar id/timestamp atribuídos pelo servidor) ou `"error"` (`{"type": "error", "error": "..."}`, só para quem causou o erro).
- **CORS:** variável de ambiente `CORS_ALLOWED_ORIGINS` (lista separada por vírgula, vazia por padrão) em ambos os servidores, mesmo mecanismo (`internal/httpapi/cors.go`, idêntico nos dois). Preflight `OPTIONS` respondido com `204`; origem liberada ecoa em `Access-Control-Allow-Origin` + `Vary: Origin`; `Access-Control-Allow-Methods: GET, POST, PATCH, PUT, DELETE, OPTIONS` (lista fixa e abrangente — **atualizar aqui ao adicionar uma rota com método novo**, é a causa mais comum de erro de CORS neste projeto apesar da origem estar liberada, ver `docs/architecture.md`). O `CheckOrigin` do upgrader de WebSocket usa a mesma allowlist, com fallback ao comportamento padrão do gorilla (sem header `Origin`, ou `Origin == Host`, sempre passa).
- **`GET /healthz`:** sem autenticação, nos dois servidores. `{"status":"ok","version":"..."}`.
- **TLS obrigatório (`REQUIRE_TLS`):** desligado por padrão nos dois servidores. Quando `true`, toda requisição (exceto `/healthz`) precisa chegar com o header `X-Forwarded-Proto: https` (setado pelo proxy reverso na frente — nenhum dos dois binários termina TLS) ou é rejeitada com `426`. Ver `docs/architecture.md`, "Criptografia em trânsito obrigatória".

## `server-central`

Base URL: `VITE_SERVER_CENTRAL_URL` no client (`http://localhost:8081` em dev).

### REST

| Método | Rota | Auth | Request | Response | Erros |
|---|---|---|---|---|---|
| GET | `/healthz` | não | — | `{status, version}` | — |
| GET | `/api/me` | Bearer | — | `{accountId, oidcSubject, createdAt, displayName?, avatarUrl?}` | — |
| PUT | `/api/me/e2e-public-key` | Bearer | `{publicKey}` (base64, 32 bytes) | `204` | `400` tamanho inválido |
| GET | `/api/servers` | Bearer | — | `{servers: [{id, address, name, iconUrl?, addedAt}]}` | — |
| POST | `/api/servers` | Bearer | `{address, name, iconUrl?}` | `201` + `KnownServer` | `400` address/name vazios |
| DELETE | `/api/servers/{id}` | Bearer | — | `204` | `404` |
| GET | `/api/presence` | Bearer | — | `{friends: [{accountId, online}]}` — snapshot dos amigos aceitos | — |
| GET | `/api/presence/ws` | Bearer (subprotocolo) | upgrade WS | ver abaixo | — |
| GET | `/api/friends` | Bearer | — | `{friends: [{accountId, displayName?, avatarUrl?, e2ePublicKey?}]}` | — |
| POST | `/api/friends/invites` | Bearer | — | `201` `{code, createdAt}` | — |
| POST | `/api/friends/invites/{code}/redeem` | Bearer | — | `201` `{friendAccountId}` | `400` convite próprio, `404`, `409` já usado, `410` expirado |
| GET | `/api/dms/{accountId}/messages?before=&limit=` | Bearer | — | `{messages: [DirectMessage]}` | `403` se não são amigos |

`DirectMessage`: `{id, senderId, recipientId, ciphertext, nonce, createdAt, editedAt?}` — `ciphertext`/`nonce` são base64, opacos ao servidor (criptografia ponta-a-ponta, ver `docs/architecture.md`, "Decisão: criptografia ponta-a-ponta em DMs"). O client decifra localmente com a chave pública atual do outro lado da conversa (`GET /api/friends`) + a chave privada do dispositivo.

### WebSocket — `GET /api/presence/ws`

Uma conexão por sessão do client, mantida aberta enquanto online; serve **presença e DMs na mesma conexão** (ver `docs/architecture.md`, "Decisão: DMs entregues no mesmo WebSocket de presença"). Ao conectar (primeira conexão da conta) e desconectar (última conexão), o servidor emite `presence.update` para cada amigo aceito online.

- **client → servidor:** só `dm.create` — `{"type": "dm.create", "recipientId": "...", "ciphertext": "...", "nonce": "..."}` (`ciphertext`/`nonce` base64, cifrados no client antes de enviar). Rejeitado com `error` se: destinatário é o próprio remetente, `nonce` não tem 24 bytes decodificados, `ciphertext` vazio ou > 8192 bytes decodificados, ou remetente/destinatário não são amigos aceitos (`FriendshipStore.AreFriends`).
- **servidor → client:**
  - `presence.update` — `{"type": "presence.update", "accountId": "...", "online": bool}`, só para amigos aceitos.
  - `dm.created` — `{"type": "dm.created", "message": DirectMessage}`, broadcast para remetente e destinatário.
  - `error` — `{"type": "error", "error": "..."}`.

### Rate limiting

Token bucket em memória por IP, aplicado a toda a API exceto `/healthz` (`RATE_LIMIT_RPM`, padrão 120; `RATE_LIMIT_BURST`, padrão 20; chave = `X-Forwarded-For` ou `RemoteAddr`). Excesso responde `429 Too Many Requests` com header `Retry-After`. Ver `docs/architecture.md`, "Decisão: rate limiting em server-central".

## `server-channel`

Base URL: `KnownServer.baseUrl`, uma por servidor cadastrado no client (endereço completo informado pelo self-hoster ou embutido num link de convite).

### REST

| Método | Rota | Auth | Request | Response | Erros |
|---|---|---|---|---|---|
| GET | `/healthz` | não | — | `{status, version}` | — |
| POST | `/api/join` | Bearer (sem `RequireMember`) | `{code?}` | `200` (já membro) ou `201` `{memberId, founder?}` | `403` sem convite (exceto fundador) ou banido, `404`/`410`/`409` convite inválido |
| GET | `/api/me` | Bearer + membro | — | `{memberId, oidcSubject, nickname?, joinedAt, isOwner?, permissions, roleIds?}` | — |
| PATCH | `/api/me` | Bearer + membro | `{nickname: string \| null}` | `Me` (mesmo shape de GET) | `400` apelido > 64 chars |
| GET | `/api/members` | Bearer + membro | — | `{members: [{id, nickname?, joinedAt, isOwner?, roleIds?}]}` — só membros ativos, sem quem foi expulso | — |
| POST | `/api/members/{memberId}/kick` | Bearer + membro + `KickMembers` | — | `204` | `400` alvo é você mesmo, `403` alvo é o dono, `404` membro, `409` já expulso |
| POST | `/api/members/{memberId}/ban` | Bearer + membro + `BanMembers` | `{reason?}` | `204` | `400` alvo é você mesmo, `403` alvo é o dono, `404` membro |
| GET | `/api/bans` | Bearer + membro + `BanMembers` | — | `{bans: [{oidcSubject, bannedByMemberId?, reason?, createdAt, lastNickname?}]}` | — |
| DELETE | `/api/bans/{oidcSubject}` | Bearer + membro + `BanMembers` | — | `204` | `404` não banido |
| GET | `/api/categories` | Bearer + membro | — | `{categories: [{id, name, position, createdAt}]}` — só com canal visível | — |
| GET | `/api/channels` | Bearer + membro | — | `{channels: [{id, categoryId?, name, type, position, createdAt}]}` — só visíveis | — |
| GET | `/api/channels/{id}/messages?before=&limit=` | Bearer + membro + `ViewChannels` | — | `{messages: [Message]}` | `400` canal não é texto, `403`, `404` |
| POST | `/api/channels/{id}/messages` | Bearer + membro + `SendMessages` | `multipart/form-data`: `content?`, `file?` (pelo menos um) | `201` `Message` | `400` sem conteúdo nem anexo, `413` anexo maior que `ATTACHMENT_MAX_MB`, `403`, `404` |
| GET | `/api/attachments/{id}` | Bearer + membro + `ViewChannels` (do canal da mensagem do anexo) | — | bytes do arquivo (`Content-Type`/`Content-Disposition` do anexo) | `403`, `404` |
| GET | `/api/channels/{id}/threads` | Bearer + membro + `ViewChannels` | — | `{threads: [Thread]}` | `400` canal não é forum |
| GET | `/api/threads/{id}/messages?before=&limit=` | Bearer + membro + `ViewChannels` | — | `{messages: [Message]}` | `404` thread |
| GET | `/api/channels/{id}/ws` | Bearer (subprotocolo) + membro + `ViewChannels` | upgrade WS | ver abaixo | `400` tipo de canal não suporta WS, `403` |
| POST | `/api/channels/{id}/voice/token` | Bearer + membro + `Voice` | — | `{token, roomName, url}` | `400` canal não é voz, `403` |
| GET | `/api/channels/{id}/overwrites` | Bearer + membro + `ManageRoles` | — | `{overwrites: [{roleId, allow, deny}]}` | `404` canal |
| PUT | `/api/channels/{id}/overwrites/{roleId}` | Bearer + membro + `ManageRoles` | `{allow, deny}` | `Overwrite` | `403` allow excede permissão própria (`Grants`), `404` canal |
| DELETE | `/api/channels/{id}/overwrites/{roleId}` | Bearer + membro + `ManageRoles` | — | `204` | `404` |
| POST | `/api/invites` | Bearer + membro + `ManageInvites` | `{maxUses?, expiresAt?}` | `201` `Invite` | `400` maxUses ≤ 0 |
| GET | `/api/invites` | Bearer + membro + `ManageInvites` | — | `{invites: [Invite]}` | — |
| DELETE | `/api/invites/{id}` | Bearer + membro + `ManageInvites` | — | `204` | `404` |
| GET | `/api/roles` | Bearer + membro | — | `{roles: [Role]}` — aberto a todo membro | — |
| POST | `/api/roles` | Bearer + membro + `ManageRoles` | `{name, color?, permissions, position}` | `201` `Role` | `400` name vazio, `403` `Grants` |
| PATCH | `/api/roles/{id}` | Bearer + membro + `ManageRoles` | idem | `Role` | `403` `Grants`, `404` |
| DELETE | `/api/roles/{id}` | Bearer + membro + `ManageRoles` | — | `204` | `400` role default, `404` |
| POST | `/api/members/{memberId}/roles/{roleId}` | Bearer + membro + `ManageRoles` | — | `204` | `400` role default, `403` `Grants`, `404` membro/role |
| DELETE | `/api/members/{memberId}/roles/{roleId}` | Bearer + membro + `ManageRoles` | — | `204` | `400` role default, `404` |

`Message`: `{id, channelId, threadId?, authorMemberId, content, createdAt, editedAt?, attachments?: [Attachment]}` — `attachments` só em mensagem de canal de texto (canal forum fora do escopo, ver docs/architecture.md, "Decisão: upload de anexo em mensagem"). `Attachment`: `{id, filename, contentType, sizeBytes, url}` — `url` é relativo (`/api/attachments/{id}`) e exige o mesmo Bearer token de qualquer outra rota, não dá pra usar direto num `<img src>` ou link de download. `Thread`: `{id, channelId, title, authorMemberId, createdAt}`. `Role`: `{id, name, color?, permissions, position, isDefault, createdAt}`. `Invite`: `{id, code, createdByMemberId, maxUses?, uses, expiresAt?, createdAt}`. `Overwrite`: `{roleId, allow, deny}`.

**Bits de permissão** (`internal/permissions`, `int64`): `ViewChannels=1, SendMessages=2, Voice=4, ManageInvites=8, ManageRoles=16, Administrator=32, KickMembers=64, BanMembers=128`. `Owner=-1` (dono do bootstrap, ignora tudo). `Grants(base, target)` impede que `ManageRoles` sozinho conceda um bit que quem chama não possui — ver `docs/architecture.md`, "Decisão: ManageRoles não concede permissões além das próprias". Kick/ban não passam por `Grants` (não concedem bit nenhum a ninguém) e não têm checagem de hierarquia entre roles — só o bit e "não pode ser o dono/você mesmo", ver `docs/architecture.md`, "Decisão: kick/ban de membro".

### WebSocket — `GET /api/channels/{id}/ws`

Mesma rota para canal de **texto** e **forum** (`docs/architecture.md`, "Decisão: canal forum"); o tipo do canal decide quais frames são aceitos. `SendMessages` é checado uma vez na conexão, não por frame.

**Canal de texto:**
- client → servidor: `message.create` — `{"type": "message.create", "content": "..."}`; `message.update` — `{"type": "message.update", "id": "...", "content": "..."}` (só o autor); `message.delete` — `{"type": "message.delete", "id": "..."}` (autor ou `Administrator`).
- servidor → client: `message.created` — `{"type": "message.created", "message": Message}`; `message.updated` — `{"type": "message.updated", "message": Message}`; `message.deleted` — `{"type": "message.deleted", "id": "...", "channelId": "..."}`. Todos broadcast a todo o canal.

**Canal forum** (mesmo hub, particionado por `channel_id`, não por thread — todo mundo conectado ao canal recebe todo evento, o client filtra por `threadId`):
- client → servidor: `thread.create` — `{"type": "thread.create", "title": "...", "content": "..."}` (abre thread + post inicial); `post.create` — `{"type": "post.create", "threadId": "...", "content": "..."}`.
- servidor → client: `thread.created` — `{"type": "thread.created", "thread": Thread, "message": Message}`; `post.created` — `{"type": "post.created", "message": Message}`.

Em ambos os casos: `error` para conteúdo vazio, acima do limite (mensagem: 4000 chars, título de thread: 200 chars), sem `SendMessages`, thread de outro canal, ou tipo de frame desconhecido. `message.update`/`message.delete` também retornam `error` para mensagem não encontrada, mensagem de outro canal, ou sem autoria/`Administrator`.

### Rate limiting

Dois token buckets em memória, chaves diferentes (ver `docs/architecture.md`, "Decisão: rate limiting em server-channel"):
- **REST** (toda a API exceto `/healthz`, incluindo o handshake de `GET /api/channels/{id}/ws`): por IP, `RATE_LIMIT_RPM` (padrão 120) / `RATE_LIMIT_BURST` (padrão 20). Excesso responde `429 Too Many Requests` com header `Retry-After`.
- **Frames de WebSocket** (dentro de uma conexão de canal já aberta): por membro, `RATE_LIMIT_WS_RPM` (padrão 60) / `RATE_LIMIT_WS_BURST` (padrão 10). Excesso responde com um frame `error` (conexão permanece aberta, frame é descartado).

## Não confirmado / fora do escopo deste documento

- Formato exato de erro de validação do Authentik (fora do código deste repo).
- Rotas administrativas do LiveKit/coturn (não expostas pela API do `server-channel` — a integração é só emissão de token, ver `docs/architecture.md`).
