// Ponte exposta por electron/preload.ts via contextBridge. Ausente no
// web/PWA: sempre checar `window.ffcomElectron` antes de usar.
interface FfcomElectronBridge {
  // Registra (ou, com null, remove) o atalho global de mutar. Resolve false
  // se o sistema recusar o registro (combinação já usada por outro programa).
  setMuteShortcut(accelerator: string | null): Promise<boolean>
  // Chamado a cada aperto do atalho global; devolve a função que remove o
  // listener.
  onMuteShortcut(callback: () => void): () => void
  // Liga (ou, com null, desliga) o push-to-talk em segundo plano: o main
  // passa a avisar o apertar da tecla com a janela fora de foco e o soltar
  // sempre. Resolve false se o hook de teclado não subir (ex. macOS sem
  // permissão de Acessibilidade).
  setPushToTalkKey(accelerator: string | null): Promise<boolean>
  // true ao apertar, false ao soltar; devolve a função que remove o listener.
  onPushToTalk(callback: (pressed: boolean) => void): () => void
  // Atualização do app desktop (electron-updater no main): true quando uma
  // versão nova já foi baixada e espera o clique no botão de atualizar.
  getUpdateReady(): Promise<boolean>
  // Chamado quando termina de baixar uma versão nova; devolve a função que
  // remove o listener.
  onUpdateReady(callback: () => void): () => void
  // Fecha, instala a versão baixada e reabre o app.
  applyUpdate(): Promise<void>
  // Segundos sem teclado nem mouse no sistema inteiro (powerMonitor), para
  // o "ausente" automático (hooks/useIdle.ts).
  getSystemIdleSeconds(): Promise<number>
  // Telas e janelas que dá para compartilhar, com miniatura (data URL; vazia
  // quando o sistema não entrega, ex. macOS sem permissão de gravação).
  getDisplaySources(): Promise<DisplaySource[]>
  // Guarda a fonte escolhida no seletor para o próximo getDisplayMedia
  // (setScreenShareEnabled). Resolve false se o id não veio da última
  // listagem.
  chooseDisplaySource(id: string, audio: boolean): Promise<boolean>
  // Login no navegador do sistema (só no app empacotado): abre a URL de
  // autorização do Authentik fora do app. Resolve false se a URL não for https.
  openExternalSignIn(url: string): Promise<boolean>
  // Devolve (e esquece) a URL ffcom://auth/callback que o sistema entregou ao
  // app, ou null se não houver nenhuma esperando.
  takeAuthCallback(): Promise<string | null>
  // Chamado quando chega uma URL de retorno do login; devolve a função que
  // remove o listener. A URL em si vem de takeAuthCallback.
  onAuthCallback(callback: () => void): () => void
  // true no Windows, o único sistema em que o Electron captura o áudio do
  // computador junto com a tela.
  canShareSystemAudio: boolean
}

interface DisplaySource {
  id: string
  name: string
  kind: 'screen' | 'window'
  thumbnail: string
}

interface Window {
  ffcomElectron?: FfcomElectronBridge
}
