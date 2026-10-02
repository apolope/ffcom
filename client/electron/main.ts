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
import electronUpdater from 'electron-updater'
import { UiohookKey, uIOhook, type UiohookKeyboardEvent } from 'uiohook-napi'

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

// Login no navegador do sistema (ver docs/architecture.md, "Decisão: login
// do app desktop no navegador do sistema"). O renderer abre a URL de
// autorização com shell.openExternal; o Authentik volta para
// ffcom://auth/callback, que o sistema entrega ao app: no Windows e no Linux
// como argumento de uma segunda instância (repassado pelo evento
// second-instance, graças ao single instance lock), no macOS pelo open-url.
// Se o app estava fechado, a URL chega no argv da primeira instância. A URL
// fica guardada até o renderer buscá-la (takeAuthCallback), então tanto o
// aviso quanto a busca ao montar levam ao mesmo lugar, uma vez só.
const AUTH_SCHEME = 'ffcom'
let pendingAuthCallback: string | undefined

function isAuthCallbackUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === `${AUTH_SCHEME}:` && url.host === 'auth' && url.pathname === '/callback'
  } catch {
    return false
  }
}

function receiveAuthCallback(argvOrUrl: string[] | string) {
  const candidates = typeof argvOrUrl === 'string' ? [argvOrUrl] : argvOrUrl
  const url = candidates.find(isAuthCallbackUrl)
  const win = BrowserWindow.getAllWindows()[0]
  if (win) {
    if (win.isMinimized()) win.restore()
    win.focus()
  }
  if (!url) return
  pendingAuthCallback = url
  if (win) win.webContents.send('ffcom:auth-callback')
}

function registerAuthIpc() {
  ipcMain.handle('ffcom:open-external-sign-in', (_event, url: unknown) => {
    if (typeof url !== 'string' || !url.startsWith('https://')) return false
    void shell.openExternal(url)
    return true
  })
  ipcMain.handle('ffcom:take-auth-callback', () => {
    const url = pendingAuthCallback
    pendingAuthCallback = undefined
    return url ?? null
  })
}

// Só o app empacotado se registra como dono do ffcom://: o dev apontaria o
// esquema para o electron.exe do node_modules e roubaria o retorno do login
// do app instalado. No dev o login continua dentro da janela.
const gotSingleInstanceLock = app.requestSingleInstanceLock()
if (!gotSingleInstanceLock) {
  app.quit()
} else {
  if (app.isPackaged) app.setAsDefaultProtocolClient(AUTH_SCHEME)
  pendingAuthCallback = process.argv.find(isAuthCallbackUrl)
  app.on('second-instance', (_event, argv) => receiveAuthCallback(argv))
  app.on('open-url', (event, url) => {
    event.preventDefault()
    receiveAuthCallback(url)
  })
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

// Push-to-talk com o app em segundo plano (ver docs/architecture.md,
// "Decisão: push-to-talk", revisão de 2026-09-29). O globalShortcut só
// entrega o apertar; o hook de teclado nativo (uiohook-napi, binário N-API
// pré-compilado por plataforma) entrega também o soltar. Com a janela em
// foco quem trata a tecla é o renderer (que ignora campos de digitação), então
// o apertar só é repassado com a janela fora de foco; o soltar é repassado
// sempre, para fechar uma fala que começou fora de foco. O hook só roda
// enquanto há uma tecla registrada (conectado na voz, modo "apertar para
// falar").
interface PushToTalkKey {
  keycode: number
  ctrl: boolean
  alt: boolean
  shift: boolean
  meta: boolean
}

function pushToTalkKeycode(key: string): number | undefined {
  const keys = UiohookKey as Record<string, number>
  if (/^[A-Z0-9]$/.test(key) || /^F([1-9]|1\d|2[0-4])$/.test(key) || key === 'Space') return keys[key]
  const m = /^num(\d)$/.exec(key)
  return m ? keys[`Numpad${m[1]}`] : undefined
}

let pushToTalkKey: PushToTalkKey | undefined
let pushToTalkWindow: BrowserWindow | undefined
let pushToTalkHeld = false
let hookRunning = false

function onHookKeyDown(e: UiohookKeyboardEvent) {
  const k = pushToTalkKey
  const win = pushToTalkWindow
  if (!k || !win || win.isDestroyed() || e.keycode !== k.keycode || pushToTalkHeld) return
  if (e.ctrlKey !== k.ctrl || e.altKey !== k.alt || e.shiftKey !== k.shift || e.metaKey !== k.meta) return
  if (win.isFocused()) return
  pushToTalkHeld = true
  win.webContents.send('ffcom:push-to-talk', true)
}

function onHookKeyUp(e: UiohookKeyboardEvent) {
  const k = pushToTalkKey
  const win = pushToTalkWindow
  // Só a tecla principal: os modificadores podem ser soltos antes.
  if (!k || e.keycode !== k.keycode) return
  pushToTalkHeld = false
  if (win && !win.isDestroyed()) win.webContents.send('ffcom:push-to-talk', false)
}

function stopPushToTalkHook() {
  pushToTalkKey = undefined
  pushToTalkHeld = false
  if (hookRunning) {
    uIOhook.stop()
    hookRunning = false
  }
}

function registerPushToTalkIpc() {
  uIOhook.on('keydown', onHookKeyDown)
  uIOhook.on('keyup', onHookKeyUp)
  ipcMain.handle('ffcom:set-push-to-talk-key', (event, accelerator: unknown) => {
    stopPushToTalkHook()
    if (accelerator === null) return true
    if (typeof accelerator !== 'string' || !SHORTCUT_FORMAT.test(accelerator)) return false
    const parts = accelerator.split('+')
    const keycode = pushToTalkKeycode(parts[parts.length - 1])
    const win = BrowserWindow.fromWebContents(event.sender)
    if (keycode === undefined || !win) return false
    pushToTalkKey = {
      keycode,
      ctrl: parts.includes('Ctrl'),
      alt: parts.includes('Alt'),
      shift: parts.includes('Shift'),
      meta: parts.includes('Super'),
    }
    pushToTalkWindow = win
    try {
      uIOhook.start()
      hookRunning = true
      return true
    } catch (err) {
      // Ex. macOS sem a permissão de Acessibilidade: segue só com a janela em foco.
      console.warn('[ffcom] push-to-talk em segundo plano indisponível', err)
      pushToTalkKey = undefined
      return false
    }
  })
}

// Atualização do app desktop pelo mesmo botão verde do web (ver
// docs/architecture.md, "Decisão: distribuição e atualização do app
// desktop"). O electron-updater lê o latest.yml da release fixa
// desktop-stable (app-update.yml, gerado pelo electron-builder a partir de
// build.publish no package.json), baixa a versão nova em segundo plano e só
// então avisa o renderer. Clicar instala e reabre; sem clicar, instala ao
// fechar o app. Só no app empacotado: no dev não há app-update.yml.
const UPDATE_CHECK_INTERVAL_MS = 30 * 60_000
let updateDownloaded = false

function registerAutoUpdate() {
  const { autoUpdater } = electronUpdater
  ipcMain.handle('ffcom:get-update-ready', () => updateDownloaded)
  ipcMain.handle('ffcom:apply-update', () => {
    // isSilent: sem a tela do instalador; isForceRunAfter: reabre o app.
    if (updateDownloaded) autoUpdater.quitAndInstall(true, true)
  })
  if (!app.isPackaged) return
  autoUpdater.on('update-downloaded', () => {
    updateDownloaded = true
    for (const win of BrowserWindow.getAllWindows()) win.webContents.send('ffcom:update-ready')
  })
  autoUpdater.on('error', (err) => console.warn('[ffcom] atualização', err))
  const check = () => {
    autoUpdater.checkForUpdates().catch((err: unknown) => console.warn('[ffcom] atualização', err))
  }
  check()
  setInterval(check, UPDATE_CHECK_INTERVAL_MS)
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
  if (!gotSingleInstanceLock) return
  registerAppProtocol()
  registerAuthIpc()
  registerMuteShortcutIpc()
  registerPushToTalkIpc()
  registerSystemIdleIpc()
  registerDisplayMediaIpc()
  registerAutoUpdate()
  createWindow()
})

app.on('will-quit', () => {
  globalShortcut.unregisterAll()
  stopPushToTalkHook()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow()
})
