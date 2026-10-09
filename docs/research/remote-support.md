# Suporte remoto "pedir ajuda a um familiar" no FFCom

**Status:** pesquisa e desenho concluídos em 2026-10-09, nada implementado e nenhuma decisão tomada pelo dono ainda. Para retomar, decidir o MVP (seção "Plano em fases") e rever as fontes, que mudam rápido (golpes com Quick Assist e RustDesk, E2EE do LiveKit). Ver também [`android-screenshare.md`](android-screenshare.md), que cobre a parte de compartilhar a tela no Android.

Pesquisa e desenho, 2026-10-09. Nada foi implementado. Base: `docs/architecture.md` (E2E de DM, amizade por convite, LiveKit, compartilhamento de tela no Electron, push, Android fases 1 a 6), `docs/protocol.md` e `docs/permissions.md`.

## Resumo e recomendação

**MVP recomendado: "ajuda guiada", só visualização.** Quem precisa de ajuda compartilha a tela. O ajudante vê, fala por voz e desenha setas e círculos que aparecem por cima da tela de quem pediu (no desktop). O ajudante **não controla nada**. Funciona no desktop (Electron) e na web como quem pede, e em qualquer plataforma como ajudante. O controle remoto (mouse e teclado) fica para uma fase posterior: só no Electron Windows, só para "ajudantes de confiança" cadastrados com antecedência, e só depois de uma revisão de segurança externa. No Android, quem pede pode só compartilhar a tela. Controle pelo AccessibilityService **não recomendado**.

Onde roda: **só server-central e um LiveKit da instância oficial, com E2EE**. O server-channel (self-hosted, de qualquer pessoa) fica fora do fluxo.

As três decisões de segurança mais importantes:
1. **O ajudante precisa estar numa lista de "ajudantes de confiança" montada antes, com carência.** A amizade aceita há pelo menos 7 dias é o piso para a visualização. Para o controle, a pessoa precisa estar na lista. Incluir alguém nessa lista só vale depois de 72 h e avisa os outros ajudantes. É isso que quebra o golpe "o banco ligou, me adiciona aí agora".
2. **O pedido nasce só do lado de quem precisa, e a sessão só começa depois de três passos: pedido, aceite e confirmação.** A confirmação é feita com um código de verificação (emojis) lido em voz alta, calculado a partir das chaves de E2E que cada lado enxerga. Esse código denuncia um servidor que troque chaves no meio.
3. **Nada é desacompanhado e nada fica para depois.** A sessão tem tempo máximo e acaba se ninguém fizer nada. Uma faixa vermelha fixa com "Parar agora" e um atalho global ficam visíveis o tempo todo. Não há sessão salva, acesso permanente nem transferência de arquivos. A entrada remota nunca alcança as janelas do próprio FFCom.

## Ameaças

| Ameaça | Como acontece | Resposta no desenho |
|---|---|---|
| Golpista convence o idoso ("falso banco/Microsoft") | Liga, cria urgência e pede que a vítima instale o app e passe o código. É o padrão documentado em Quick Assist e RustDesk | Ajudante tem que ser amigo há 7 dias ou mais (lista de confiança para controle). Sem código digitável que a vítima possa ditar. Avisos em linguagem simples. Aviso opcional a outros familiares |
| Conta do ajudante invadida | Senha vazada do Authentik | Aceite exige login recente (`prompt=login`/`max_age`). Código de verificação lido por voz (o invasor não tem a voz do familiar). Controle só para a lista de confiança. Log visível |
| server-channel malicioso | Operador desconhecido | Fora do fluxo: não emite token, não vê sala, não recebe evento |
| server-central ou LiveKit comprometido, MITM | Troca de chave pública no diretório, ou leitura da mídia no SFU | E2EE do LiveKit com chave distribuída por NaCl box. Código de verificação calculado sobre as chaves que cada lado viu |
| Sequestro ou repetição de sessão | Token do LiveKit reaproveitado, pedido antigo reaceito | Sala com nome aleatório, token de vida curta preso à identidade, pedido de uso único com prazo. Chave E2E nova por sessão |
| Persistência | Ajudante "volta depois" | Não existe acesso salvo. Cada sessão é um pedido novo. Ao encerrar, a sala é fechada e a chave é descartada |
| Sobreposição, UI falsa, clickjacking no consentimento | O ajudante desenha algo que parece um botão do sistema, ou clica ele mesmo em "permitir" | Anotações só como formas (sem texto livre), em cor e estilo fixos, sumindo sozinhas. Confirmação em janela do FFCom, com botão que só habilita após 3 s e não aceita Enter repetido. A entrada remota é descartada sobre as janelas do FFCom |
| Acesso desacompanhado | Pessoa sai de perto com a sessão aberta | Fim por inatividade, tempo máximo e nova confirmação periódica. Sem modo "sem supervisão" |
| Elevação de privilégio | Ajudante tenta aprovar UAC ou rodar algo como administrador | A área de trabalho segura do UAC não é capturada nem aceita SendInput de processo comum (UIPI). **Não** usar `uiAccess` |
| Vazamento de dados | Senhas, app do banco, códigos na tela | Sem transferência de arquivo nem área de transferência. Botão "Pausar tela". Compartilhar uma janela por padrão. No Android 15 o sistema esconde campos de senha e OTP |
| Spam de pedidos | Notificações em massa | Só quem precisa cria pedido, para alguém da própria lista. Limite de pedidos por hora. O ajudante não tem botão "oferecer ajuda" |

Limite honesto: nenhum desenho impede que alguém de fato da família, mal-intencionado, receba ajuda autorizada. Consentimento obtido por engano não se resolve só com criptografia, como a própria RustDesk admite.

## Como os outros fazem

- **Chrome Remote Desktop (suporte remoto):** quem pede gera um código de uso único que expira em 5 minutos. Depois aparece um diálogo com o e-mail do ajudante, e a cada 30 minutos é preciso confirmar de novo. O código ditado por telefone é justamente o ponto que golpistas exploram.
- **Windows Quick Assist:** já vem no Windows 11. A Microsoft documentou em 2024-05 o grupo Storm-1811 usando o app em vishing para instalar o ransomware Black Basta. Prometeu avisos dentro do app e mais transparência entre ajudante e ajudado. Imprensa de 2025-04 cita o bloqueio de cerca de 4.415 conexões suspeitas por dia. A recomendação oficial é só aceitar ajuda que você mesmo procurou.
- **TeamViewer QuickSupport e RustDesk:** ID e senha lidos pela vítima. A RustDesk é a ferramenta mais usada por golpistas segundo a Doctor Web. A empresa respondeu com avisos no site e no fluxo Android, login obrigatório no servidor público e saída da Google Play.
- **Apple (iOS 18, SharePlay no FaceTime):** o ajudante pode desenhar marcações temporárias e pedir controle, que a outra pessoa precisa permitir. A ação local tem prioridade sobre a remota. Só funciona com quem está nos contatos. É o modelo mais próximo do que se propõe aqui.
- **Google Meet:** só compartilha tela, sem controle. Útil como referência do nível "ver e falar".
- **Android 15 e 16:** o 15 esconde conteúdo de notificações, OTP e campos de senha durante o compartilhamento, e por padrão compartilha só um app. O 16 bloqueia ativar acessibilidade e instalar apps de fora durante ligação com número desconhecido, e avisa ao abrir app de banco com a tela compartilhada.

Lição comum: o ponto fraco é o código ou o convite que a vítima repassa a um estranho. O FFCom tem uma vantagem estrutural, porque já existe um grafo de amizade por convite. O desenho deve usá-lo e **nunca** ter um código digitável.

## Desenho de segurança

### Pré-condições (configuradas antes, com calma)

- Em Configurações, "Quem pode me ajudar": a lista de ajudantes de confiança, escolhida entre amigos aceitos. Uma inclusão só vale **72 h depois**, e os ajudantes já cadastrados recebem aviso. A remoção é imediata. Idealmente um familiar ajuda a montar a lista num encontro presencial.
- Níveis por ajudante: "ver e orientar" (padrão) ou "também pode controlar" (só desktop).
- Piso para quem não está na lista: amizade `accepted` há 7 dias ou mais (`friendships.updated_at`; ver Riscos), e somente "ver e orientar". Opcional, pode vir desligado.
- Opção "Avisar também": contatos que recebem uma notificação quando uma sessão começa.

### Fluxo passo a passo

```
Pedinte (P)                 server-central              Ajudante (A)
 |-- POST /api/support/requests {helperId, nivel} -->|
 |   (checa lista/7 dias, limite, sem sessão ativa)   |
 |                         |-- push + frame support.requested -->|
 |                         |<-- POST .../accept (login recente) --|
 |                         |   A gera chave de sessão K e manda   |
 |                         |   box(K, pubP) (NaCl, chave da conta)|
 |<-- support.accepted {box, pubA vista pelo servidor} --|
 | P mostra aviso + código de verificação; A mostra o mesmo código
 | ambos leem em voz alta (áudio já cifrado com K)
 |-- POST .../confirm (botão com espera de 3 s) ----->|
 |                         |-- tokens LiveKit (TTL curto, sala aleatória) -->|
 |==== sessão E2EE: tela de P, voz, anotações de A (canal de dados cifrado) ====|
 |-- "Parar agora" / atalho / tempo / inatividade --> POST .../end
 |                         | fecha a sala, grava o log, avisa contatos
```

1. **Pedido.** Só o app de P cria. O botão fica no perfil do amigo ("Pedir ajuda a Maria"). O pedido vale 5 minutos, é de uso único e permite uma sessão ativa por conta. Limite: 3 pedidos por hora por conta e 10 por dia. Recusas seguidas do mesmo ajudante bloqueiam novos pedidos a ele por 1 hora.
2. **Aceite.** A recebe push e frame no WebSocket de presença. Aceitar exige login recente no Authentik (reautenticação OIDC). A gera K (32 bytes aleatórios) e a cifra com `nacl.box` para a chave pública de P.
3. **Antes da confirmação, P vê uma tela de aviso** sem rolagem e com letras grandes: "Você está pedindo ajuda a **Maria Silva**. Ela vai ver sua tela. **Nenhum banco, a Microsoft, o Google ou a polícia vai pedir isso. Se alguém ligou pedindo, desligue.**" Em seguida vem o código de verificação: 5 emojis derivados de `SHA-256(sessionId ‖ pubP_vista_por_P ‖ pubA_vista_por_P)`. A calcula o mesmo com o que ele viu. Se o servidor tiver trocado chaves, os códigos ficam diferentes. Os dois conferem em voz alta. A voz pode já estar ligada nessa etapa, para P reconhecer o familiar.
4. **Confirmação.** O botão "Sim, é a Maria, pode ver" só habilita após 3 s, exige foco na janela e ignora tecla repetida. Um botão "Não reconheço" encerra a sessão e oferece bloquear o pedido.
5. **Sessão.** É uma sala dedicada no LiveKit da instância oficial, com nome aleatório. O central emite tokens com TTL de 2 minutos para entrar, só para as duas identidades. P recebe `canPublish`. A recebe só `canSubscribe` e `canPublishData` (e microfone). Todas as tracks e os dados (anotações e o ponteiro) vão cifrados com K (`encryption` do `livekit-client`, `ExternalE2EEKeyProvider`).
   - Uma faixa vermelha fixa em P, sempre visível: "Maria está vendo sua tela · 12:34 · Pausar · **Parar agora**". No Electron é uma janela `alwaysOnTop` excluída da captura (`setContentProtection`). O atalho global "Parar agora" (ex. Ctrl+Alt+Shift+P, pelo `globalShortcut`) funciona com qualquer app na frente.
   - Anotações: setas, círculos e realce, numa cor reservada, sumindo em 5 s, desenhadas numa janela transparente que deixa o clique passar (`setIgnoreMouseEvents`). Sem texto livre.
   - Tempo máximo de 45 minutos, nova confirmação a cada 15 minutos ("Continuar?", que encerra se ninguém responder em 60 s) e fim após 5 minutos sem tela ou sem os dois na sala.
6. **Controle (fase posterior).** É uma concessão separada e revogável: A pede, P confirma em diálogo próprio e o controle dura no máximo 10 minutos. Mexer o mouse local pausa o remoto por 2 s, como no iOS. A entrada remota que cairia sobre janelas do FFCom é descartada. Sem área de transferência e sem teclas de sistema (Win, Ctrl+Alt+Del).
7. **Encerramento.** O fim pode vir de qualquer lado, do tempo ou da inatividade. O central fecha a sala pela API do LiveKit, K é descartada e o log é gravado: quem, quando, duração, níveis e quem encerrou. O log não guarda conteúdo. P vê o histórico em "Ajudas recebidas", e os contatos de "Avisar também" recebem um resumo.

### Onde roda

O **server-central** já é a fonte de amizades, chaves públicas e push, então é o único lugar que pode aplicar as regras de lista e carência. O **LiveKit** deve ser dedicado ou da instância oficial, com API key própria do central. A mídia não passa pelo central. Com E2EE, quem opera o SFU vê só metadados. O **server-channel fica fora**, porque é de terceiros e não conhece a amizade. Alternativa: WebRTC 1:1 direto com sinalização pelo WebSocket de presença. É E2E por natureza (DTLS-SRTP), com o código derivado das impressões DTLS, mas exige TURN próprio no central e código novo fora do `useVoiceChannel`. Fica como plano B.

## Níveis de capacidade por plataforma

| Capacidade (quem pede) | Electron Windows | Electron macOS | Electron Linux | Web/PWA | Android (Capacitor) |
|---|---|---|---|---|---|
| a) Ver tela + voz | Sim (`desktopCapturer`, seletor próprio já existe) | Sim, exige permissão de Gravação de Tela | X11 sim; Wayland pelo portal (fonte genérica) | Sim (`getDisplayMedia`) | Sim (MediaProjection, consentimento por sessão; ver pesquisa paralela) |
| b) Anotações visíveis para quem pede | Sim (janela transparente) | Sim | X11 sim; Wayland incerto | Só dentro da aba do FFCom, inútil para outros apps | Exige "sobrepor a outros apps" (permissão sensível). Fase posterior, se houver |
| Ponteiro do ajudante visto pelo ajudante | Sim em todas | | | | |
| c) Controle remoto | Possível (SendInput; UIPI bloqueia janelas elevadas e o UAC, o que é desejável) | Possível, exige Acessibilidade (TCC) | Wayland pelo portal RemoteDesktop; X11 via XTest | **Não** | **Não recomendado** (AccessibilityService; APK de fora da Play cai em "configuração restrita" desde o Android 13; alvo clássico de malware bancário) |
| Ajudante (só assistir e anotar) | Sim | Sim | Sim | Sim | Sim |

Para o controle no Electron: `uiohook-napi` só escuta e não injeta. Para injetar seria preciso nut.js (fork mantido) ou um módulo N-API próprio sobre `SendInput`/`CGEvent`/portal, com `electron-rebuild`. O app não é assinado hoje. Isso pesa: um app que injeta entrada e se atualiza sozinho vira alvo valioso, e o canal de atualização passa a precisar de assinatura antes da fase de controle.

## Plano em fases

| Fase | Escopo | Esforço | Riscos |
|---|---|---|---|
| 0 | Central: tabelas `support_helpers` (com `effective_at`), `support_sessions` (log), rotas de pedido, aceite, confirmação e fim, frames no WebSocket de presença, push, limites, LiveKit oficial com API key do central | 1,5 a 2 semanas | Infra do LiveKit no central; carência mal calibrada |
| 1 (MVP) | Client: telas de pedido, aviso, código de verificação e confirmação; sala E2EE com chave por NaCl box; faixa vermelha, atalho, Pausar, temporizadores; P no Electron e na web; A em qualquer plataforma; ponteiro só no lado de A | 2 a 3 semanas | E2EE do LiveKit no Electron e no Chromium Android (testar); textos para idosos |
| 2 | Anotações sobrepostas no Electron (Windows primeiro), canal de dados cifrado | 1 semana | Multi-monitor e escala de DPI |
| 3 | P no Android, só visualização + voz, reaproveitando o compartilhamento de tela do Android | 1 a 2 semanas (depois da pesquisa paralela) | OEMs e o serviço em primeiro plano `mediaProjection` |
| 4 | Controle remoto no Electron Windows, só para ajudantes de confiança, com concessão separada; antes disso, assinatura de código e revisão externa | 3 a 4 semanas + revisão | É a maior superfície de abuso do projeto |
| 5 | macOS e Linux para controle; anotações no Android | a avaliar | Permissões TCC, portais do Wayland |

## Riscos em aberto

- `friendships` não tem `accepted_at`: `updated_at` serve como aproximação, mas uma migração com `accepted_at` é mais limpa e auditável.
- A E2EE do LiveKit depende de Insertable Streams. É preciso confirmar no Electron 44, no Chrome, no Firefox e na WebView Android, e decidir o que fazer onde não houver suporte. Recomendação: recusar a sessão e nunca cair para sessão sem E2EE, como já se faz nas DMs.
- O código de verificação protege contra troca de chave só se as pessoas realmente conferirem. É preciso testar o texto com usuários idosos de verdade.
- Um familiar real coagido ou mal-intencionado continua sendo um risco que só o log e o "Avisar também" mitigam.
- Detectar campos de senha em outros apps (UI Automation `IsPassword` no Windows) é viável, mas exige módulo nativo e é frágil. Fica como melhoria, não como garantia.
- Um XSS na origem do app passaria a alcançar a ponte de captura e, na fase 4, de injeção. A ponte do Electron deve exigir gesto do usuário em janela própria, não só uma chamada do renderer.
- Push com o nome de quem pede é metadado visível ao Google (FCM), o mesmo nível já aceito para DMs.

## Fontes

- Microsoft Threat Intelligence, "Threat actors misusing Quick Assist in social engineering attacks leading to ransomware", 2024-05-15: https://www.microsoft.com/en-us/security/blog/2024/05/15/threat-actors-misusing-quick-assist-in-social-engineering-attacks-leading-to-ransomware/
- The Register, sobre Quick Assist e Storm-1811, 2024-05-16: https://www.theregister.com/2024/05/16/microsoft_quick_assist_crime/
- Genbeta, Microsoft e as 4.415 conexões suspeitas por dia, 2025-04: https://www.genbeta.com/seguridad/microsoft-alerta-maximo-cuidado-esta-app-nativa-windows-10-11-estan-usando-para-estafar-masivamente
- Microsoft, "Avoid and report Microsoft technical support scams" (acessado em 2026-10-09): https://support.microsoft.com/security/avoid-and-report-microsoft-technical-support-scams
- Google, ajuda do Chrome Remote Desktop (acessado em 2026-10-09): https://support.google.com/chrome/answer/1649523 ; guias de terceiros confirmando o código de 5 min e a confirmação a cada 30 min: https://obrienmedia.co.uk/help/remote-support
- RustDesk, "RustDesk and remote access scams" (acessado em 2026-10-09): https://rustdesk.com/blog/rustdesk-and-remote-access-scams ; Doctor Web, 2023: https://news.drweb.com/show/?i=14755
- BGR, controle remoto no SharePlay do iOS 18, 2024: https://bgr.com/tech/shareplays-new-remote-control-feature-in-ios-18-and-ipados-18-will-save-my-relationship-with-my-parents/ ; iDrop News: https://www.idropnews.com/how-to/ios-18-lets-you-give-remote-control-of-your-iphone-or-ipad-via-facetime/226438
- Google Security Blog, Android 15 (I/O), 2024-05-15: https://security.googleblog.com/2024/05/io-2024-whats-new-in-android-security.html
- Google Security Blog, Android 16, 2025-05-13: https://security.googleblog.com/2025/05/whats-new-in-android-security-privacy-2025.html ; TechCrunch, 2025-05-13: https://techcrunch.com/2025/05/13/google-announces-new-security-features-for-android-for-protection-against-scam-and-theft
- Google Play, política da API de Acessibilidade (acessado em 2026-10-09): https://support.google.com/googleplay/android-developer/answer/10964491
- 9to5Google, configuração restrita no Android 13, 2022-05-06: https://9to5google.com/2022/05/06/android-13-sideloaded-accessibility/ ; BleepingComputer, contorno por instalação em sessão, 2022: https://www.bleepingcomputer.com/news/security/malware-devs-already-bypassed-android-13s-new-security-feature/
- LiveKit, criptografia de ponta a ponta (acessado em 2026-10-09): https://docs.livekit.io/transport/encryption/
- Microsoft, SendInput e UIPI: https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-sendinput ; Wikipedia, UIPI: https://en.wikipedia.org/wiki/User_Interface_Privilege_Isolation
- GNOME, Remote desktop e screencast no Wayland: https://live.gnome.org/Projects/Mutter/RemoteDesktop ; nut.js: https://nutjs.dev/docs
