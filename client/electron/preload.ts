// Preload script — roda antes do renderer carregar e expõe ao renderer só a
// ponte abaixo (tipos em src/electron.d.ts), nunca o ipcRenderer inteiro.
import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron'

contextBridge.exposeInMainWorld('ffcomElectron', {
  setMuteShortcut: (accelerator: string | null): Promise<boolean> =>
    ipcRenderer.invoke('ffcom:set-mute-shortcut', accelerator),
  onMuteShortcut: (callback: () => void): (() => void) => {
    const listener = (_event: IpcRendererEvent) => callback()
    ipcRenderer.on('ffcom:mute-shortcut', listener)
    return () => {
      ipcRenderer.removeListener('ffcom:mute-shortcut', listener)
    }
  },
  getSystemIdleSeconds: (): Promise<number> => ipcRenderer.invoke('ffcom:get-system-idle-seconds'),
  getDisplaySources: (): Promise<unknown> => ipcRenderer.invoke('ffcom:get-display-sources'),
  chooseDisplaySource: (id: string, audio: boolean): Promise<boolean> =>
    ipcRenderer.invoke('ffcom:choose-display-source', id, audio),
  // O áudio do sistema ('loopback' no main) só existe no Windows.
  canShareSystemAudio: process.platform === 'win32',
})
