// Ponte exposta por electron/preload.ts via contextBridge. Ausente no
// web/PWA: sempre checar `window.ffcomElectron` antes de usar.
interface FfcomElectronBridge {
  // Registra (ou, com null, remove) o atalho global de mutar. Resolve false
  // se o sistema recusar o registro (combinação já usada por outro programa).
  setMuteShortcut(accelerator: string | null): Promise<boolean>
  // Chamado a cada aperto do atalho global; devolve a função que remove o
  // listener.
  onMuteShortcut(callback: () => void): () => void
  // Segundos sem teclado nem mouse no sistema inteiro (powerMonitor), para
  // o "ausente" automático (hooks/useIdle.ts).
  getSystemIdleSeconds(): Promise<number>
}

interface Window {
  ffcomElectron?: FfcomElectronBridge
}
