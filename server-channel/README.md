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

## Hospedando atrás de NAT (ex. em casa)

A maioria de quem autohospedar um `server-channel` vai estar numa rede
residencial: sem IP público fixo e atrás do roteador do provedor. Dois
ajustes são necessários além de preencher o `.env`.

### Port-forwarding

Encaminhe estas portas no roteador para o IP interno (LAN) da máquina que
roda o `docker compose` — todas vêm do próprio `docker-compose.yml` deste
diretório:

| Porta (host) | Protocolo | Serviço | Variável no `.env` |
| --- | --- | --- | --- |
| 8080 | TCP | `app` (API REST + WebSocket) | `SERVER_CHANNEL_PORT` |
| 7880 | TCP | `livekit` (sinalização) | fixa no compose |
| 7881 | TCP | `livekit` (RTC fallback via TCP) | fixa no compose |
| 50000–50100 | UDP | `livekit` (mídia RTC) | `LIVEKIT_RTC_PORT_RANGE_START`/`_END` |
| 3478 | TCP + UDP | `coturn` (TURN/STUN) | fixa no compose |
| 49160–49200 | UDP | `coturn` (relay) | `TURN_RELAY_MIN_PORT`/`_MAX_PORT` |

Se alguma dessas portas já estiver em uso na rede (ex. outro serviço no
mesmo roteador), ajuste a variável correspondente no `.env` e o mapeamento
no roteador juntos — o compose já usa `${VAR}` dos dois lados (porta do
host e `--listening-port`/flags do coturn), então não precisa editar o
`docker-compose.yml`.

### DNS dinâmico

IP residencial normalmente muda de tempos em tempos (reconexão do modem,
DHCP do provedor). Como `LIVEKIT_PUBLIC_URL` e o endereço que os membros
usam para adicionar o servidor (`AddServerDialog` no client) precisam
apontar para um host estável, configure um serviço de DNS dinâmico (ex.
[DuckDNS](https://www.duckdns.org/), No-IP, Dynu — muitos roteadores
domésticos já têm cliente DDNS embutido nas configurações) e use o
hostname resultante em vez do IP bruto:

- `LIVEKIT_PUBLIC_URL=ws://seu-host.duckdns.org:7880` (ou `wss://` se
  houver TLS na frente, fora do escopo deste compose de referência).
- Endereço divulgado aos membros: `http://seu-host.duckdns.org:8080`.

O LiveKit descobre sozinho seu IP público via STUN (`use_external_ip: true`
já configurado no compose) — nenhuma ação manual necessária aí, mesmo com
IP dinâmico.

**Limitação conhecida:** `TURN_EXTERNAL_IP` do coturn precisa ser um IP
literal (o `--external-ip` do `turnserver` não resolve hostname a cada
alocação de relay) — não dá pra apontar para o hostname DDNS diretamente.
Se seu IP público mudar, atualize `TURN_EXTERNAL_IP` no `.env` e rode
`docker compose up -d coturn` para recriar o serviço com o IP novo. Quem
tiver IP público praticamente estável na prática (a maioria dos planos
residenciais só muda em reconexões raras) pode tratar isso como manutenção
ocasional em vez de automatizar; automatizar (script que compara o IP
público atual e reinicia o `coturn` quando muda) fica como melhoria futura
se virar fricção real.
