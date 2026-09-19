# server-channel

O "servidor" propriamente dito do FFCom: categorias, canais de texto/voz/forum. Autohospedado por quem cria a comunidade, na própria máquina ou num VPS. Ver decisões em [`../docs/architecture.md`](../docs/architecture.md).

## Stack planejada

- Go
- [LiveKit](https://github.com/livekit/livekit) (self-hosted) para voz/vídeo/compartilhamento de tela
- [coturn](https://github.com/coturn/coturn) para TURN/NAT traversal
- PostgreSQL
- WebSocket para texto, presença e sinalização
- Docker Compose como unidade de deploy (app + LiveKit + coturn + Postgres)

## Responsabilidade

- Estrutura de categorias e canais (texto, voz, forum).
- Mensagens de texto e posts de forum.
- Permissões/roles por servidor e por canal.
- Convites.
- Orquestração do LiveKit: criação de sala por canal de voz, emissão de token de acesso por permissão.

Ver [`../TODO.md`](../TODO.md), seção "server-channel", para o que falta. Implementado até aqui: modelo de dados completo, autenticação (validação local de JWT contra o Authentik central), `GET /api/me`, e canal de texto (`GET /api/channels/{id}/messages` para histórico REST paginado, `GET /api/channels/{id}/ws` para envio/recebimento em tempo real via WebSocket).

## Rodando via Docker Compose

```
cp .env.example .env
# edite .env: defina POSTGRES_PASSWORD, LIVEKIT_API_KEY/SECRET,
# TURN_EXTERNAL_IP e TURN_STATIC_AUTH_SECRET (gerar segredos: openssl rand -hex 32)
docker compose up -d
```

Sobe quatro serviços: `postgres`, `livekit`, `coturn` e `app` (o binário
`server-channel` em si, buildado a partir do `Dockerfile` local).

Se o servidor for hospedado atrás de NAT (ex. em casa), `TURN_EXTERNAL_IP`
precisa ser o IP público do host, e as portas do LiveKit (7880/tcp, 7881/tcp,
faixa UDP de mídia) e do coturn (3478 tcp/udp, faixa de relay UDP) precisam
estar encaminhadas no roteador — guia dedicado a isso ainda é TODO
(`../TODO.md`, seção Infraestrutura/DevOps).
