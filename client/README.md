# client

Interface gráfica do FFCom. Mesma base de código para web (PWA) e desktop (Electron), conforme decisão registrada em [`../docs/architecture.md`](../docs/architecture.md).

## Stack planejada

- React + TypeScript, bundler Vite
- Electron para empacotar desktop (Windows/macOS/Linux)
- `livekit-client` (+ componentes React do LiveKit) para voz/vídeo/compartilhamento de tela
- WebSocket para chat de texto, presença e sinalização

## Responsabilidade

- Conectar-se ao `server-central` (login, lista de amigos, lista de servidores).
- Conectar-se a um ou mais `server-channel` via IP ou DNS informado pelo usuário.
- Renderizar a organização servidor → categoria → canal (texto/voz/forum).

## Scripts

- `npm run dev` — dev server Vite (web/PWA), sem Electron.
- `npm run dev:electron` — mesmo dev server, mas abre em uma janela Electron (`electron/main.ts` + `electron/preload.ts`, via `vite-plugin-electron`).
- `npm run build` / `npm run build:electron` — build de produção da SPA (`dist/`); a variante `:electron` também empacota `main`/`preload` em `dist-electron/`.
- `npm run electron` — roda o build já gerado (`dist/` + `dist-electron/`) num binário Electron local, sem empacotar instalador.

Empacotamento para distribuição (instaladores Windows/macOS/Linux) ainda não existe — ver [`../TODO.md`](../TODO.md), seção "client".

Nenhuma feature de produto ainda — só o scaffolding do wrapper Electron e da SPA.
