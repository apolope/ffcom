# Histórico de versões

O que cada versão publicada implementou, da mais nova para a mais antiga. Este arquivo é a fonte do histórico, e o [`CHANGELOG.en.md`](CHANGELOG.en.md) é a tradução em inglês: toda entrada nasce aqui e é traduzida lá no mesmo commit, com as mesmas seções na mesma ordem (mesmo componente, versão e data). A home page (`https://ffcom.a3sitsolutions.com.br/#versoes`) lê o arquivo do idioma de quem visita direto do `main`, cada workflow de deploy recusa uma tag que não tenha a sua entrada nos dois arquivos (`scripts/check-changelog.sh`), e as releases do GitHub (`client` e `channel`) levam as duas versões, português primeiro e inglês embaixo (`scripts/changelog-notes.sh`).

Cada componente tem versão própria (semver independente, ver `docs/architecture.md`), então as seções abaixo são por tag, não por data.

## Formato

Uma seção por tag publicada, no formato exato abaixo (o site e a checagem do CI dependem dele; a linha do título é idêntica nos dois arquivos, só os itens são traduzidos):

```
## <componente> v<X.Y.Z> · <AAAA-MM-DD>
- O que mudou, em linguagem de quem usa, uma linha por item.
```

`<componente>` é um de: `client` (o app), `central` (server-central), `channel` (server-channel), `channel-image` (imagem do container do server-channel), `site` (esta home page). Itens aceitam `código` entre crases e **negrito**. Texto fora das seções (como este) é ignorado pelo site.

## client v0.24.6 · 2026-10-09
- Quem usa o FFCom no navegador do Android ou do Windows ganha um botão no rail, abaixo do de atualizar, que instala o app do sistema: no Android explica antes de baixar o APK (permitir a fonte, o aviso do Play Protect e remover o atalho do navegador depois); no Windows baixa o instalador e explica o aviso do SmartScreen.
- O "x" no canto do botão esconde a oferta por 30 dias neste navegador, e o item "Instalar o app para ..." continua no menu do avatar. O botão não aparece dentro dos apps.
- No Chrome do Android, o botão some quando o app já está instalado (a partir desta versão do app).

## client v0.24.5 · 2026-10-09
- Uma queda curta de rede durante a chamada não tira mais você da sala: o app reconecta sozinho, inclusive no Android com a tela apagada. Enquanto isso, a chamada e a notificação mostram "Reconectando…", e o microfone volta como estava.

## client v0.24.4 · 2026-10-09
- No app Android, canais silenciados mostram um 🔕 depois do nome, e um servidor silenciado mostra o 🔕 no ícone e no topo da lista de canais.

## client v0.24.3 · 2026-10-09
- As notificações do celular chegam mesmo com o FFCom aberto e escondido numa aba ou minimizado no computador. Só deixa de notificar quem está com o canal na tela, com a janela em foco e sem estar ausente.
- No app Android, um sino no topo do canal silencia as notificações dele.
- No app Android, o botão de atualização volta a aparecer quando sai uma versão nova do app.

## channel v0.11.2 · 2026-10-09
- As notificações do celular chegam mesmo com o FFCom aberto e escondido numa aba ou minimizado no computador: o servidor só deixa de notificar quem está de fato vendo o canal, com a tela visível, a janela em foco e sem estar ausente. Antes, qualquer canal aberto em qualquer aparelho segurava a notificação.
- O log do servidor ganhou uma linha por mensagem com quantas pessoas o push considerou, quantas estavam vendo o canal e quantas foram notificadas, só com contagens.

## central v0.15.2 · 2026-10-09
- O log do servidor ganhou uma linha por aviso de mensagem com quantos tokens de push chegaram, quantos eram desconhecidos, silenciados ou acima do limite e quantos viraram notificação, só com contagens.

## client v0.24.2 · 2026-10-09
- No app Android, as notificações mostram a foto de quem mandou, nas mensagens dos canais, nas mensagens diretas e nos avisos de amizade. Quem não tem foto continua com a letra. (A v0.24.1, com a mesma mudança, não chegou a ser publicada.)

## site v0.9.1 · 2026-10-09
- A seção de Privacidade explica que as notificações do app Android levam também um link temporário para a foto de quem mandou, que vale por até 48 horas.

## client v0.24.1 · 2026-10-09
- No app Android, as notificações mostram a foto de quem mandou, nas mensagens dos canais, nas mensagens diretas e nos avisos de amizade. Quem não tem foto continua com a letra.

## channel v0.11.1 · 2026-10-09
- As notificações push levam ao server-central quem mandou a mensagem, para o app Android mostrar a foto da pessoa.

## central v0.15.1 · 2026-10-09
- As notificações push levam um link temporário para a foto de quem mandou, que o app Android baixa sem login. O link vale por até 48 horas e só serve para aquela foto. `CENTRAL_PUBLIC_URL` diz o endereço público do servidor para montar o link (vazia: a instância oficial).

## site v0.9.0 · 2026-10-09
- Botão "Baixar para Android", com o passo a passo para instalar o APK fora da Play Store e um link para todas as versões do app no GitHub, com as notas de cada uma.
- Seção nova de Privacidade, no rodapé: o que a instância oficial guarda e por onde passa cada coisa, inclusive as notificações do app Android pelo Firebase, do Google.
- A demonstração do app na home tem agora o mesmo desenho do canal de texto do app.

## client v0.24.0 · 2026-10-09
- O FFCom agora tem app para Android: baixe o APK pelo botão "Baixar para Android" no site. O próprio app avisa quando há versão nova e instala com um toque.
- No app Android, "Entrar" abre o login no navegador do celular e volta sozinho para o app.
- Os links de convite agora são endereços `https://app.ffcom.a3sitsolutions.com.br/convite?...`: no celular com o app, abrem direto no app; no navegador e no desktop, abrem o "Adicionar servidor" já preenchido, mesmo se for preciso entrar antes. Links de convite antigos continuam valendo.
- No app Android, a chamada de voz continua com a tela apagada ou com outro app aberto, com uma notificação "Em chamada" que tem o tempo de chamada e os botões de mutar e de sair.
- No app Android, chegam notificações de mensagens novas nos canais, de mensagens diretas e de pedidos de amizade, agrupadas por canal. Tocar abre o canal, a conversa ou os Amigos.
- "Silenciar notificações" no menu do servidor e no menu de cada canal, para parar de receber os avisos de onde não interessa. A escolha fica salva na conta.
- Quem tem a nova permissão "Mover membros entre salas de voz" arrasta uma pessoa, na barra lateral, de uma sala de voz para outra.
- Os avisos de microfone negado, ausente ou em uso por outro aplicativo aparecem no seu idioma, e você entra na sala só ouvindo em vez de ficar de fora.

## channel v0.11.0 · 2026-10-09
- Notificações push no app Android: cada membro entrega ao servidor um token de push, que sai junto com a pessoa num kick ou ban, e a cada mensagem nova o servidor avisa pelo server-central quem pode ver o canal, menos o autor e quem está com o canal aberto. `FFCOM_CENTRAL_URL` escolhe o server-central (vazia: a instância oficial; `off`: desliga o push).
- Nova permissão "Mover membros entre salas de voz" (`MoveMembers`) e a rota `POST /api/voice/move`, para mover alguém de sala de voz.

## central v0.15.0 · 2026-10-09
- Notificações push do app Android pelo Firebase Cloud Messaging: registro do aparelho, token de push por servidor da lista da conta, silêncio por servidor ou canal guardado na conta, e avisos de mensagem direta (só com o nome de quem mandou) e de pedido e aceite de amizade. O texto das mensagens passa pelo servidor só em trânsito, sem ficar guardado.

## client v0.23.0 · 2026-10-08
- Em chamada, quem está falando ganha um anel verde: no ícone do microfone, na tela da chamada, e no avatar, na lista da sala na barra lateral.
- A lista de quem está em cada sala de voz, na barra lateral, não muda mais de ordem sozinha: as pessoas aparecem em ordem alfabética.
- As mensagens dos canais de texto e de fórum mostram o nome de quem enviou, em vez de um pedaço do id.
- Passar o mouse sobre uma mensagem com imagem não abre mais um espaço vazio abaixo dela: os botões Editar e Apagar aparecem por cima, no canto.

## site v0.8.0 · 2026-10-03
- O site agora tem versão em inglês: abre no idioma do seu navegador (português para quem usa português, inglês para os demais) e dá para trocar no rodapé.
- O histórico de versões, o cadastro e a área de ideias também aparecem em inglês, inclusive os avisos de erro.
- O pedido de cadastro feito em inglês recebe o e-mail de definir a senha em inglês.

## channel v0.10.0 · 2026-10-03
- Os erros da API e do WebSocket saem com um código (`{"code", "message", "params"}`), que o app traduz para o idioma de quem usa. Apps antigos continuam vendo a mensagem em português.

## client v0.22.0 · 2026-10-03
- Dá para desfazer uma amizade: botão direito no amigo, na lista de Amigos, e "Remover amigo". Ele some da sua lista e da dele, e a conversa aberta fecha nos dois lados. As mensagens ficam guardadas e voltam a aparecer se vocês forem amigos de novo.
- O FFCom agora fala inglês: em "Idioma", no menu do seu avatar, escolha português ou inglês e a interface troca na hora, sem recarregar. A escolha fica salva na conta e vale em todos os seus dispositivos; sem escolha, o app segue o idioma do sistema.
- Datas, horas e números aparecem no formato do idioma escolhido.
- Os avisos de erro dos servidores aparecem no seu idioma, em vez de sempre em português.
- As telas de login e de recuperação de senha abrem no idioma do app.
- No app desktop, a barra de menu em inglês (File, Edit, View...) saiu no Windows e no Linux; copiar, colar, desfazer e o zoom (Ctrl com +, - e 0) continuam funcionando. No macOS, o menu segue o idioma do app.

## central v0.14.0 · 2026-10-03
- Desfazer amizade (`DELETE /api/friends/{accountId}`), por qualquer um dos dois lados, avisando os dois na hora. Depois disso qualquer um pode pedir amizade de novo.
- O idioma escolhido fica guardado na conta (`PATCH /api/me` com `language`, devolvido no `GET /api/me`), para valer em todos os dispositivos.
- Os erros da API e do WebSocket saem com um código (`{"code", "message", "params"}`), que o app traduz para o idioma de quem usa. Apps antigos continuam vendo a mensagem em português.
- O pedido de cadastro guarda o idioma do site, e o e-mail para definir a senha, enviado na aprovação, sai nesse idioma.

## client v0.21.0 · 2026-10-02
- A chamada de voz não cai mais ao abrir um canal de texto, um fórum, outro servidor ou os Amigos: no pé da lista de canais aparece "Voz conectada", com o canal da chamada, o botão de mutar, o de sair e o atalho para voltar a ele.
- Os atalhos de mutar e de **apertar para falar** funcionam com qualquer tela aberta durante a chamada.
- No app desktop, "Entrar" abre o login no seu navegador, com a conta e as senhas que você já salvou lá, e volta sozinho para o app. Depois de "Sair", o próximo "Entrar" pede a senha, para dar para trocar de conta.

## client v0.20.0 · 2026-09-30
- Quem foi expulso de um servidor e ainda o tem na lista vê o aviso "Você não é mais membro", com a opção de voltar colando um convite novo ou de tirar o servidor da lista, em vez de uma tela vazia. Vale também para quem é expulso com o servidor aberto.

## channel v0.9.0 · 2026-09-30
- Canal privado funciona como no Discord: negar "Ver canal" para @everyone e permitir para uma role libera o canal para quem tem a role (antes só Administrador e dono entravam). Permitir numa role também vence Negar em outra role da mesma pessoa.
- Quem gerencia roles sem ser Administrador não consegue mais apagar, editar, tirar de alguém ou mexer nas permissões de canal de uma role com permissões que ele não tem, e ninguém expulsa ou bane quem tem permissões que ele não tem.

## client v0.19.1 · 2026-09-30
- A janela de permissões do canal explica como fazer um canal privado: negue "Ver canal" para @everyone e permita para as roles que entram.

## client v0.19.0 · 2026-09-30
- O link de convite leva o nome do servidor: quem cola o link já vê o nome preenchido, e todos passam a chamar o servidor do mesmo jeito.

## client v0.18.0 · 2026-09-30
- **Som quando alguém entra ou sai da chamada**, inclusive você mesmo: três notas subindo ao entrar e descendo ao sair. Dá para desligar em "Som ao entrar e sair", na barra do canal de voz.

## client v0.17.7 · 2026-09-29
- No app desktop, a dica do botão verde explica que ele fecha, instala a versão nova e abre de novo, em vez de falar em recarregar a página.

## site v0.7.0 · 2026-09-29
- Botão **Baixar para Windows** no topo, sempre com a versão mais recente do app desktop, e a explicação do aviso do Windows ao instalar.

## client v0.17.6 · 2026-09-29
- **App para Windows publicado**, com instalador para baixar pelo site.
- O app desktop avisa quando há versão nova pelo mesmo botão verde do navegador: um clique instala e reabre.

## client v0.17.5 · 2026-09-29
- App desktop: **apertar para falar funciona com o FFCom em segundo plano**, por exemplo com um jogo em foco, sem bloquear a tecla nos outros programas.

## client v0.17.4 · 2026-09-29
- No celular, o botão "voltar" do Android fecha a gaveta aberta em vez de sair do app.

## central v0.13.1 · 2026-09-29
- O app desktop é sempre aceito pelo servidor, sem precisar configurar nada.

## channel v0.8.1 · 2026-09-29
- O app desktop é sempre aceito pelo servidor, sem precisar configurar nada: quem hospeda o próprio servidor não precisa mexer no `.env` para o app instalado funcionar.

## client v0.17.3 · 2026-09-29
- O menu do avatar mostra a versão do app, para saber em qual versão cada aparelho está.
- App desktop: o login volta ao app em vez de parar numa tela com erro de carregamento.

## client v0.17.2 · 2026-09-29
- Chave de depuração para testar a conexão de voz só pelo servidor TURN (`ffcom:forceRelay` no armazenamento local do navegador); sem ela, nada muda.

## client v0.17.1 · 2026-09-29
- O botão verde de atualizar volta a aparecer quando saem duas versões seguidas com o app aberto; antes, se a primeira não terminasse de baixar, as seguintes só apareciam depois de recarregar a página.

## client v0.17.0 · 2026-09-29
- **Tela de login com o logo do FFCom**, e "Entrar" leva a uma página de login também com a marca do FFCom, em vez da marca genérica do Authentik. Quem já estava logado entra mais uma vez depois desta versão.
- A tela de "Saindo…" mostra o logo e um indicador de progresso.
- Depois de definir a senha pelo "Esqueci minha senha", o app já entra sem pedir a senha de novo.

## channel v0.8.0 · 2026-09-29
- Aceita o login feito pela página do FFCom no Authentik (`auth.ffcom`), além do endereço antigo: `OIDC_ISSUER_URL` pode listar mais de um emissor, separados por vírgula.

## central v0.13.0 · 2026-09-29
- Aceita o login feito pela página do FFCom no Authentik (`auth.ffcom`), além do endereço antigo: `OIDC_ISSUER_URL` pode listar mais de um emissor, separados por vírgula.

## client v0.16.0 · 2026-09-28
- **Nome de exibição:** "Alterar nome de exibição" no menu do seu avatar escolhe o nome que amigos, mensagens diretas e servidores veem; o apelido de cada servidor continua valendo por cima dele. Deixar vazio volta ao nome da sua conta.
- **No celular**, o chat ocupa a tela inteira: servidores e canais abrem numa gaveta à esquerda (☰ ou arrastando) e a lista de membros numa gaveta à direita.
- Ao sair, a tela mostra "Saindo…" até o fim, sem o botão de entrar que levava de volta à mesma conta; o botão de entrar fica bloqueado enquanto o login abre.
- "Esqueci minha senha" na tela de login, para definir uma senha nova e voltar ao app.
- **App para computador:** compartilhar tela agora funciona, com um seletor de telas e janelas em miniatura e, no Windows, a opção de compartilhar o áudio do computador. O app ganhou o ícone do FFCom, e links externos (código-fonte, licenças, recuperar senha) abrem no navegador.
- Imagens e arquivos das mensagens não são mais baixados de novo a cada hora, quando a sessão é renovada.

## central v0.12.0 · 2026-09-28
- Nova rota `PUT /api/me/display-name` para escolher o nome de exibição da conta (vazio volta ao nome do Authentik); `GET /api/me` passa a trazer `customDisplayName` quando há um nome escolhido.
- O e-mail de definir senha de um cadastro aprovado sai com a marca do FFCom e leva a uma página do FFCom que termina de volta no app.

## site v0.6.0 · 2026-09-28
- Link "Esqueci minha senha" logo abaixo de "Entrar no FFCom".

## channel-image v1.1.0 · 2026-09-28
- A imagem do servidor agora também sai para **linux/arm64** (Raspberry Pi, VPS ARM); o `docker pull` escolhe a versão da máquina sozinho.
- O lançador mostra as licenças de terceiros com `--licenses`.

## client v0.15.1 · 2026-09-28
- Digitar o link de convite em "Adicionar servidor" não corta mais o código no primeiro caractere; colar o link continua preenchendo o endereço e o código de uma vez.

## client v0.15.0 · 2026-09-28
- As roles do servidor agora podem ser editadas depois de criadas: "Editar" em cada role de "Gerenciar membros" mostra as permissões para ligar e desligar, inclusive na **@everyone** (por exemplo, "Criar convites" para todo mundo).
- Enter no nome cria ou salva a categoria e o canal, sem precisar clicar no botão.

## site v0.5.0 · 2026-09-27
- Quem já tem conta em outro serviço da infra A3S marca "Já tenho conta" no formulário e pede só o acesso, sem escolher usuário nem apelido; aprovado, entra no FFCom com o usuário e a senha de sempre.
- O formulário avisa enquanto você digita se o nome de usuário já está em uso ou é reservado.

## central v0.11.0 · 2026-09-27
- Pedido de quem já tem conta no Authentik: a aprovação só dá acesso ao FFCom à conta existente, sem criar outra nem mandar e-mail de senha. A mensagem de aprovação avisa, desde o envio, quando o e-mail do pedido já é de alguma conta.
- Nomes de usuário são conferidos sem diferenciar maiúsculas, e nomes como `admin`, `root` e `suporte` ficam reservados.
- Nova rota pública `GET /api/signup-requests/username-available` para o formulário conferir o nome enquanto a pessoa digita.

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
