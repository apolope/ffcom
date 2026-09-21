# FFCom

Alternativa self-hosted ao Discord: organização em servidores, categorias e canais (texto, voz, forum), mas cada "servidor" é hospedado por quem cria a comunidade, via IP ou DNS próprio, em vez de depender de uma empresa central controlando tudo.

## Arquitetura

```
                         ┌────────────────────────┐
                         │      server-central     │
                         │  ffcom.a3sitsolutions   │
                         │  .com (instância única) │
                         │                          │
                         │  login, perfil, amigos,  │
                         │  lista de servidores     │
                         └───────────┬──────────────┘
                                     │ auth / diretório
                                     │
                         ┌───────────▼──────────────┐
                         │          client           │
                         │  React + TS, mesma base   │
                         │  web (PWA) + Electron     │
                         └───────────┬──────────────┘
                                     │ conecta via IP/DNS
                                     │ (um client fala com N server-channel)
                 ┌───────────────────┼───────────────────┐
                 │                   │                   │
        ┌────────▼────────┐ ┌────────▼────────┐ ┌────────▼────────┐
        │  server-channel  │ │  server-channel  │ │  server-channel  │
        │  self-hosted por │ │  self-hosted por │ │  self-hosted por │
        │  cada comunidade │ │  cada comunidade │ │  cada comunidade │
        │                  │ │                  │ │                  │
        │  categorias,     │ │  categorias,     │ │  categorias,     │
        │  canais texto/   │ │  canais texto/   │ │  canais texto/   │
        │  forum, + LiveKit│ │  forum, + LiveKit│ │  forum, + LiveKit│
        │  (voz/vídeo)     │ │  (voz/vídeo)     │ │  (voz/vídeo)     │
        └──────────────────┘ └──────────────────┘ └──────────────────┘
```

- **[client](client/)** — interface gráfica, web + desktop (Electron) a partir da mesma base de código.
- **[server-central](server-central/)** — instância única oficial em `ffcom.a3sitsolutions.com`: login, perfil, lista de amigos, diretório de servidores.
- **[server-channel](server-channel/)** — o "servidor" propriamente dito, hospedado por quem cria a comunidade (categorias, canais de texto/voz/forum).

## Documentação

- [`docs/architecture.md`](docs/architecture.md) — decisões técnicas, alternativas consideradas e por quê (formato ADR).
- [`docs/protocol.md`](docs/protocol.md) — protocolo/API entre `client`, `server-central` e `server-channel`.
- [`TODO.md`](TODO.md) — backlog unificado por tema.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — ambiente de desenvolvimento, convenções de código, testes/lint e processo de contribuição.

## Status

Implantação de teste no ar desde 2026-09-21 (`app.ffcom.a3sitsolutions.com.br`, infra do `a3s-network`), com login OIDC, chat de texto, voz/vídeo, forum, roles/permissões, convites e DMs funcionando ponta a ponta. Restam principalmente testes com múltiplos usuários reais e a decisão do domínio de produção definitivo (`.com` vs `.com.br`) — ver `TODO.md` para o detalhe.
