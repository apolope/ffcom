import {
  app,
  BrowserWindow,
  desktopCapturer,
  globalShortcut,
  ipcMain,
  net,
  powerMonitor,
  protocol,
  session,
  shell,
} from 'electron'
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

// Atalho global de mutar (ver docs/architecture.md, "Decisão: atalho de
// teclado para mutar"). O renderer registra ao conectar na voz e remove ao
// sair, para a combinação não ficar presa nos outros programas enquanto
// ninguém está numa chamada. Um atalho só por vez: registrar outro troca o
// anterior.
const SHORTCUT_FORMAT = /^((Ctrl|Alt|Shift|Super)\+)*([A-Z0-9]|num\d|F([1-9]|1\d|2[0-4])|Space)$/
let muteShortcut: string | undefined

function registerMuteShortcutIpc() {
  ipcMain.handle('ffcom:set-mute-shortcut', (event, accelerator: unknown) => {
    if (muteShortcut) {
      globalShortcut.unregister(muteShortcut)
      muteShortcut = undefined
    }
    if (accelerator === null) return true
    if (typeof accelerator !== 'string' || !SHORTCUT_FORMAT.test(accelerator)) return false
    const sender = event.sender
    // register devolve false quando outro programa já tem a combinação.
    const ok = globalShortcut.register(accelerator, () => {
      if (!sender.isDestroyed()) sender.send('ffcom:mute-shortcut')
    })
    if (ok) muteShortcut = accelerator
    return ok
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

  // Links com target="_blank" (código-fonte, licenças, "Esqueci minha
  // senha") abririam uma janela solta do próprio app: http(s) vai para o
  // navegador do sistema. O resto (a object URL de um anexo, por exemplo)
  // continua abrindo numa janela do app, como antes.
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('https://') || url.startsWith('http://')) {
      void shell.openExternal(url)
      return { action: 'deny' }
    }
    return { action: 'allow' }
  })

  if (VITE_DEV_SERVER_URL) {
    win.loadURL(VITE_DEV_SERVER_URL)
  } else {
    win.loadURL(`${APP_ORIGIN}/index.html`)
  }
}

// Tempo sem teclado nem mouse no sistema inteiro, para o "ausente"
// automático (client/src/hooks/useIdle.ts): no app desktop a pessoa pode
// estar jogando com o FFCom em segundo plano, e a página sozinha não vê essa
// atividade. Ver docs/architecture.md, "Decisão: status de presença e
// avatar nas listas de membros".
function registerSystemIdleIpc() {
  ipcMain.handle('ffcom:get-system-idle-seconds', () => powerMonitor.getSystemIdleTime())
}

// Compartilhamento de tela (ver docs/architecture.md, "Decisão:
// compartilhamento de tela no Electron"). O Electron não tem o seletor nativo
// do navegador: sem um handler aqui, o getDisplayMedia do renderer falha. O
// renderer lista as fontes, a pessoa escolhe no seletor próprio
// (components/ScreenSharePicker.tsx), a escolha fica guardada e o handler a
// consome no getDisplayMedia seguinte, uma vez só. Sem escolha guardada, o
// pedido é recusado.
interface DisplaySourceChoice {
  id: string
  name: string
  audio: boolean
}
let listedSources = new Map<string, string>()
let displaySourceChoice: DisplaySourceChoice | undefined

function registerDisplayMediaIpc() {
  ipcMain.handle('ffcom:get-display-sources', async () => {
    const sources = await desktopCapturer.getSources({
      types: ['screen', 'window'],
      thumbnailSize: { width: 320, height: 180 },
    })
    listedSources = new Map(sources.map((s) => [s.id, s.name]))
    return sources.map((s) => ({
      id: s.id,
      name: s.name,
      kind: s.id.startsWith('screen:') ? 'screen' : 'window',
      thumbnail: s.thumbnail.isEmpty() ? '' : s.thumbnail.toDataURL(),
    }))
  })
  // Só aceita um id da última listagem, para o renderer não pedir uma fonte
  // que a pessoa não viu no seletor.
  ipcMain.handle('ffcom:choose-display-source', (_event, id: unknown, audio: unknown) => {
    const name = typeof id === 'string' ? listedSources.get(id) : undefined
    if (name === undefined) return false
    displaySourceChoice = { id: id as string, name, audio: audio === true }
    return true
  })
  session.defaultSession.setDisplayMediaRequestHandler((request, callback) => {
    const choice = displaySourceChoice
    displaySourceChoice = undefined
    if (!choice) {
      // Recusa. Tem que ser null: com {} o Electron lança "Video was
      // requested, but no video stream was provided" aqui no main. O tipo do
      // callback não inclui null, mas a validação do Electron aceita.
      callback(null as unknown as Electron.Streams)
      return
    }
    // 'loopback' é o áudio do sistema inteiro, inclusive as vozes da sala que
    // este app toca (o renderer pede restrictOwnAudio para tirá-las); só
    // existe no Windows. 'loopbackWithMute' não serve: muta o som do
    // computador de quem compartilha enquanto captura.
    const audio = choice.audio && request.audioRequested && process.platform === 'win32'
    callback({ video: { id: choice.id, name: choice.name }, ...(audio ? { audio: 'loopback' as const } : {}) })
  })
}

app.whenReady().then(() => {
  registerAppProtocol()
  registerMuteShortcutIpc()
  registerSystemIdleIpc()
  registerDisplayMediaIpc()
  createWindow()
})

app.on('will-quit', () => {
  globalShortcut.unregisterAll()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow()
})
