# Rate limits e orçamento de requisições

Este documento responde a uma pergunta só: com o que o client faz hoje, alguém
usando o FFCom normalmente estoura o rate limit? Serve para conferir o
orçamento antes de adicionar um poll novo, encurtar um intervalo ou fazer uma
tela disparar mais requisições ao abrir. Toda mudança nesses pontos deve
atualizar as tabelas daqui.

As decisões de desenho (por que token bucket em memória, por que dois limiters
em server-channel) ficam em `docs/architecture.md`, nas seções "Decisão: rate
limiting em server-central", "Decisão: rate limiting em server-channel" e
"Decisão: rate limit por usuário em vez de por IP" e "Decisão: rate limit por
usuário e dispositivo".

## Regras

Os dois servidores usam o mesmo token bucket (`internal/httpapi/ratelimit.go`,
duplicado em cada módulo). Cada chave tem um balde com capacidade `BURST`
fichas, que recarrega continuamente a `RPM / 60` fichas por segundo. Cada
requisição gasta uma ficha; sem ficha, a resposta é `429 Too Many Requests`
com `Retry-After: 1`. Balde parado por 10 minutos é descartado (volta cheio na
próxima requisição).

| Limiter | Onde | Chave | Padrão (RPM / burst) | Variáveis |
|---|---|---|---|---|
| REST de server-central | toda a API, menos `/healthz` | `sub` + `sid` do token verificado (só `sub` se o token não trouxer `sid`); sem token válido, IP | 120 / 60 | `RATE_LIMIT_RPM`, `RATE_LIMIT_BURST` |
| REST de server-channel | toda a API, menos `/healthz`, incluindo o handshake do WebSocket de canal | `sub` + `sid` do token verificado (só `sub` se o token não trouxer `sid`); sem token válido, IP | 120 / 60 | `RATE_LIMIT_RPM`, `RATE_LIMIT_BURST` |
| Frames do WebSocket de canal | cada frame recebido numa conexão já aberta (`message.create`, `message.update`, `message.delete`, `thread.create`, `post.create`) | id do membro + `sid` do token do handshake | 60 / 10 | `RATE_LIMIT_WS_RPM`, `RATE_LIMIT_WS_BURST` |

Consequências da chave por usuário e dispositivo que importam para a conta:

1. **O orçamento é do par pessoa/dispositivo.** O `sid` é o hash da sessão de
   login no Authentik, que vem assinado dentro do access token. Celular, PC e
   notebook da mesma conta logam cada um por conta própria e têm baldes
   separados; o app Electron e o navegador no mesmo PC também. Abas do mesmo
   navegador dividem a sessão (o `oidc-client-ts` guarda o token em
   `localStorage`) e portanto o balde. As tabelas abaixo são por instância do
   client; multiplique pelo número de abas abertas no mesmo navegador.
2. **Cada servidor tem seu próprio balde.** server-central e cada
   server-channel contam separado; gastar tudo num server-channel não afeta
   outro.
3. **Contas diferentes atrás do mesmo IP não se atrapalham.** Vale para teste
   local com duas contas, escritório e CGNAT de operadora.
4. **Requisição sem token válido cai no balde do IP** (`ip:<endereço>`), que é
   separado dos baldes de usuário. Token expirado entra aqui também, então um
   client com token vencido em loop pode travar outros anônimos do mesmo IP,
   nunca usuários autenticados.
5. **Burst é o que protege a abertura de tela; RPM é o que protege o uso
   parado.** Uma tela que dispara 25 requisições de uma vez estoura mesmo que
   a média do minuto seja baixa.

## Requisições recorrentes (uso parado)

Por instância do client, com a pessoa só olhando a tela.

### server-channel aberto (o selecionado no rail)

| Requisição | Origem | Intervalo | Por minuto |
|---|---|---|---|
| `GET /api/categories` | `useServerStructure` | 20 s | 3 |
| `GET /api/channels` | `useServerStructure` | 20 s | 3 |
| `GET /api/voice/participants` | `useVoiceParticipants` (só se o servidor tem canal de voz visível) | 10 s | 6 |
| **Total** | | | **12** |

### Cada server-channel em segundo plano (no rail, não selecionado)

| Requisição | Origem | Intervalo | Por minuto |
|---|---|---|---|
| `GET /api/channels` | `useServersUnread` (pílula de não lido) | 60 s | 1 |

### server-central

Nenhum poll. Presença, DMs e status chegam pelo WebSocket de presença
(`GET /api/presence/ws`), que conta uma ficha só no handshake.

### Fora dos limiters

O `registration.update()` do service worker (`useAppUpdate`, a cada 5 min e ao
voltar para a aba) vai para o host do client web, não para as APIs. Tokens do
LiveKit são pedidos a server-channel só ao entrar na sala; o áudio e vídeo
depois disso não passam por server-channel.

### Folga

Com 120 RPM e 12 por minuto por instância no servidor aberto, uma conta
aguenta **10 instâncias** olhando o mesmo servidor ao mesmo tempo antes de o
RPM sozinho virar problema. Em segundo plano (1/min) o limite é irrelevante.

## Requisições por evento (rajadas)

Contadas no balde do servidor indicado. É aqui que o burst aperta.

**Em dev tudo dobra.** O `StrictMode` do React (`client/src/main.tsx`) monta,
desmonta e monta de novo cada efeito, e o fetch da primeira montagem sai
mesmo sendo descartado. Medido em 2026-09-25 abrindo o app com um servidor e
um canal de texto: 16 requisições ao server-channel selecionado em 100 ms.
Em produção o mesmo cenário faz 8 a 10. As tabelas abaixo são de produção.

### Abrir o app (login ou recarregar a página)

server-central:

| Requisição | Origem |
|---|---|
| `GET /api/me` | `useMyProfile` |
| `GET /api/me` | `useE2EKeys` (busca o perfil de novo para a chave E2E) |
| `GET /api/servers` | `useKnownServers` |
| `GET /api/friends` | `useFriends` |
| `GET /api/presence` | `useFriends` |
| `GET /api/presence/ws` (handshake) | `useFriends` |
| `POST /api/accounts/lookup` | `useAccountsBySubject`, 1 por lote de membros sem cache |
| `GET /api/avatars/{id}` | `UserAvatar`, **1 por avatar distinto visível** (cache em memória depois) |
| **Total** | **7 + avatares distintos** |

server-channel selecionado: soma de "Abrir um servidor" e "Abrir um canal"
abaixo. Cada server-channel em segundo plano: 1 (`GET /api/channels`).

### Abrir um servidor (clicar no rail)

| Requisição | Origem |
|---|---|
| `GET /api/me` | `useMe` |
| `PUT /api/me/profile-name` | `useMe`, só se o nome do perfil mudou desde a última vez |
| `GET /api/categories` + `GET /api/channels` | `useServerStructure` |
| `GET /api/voice/participants` | `useVoiceParticipants` |
| `GET /api/members` + `GET /api/roles` | `useServerMembers` |
| `GET /api/bans` | `useServerMembers`, só com permissão de banir |
| **Total** | **6 a 8** |

### Abrir um canal

| Tipo | Requisições | Total |
|---|---|---|
| Texto | `GET /api/channels/{id}/messages`, `GET /api/me` (`TextChannelView`), handshake `GET /api/channels/{id}/ws` | 3 |
| Forum | `GET /api/channels/{id}/threads`, `GET /api/me` (`ForumChannelView`), handshake do WS; abrir uma thread soma `GET /api/threads/{id}/messages` | 3 (+1 por thread) |
| Voz | `POST /api/channels/{id}/voice/token` ao entrar | 1 |

Anexos de imagem no histórico somam `GET /api/attachments/{id}`, 1 por anexo
exibido.

### Renovação do access token

O token dura 1 hora (`access_token_validity` do provider no Authentik) e o
client renova sozinho. O `accessToken` novo reinicia todos os efeitos que
dependem dele, então a cada hora acontece de novo a rajada de "abrir o app" em
server-central e a de "abrir um servidor" + "abrir um canal" no servidor
selecionado. O uso parado recarrega 2 fichas por segundo e gasta 0,2, então o balde
está cheio quando a renovação chega e passa sem 429.

### Antes da lista de servidores chegar

Nenhuma requisição a server-channel: os hooks que dependem do servidor
selecionado esperam o `baseUrl`. (Até 2026-09-25 `useServerStructure` não
esperava e fazia `GET /api/categories` e `/api/channels` relativos, que caíam
no host do client.)

## Cenários conferidos

A conta de cada cenário é por conta de usuário, no pior servidor envolvido.

| Cenário | Gasto | Resultado com 120 / 60 |
|---|---|---|
| Abrir o app com um servidor e um canal de texto | 9 a 11 no server-channel (16 em dev), 7 + avatares no central | Passa |
| Recarregar duas vezes seguidas em dev | 32 no server-channel | Passa (com o burst antigo de 20, dava 429; foi o que motivou a mudança) |
| Três abas do mesmo navegador abrindo juntas | 27 a 33 (48 em dev) | Passa |
| Mesma conta no celular, PC e notebook | balde separado por dispositivo | Passa; com a chave só por `sub`, os três dividiam o mesmo balde |
| Trocar de canal de texto a cada 1 s | 3 por troca, recarga de 2/s | Perde 1 ficha por segundo; depois de uns 40 s seguidos começa 429 |
| Trocar de canal de texto a cada 2 s | 3 por troca, recarga de 4 por troca | Sustentável indefinidamente |
| Lista de membros com 60 pessoas com avatar, cache vazio | 60 + 7 no central | **Estoura**: uns 7 avatares recebem 429 e caem para a inicial |
| 10 servidores no rail, um aberto | 12/min no aberto, 1/min em cada um dos outros | Passa com folga |
| Duas contas de teste no mesmo PC | balde separado por conta | Passa; com a chave por IP antiga, as duas dividiam o mesmo balde |

## Pontos de atenção

Os três primeiros viram 429 em uso legítimo e merecem correção no client; o
resto é folga que dá para recuperar se o orçamento apertar.

1. **Avatares em rajada** no server-central. Cada avatar distinto é uma
   requisição autenticada, disparadas todas juntas quando a lista de membros
   aparece. Com mais de uns 50 avatares novos na tela, parte falha. Caminhos:
   servir avatar por URL pública com hash (sem Bearer, cacheável pelo
   navegador), ou enfileirar as buscas com concorrência limitada.
2. **Várias abas do mesmo navegador abrindo juntas** somam as rajadas no
   mesmo balde (dispositivos diferentes não, ver as regras acima). Um client que recebe 429 hoje não tenta de novo (o histórico
   fica em erro até trocar de canal). Tratar 429 com uma nova tentativa depois
   do `Retry-After` resolve para todas as rotas de uma vez.
3. **Troca rápida de canal** gasta 3 fichas por clique.
4. **`GET /api/me` repetido.** `TextChannelView` e `ForumChannelView` buscam de
   novo o que `useMe` já tem em `App.tsx`; passar `me` por prop derruba o custo
   de abrir um canal de 3 para 2. No server-central, `useE2EKeys` repete a
   busca de `useMyProfile`.
5. **Poll de participantes de voz** é o maior gasto parado (metade dos 12 por
   minuto). Se o orçamento apertar, é o primeiro candidato a virar evento pelo
   WebSocket.

## Como conferir uma mudança

Antes de adicionar ou mudar requisição recorrente ou de abertura de tela:

1. Some o novo gasto na tabela certa acima (por minuto para poll, por evento
   para rajada).
2. Poll: o total por minuto de uma instância no servidor aberto deve ficar
   abaixo de 1/5 do RPM (24 com o padrão), para caberem cinco instâncias da
   mesma conta.
3. Rajada: a abertura mais cara (app + servidor + canal no mesmo balde) deve
   ficar abaixo de 1/3 do burst (20 com o padrão, contando a versão de
   produção), para caberem três instâncias da mesma conta abrindo juntas.
4. Se não couber, prefira evento pelo WebSocket, cache ou concorrência
   limitada a subir o limite. Subir `RATE_LIMIT_*` vale para quem hospeda a
   instância, mas o padrão é o que a maioria dos self-hosters vai usar.
