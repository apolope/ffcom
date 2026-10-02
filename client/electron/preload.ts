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
  setPushToTalkKey: (accelerator: string | null): Promise<boolean> =>
    ipcRenderer.invoke('ffcom:set-push-to-talk-key', accelerator),
  onPushToTalk: (callback: (pressed: boolean) => void): (() => void) => {
    const listener = (_event: IpcRendererEvent, pressed: boolean) => callback(pressed)
    ipcRenderer.on('ffcom:push-to-talk', listener)
    return () => {
      ipcRenderer.removeListener('ffcom:push-to-talk', listener)
    }
  },
  getUpdateReady: (): Promise<boolean> => ipcRenderer.invoke('ffcom:get-update-ready'),
  onUpdateReady: (callback: () => void): (() => void) => {
    const listener = (_event: IpcRendererEvent) => callback()
    ipcRenderer.on('ffcom:update-ready', listener)
    return () => {
      ipcRenderer.removeListener('ffcom:update-ready', listener)
    }
  },
  applyUpdate: (): Promise<void> => ipcRenderer.invoke('ffcom:apply-update'),
  getSystemIdleSeconds: (): Promise<number> => ipcRenderer.invoke('ffcom:get-system-idle-seconds'),
  getDisplaySources: (): Promise<unknown> => ipcRenderer.invoke('ffcom:get-display-sources'),
  chooseDisplaySource: (id: string, audio: boolean): Promise<boolean> =>
    ipcRenderer.invoke('ffcom:choose-display-source', id, audio),
  openExternalSignIn: (url: string): Promise<boolean> => ipcRenderer.invoke('ffcom:open-external-sign-in', url),
  takeAuthCallback: (): Promise<string | null> => ipcRenderer.invoke('ffcom:take-auth-callback'),
  onAuthCallback: (callback: () => void): (() => void) => {
    const listener = (_event: IpcRendererEvent) => callback()
    ipcRenderer.on('ffcom:auth-callback', listener)
    return () => {
      ipcRenderer.removeListener('ffcom:auth-callback', listener)
    }
  },
  // O áudio do sistema ('loopback' no main) só existe no Windows.
  canShareSystemAudio: process.platform === 'win32',
})
