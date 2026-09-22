# Contribuindo com o FFCom

Guia prático para quem for mexer no código. Decisões técnicas (o quê e por
quê) ficam em [`docs/architecture.md`](docs/architecture.md); backlog fica
em [`TODO.md`](TODO.md) — leia os dois antes de começar, este documento não
duplica nenhum dos dois.

## Estrutura do projeto

Três componentes independentes, cada um com seu próprio módulo/`package.json`
e README com detalhes de stack, responsabilidade e como rodar:

- [`client/`](client/README.md) — React + TypeScript (Vite), web/PWA e
  Electron a partir da mesma base.
- [`server-central/`](server-central/README.md) — Go, instância única.
- [`server-channel/`](server-channel/README.md) — Go, self-hosted por
  comunidade.

Não existe canal direto `server-central` ↔ `server-channel` — os dois só se
relacionam através do `client` (ver `docs/architecture.md`, "Decisão:
protocolo entre client, server-central e server-channel"). Protocolo REST/
WebSocket entre os três está documentado em
[`docs/protocol.md`](docs/protocol.md).

## Ambiente de desenvolvimento

Caminho documentado (e testado) para cada componente é Docker Compose — ver
a seção "Rodando via Docker Compose" do README de `server-central` e
`server-channel`, e os scripts `npm run dev`/`dev:electron` do README de
`client`.

Para iterar mais rápido nos dois servidores Go sem rebuildar a imagem, `go
run .` funciona direto contra o Postgres já subido pelo `docker compose up
postgres` do componente — só ajuste `DATABASE_URL`/`SERVER_PORT` (o
`docker-compose.yml` de cada um mostra os valores exatos que o container usa)
e as demais variáveis do `.env.example` local.

## Convenções de código

Não repetidas aqui porque já estão registradas, decisão a decisão, em
`docs/architecture.md` — a leitura relevante antes de tocar em cada parte:

- **Go (`server-central`, `server-channel`):** sem ORM (`pgx`/`pgxpool` com
  SQL escrito à mão em `internal/store`), sem framework HTTP, migrations
  embutidas no binário via `//go:embed` e aplicadas automaticamente no boot,
  dependências deliberadamente enxutas (ex.: token do LiveKit assinado com
  `golang-jwt` em vez do SDK oficial — ver "Decisão: integração de voz com
  LiveKit"). Manter esse padrão em código novo; se uma dependência nova
  parecer necessária, checar primeiro se o motivo já foi discutido/revisitado
  em alguma decisão existente.
- **Client:** sem lib de estado/roteamento além do que já está em uso (nem
  `react-router`); `oidc-client-ts` puro para auth, sem wrapper adicional —
  ver "Decisão: login OIDC no client".
- **Config:** tudo via variável de ambiente (`.env`), nunca arquivo montado
  nem valor hardcoded — ver "Decisão: injeção de config no Docker Compose de
  server-channel". `.env` nunca é commitado; só o `.env.example`.

## Testes e lint

`.github/workflows/ci.yml` roda em todo `push`/`pull_request` (separado dos
workflows de deploy, que só disparam em tag — ver `docs/architecture.md`,
"Decisão: CI de testes/lint"): `go vet`/`go test` para `server-central` e
`server-channel`, `npm run lint`/`npm run build` para `client`. Rode os
mesmos comandos localmente antes de commitar, para não depender só do CI:

```
# server-central/ e server-channel/
go vet ./...
go test ./...

# client/
npm run lint
npm run build   # tsc -b pega erro de tipo que o lint sozinho não pega
```

## Commits

Mensagens em português, imperativo, sem prefixo de tipo (`feat:`/`fix:` etc.)
— ver `git log` para o padrão em uso, ex. `Corrigir escalonamento de
privilégio via ManageRoles em server-channel`, `Adicionar rate limiting por
IP em server-central`. Uma linha resume o quê; se o porquê não for óbvio,
ele vai para `docs/architecture.md`, não para o corpo do commit.

## Registrando decisões e atualizando o backlog

Ao fechar um item do `TODO.md`:

1. Marque como feito (`- [x]`).
2. Se alguma decisão de design foi tomada no processo e ainda não está
   documentada, registre em `docs/architecture.md` no mesmo formato já usado
   ("Decisão: ..." + alternativas consideradas + razão + quando revisitar).
3. Se a decisão bloquear outros itens do backlog, deixe isso explícito na
   entrada de `TODO.md` que referencia o tema.

Isso mantém `docs/architecture.md` como fonte viva em vez de exigir
reconstruir contexto de conversas ou commits antigos a cada retomada do
projeto.
