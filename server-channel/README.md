# server-channel

O "servidor" propriamente dito do FFCom: categorias, canais de texto/voz/forum. Autohospedado por quem cria a comunidade, na própria máquina ou num VPS. Ver decisões em [`../docs/architecture.md`](../docs/architecture.md).

## Stack planejada

- Go
- [LiveKit](https://github.com/livekit/livekit) (self-hosted) para voz/vídeo/compartilhamento de tela
- TURN embutido do LiveKit para NAT traversal
- PostgreSQL
- WebSocket para texto, presença e sinalização
- Docker Compose como unidade de deploy (app + LiveKit + Postgres)

## Responsabilidade

- Estrutura de categorias e canais (texto, voz, forum).
- Mensagens de texto e posts de forum.
- Permissões/roles por servidor e por canal.
- Convites.
- Orquestração do LiveKit: criação de sala por canal de voz, emissão de token de acesso por permissão.

Ver [`../TODO.md`](../TODO.md), seção "server-channel", para o estado atual item a item — hoje já cobre canais de texto/voz/forum, permissões/roles, overwrites por canal, convites e lista de membros; o que resta é principalmente teste de ponta a ponta com múltiplos usuários reais (ver seção "Primeira implantação de teste" do TODO).

## Rodando via Docker Compose

```
cp .env.example .env
# edite .env: defina POSTGRES_PASSWORD e LIVEKIT_API_KEY/SECRET
# (gerar segredos: openssl rand -hex 32)
docker compose up -d
```

Sobe três serviços: `postgres`, `livekit` e `app`. O `app` não é
compilado na sua máquina: o compose puxa a imagem publicada
`ghcr.io/apolope/ffcom-channel:1`, e o serviço `server-channel` dentro dela
se atualiza sozinho (ver "Atualizações" abaixo). Nada de Go ou de clone
compilável é necessário; basta este diretório com o `docker-compose.yml` e o
`.env`.

Desde `channel-image-v1.1.0` a imagem sai para `linux/amd64` e
`linux/arm64` (Raspberry Pi, VPS ARM); o `docker pull` escolhe a variante
da máquina sozinho. A `1.0.0` é só `linux/amd64`.

Para desenvolver no código do `server-channel` (compilar o que está no
disco em vez de usar a imagem publicada), o caminho é o override
`docker-compose.dev.yml`, descrito em
[`../CONTRIBUTING.md`](../CONTRIBUTING.md). Ele não serve para hospedar.

## Atualizações

A imagem e o serviço têm versões separadas. A tag da imagem (`:1`) é a
versão do **contrato** do container: variáveis de ambiente, portas, volumes e
healthcheck. Ela muda raramente e só quebra compatibilidade quando o número
principal muda (`1` → `2`). Existe também `:latest`, que segue a imagem mais
nova e por isso pode pular para um número principal novo sem aviso; para
hospedar, prefira `:1`. O serviço (`server-channel` `0.7.0`, `0.7.1`,
...) é baixado pelo próprio container: um lançador (`ffcom-runtime`) roda
como processo principal, consulta um índice de versões assinado publicado
nos releases do GitHub, confere assinatura e sha256 do binário, troca a
versão em execução e volta para a anterior se a nova não subir.

A primeira checagem acontece logo depois que o serviço sobe, e as seguintes
a cada `FFCOM_UPDATE_INTERVAL` (mais uma folga aleatória de até 25%, para
nem todo servidor atualizar no mesmo minuto). A troca derruba as conexões de
chat por alguns segundos; o client reconecta sozinho e a voz, que vai direto
ao LiveKit, não cai. A imagem traz uma cópia do serviço (a "semente"), então
o primeiro boot funciona mesmo sem acesso ao GitHub.

Por padrão o servidor recebe sozinho só as correções (patches) do minor que
veio com a imagem: uma imagem com semente `0.7.x` acompanha `0.7.1`, `0.7.2`
etc., mas não sobe para `0.8.0`. Mudanças de minor chegam quando você
atualiza a imagem e ela traz uma semente de minor mais novo (o container sobe
direto nela) ou quando você escolhe outra trilha.

### Variáveis

Todas opcionais, no `.env`; vazias valem o padrão. Valor inválido impede o
container de subir, com o erro no log.

| Variável | Valores | Padrão |
| --- | --- | --- |
| `FFCOM_CHANNEL_VERSION` | vazio: o minor da semente da imagem (ex. `0.7`); `X.Y`: só patches desse minor; `X.Y.Z`: fixa nessa versão; `latest`: qualquer versão nova, inclusive de minor | vazio |
| `FFCOM_AUTO_UPDATE` | `true` troca sozinho; `false` só registra no log que há versão nova | `true` |
| `FFCOM_UPDATE_INTERVAL` | duração no formato do Go (`30m`, `6h`); abaixo de `1m` vira `1m` | `1h` |

Mudou alguma delas no `.env`? Recrie o container para ela valer:
`docker compose up -d app`.

Forks que publicam o próprio índice usam `FFCOM_RELEASE_INDEX_URL` (a URL
base onde ficam `index.json` e `index.json.sig`) e `FFCOM_RELEASE_PUBKEY` (a
chave pública ed25519 em base64, gerada por `cmd/release-tool keygen`, que
substitui a chave embutida; o log avisa quando ela está em uso). Essas duas
não estão no `docker-compose.yml` de referência: acrescente-as ao
`environment` do `app` se precisar.

### Checar agora

Para não esperar o intervalo (ex. logo depois de um release):

```
docker compose kill -s HUP app
```

O container continua rodando; o sinal só antecipa a checagem.

### Qual versão está rodando

`docker ps` mostra só a tag da imagem. A versão do serviço aparece no
`/healthz` (na porta de `SERVER_CHANNEL_PORT`, `8080` por padrão), que não
exige login e continua acessível com `REQUIRE_TLS=true`:

```
curl -s http://localhost:8080/healthz
# {"status":"ok","version":"0.7.1"}
```

e nas linhas do lançador no log, todas começando com `ffcom-runtime:`
(trilha em uso, `server-channel 0.7.1 no ar`, `troca concluída: 0.7.0 →
0.7.1`, avisos e rollbacks):

```
docker compose logs app | grep ffcom-runtime:
```

### Fixar ou voltar de versão

Com `FFCOM_CHANNEL_VERSION=X.Y.Z` o servidor fica nessa versão e não
atualiza mais sozinho até você mudar a variável. É também o jeito de voltar
para uma versão anterior: fixe a versão desejada, rode `docker compose up -d
app` e, logo depois de subir, o lançador troca para ela (só a trilha fixa
permite descer; com `X.Y` ou `latest` a troca é sempre para frente). A
versão precisa existir no índice e `FFCOM_AUTO_UPDATE` não pode estar em
`false`, que bloqueia qualquer troca, inclusive essa.

Voltar uma versão não desfaz migrations do banco. As migrations seguem a
regra expand/contract (uma migration nunca quebra o binário da versão
anterior), e o serviço sobe normalmente num banco que está à frente dele,
então voltar **uma** versão é seguro; o schema continua o da mais nova.
Voltar várias versões não tem essa garantia.

Para sair do pin, esvazie a variável (ou ponha `X.Y`/`latest`) e rode
`docker compose up -d app` de novo.

### Rollback automático

O lançador desfaz sozinho uma troca que deu errado, em dois casos:

- **A versão nova não sobe:** se ela não responde `/healthz` com a versão
  esperada em até 60s (ou sai antes disso), o lançador para ela e volta para
  a anterior, sem sair do container.
- **A versão nova sobe, mas fica caindo:** se, nas 24h seguintes à troca,
  ela sai sozinha 5 vezes (seguidas ou espalhadas desde a troca), o lançador
  volta para a anterior e registra `ROLLBACK` no log. Isso não vale para uma
  versão fixada com `X.Y.Z` nem para a versão que veio da semente da imagem;
  nesses casos, e numa versão que já roda há mais de 24h, 5 quedas rápidas
  seguidas fazem o lançador sair com erro para o Docker reiniciar o
  container (`restart: unless-stopped`).

Nos dois casos a versão recusada vai para o arquivo `/data/runtime/bad`, uma
por linha, e não é tentada de novo: o servidor espera sair uma versão mais
nova (nem um pin `X.Y.Z` a aceita enquanto estiver ali). Falha de rede, índice
indisponível ou assinatura inválida não marcam nada; só adiam a atualização
para a próxima checagem. Se você corrigiu a causa (ex. configuração) e quer
tentar de novo uma versão recusada, tire a linha dela e force a checagem:

```
docker compose exec app sed -i '/^0\.7\.1$/d' /data/runtime/bad
docker compose kill -s HUP app
```

### Quando uma versão exige imagem nova

Cada versão do serviço declara a versão mínima do contrato que precisa. Se
sair, dentro da sua trilha, uma versão que a sua imagem não atende, o
servidor continua na mais nova que ela aceita e o log mostra, a cada
checagem:

```
ffcom-runtime: versão 0.9.0 existe mas exige imagem com runtime >= 1.1.0 (esta é 1.0.0); atualize a imagem para recebê-la
```

Dentro do mesmo número principal, basta puxar a imagem de novo (a tag `:1`
acompanha todo o contrato 1.x):

```
docker compose pull app
docker compose up -d app
```

Se a exigência for de outro número principal (ex. runtime `>= 2.0.0`), a
tag no `docker-compose.yml` precisa mudar para `:2`; leia antes as notas do
release da imagem, porque major é quebra de contrato (porta ou volume novo,
por exemplo). Vale puxar a imagem de tempos em tempos mesmo sem esse aviso:
é por ela que chegam as correções de segurança da base Alpine.

O volume `runtime_data` (`/data/runtime`) guarda os binários baixados
(a versão ativa e a anterior), a versão ativa (`current`) e a lista `bad`.
Não precisa de backup: sem ele, o container recomeça pela semente da imagem
e baixa o que faltar pelo índice.

## Migrando da imagem antiga

Quem já hospeda com a imagem antiga (`ghcr.io/apolope/ffcom-channel:0.x`,
em que cada tag era uma versão do serviço) ou com o compose antigo que
compilava localmente (`build: .`) precisa de uma migração única. O banco não
muda de lugar; a novidade é que a imagem nova roda como usuário não-root
(UID `10001`) e os anexos gravados pela antiga pertencem a `root`.

1. Faça backup do banco e dos anexos antes
   ([`../docs/backup-restore.md`](../docs/backup-restore.md)). A versão nova
   aplica migrations no primeiro boot, e as imagens `0.x` recusam subir
   num banco com migration que não conhecem; o caminho de volta é o backup.
2. Atualize o `docker-compose.yml`: `git pull` num clone deste repositório
   já traz o novo. Se você mantém um compose próprio, no serviço `app`:
   troque `build: .` ou `image: ...:0.x` por `image:
   ghcr.io/apolope/ffcom-channel:1`; acrescente `restart: unless-stopped` e
   `stop_grace_period: 40s` (o serviço leva até 25s para encerrar com
   calma, e os 10s padrão do Docker cortariam no meio); monte um volume novo
   em `/data/runtime` (`runtime_data:/data/runtime`, declarado também em
   `volumes:` no topo); e, se quiser, repasse `FFCOM_CHANNEL_VERSION`,
   `FFCOM_AUTO_UPDATE` e `FFCOM_UPDATE_INTERVAL`, como no compose de
   referência.
3. Pare o `app` e descubra o nome real do volume de anexos (o Compose
   prefixa com o nome do projeto, por padrão o do diretório, ex.
   `server-channel_attachments_data`):

   ```
   docker compose stop app
   docker volume ls | grep attachments
   ```

   Ou, direto pelo container antigo, os volumes montados e onde:

   ```
   docker inspect -f '{{range .Mounts}}{{.Name}} -> {{.Destination}}{{println}}{{end}}' $(docker compose ps -aq app)
   ```

4. Passe os anexos para o UID da imagem nova (troque o nome pelo que
   apareceu acima):

   ```
   docker run --rm -v server-channel_attachments_data:/d alpine chown -R 10001:10001 /d
   ```

   Com bind mount em vez de volume nomeado, é `sudo chown -R 10001:10001` no
   diretório do host. Sem isso, o serviço sobe mas falha ao gravar anexo
   novo.
5. Puxe a imagem e suba:

   ```
   docker compose pull app
   docker compose up -d
   ```

6. Confira `curl -s http://localhost:8080/healthz` e as linhas
   `ffcom-runtime:` do log (seção "Atualizações"). No primeiro boot o
   volume `runtime_data` está vazio e o lançador usa a semente da imagem.

A imagem antiga ficava com o nome gerado pelo build (ex.
`server-channel-app`) ou com a tag `0.x`; depois de confirmar que está tudo
no ar, dá para apagá-la com `docker image rm`.

## Notificações push (`FFCOM_CENTRAL_URL`)

Quem usa o app Android recebe notificação de mensagem nova com o app
fechado. O Firebase, que entrega a notificação no celular, só aceita pedidos
do `server-central` oficial; este servidor não guarda credencial nenhuma do
Google. O caminho é este:

1. O app pede ao `server-central` um token de notificação para este servidor
   (só sai para servidor que está na lista da pessoa) e o entrega aqui
   (`PUT /api/me/push-grant`). Ele fica guardado na tabela
   `member_push_grants`, um por membro, e é apagado num kick ou ban.
2. A cada mensagem nova, depois de entregar a mensagem a quem está conectado,
   o servidor monta numa fila separada a lista de quem deve ser avisado:
   membros com token, que podem ver o canal (`ViewChannels`), menos o autor e
   menos quem está com aquele canal aberto agora.
3. Manda ao `server-central` (`POST /api/push/notify`) os tokens dessas
   pessoas, o endereço do servidor, o canal (id e nome), o nome de quem
   escreveu e o texto da mensagem (ou o nome do anexo). O central confere os
   tokens e os silêncios de cada pessoa e repassa ao Firebase. **O texto só
   passa em trânsito:** o central não grava nada da mensagem.

Nada disso atrasa a mensagem: se o central estiver fora do ar ou lento, a
conversa segue normal e só a notificação se perde, com uma linha no log.

| Variável | Valores | Padrão |
| --- | --- | --- |
| `FFCOM_CENTRAL_URL` | vazio: a instância oficial (`https://central.ffcom.a3sitsolutions.com.br`); uma URL: outro `server-central` (fork ou instância de testes); `off`: não manda nada ao central, e ninguém recebe push deste servidor | vazio |

Em desenvolvimento local, use `off` ou o central local
(`http://host.docker.internal:8082` com o `scripts/dev-local.ps1`), para não
mandar tokens de teste à instância oficial.

## TLS / HTTPS

O `app` deste compose fala HTTP puro na porta `8080` (mesmo valendo para
`livekit`) — não há terminação TLS embutida no binário. Para expor
publicamente (fora de teste em LAN), coloque um proxy reverso na frente que
termine TLS e encaminhe para `localhost:8080`. Quem não tem preferência
formada, [Caddy](https://caddyserver.com/) é o caminho mais simples: emite e
renova certificado Let's Encrypt sozinho, sem passo manual, a partir de um
`Caddyfile` de poucas linhas:

```
seu-host.duckdns.org {
	reverse_proxy localhost:8080
}
```

(Nginx Proxy Manager ou Traefik funcionam igual se você já usa um deles para
outros serviços — a única exigência é que o WebSocket de `GET
/api/channels/{id}/ws` seja repassado com upgrade de conexão, o que os três
fazem por padrão.)

Depois de trocar para HTTPS, atualize dois lugares que passam a apontar para
o novo esquema/host: `CORS_ALLOWED_ORIGINS` (se o `client` também mudar de
origem) e o endereço divulgado aos membros ao gerar convites
(`InviteServerDialog` no client usa o que estiver na barra de endereço/base
URL configurada, não precisa de variável própria aqui).

Com o proxy no ar, ligue `REQUIRE_TLS=true` no `.env` — o `app` passa a
recusar (`426 Upgrade Required`) qualquer requisição em que o proxy não
sinalize `X-Forwarded-Proto: https` (Caddy, Nginx Proxy Manager e Traefik
fazem isso por padrão), fechando o caso de alguém expor `8080` direto sem
TLS nenhum. Deixe desligado (padrão) enquanto testar em LAN sem proxy.

## Backup / restore

Nenhum backup automático embutido — ver [`../docs/backup-restore.md`](../docs/backup-restore.md) para o procedimento de `pg_dump`/`pg_restore` do banco e dos anexos (`attachments_data`).

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
| 3478 | UDP | `livekit` (TURN) | `TURN_UDP_PORT` |

Se alguma dessas portas já estiver em uso na rede (ex. outro serviço no
mesmo roteador), ajuste a variável correspondente no `.env` e o mapeamento
no roteador juntos — o compose já usa `${VAR}` dos dois lados (porta do
host e do container), então não precisa editar o
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

O TURN também usa esse IP descoberto, então não há IP para configurar à mão
nem para atualizar quando ele muda. O relay do TURN manda a mídia para o
próprio IP público, o que exige NAT loopback (hairpin) no roteador; a maioria
dos roteadores domésticos tem. Sem ele, só quem depende do relay (redes que
bloqueiam UDP direto às portas de mídia) fica sem voz.
