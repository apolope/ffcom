import { app, BrowserWindow, net, protocol } from 'electron'
import { existsSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

const VITE_DEV_SERVER_URL = process.env.VITE_DEV_SERVER_URL

const DIST_DIR = path.join(__dirname, '../dist')
const APP_SCHEME = 'app'
const APP_ORIGIN = `${APP_SCHEME}://ffcom`

// Esquema customizado (em vez de `file://` via loadFile) para o build
// empacotado. Dois problemas que ele resolve de uma vez:
// 1. As URLs de asset geradas pelo Vite são raiz-absoluta (`/assets/...`);
//    sob `file://`, "/" resolve para a raiz do sistema de arquivos, não
//    para `dist/`, quebrando o carregamento dos assets. Um scheme
//    registrado como "standard" resolve "/" contra `DIST_DIR` (ver
//    `registerAppProtocol` abaixo).
// 2. Login OIDC (`client/src/auth/userManager.ts`) usa `window.location.origin`
//    como `redirect_uri` — `file://` renderiza cada documento numa origin
//    opaca diferente (o Authentik não teria uma origin estável para
//    cadastrar). `app://ffcom` é uma origin fixa, igual em toda execução do
//    app empacotado, cadastrável como redirect_uri de verdade. Ver
//    docs/architecture.md, "Decisão: callback OIDC no Electron empacotado".
protocol.registerSchemesAsPrivileged([
  {
    scheme: APP_SCHEME,
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      corsEnabled: true,
    },
  },
])

function resolveDistFile(pathname: string): string {
  const relative = decodeURIComponent(pathname).replace(/^\/+/, '')
  const candidate = path.join(DIST_DIR, relative || 'index.html')
  const isInsideDist = candidate === DIST_DIR || candidate.startsWith(DIST_DIR + path.sep)
  if (isInsideDist && existsSync(candidate) && statSync(candidate).isFile()) {
    return candidate
  }
  // Fallback de SPA: rotas sem arquivo correspondente (ex. /auth/callback,
  // o retorno do login OIDC) servem o index.html, mesmo padrão do
  // `try_files` em `client/nginx.conf` para o build web.
  return path.join(DIST_DIR, 'index.html')
}

function registerAppProtocol() {
  protocol.handle(APP_SCHEME, (request) => {
    const { pathname } = new URL(request.url)
    return net.fetch(pathToFileURL(resolveDistFile(pathname)).toString())
  })
}

function createWindow() {
  const win = new BrowserWindow({
    width: 1280,
    height: 800,
    webPreferences: {
      preload: path.join(__dirname, 'preload.mjs'),
    },
  })

  if (VITE_DEV_SERVER_URL) {
    win.loadURL(VITE_DEV_SERVER_URL)
  } else {
    win.loadURL(`${APP_ORIGIN}/index.html`)
  }
}

app.whenReady().then(() => {
  registerAppProtocol()
  createWindow()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow()
})
