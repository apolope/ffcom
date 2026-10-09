# Compartilhamento de tela no app Android do FFCom: viabilidade

**Status:** pesquisa concluída em 2026-10-09, nada implementado. Para retomar, começar pela Etapa 0 (conferir "só assistir" no aparelho) e depois o spike da fase 1. Rever antes as versões citadas (SDK do LiveKit, Capacitor, Android), que podem ter mudado.

Pesquisa de 2026-10-09, só leitura. Base: `docs/android-runbook.md`, `docs/architecture.md`, `client/src/hooks/useVoiceChannel.ts`, `client/src/components/VoiceChannelView.tsx`, `server-channel/internal/httpapi/voice.go` e `server-channel/internal/livekit/token.go`.

## Resumo e recomendação

- **Assistir** a uma tela compartilhada já deve funcionar no app: é só uma track de vídeo remota, a mesma que o Chrome do Android já recebe. Falta conferir num aparelho (tela cheia na WebView e desempenho).
- **Compartilhar pela WebView não dá:** `getDisplayMedia` não existe no Chrome do Android nem na WebView em 2026, e não há sinal de roadmap. Nenhuma flag resolve.
- **O caminho viável é nativo:** MediaProjection + SDK Android do LiveKit, entrando na sala como um **segundo participante** (`<memberId>:screen`) com token próprio, só com permissão de publicar tela.
- **Recomendação: go condicional**, em duas etapas. Primeiro publicar "só assistir" como suportado (verificação, quase sem código). Depois um spike de 1 a 2 dias do plugin nativo; se passar no Samsung Fold7 do dono, seguir com a v1 **sem áudio da tela**. Esforço total estimado: 2 a 3 semanas de trabalho.

## Situação atual no app

- **Web/desktop:** `toggleScreenShare` chama `room.localParticipant.setScreenShareEnabled(next, { audio })` do `livekit-client` 2.22; a tile remota é criada de forma imperativa em `TrackSubscribed`, com rótulo "nome (tela)", foco por clique e botão ⛶ (Fullscreen API). Electron tem seletor próprio (`ScreenSharePicker`).
- **Android:** `VoiceChannelView.tsx` esconde o botão "Compartilhar tela" quando `navigator.mediaDevices.getDisplayMedia` não existe (decisão "layout móvel com gavetas"). Na WebView do app ele também some.
- **Casca nativa:** Capacitor 8.5.3, min SDK 29, target/compile 36. `CallService` em primeiro plano com tipo `microphone|mediaPlayback`; plugins `FfcomUpdate`, `FfcomCall`, `FfcomPush`. A chamada inteira (WebRTC) roda dentro da WebView.
- **server-channel:** `POST /api/channels/{id}/voice/token` exige o bit `Voice` e assina, sem SDK, um JWT com `sub = member.ID`, `name`, `room = channel.ID` e `canPublish/canSubscribe/canPublishData` sem restrição de fonte. Não há `attributes` nem `canPublishSources`. `voice_participants.go` e `voice_move.go` tratam toda identity como um `member.ID`.
- **LiveKit:** `livekit/livekit-server:latest` nos dois compose, então atributos de participante (LiveKit 1.7+) estão disponíveis.

## Opções técnicas

| Opção | Complexidade | UX | Riscos |
|---|---|---|---|
| A. `getDisplayMedia` na WebView | Impossível hoje | n/a | Na MDN BCD o Chrome Android está `false` (de 72 a 88 a API existia, mas sempre rejeitava) e a WebView espelha o Chrome. Twilio (jun/2026) confirma que nenhum navegador móvel tem a API |
| B. Track nativa injetada no `RTCPeerConnection` da WebView | Impossível na prática | n/a | A WebView não aceita fonte de mídia customizada; só existe `getUserMedia` mediado por `onPermissionRequest`. O único atalho seria mandar quadros pela ponte JS para um `canvas.captureStream()`: base64 por quadro, CPU e latência inviáveis |
| C. SDK nativo como **2º participante** (`<memberId>:screen`) | Média | Boa: um toque, diálogo do sistema, chip na barra de status. Os outros veem "Fulano (tela)" na tile de sempre | Duas conexões WebRTC no mesmo aparelho; casar o ciclo de vida das duas; clientes antigos mostram um participante a mais |
| D. Mover a chamada inteira para o SDK nativo | Alta | A melhor no longo prazo | Reescreve voz, câmera, volume por pessoa, ruído, mover e expulsar, e toda a UI de chamada em nativo. Contraria a decisão de manter uma interface só |
| E. Só assistir (sem compartilhar) | Quase nula | Boa para quem assiste | Nenhum, além de conferir tela cheia e codecs |

## Desenho recomendado (opção C, precedida pela E)

**Etapa 0, só assistir (já):** conferir no app tile, foco e ⛶. A Fullscreen API na WebView depende de `WebChromeClient.onShowCustomView`; se o `BridgeWebChromeClient` do Capacitor não abrir a tela cheia, o `catch` já existente deixa só o modo foco, o que é aceitável. Registrar no runbook.

**server-channel**
1. Rota nova `POST /api/channels/{id}/voice/screen-token`, com a mesma checagem do bit `Voice` (ou um bit novo de "transmitir", se o sistema de permissões quiser).
2. Token com `sub = <member.ID>:screen`, o mesmo `name`, `canPublish: true`, `canPublishSources: ["screen_share","screen_share_audio"]`, `canSubscribe: false`, `canPublishData: false` e `attributes: {"ffcom.screenOf": "<member.ID>"}`. A identity deve ser distinta: o LiveKit derruba a sessão anterior quando uma identity repetida entra. `canSubscribe: false` evita que o aparelho baixe de novo os vídeos que a WebView já recebe.
3. `voice_participants.go` passa a ignorar identities com `:screen` (ou com o atributo); `voice_move.go` move ou desconecta junto o `<id>:screen` ao mover ou expulsar o membro.

**Android nativo (plugin `FfcomScreenShare`, Java chamando o SDK em Kotlin)**
1. Dependência `io.livekit:livekit-android` (2.29.0, de 20/09/2026). O APK cresce alguns MB por ABI por causa da libwebrtc (medir).
2. Manifesto: `FOREGROUND_SERVICE_MEDIA_PROJECTION` e serviço com `foregroundServiceType="mediaProjection"`. O SDK traz um `ScreenCaptureService` padrão (notificação própria); conferir no manifesto mesclado. Alternativa: acrescentar `mediaProjection` ao `CallService` e chamar `startForeground` de novo com os três tipos.
3. Fluxo por sessão (Android 14+): `createScreenCaptureIntent()` (consentimento), serviço em primeiro plano com tipo `mediaProjection` iniciado **depois** do consentimento e **antes** de `getMediaProjection`, `Room.connect(url, screenToken)` e `setScreenShareEnabled(true, data)`. O consentimento vale uma vez: cada novo compartilhamento pede de novo, e o token de projeção não se reutiliza.
4. Parâmetros conservadores: 720p ou 1080p no lado maior, 15 fps, `maintain-resolution` (padrão do SDK desde a 2.28), sem simulcast.
5. Eventos para a WebView: `started`, `stopped(reason)` (callback `onStop`: chip da barra de status, tela bloqueada a partir do Android 15 QPR1, outra projeção), `error`. Encerrar o compartilhamento quando a chamada da WebView acaba (`FfcomCall.stop`) e quando o processo volta sem chamada.
6. Android 14+ oferece "um app" ou "tela inteira" sem código extra. Avisar na UI que, na tela inteira, o próprio FFCom aparece (efeito espelho) e que notificações e campos sensíveis podem ser ocultados pelo sistema (proteções de compartilhamento do Android 15).

**client web**
1. `isAndroidApp()`: o botão aparece e `toggleScreenShare` chama o plugin em vez de `setScreenShareEnabled`. `screenSharing` passa a vir dos eventos do plugin ou, melhor, da presença de `<eu>:screen` na sala (fonte única, como hoje).
2. Em todos os clients: participante com `attributes["ffcom.screenOf"]` não entra na lista nem toca som de entrada/saída, e a tile usa o nome do dono ("Fulano (tela)"). O 🖥️ da lista vai para o dono. Volume do áudio da tela, quando existir, usa a chave `audio.screen` do dono.

**Áudio da tela (fase opcional):** `AudioPlaybackCapture` (API 29+) exige a mesma MediaProjection e `RECORD_AUDIO`, e só captura apps que permitem (uso `MEDIA`, `GAME` ou `UNKNOWN`, sem opt-out). Áudio de chamada (`VOICE_COMMUNICATION`), inclusive o do próprio FFCom, fica de fora, o que evita eco. O `ScreenAudioCapturer` do SDK **mistura** o som no microfone do participante; como o `:screen` não publica microfone, seria preciso publicar uma track própria de fonte `screen_share_audio`. Isso exige spike próprio.

## Riscos e testes

- **Ciclo de vida duplo:** WebView fora da chamada e o nativo ainda transmitindo (ou o contrário). Testar com o app em segundo plano, a tela apagada (a projeção para sozinha a partir do Android 15 QPR1), o processo morto e a troca de rede.
- **Térmica e bateria:** captura + codificação por hardware + uma segunda PeerConnection. Medir 20 min a 1080p/15 fps; ter 720p como padrão no Samsung se esquentar.
- **Fabricantes:** One UI mata serviços em primeiro plano com mais agressividade, tem diálogo de consentimento próprio e codificador VP8 por hardware irregular (o SDK cai para software). Testar no **Fold7 dobrado e aberto** (`onCapturedContentResize` na troca de tela e na rotação) e num Android 10 e num 14 ou mais.
- **Clientes antigos:** Electron ou PWA ainda não atualizados mostram "Fulano (tela)" como participante a mais, sem quebrar nada.
- **Recebimento:** o `livekit-client` publica VP8 por padrão. A WebView decodifica VP8 e AV1 (dav1d) em software e H.264 conforme o aparelho; tela 1080p em VP8 é tranquila num celular atual. Conferir em `chrome://inspect` o `framesDecoded` e o `decoderImplementation`.
- **Segurança:** token `:screen` sem assinatura nem dados e restrito à fonte de tela; ninguém consegue se passar por outro membro porque a identity sai do servidor.

## Estimativa

| Fase | Esforço |
|---|---|
| 0. Conferir "só assistir" no app, tela cheia, runbook | 0,5 dia |
| 1. Spike: plugin mínimo + SDK + token fixo, publicar 1 tela no Fold7 | 1 a 2 dias |
| 2. server-channel: rota, grant, filtro na presença, mover/expulsar, testes | 1 dia |
| 3. Plugin completo: serviço, eventos, parâmetros, encerramento casado | 3 a 4 dias |
| 4. Client: botão no Android, mesclar `:screen` ao dono, i18n, CHANGELOG | 2 dias |
| 5. Testes em aparelhos e ajustes | 2 a 3 dias |
| 6. (opcional) Áudio da tela | 2 a 4 dias |

Total da v1 sem áudio: cerca de 10 a 13 dias. Muda o APK (casca nativa) e exige `channel-v*` antes do `client-v*`.

## Fontes (acessadas em 2026-10-09)

- MDN browser-compat-data, `MediaDevices.getDisplayMedia` (`chrome_android: false`, `webview_android: mirror`): https://github.com/mdn/browser-compat-data/blob/main/api/MediaDevices.json
- caniuse, `getDisplayMedia`: https://caniuse.com/mdn-api_mediadevices_getdisplaymedia
- Twilio, screen share no Chrome (sem suporte em navegador móvel, atualizado em jun/2026): https://www.twilio.com/docs/video/screen-share-chrome
- W3C mediacapture-screen-share, discussão sobre a API em celulares (jan/2025): https://lists.w3.org/Archives/Public/public-webrtc-logs/2025Jan/0066.html
- Android, Media projection (Android 14, token único, ordem, callbacks, chip e parada ao bloquear no 15 QPR1): https://developer.android.com/media/grow/media-projection
- Android 14, App screen sharing: https://developer.android.com/about/versions/14/features/app-screen-sharing
- Android, referência `MediaProjectionManager` (serviço antes de `getMediaProjection`): https://developer.android.com/reference/android/media/projection/MediaProjectionManager
- Android, captura de áudio de reprodução: https://developer.android.com/media/platform/av-capture
- Proteções de compartilhamento do Android 15 (Android Authority, abr/2024, beta): https://androidauthority.com/android-15-apps-selectively-hide-sensitive-content-screen-sharing-3436855
- LiveKit, Screen sharing (Android com `createScreenCaptureIntent` + `setScreenShareEnabled(true, data)`): https://docs.livekit.io/transport/media/screenshare/
- LiveKit Android, `setScreenShareEnabled`: https://docs.livekit.io/reference/client-sdk-android/livekit-android-sdk/io.livekit.android.room.participant/-local-participant/set-screen-share-enabled.html
- LiveKit Android, releases (2.29.0 em 20/09; correções de `ScreenAudioCapturer` e `ScreenCaptureService` na 2.28.0): https://github.com/livekit/client-sdk-android/releases
- LiveKit, atributos de participante: https://docs.livekit.io/transport/data/state/participant-attributes/
- LiveKit, tokens e grants (`canPublishSources`): https://docs.livekit.io/home/get-started/authentication/
- Plugin Capacitor mais próximo (grava em arquivo, não publica no LiveKit): https://github.com/Cap-go/capacitor-screen-recorder

Não verificado: mudanças de MediaProjection específicas do Android 16 (nenhuma fonte oficial encontrada além do chip da barra de status) e se o `ScreenCaptureService` do SDK já declara o tipo `mediaProjection` no próprio manifesto.
