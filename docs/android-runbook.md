# App Android — runbook

Passo a passo para criar, publicar e manter o app Android do FFCom: o que fazer à mão (chaves, Firebase, Authentik), o que muda em cada componente e como conferir cada fase. As decisões abaixo foram tomadas com o dono em 2026-10-08. Ao implementar cada fase, registrar a decisão correspondente em [`architecture.md`](architecture.md) e marcar o item no [`TODO.md`](../TODO.md), como nas outras frentes.

## Decisões

| Tema | Decisão | Por quê |
|---|---|---|
| Tecnologia | **Capacitor** empacotando o `client` atual, carregando `https://app.ffcom.a3sitsolutions.com.br` (URL remota, não arquivos embutidos) | A parte web se atualiza a cada deploy de `client-v*`, como o navegador e o PWA. O APK só precisa mudar quando a casca nativa muda. |
| Distribuição | **APK pelo GitHub Releases**, sem Play Store | Mesmo modelo do desktop ("Decisão: distribuição e atualização do app desktop"): APK na release da versão e índice fixo `android-stable`. |
| Versão | **Junto da `client-v*`**: um job a mais no `deploy-ffcom-client.yml` | Mesma versão, mesma entrada no CHANGELOG (componente `client`). Sai um APK a cada release do client, mesmo sem mudança nativa. |
| Atualização do APK | O app consulta o índice `android-stable`, baixa o APK novo e mostra o mesmo botão verde; tocar abre o instalador do Android | O Android não instala em silêncio fora da loja. Na primeira vez a pessoa libera "instalar apps desta fonte". |
| Identificador | **`br.com.a3sitsolutions.ffcom`** | Não muda nunca: outro id é outro app. |
| Android mínimo | **Android 10 (API 29)** | |
| Login | **No navegador do sistema** (Custom Tab), voltando ao app por **App Link https** `https://app.ffcom.a3sitsolutions.com.br/auth/android` | RFC 8252, como o desktop. App Link verificado (`assetlinks.json`) garante que só o app assinado com a nossa chave recebe o código; o PKCE continua valendo. |
| Convites | Link novo **`https://app.ffcom.a3sitsolutions.com.br/convite?server=<endereço>&invite=<código>&name=<nome>`**, aberto pelo app via App Link | O endereço do server-channel é de qualquer domínio e não dá para verificar. Pelo nosso domínio, quem tem o app abre nele, e quem não tem cai no navegador. Os links antigos continuam aceitos ao colar. |
| Chamada em 2º plano | **Serviço em primeiro plano** (tipo `microphone`) com a notificação "Em chamada" enquanto conectado | Sem isso, o Android corta o microfone ou mata a chamada com a tela apagada ou em outro app. |
| Push: o que notifica | **Toda mensagem de canal**, mais DM e pedido/aceite de amizade | Pedido do dono. Uma DM é cifrada de ponta a ponta, então a notificação de DM mostra só o autor. |
| Push: confiança | **Token por pessoa e servidor**: o app pede ao server-central um token de push para aquele servidor (só se ele está na lista da conta) e o entrega ao server-channel. O server-channel só notifica quem lhe deu token; tirar o servidor da lista revoga o token. | Nenhum server-channel self-hosted recebe credencial global, e um servidor mal-intencionado não consegue notificar quem não é membro dele. |
| Push: conteúdo | **Autor e texto** ("Marina em #geral · Família da Ana: Ficaram lindas!") | O texto passa pelo server-central e pelo Firebase (Google) só em trânsito. Nada fica guardado no server-central. |
| Push: excesso | **Silenciar servidor ou canal** (guardado na conta), **agrupar por canal** ("5 mensagens novas") e **não notificar quem está com o canal aberto** | |

## Pré-requisitos manuais (dono)

Coisas que só o dono pode fazer, fora do código. Cada uma diz em que fase passa a ser necessária.

1. **Chave de assinatura do APK** (fase 1). Gerar uma vez:
   ```
   keytool -genkeypair -v -keystore ffcom-android.jks -alias ffcom -keyalg RSA -keysize 4096 -validity 36500
   ```
   - Guardar o `.jks` e as duas senhas **fora do repo e em dois lugares**, como um gerenciador de senhas e um backup offline. **Perder a chave significa nunca mais conseguir atualizar o app instalado**: um APK assinado com outra chave não instala por cima, e cada pessoa teria de desinstalar e instalar de novo.
   - Secrets do GitHub (Settings → Secrets → Actions): `ANDROID_KEYSTORE_BASE64` (o `.jks` em base64), `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` (`ffcom`) e `ANDROID_KEY_PASSWORD`.
   - Anotar o SHA-256 do certificado (`keytool -list -v -keystore ffcom-android.jks`). Ele vai no `assetlinks.json`.
2. **Authentik** (fase 3): acrescentar `https://app.ffcom.a3sitsolutions.com.br/auth/android` aos `redirect_uris` do provider `ffcom`, pelo blueprint `infra/authentik/blueprints/providers-ffcom.yaml` no repo `abs-3d-printer` (mesmo procedimento dos redirects anteriores; ver `docs/prompts/`).
3. **Firebase** (fase 6):
   - Criar o projeto (conta Google do dono), registrar o app Android `br.com.a3sitsolutions.ffcom` e baixar o `google-services.json`. Esse arquivo não é segredo, mas fica fora do repo por ser específico da instância. Ele vai como secret `ANDROID_GOOGLE_SERVICES_JSON`, e o CI escreve o arquivo.
   - Criar uma conta de serviço com o papel "Firebase Cloud Messaging API Admin" e gerar a chave JSON. Ela vai **só** no `.env` do server-central da instância (`FCM_SERVICE_ACCOUNT_JSON`). É segredo.
4. **Aparelho de teste**: Android 10 ou mais novo, com Depuração USB ligada (o mesmo roteiro do `chrome://inspect` registrado no TODO). O ideal é testar também num Samsung, pela economia de bateria agressiva.

## Fases

Cada fase termina num estado publicável. A **primeira versão pública do APK sai depois da fase 3**, porque sem o login no navegador ela nasceria com um fluxo que vai mudar. As fases 4 a 6 entram em releases seguintes do client.

### Fase 1 — casca Capacitor e build no CI

1. Em `client/`: instalar `@capacitor/core`, `@capacitor/cli` e `@capacitor/android` (versão estável atual, conferida na hora), rodar `npx cap init FFCom br.com.a3sitsolutions.ffcom` e `npx cap add android`. A pasta `client/android/` é commitada; `build/`, `.gradle/` e `local.properties` ficam no `.gitignore`.
2. `capacitor.config.ts`:
   - `server.url = 'https://app.ffcom.a3sitsolutions.com.br'`;
   - `server.allowNavigation` só com o próprio domínio. O login sai para o navegador na fase 3; nada mais navega dentro da WebView.
   - `android.minSdkVersion = 29`.
3. Permissões no `AndroidManifest.xml`: `INTERNET`, `RECORD_AUDIO`, `CAMERA`, `MODIFY_AUDIO_SETTINGS`. O `getUserMedia` da WebView pede a permissão do Android na hora (o `BridgeWebChromeClient` do Capacitor trata isso). Conferir que negar leva ao aviso de microfone traduzido que já existe, e não a um erro cru.
4. Detectar o app no client: `Capacitor.isNativePlatform()` (`client/src/lib/platform.ts`, ao lado da detecção do Electron), para esconder o que não existe no Android e ligar o que só existe nele.
5. Botão "voltar": o `popstate` que fecha as gavetas (`client-v0.17.4`) continua valendo. Sem nada para fechar e sem histórico, o voltar minimiza o app (`App.minimizeApp()`) em vez de fechar a chamada.
6. Ícone e splash a partir de `client/public/favicon.svg` (`@capacitor/assets`), gerados uma vez e commitados, como os ícones do PWA.
7. CI: job `android` no `deploy-ffcom-client.yml`, em paralelo ao `desktop-windows`:
   - `ubuntu-latest` com JDK 21 e Android SDK (`android-actions/setup-android`);
   - restaurar a keystore a partir dos secrets;
   - `./gradlew assembleRelease`;
   - `versionName` = `X.Y.Z` da tag e `versionCode` = `X*1000000 + Y*1000 + Z`, para ser sempre crescente;
   - anexar `FFCom-<X.Y.Z>.apk` à release `client-vX.Y.Z`.
8. Índice `android-stable` (pre-release fixa, como a `desktop-stable`):
   - `latest.json` com `{ "version", "versionCode", "url", "sha256" }`;
   - `FFCom.apk`, com nome estável, para o botão de download do site.

**Conferir:** APK do CI instalado no aparelho por cabo (`adb install`). O app abre o FFCom, entra numa chamada com áudio nos dois sentidos, a câmera funciona, o voltar fecha a gaveta e um deploy de `client-v*` aparece no app pelo botão verde de sempre (parte web).

### Fase 2 — atualização do próprio APK

1. Plugin nativo pequeno (`client/android/app/src/main/java/.../UpdatePlugin.java`):
   - consulta `https://github.com/apolope/ffcom/releases/download/android-stable/latest.json` ao abrir o app e a cada 30 min, como o desktop;
   - compara com o `versionCode` instalado e baixa o APK em segundo plano (para a pasta do app);
   - confere o `sha256` e só então avisa a parte web.
2. O client mostra o **mesmo botão verde**. Tocar abre o instalador do Android (`FileProvider` + `ACTION_VIEW`), com a permissão `REQUEST_INSTALL_PACKAGES`. Na primeira vez o Android leva para "Permitir desta fonte"; o app explica isso antes, num aviso curto.
3. Sem conexão ou com o GitHub fora: falha em silêncio e tenta de novo depois, como o desktop.

**Conferir:** instalar a versão N e publicar a N+1. O botão aparece, tocar instala por cima, e o login e as preferências continuam. Um APK com hash errado não é oferecido.

### Fase 3 — login no navegador do sistema (primeira versão pública)

1. No Android, o client não navega para o Authentik. Ele gera a URL de autorização (`oidc-client-ts`, com state e PKCE guardados na WebView, como hoje) e a abre num Custom Tab (`@capacitor/browser`), usando `redirect_uri = https://app.ffcom.a3sitsolutions.com.br/auth/android`.
2. App Link: um intent filter `autoVerify` para `https://app.ffcom.a3sitsolutions.com.br` com os caminhos `/auth/android` (e `/convite`, da fase 4).
3. Publicar `client/public/.well-known/assetlinks.json` com o pacote e o SHA-256 da chave. Ele precisa sair como `application/json`; conferir o `nginx.conf` do client.
4. O app recebe a URL (`App.addListener('appUrlOpen')`), fecha o Custom Tab e entrega `code` e `state` ao `signinRedirectCallback` na WebView. É a mesma ideia do `ffcom://auth/callback` do desktop ("Decisão: login do app desktop no navegador do sistema").
5. No navegador, sem o app, `/auth/android` explica que esse endereço é do app e oferece "Abrir no navegador".
6. Site: botão "Baixar para Android" ao lado do de Windows, apontando para `android-stable/FFCom.apk`, com a nota sobre "instalar apps desta fonte" e o aviso do Play Protect (APK fora da loja).

**Conferir:** `adb shell pm get-app-links br.com.a3sitsolutions.ffcom` mostra o domínio como `verified`. Login com a conta salva no Chrome, volta direto ao app, e logout e login de novo funcionam. Conferir também que o desktop e o web continuam logando.

**Build local:** o APK de debug não verifica o App Link (a chave de debug não está no `assetlinks.json`, de propósito). Para testar o login com ele, `adb shell pm set-app-links-user-selection --user cur --package br.com.a3sitsolutions.ffcom true app.ffcom.a3sitsolutions.com.br`. Detalhes, e por que o login no app sempre pede a senha, em "Decisão: login do app Android no navegador do sistema (fase 3)".

### Fase 4 — convites que abrem o app

1. `InviteServerDialog` passa a gerar `https://app.ffcom.a3sitsolutions.com.br/convite?server=<endereço>&invite=<código>&name=<nome>`.
2. O client trata a rota `/convite` em qualquer plataforma: abre o diálogo de entrar no servidor já preenchido. Ele também vale no navegador e no desktop.
3. `parseInviteLink` aceita os dois formatos: o novo e o antigo (`<endereço>?invite=`).
4. No app, o App Link `/convite` (fase 3) leva a URL para a WebView.

**Conferir:** um link mandado pelo WhatsApp abre direto no app e mostra o convite. Sem o app, abre no navegador. Um link antigo colado no "Adicionar servidor" continua funcionando.

### Fase 5 — chamada em segundo plano

1. Serviço em primeiro plano `foregroundServiceType="microphone"` (permissões `FOREGROUND_SERVICE` e `FOREGROUND_SERVICE_MICROPHONE`), com notificação fixa "Em chamada em <sala> · <servidor>" e as ações **Mutar** e **Sair**.
2. A parte web avisa o nativo ao conectar e ao sair. Isso se pendura no `setVoiceConnected` de `lib/voiceActivity.ts`, que já existe para o "ausente". O serviço precisa começar com o app em primeiro plano (exigência do Android 14), ou seja, no clique de entrar.
3. As ações da notificação voltam para a WebView (`toggleMic` / `leave`).
4. Pedir para tirar o app da otimização de bateria só se o teste no Samsung mostrar a chamada caindo.

**Conferir:** em chamada com outra pessoa, apagar a tela por 10 min e trocar de app. Os dois continuam se ouvindo, e "Sair" na notificação encerra a chamada.

### Fase 6 — notificações push

**server-central**
1. Tabelas:
   - `push_devices` (conta, token FCM, plataforma, último uso);
   - `push_grants` (hash do token, conta, endereço do servidor, criado em);
   - `push_mutes` (conta, endereço do servidor, canal opcional).
2. Rotas autenticadas pela conta:
   - `PUT/DELETE /api/push/devices`;
   - `POST /api/push/grants {serverAddress}`, que só aceita servidor que está em `known_servers` da conta e devolve o token uma vez;
   - `GET/PUT /api/push/mutes`.
   - Tirar um servidor da lista apaga os grants dele.
3. Rota do server-channel: `POST /api/push/notify` com `{grants: [token], serverName, channelId, channelName, author, text}`.
   - Para cada grant válido: confere o silêncio e manda para os aparelhos da conta pela API HTTP v1 do FCM (conta de serviço do `.env`, OAuth2 assinado localmente, como o JWT do LiveKit).
   - O texto é truncado e **não é gravado**.
   - Rate limit por grant e por servidor.
   - Grant desconhecido é ignorado sem erro, para não revelar quais tokens existem.
4. DM e amizade: o próprio server-central notifica. Na DM vai só "Nova mensagem de <nome>", porque o conteúdo é cifrado.

**server-channel**
1. `PUT/DELETE /api/me/push-grant {token}` guarda o token do membro. Ele sai junto com a pessoa num kick/ban.
2. Ao criar mensagem, os destinatários são quem tem `ViewChannels` no canal, menos o autor e menos quem está com o WebSocket daquele canal aberto agora (o `realtime.Hub` sabe). O envio vai para o central numa fila assíncrona, em lote, sem atrasar o envio da mensagem. Se falhar, só registra no log.
3. Endereço do server-central: variável nova `FFCOM_CENTRAL_URL` (padrão: a instância oficial). Sem ela, ou com o central fora, o servidor funciona normalmente, só sem push.

**client**
1. Pedir permissão de notificação (Android 13+) depois do login, com uma explicação antes, e registrar o token FCM em `/api/push/devices`. Feito com um plugin próprio (`FfcomPush`) em vez do `@capacitor/push-notifications`, cujo serviço disputaria o `MESSAGING_EVENT` com o do item 3; ver "Decisão: notificações push no app Android (fase 6, client)".
2. Para cada servidor da lista: pedir o grant ao central e entregá-lo ao server-channel. Repetir quando um servidor é adicionado. Um servidor antigo, sem a rota, fica sem push, sem erro na tela.
3. Agrupamento: mensagens de dados (sem `notification` no FCM) tratadas por um `FirebaseMessagingService` próprio, com uma notificação por canal (`MessagingStyle`, as últimas linhas e a contagem).
4. Tocar na notificação abre o servidor e o canal.
5. Menu do servidor e do canal: "Silenciar notificações", gravado em `/api/push/mutes`.

**Conferir:**
- mensagem de outra pessoa com o app fechado chega com autor e texto;
- várias seguidas no mesmo canal viram uma notificação só;
- com o canal aberto, não chega push;
- canal silenciado não notifica;
- tirar o servidor da lista para as notificações dele;
- um server-channel sem `FFCOM_CENTRAL_URL` continua funcionando;
- num servidor de teste, mandar `notify` com um grant de outro servidor não entrega nada.

### Documentação ao longo das fases

- `architecture.md`: uma decisão por fase, com contexto, alternativas e o que foi verificado.
- `protocol.md`: as rotas novas de push nos dois servidores e o formato do convite.
- `permissions.md`: só se alguma rota nova depender de bit (não está previsto).
- README do client: como gerar o APK localmente.
- README do server-channel: `FFCOM_CENTRAL_URL` e o que o servidor passa ao central (destinatários, autor e texto, só em trânsito).
- Site: botão de download (fase 3) e a política de privacidade, mencionando o Firebase (fase 6).

## Operação

- **Release:** nada muda para quem publica. A tag `client-vX.Y.Z` gera o web, o desktop e o APK. A entrada do CHANGELOG continua sendo do componente `client`; mudanças só do Android entram nela com "No app Android, …".
- **Mudou só a parte web:** o app já mostra pelo botão verde de sempre. O APK novo da mesma release é idêntico na parte nativa, e instalá-lo é opcional.
- **Rotacionar a conta de serviço do Firebase:** gerar uma chave nova, trocar no `.env` do server-central, reiniciar e revogar a antiga no console do Google Cloud.
- **Chave do APK vazou:** não há troca sem reinstalar. Gerar uma chave nova, publicar com outro aviso e orientar a desinstalar e instalar de novo. Por isso o backup e o sigilo do `.jks` importam.

## Problemas conhecidos e o que olhar

- **"App não instalado" ao atualizar:** o APK foi assinado com outra chave (build local instalado por cima do oficial, ou vice-versa). Desinstalar e instalar o oficial.
- **Play Protect avisa ou bloqueia:** é esperado para um APK fora da loja. "Instalar mesmo assim"; se virar bloqueio, enviar o APK para análise no Play Protect.
- **App Link abre no navegador:** `assetlinks.json` fora do ar, com o tipo errado ou com o SHA-256 de outra chave. Conferir com `adb shell pm verify-app-links --re-verify br.com.a3sitsolutions.ffcom`.
- **Push não chega num Samsung:** economia de bateria. Configurações → Bateria → Uso de bateria em segundo plano → "Sem restrições" para o FFCom.
- **Chamada cai com a tela apagada:** o serviço não subiu. Ver se a notificação "Em chamada" aparece e, no logcat, se houve `ForegroundServiceStartNotAllowedException`.
