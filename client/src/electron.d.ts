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
