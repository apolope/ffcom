# client

Interface gráfica do FFCom. Mesma base de código para web (PWA) e desktop (Electron), conforme decisão registrada em [`../docs/architecture.md`](../docs/architecture.md).

## Stack planejada

- React + TypeScript, bundler Vite
- Electron para empacotar desktop (Windows/macOS/Linux)
- `livekit-client` (+ componentes React do LiveKit) para voz/vídeo/compartilhamento de tela
- WebSocket para chat de texto, presença e sinalização

## Responsabilidade

- Conectar-se ao `server-central` (login, lista de amigos, lista de servidores, DMs).
- Conectar-se a um ou mais `server-channel` via IP ou DNS informado pelo usuário.
- Renderizar a organização servidor → categoria → canal (texto/voz/forum), com administração de roles/permissões e convites.

Estado atual: todos os itens acima implementados — ver [`../TODO.md`](../TODO.md), seção "client", para o detalhe item a item.

## Scripts

- `npm run dev` — dev server Vite (web/PWA), sem Electron.
- `npm run dev:electron` — mesmo dev server, mas abre em uma janela Electron (`electron/main.ts` + `electron/preload.ts`, via `vite-plugin-electron`).
- `npm run build` / `npm run build:electron` — build de produção da SPA (`dist/`); a variante `:electron` também empacota `main`/`preload` em `dist-electron/`.
- `npm run electron` — roda o build já gerado (`dist/` + `dist-electron/`) num binário Electron local, sem empacotar instalador.
- `npm run package` — empacota para a plataforma do host atual (`electron-builder`, sem publicar). `package:win`/`package:mac`/`package:linux` forçam uma plataforma específica; instaladores saem em `release/` (gitignored).

## Empacotamento (instaladores)

`electron-builder` gera: NSIS (`.exe`) no Windows, DMG no macOS, AppImage no Linux — configuração em `package.json` (campo `"build"`). Nenhum ícone customizado nem assinatura de código configurados ainda (usa o ícone padrão do Electron; instalador Windows fica sem assinatura Authenticode, o que dispara aviso do SmartScreen ao instalar).

Build cruzado tem limite real do `electron-builder`, não deste projeto: Windows e Linux podem ser empacotados a partir de qualquer host com Docker (`electronuserland/builder`, ou nativamente no Windows para o alvo `win`); **macOS exige rodar em um host macOS** (assinatura/notarização usam ferramentas da Apple que não existem em outro SO) — `package:mac` só funciona lá.

Verificado nesta sessão (2026-09-21): build + empacotamento + execução do instalador para Windows (nativo) e o AppImage para Linux (via `docker run electronuserland/builder:22`). macOS não verificado — sem host disponível.

## Gerar o APK localmente

O app Android é o Capacitor carregando o client publicado (`https://app.ffcom.a3sitsolutions.com.br`, em `capacitor.config.ts`), então o APK só muda quando muda a parte nativa em `android/`. O passo a passo das fases está em [`../docs/android-runbook.md`](../docs/android-runbook.md), e a decisão em [`../docs/architecture.md`](../docs/architecture.md), "Decisão: app Android com Capacitor (fase 1)".

Precisa de JDK 21 e do Android SDK (plataforma 36), com `ANDROID_HOME` apontando para ele (no Windows, `%LOCALAPPDATA%\Android\Sdk` quando instalado pelo Android Studio ou pelo `sdkmanager`). Na pasta `client/`:

```
npm ci
npm run build
npx cap sync android
cd android
./gradlew assembleDebug
```

O APK sai em `android/app/build/outputs/apk/debug/app-debug.apk` e instala por cabo com `adb install -r app-debug.apk`. Sem propriedades, ele sai com versão `0.0.0` (`versionCode` 1); para simular uma versão, `./gradlew assembleDebug -PffcomVersionName=0.23.1 -PffcomVersionCode=23001`.

- O build de debug é assinado com a chave de debug da máquina. O Android não instala um APK assinado com outra chave por cima do oficial ("App não instalado"): para testar no aparelho em que está o oficial, desinstalar antes.
- O `assembleRelease` assinado é do CI (job `android` do `deploy-ffcom-client.yml`), com a keystore dos secrets. Localmente, ele só assina com `ANDROID_KEYSTORE_FILE`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` e `ANDROID_KEY_PASSWORD` no ambiente; sem elas, sai sem assinatura.
- Atualização do próprio APK (`UpdatePlugin.java`; ver "Decisão: atualização do APK (fase 2)" em `docs/architecture.md`): no build `0.0.0` ela fica desligada. Para ver o botão verde com um build local, gerar com uma versão abaixo da publicada no `android-stable` (por exemplo `-PffcomVersionName=0.1.0 -PffcomVersionCode=1000`); o APK oficial baixado ainda não instala por cima de um de debug, pela chave. O que o plugin faz aparece no logcat com a tag `FfcomUpdate` (`adb logcat -s FfcomUpdate`). Os testes JVM das regras puras rodam com `./gradlew testDebugUnitTest`.
- `npm run generate:android-assets` refaz o ícone e a splash a partir de `public/favicon.svg`. Os PNGs gerados ficam versionados em `android/app/src/main/res/`.
- `npx cap sync android` copia o `dist/` para dentro do projeto nativo (fallback do Capacitor, fora do git) e atualiza os plugins; rodar de novo depois de instalar ou atualizar um plugin `@capacitor/*`.
