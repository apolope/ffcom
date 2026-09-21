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
