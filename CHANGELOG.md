# Histórico de versões

O que cada versão publicada implementou, da mais nova para a mais antiga. Este arquivo é a fonte única do histórico: a home page (`https://ffcom.a3sitsolutions.com.br/#versoes`) lê este arquivo direto do `main`, e cada workflow de deploy recusa uma tag que não tenha a sua entrada aqui.

Cada componente tem versão própria (semver independente, ver `docs/architecture.md`), então as seções abaixo são por tag, não por data.

## Formato

Uma seção por tag publicada, no formato exato abaixo (o site e a checagem do CI dependem dele):

```
## <componente> v<X.Y.Z> · <AAAA-MM-DD>
- O que mudou, em linguagem de quem usa, uma linha por item.
```

`<componente>` é um de: `client` (o app), `central` (server-central), `channel` (server-channel), `channel-image` (imagem do container do server-channel), `site` (esta home page). Itens aceitam `código` entre crases e **negrito**. Texto fora das seções (como este) é ignorado pelo site.

## site v0.4.0 · 2026-09-27
- Nova seção "Participe" para pedir conta na instância oficial: nome, usuário, e-mail, apelido e o motivo. Antes e depois do envio, a página avisa que sem um server-channel próprio ou um convite só dá para usar as mensagens diretas.
- O FFCom agora é publicado sob a licença AGPL-3.0, com link para ela no rodapé e na seção "Onde estamos".

## central v0.10.0 · 2026-09-27
- Pedidos de cadastro pela home: cada pedido chega ao grupo da equipe no Telegram com botões de aprovar e reprovar. Aprovado, a conta é criada já com acesso ao FFCom, o apelido escolhido vira o nome exibido, e chega um e-mail para escolher a senha.
- O binário traz as licenças dos componentes de terceiros, que `--licenses` imprime.

## site v0.3.0 · 2026-09-27
- Moderadores podem excluir ideias em qualquer aba, com confirmação no segundo clique, e devolver uma ideia implementada ao ranking.

## central v0.9.0 · 2026-09-27
- Moderadores podem excluir uma ideia de vez, com os votos. Diferente de recusar, a exclusão devolve ao autor a sugestão do dia, se a ideia for de hoje.

## site v0.2.2 · 2026-09-26
- Quando o texto não é uma sugestão de melhoria, a varinha mostra o que falta sem trocar o texto, e um envio descartado volta para o campo com a explicação, para reescrever.

## central v0.8.0 · 2026-09-26
- A varinha e a checagem final passam a conferir se o texto é mesmo uma sugestão de melhoria. Opinião genérica ("aplicativo ruim"), teste ou pergunta sem proposta não é publicada: a pessoa recebe uma dica do que faltou e pode reescrever, sem perder a sugestão do dia (até 3 descartes por dia).
- Reclamação sobre um problema concreto (ex. áudio cortando) continua valendo e vira o pedido de resolvê-lo.

## site v0.2.1 · 2026-09-26
- Corrige o botão "Entrar para sugerir", que não levava ao login: a página deixa de buscar a configuração do Authentik pelo navegador, que era bloqueada.

## site v0.2.0 · 2026-09-26
- Seção "Ideias": sugestões de melhoria com login na conta do FFCom, ranking público por pontuação (like vale 2, dislike tira 1) e aba de ideias implementadas ligada ao histórico de versões.
- Varinha que melhora o texto da sugestão com o Claude, até 3 vezes por dia, mostrando ideias parecidas que já existem para votar nelas.
- Moderação para o grupo de administradores: aprovar ou recusar ideias retidas e marcar ideias como planejadas ou implementadas.

## central v0.7.0 · 2026-09-26
- Sugestões de melhoria: uma por pessoa por dia, votos com like e dislike, e estados planejada, implementada e recusada.
- Varinha e checagem final de conteúdo via Claude: palavras ofensivas são trocadas, sugestão ofensiva na varinha é descartada e custa 2 usos extras, e na checagem final vai para moderação.
- Grupo `ffcom-admins` do Authentik modera as sugestões.

## site v0.1.0 · 2026-09-26
- Primeira versão da home page em `ffcom.a3sitsolutions.com.br`: o que é o FFCom, o que já funciona, como funciona, como hospedar um servidor e a história do nome.
- Marca nova do projeto: dois "f" unidos pela mesma barra dentro de um balão de conversa.
- Histórico de versões lido direto do `CHANGELOG.md` do projeto no GitHub.

## client v0.14.1 · 2026-09-26
- Ícone novo: favicon e ícones do app instalado (PWA) com a marca do FFCom.

## client v0.14.0 · 2026-09-26
- Chat de texto e fórum reconectam sozinhos quando o servidor reinicia ou troca de versão, e recuperam as mensagens perdidas no intervalo.
- Renovar o login não derruba mais a conexão aberta do canal.

## channel v0.7.1 · 2026-09-26
- Primeira atualização automática real da instância oficial: o servidor trocou de versão sozinho, sem recriar o container.

## channel-image v1.0.0 · 2026-09-26
- Primeira imagem "evergreen" do servidor (`ghcr.io/apolope/ffcom-channel:1`): o container se atualiza sozinho, confere assinatura e hash de cada versão e volta para a anterior se a nova não subir.
- Funciona no primeiro boot mesmo sem acesso ao GitHub, com uma cópia do serviço embutida na imagem.

## channel v0.7.0 · 2026-09-26
- O servidor passa a ser publicado como binário assinado, num índice de versões que os containers consultam para se atualizar.
- Troca de versão a quente: as conexões de chat caem por alguns segundos e a voz não cai.

## client v0.13.0 · 2026-09-26
- Menu do próprio avatar mostra nome, usuário, e-mail e apelido.

## client v0.12.0 · 2026-09-26
- A chave das mensagens diretas cifradas fica guardada na conta, protegida por uma frase de recuperação: dá para trocar de navegador ou dispositivo sem perder as conversas.

## central v0.6.0 · 2026-09-26
- Guarda a chave de ponta a ponta da conta, cifrada pela frase de recuperação.

## central v0.5.1 · 2026-09-26
- Pedidos de amizade e lista de amigos mostram o nome da pessoa em vez de um identificador interno.

## client v0.11.0 · 2026-09-26
- Pedir amizade direto pela lista de membros de um servidor, com aviso e seção de pedidos recebidos.

## channel v0.6.0 · 2026-09-26
- Limite de requisições por usuário e por dispositivo, em vez de por endereço IP.

## central v0.5.0 · 2026-09-26
- Pedidos de amizade (enviar, aceitar, recusar) com aviso em tempo real para os dois lados.
- Limite de requisições por usuário e por dispositivo.

## client v0.10.0 · 2026-09-25
- Ordenar servidores, categorias e canais arrastando.
- Cores e barras de rolagem padronizadas no app todo.

## channel v0.5.0 · 2026-09-25
- Ordem de categorias e canais guardada no servidor, ajustável arrastando.
- Limite de requisições por usuário.

## central v0.4.0 · 2026-09-25
- Ordem da lista de servidores guardada na conta.
- Limite de requisições por usuário.

## client v0.9.0 · 2026-09-23
- Avatar e status de presença nas listas de membros e amigos.
- Recorte do avatar antes do envio, com prévia do resultado.

## channel v0.4.0 · 2026-09-23
- Lista de membros traz avatar e nome do perfil quando a pessoa não tem apelido.

## central v0.3.0 · 2026-09-23
- Avatar e status de presença disponíveis para as listas do app.

## client v0.8.1 · 2026-09-23
- Som ao apertar e soltar a tecla do push-to-talk.

## client v0.8.0 · 2026-09-23
- Supressão de ruído reforçada no microfone (RNNoise).
- Push-to-talk como alternativa ao microfone aberto.
- Volume por pessoa e "silenciar para mim" no canal de voz.

## client v0.7.0 · 2026-09-23
- Atalho de teclado configurável para mutar o microfone, com aviso sonoro.
- Botão Convidar só aparece para quem pode convidar.

## channel v0.3.0 · 2026-09-23
- Permissão de criar convites separada da de gerenciar convites.
- Quem está em cada sala de voz aparece na barra lateral.
- Servidor TURN repassado aos clients pelo LiveKit, para chamadas atravessarem redes mais fechadas.

## client v0.6.2 · 2026-09-23
- Aplicar a atualização do app sempre recarrega a página, mesmo no primeiro acesso.

## client v0.6.1 · 2026-09-23
- Botão de clipe para anexar arquivo no chat.
- Aviso "Ativar som" quando o navegador bloqueia o áudio da chamada.
- Botão Copiar sempre visível no diálogo de convite.

## client v0.6.0 · 2026-09-23
- Botão de atualizar o app quando sai versão nova, sem recarregar sozinho no meio de uma chamada.

## client v0.5.1 · 2026-09-23
- Correção no áudio compartilhado junto com a tela.

## client v0.5.0 · 2026-09-23
- Compartilhar o áudio junto com a tela.

## client v0.4.1 · 2026-09-22
- Botões do cabeçalho do servidor quebram de linha em vez de cortar o último.

## client v0.4.0 · 2026-09-22
- Ampliar o vídeo de uma pessoa ou tela no canal de voz.
- Indicador de não lida nos servidores da barra lateral.
- Permissões por canal para cada cargo.
- Quem está em cada sala de voz aparece na barra lateral.
- Administradores podem apagar mensagens de outros membros.

## client v0.3.0 · 2026-09-22
- Botão de sair da conta.
- Criar, renomear e apagar categorias e canais.

## channel v0.2.0 · 2026-09-22
- Criar, renomear e apagar categorias e canais, com permissão própria.

## central v0.2.0 · 2026-09-22
- Chave de ponta a ponta e marcação de "lido" separadas por conta, para duas contas no mesmo navegador não se misturarem.

## client v0.2.1 · 2026-09-22
- O canal selecionado não volta mais para o primeiro a cada atualização da estrutura do servidor.

## client v0.2.0 · 2026-09-22
- Câmera no canal de voz.

## channel v0.1.1 · 2026-09-22
- Correção do endereço anunciado para a mídia de voz, que impedia as chamadas de conectar na instância oficial.

## client v0.1.1 · 2026-09-22
- Republicação da 0.1.0, que tinha sido barrada por um falso positivo na checagem de segredos.

## client v0.1.0 · 2026-09-22
- Primeira versão publicada do app: login, servidores, canais de texto, voz e fórum, amigos e mensagens diretas.
- Mensagens diretas cifradas de ponta a ponta.
- Editar e apagar mensagens, anexar arquivos e imagens, indicador de não lidas.
- Avatar da conta, expulsão e banimento de membros.
- Funciona no navegador, instalável como app (PWA), e empacotável para desktop (Windows, macOS, Linux).

## channel v0.1.0 · 2026-09-22
- Primeira versão publicada do servidor: categorias, canais de texto, voz e fórum, cargos e permissões, convites.
- Anexos em mensagens, edição e exclusão, expulsão e banimento, limite de requisições.
- Conexões exigem HTTPS fora de localhost.

## central v0.1.0 · 2026-09-22
- Primeira versão publicada do servidor central: conta, perfil, amigos, lista de servidores e mensagens diretas cifradas de ponta a ponta.
- Upload de avatar, indicador de não lidas e limite de requisições.
