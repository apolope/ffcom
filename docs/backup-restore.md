# Backup e restore — FFCom

Guia de self-hosting para quem opera `server-central` e/ou `server-channel`. Nenhum dos dois componentes tem backup automático embutido — isso é responsabilidade de quem hospeda, mesma filosofia de "sem serviço extra escondido" já registrada nas outras decisões de deploy (ver [`architecture.md`](architecture.md), "Decisão: backup/restore — `pg_dump`/`pg_restore` via `docker compose exec`, sem WAL archiving").

## O que precisa ser backupeado

Cada componente (`server-central`, `server-channel`) tem sua própria fonte de dados, totalmente independente da do outro:

| Componente | Banco Postgres | Volume(s) de arquivo |
| --- | --- | --- |
| `server-central` | contas, perfis, amizades, diretório de servidores, DMs (ciphertext) | `avatars_data` (avatares de conta) |
| `server-channel` | categorias, canais, mensagens, roles/permissões, convites, membros | `attachments_data` (anexos de mensagem) |

Os volumes de arquivo guardam os bytes referenciados pelas linhas do Postgres (`accounts.avatar_id` → arquivo em `avatars_data`, `messages.attachment_id` → arquivo em `attachments_data`) — restaurar só o banco sem o volume correspondente deixa referências para arquivos que não existem mais (avatar/anexo quebrado na UI). Faça backup dos dois juntos, na mesma janela de tempo, para os dois ficarem consistentes entre si.

Os nomes de volume abaixo assumem o `docker-compose.yml` genérico de cada componente (`server-central/docker-compose.yml`, `server-channel/docker-compose.yml`), pensado para self-hosting. Quem usa outro compose (ex. `deploy/central/`, `deploy/channel/` — específico da infra `a3s-network`) tem nomes de serviço/volume diferentes; ajuste os comandos abaixo de acordo com o próprio arquivo (`docker compose config` mostra os nomes resolvidos).

## Backup do Postgres

Rodar a partir do diretório do componente (`server-central/` ou `server-channel/`), com o `docker compose up -d` já no ar. `pg_dump` roda dentro do próprio container `postgres`, sem exigir nenhuma porta exposta no host:

```
# server-central
docker compose exec -T postgres pg_dump -U ${POSTGRES_USER:-ffcom} -Fc ${POSTGRES_DB:-ffcom_central} > ffcom-central-$(date +%Y%m%d-%H%M).dump

# server-channel
docker compose exec -T postgres pg_dump -U ${POSTGRES_USER:-ffcom} -Fc ${POSTGRES_DB:-ffcom_channel} > ffcom-channel-$(date +%Y%m%d-%H%M).dump
```

`-Fc` (formato custom do `pg_dump`) gera um arquivo compactado e restaurável com `pg_restore`, inclusive em outra versão de Postgres 17.x, sem depender do `psql` batendo linha a linha um dump em SQL puro. Guarde `USER`/`DB` reais do `.env` se não estiverem exportados no shell atual (`ffcom`/`ffcom_central`/`ffcom_channel` são os padrões do compose, ver `docker-compose.yml`).

## Restore do Postgres

**Atenção:** restaurar sobrescreve o banco de destino. Rode contra uma instância vazia (ou aceite perder o estado atual dela).

```
# server-central — banco precisa existir e estar vazio antes do restore
docker compose exec -T postgres pg_restore -U ${POSTGRES_USER:-ffcom} -d ${POSTGRES_DB:-ffcom_central} --clean --if-exists < ffcom-central-20260922-0300.dump

# server-channel
docker compose exec -T postgres pg_restore -U ${POSTGRES_USER:-ffcom} -d ${POSTGRES_DB:-ffcom_channel} --clean --if-exists < ffcom-channel-20260922-0300.dump
```

`--clean --if-exists` faz o `pg_restore` derrubar as tabelas existentes antes de recriar — necessário para restaurar por cima de um banco que já tem o schema das migrations aplicado (o `app` aplica as migrations sozinho no boot, então um Postgres novo já não está "vazio" de schema assim que o container `app` sobe uma vez). Se estiver restaurando num Postgres completamente novo, suba só o `postgres` primeiro (`docker compose up -d postgres`), restaure, e só depois suba o `app`.

Depois do restore, reinicie o `app` (`docker compose restart app`) para garantir que nenhuma conexão do pool ficou presa a um estado anterior ao restore.

## Backup e restore dos volumes de arquivo

Volume nomeado do Docker, sem precisar parar o container que o usa (leitura de arquivos já fechados, sem lock exclusivo do Postgres envolvido aqui):

```
# backup — troque avatars_data / attachments_data pelo volume do componente
docker run --rm -v <projeto>_avatars_data:/data -v "$(pwd)":/backup alpine \
  tar czf /backup/ffcom-central-avatars-$(date +%Y%m%d-%H%M).tar.gz -C /data .

# restore — para dentro de um volume vazio
docker run --rm -v <projeto>_avatars_data:/data -v "$(pwd)":/backup alpine \
  tar xzf /backup/ffcom-central-avatars-20260922-0300.tar.gz -C /data
```

`<projeto>` é o prefixo que o Compose dá ao volume (nome do diretório por padrão, ex. `server-central_avatars_data`) — confirme o nome real com `docker volume ls`.

## Automação

Nenhum dos dois componentes agenda backup sozinho. Um cron simples no host, chamando os comandos de `pg_dump` acima e rotacionando arquivos antigos, já cobre a maioria dos casos de self-hosting:

```
# crontab -e — backup diário às 3h, mantém os últimos 7
0 3 * * * cd /caminho/para/server-channel && docker compose exec -T postgres pg_dump -U ffcom -Fc ffcom_channel > /backups/ffcom-channel-$(date +\%Y\%m\%d).dump && find /backups -name 'ffcom-channel-*.dump' -mtime +7 -delete
```

Guarde os backups fora da própria máquina que roda o `docker compose` (outro disco, outro host, object storage) — um backup que mora só no volume/disco que também guarda o banco original não protege contra falha de disco.

**Instância oficial de teste (`a3s-network`):** ainda não tem esse cron configurado — é um item de infra separado deste repositório, não coberto aqui.
