# Permissões do server-channel

Referência de todos os bits de permissão de um `server-channel`: o que cada um libera, onde é checado e como acrescentar um novo. As decisões por trás de cada bit ficam em [`architecture.md`](architecture.md). Este arquivo é o catálogo, e precisa ser atualizado junto com qualquer bit novo (ver [Como adicionar uma permissão](#como-adicionar-uma-permissão)).

## Como o sistema funciona

A permissão é um bitmask `int64` (`internal/permissions/permissions.go`), guardado em `roles.permissions` e nos `allow`/`deny` de `channel_role_overwrites`.

- **Permissão base** de um membro: OR das permissões de todas as roles dele, somada à role default `@everyone`, que todo membro tem sem precisar de linha em `member_roles` (`memberBasePermission`, `internal/httpapi/permissions.go`). A `@everyone` nasce com `ViewChannels | SendMessages | Voice` (7).
- **Permissão efetiva num canal:** a base, com os overwrites daquele canal aplicados. Primeiro soma todos os `allow` das roles do membro, depois remove todos os `deny` (`permissions.Effective`). Overwrite existe só por role, não por membro.
- **Dono do servidor** (`members.is_owner`, quem entrou primeiro) não é uma role: a permissão dele vale `Owner = -1` (todos os bits), passa em qualquer checagem e ignora overwrites.
- **`Administrator`** passa em qualquer checagem (`permissions.Has`) e ignora overwrites, como o `ADMINISTRATOR` do Discord.
- **Conceder bit:** quem tem `ManageRoles` só consegue criar ou editar role, ou atribuir role, com bits que ele mesmo já tem (`permissions.Grants`). Assim `ManageRoles` sozinho não vira `Administrator`.
- **Checagem na base ou no canal:** algumas permissões dizem respeito a um canal e são checadas na efetiva, então um overwrite mexe nelas. As outras dizem respeito ao servidor inteiro e são checadas só na base, onde overwrite não tem efeito. A coluna "Nível" da tabela abaixo diz qual é qual.

O client espelha os bits em `client/src/lib/permissions.ts` só para decidir o que mostrar. Quem decide de fato é sempre o servidor, em cada rota.

## Catálogo

| Bit | Valor | Nome no diálogo de roles | Nível | Libera |
|---|---|---|---|---|
| `ViewChannels` | 1 | Ver canais | canal | Ver o canal nas listas, ler o histórico, abrir o WebSocket, baixar anexos. Categoria sem nenhum canal visível some da lista. |
| `SendMessages` | 2 | Enviar mensagens | canal | Enviar mensagem, anexo, thread e post de fórum (checado ao conectar o WebSocket e no `POST` de mensagem com anexo). |
| `Voice` | 4 | Conectar e falar em voz | canal | Pedir token do LiveKit (`POST /api/channels/{id}/voice/token`). Entrar e falar são um bit só. |
| `ManageInvites` | 8 | Gerenciar convites | base | Criar, listar e revogar convites de qualquer pessoa. |
| `ManageRoles` | 16 | Gerenciar roles | base | CRUD de roles, atribuir/remover role de membro, overwrites de canal. Limitado por `Grants`. |
| `Administrator` | 32 | Administrador | base | Tudo, inclusive apagar mensagem de outra pessoa. Ignora overwrites. |
| `KickMembers` | 64 | Expulsar membros | base | `POST /api/members/{id}/kick`. |
| `BanMembers` | 128 | Banir membros | base | `POST /api/members/{id}/ban`, `GET /api/bans`, `DELETE /api/bans/{oidcSubject}`. |
| `ManageChannels` | 256 | Gerenciar categorias e canais (tudo, inclusive renomear) | base | Qualquer ação na estrutura: criar, renomear, ordenar/mover e apagar categoria e canal. É a única que permite **renomear**. |
| `CreateInvites` | 512 | Criar convites | base | Só gerar convite (`POST /api/invites`). Listar e revogar continuam exigindo `ManageInvites`. |
| `CreateChannels` | 1024 | Criar canais | base | `POST /api/channels`. |
| `ReorderChannels` | 2048 | Ordenar e mover canais entre categorias | base | `PUT /api/channels/order` (arrastar e soltar) e `PATCH /api/channels/{id}` sem `name` (só `position` e/ou `categoryId`). |
| `DeleteChannels` | 4096 | Excluir canais | base | `DELETE /api/channels/{id}`, que apaga mensagens, threads e anexos junto. |
| `CreateCategories` | 8192 | Criar categorias | base | `POST /api/categories`. |
| `ReorderCategories` | 16384 | Ordenar categorias | base | `PUT /api/categories/order` (arrastar e soltar) e `PATCH /api/categories/{id}` sem `name`. |
| `DeleteCategories` | 32768 | Excluir categorias | base | `DELETE /api/categories/{id}`. Os canais da categoria ficam sem categoria. |

### Estrutura (categorias e canais)

`ManageChannels` sempre vale todas as ações de estrutura. Os seis bits de 1024 a 32768 são fatias dele, para dar só parte do trabalho a alguém: quem pode organizar a ordem mas não apagar, ou quem pode criar canais mas não mexer nas categorias. Nenhuma fatia permite renomear, só `ManageChannels`. Por isso o `PATCH` que traz `name` exige `ManageChannels`, e o client manda só os campos que mudaram.

Quem tem qualquer bit de estrutura (`permissions.StructureBits`) vê também as categorias vazias em `GET /api/categories`. Sem isso, uma categoria recém-criada ficaria invisível justamente para quem vai pôr o primeiro canal nela.

Roles criadas antes desses bits existirem continuam como estavam: quem tinha `ManageChannels` segue podendo tudo, e ninguém ganhou fatia nenhuma automaticamente.

## Como adicionar uma permissão

1. **Bit novo sempre no fim** do bloco `const` em `server-channel/internal/permissions/permissions.go`. Reordenar ou inserir no meio muda o significado dos valores já gravados em `roles.permissions`. Cabem 63 bits úteis no `int64`.
2. **Checar na rota**, decidindo se o bit é de canal (usar a permissão efetiva, com overwrite) ou de servidor (usar a base). Responder 403 com o nome do bit na mensagem, que o client mostra.
3. **Espelhar no client:** `PERMISSIONS` em `client/src/lib/permissions.ts` (o valor tem que bater com o `1 << n` do Go) e uma linha em `PERMISSION_LABELS` em `client/src/components/ManageRolesDialog.tsx`, para aparecer nas configurações da role. `number` do JS faz bitwise em 32 bits, então, passando do bit 30, será preciso trocar para `BigInt`.
4. **Decidir o padrão da `@everyone`.** Se o bit precisar vir ligado para todo mundo, é preciso uma migration que o some em `roles.permissions` da role default. Se não, fica desligado e o dono liga quando quiser.
5. **Atualizar este arquivo** (catálogo e, se preciso, a seção temática), a tabela de rotas e a linha de bits em [`protocol.md`](protocol.md), e registrar a decisão em [`architecture.md`](architecture.md).
6. **Teste:** pelo menos um caso de 403 sem o bit e um de sucesso com ele. Os de estrutura estão em `server-channel/internal/httpapi/channels_admin_test.go`, que só roda com `FFCOM_TEST_DATABASE_URL` definida.
