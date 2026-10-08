import { registerPlugin, type PluginListenerHandle } from '@capacitor/core'
import { isAndroidApp } from './platform'

// Atualização feita fora da página, pela casca nativa: o electron-updater no
// app desktop (electron/main.ts) e o UpdatePlugin no app Android
// (client/android/.../UpdatePlugin.java). Os dois baixam a versão nova em
// segundo plano e só então avisam; hooks/useAppUpdate.ts junta esse aviso ao
// do service worker e acende o mesmo botão verde (components/UpdateButton.tsx).
// Ver docs/architecture.md, "Decisão: distribuição e atualização do app
// desktop" e "Decisão: atualização do APK (fase 2)".

export type NativeUpdateKind = 'desktop' | 'android'

// started: a instalação começou (o app fecha ou o instalador abre).
// needs-permission: o Android ainda não deixa este app instalar APKs; o client
// explica e chama requestInstallPermission.
export type NativeApplyResult = 'started' | 'needs-permission'

export interface NativeUpdater {
  kind: NativeUpdateKind
  // true quando uma versão nova já foi baixada (e, no Android, conferida).
  getUpdateReady(): Promise<boolean>
  // Chamado quando termina de baixar uma versão nova; devolve a função que
  // remove o listener.
  onUpdateReady(callback: () => void): () => void
  applyUpdate(): Promise<NativeApplyResult>
  // Só no Android: leva às configurações de "Permitir desta fonte" e resolve
  // true se a pessoa liberou.
  requestInstallPermission?(): Promise<boolean>
}

// O updater da plataforma atual, ou undefined no navegador/PWA (lá só o
// service worker atualiza).
export function getNativeUpdater(): NativeUpdater | undefined {
  const electron = window.ffcomElectron
  if (electron) {
    return {
      kind: 'desktop',
      getUpdateReady: () => electron.getUpdateReady(),
      onUpdateReady: (callback) => electron.onUpdateReady(callback),
      applyUpdate: async () => {
        await electron.applyUpdate()
        return 'started'
      },
    }
  }
  if (isAndroidApp()) return androidUpdater()
  return undefined
}

// Contrato com o UpdatePlugin.java (@CapacitorPlugin(name = "FfcomUpdate")).
interface UpdateInfo {
  ready: boolean
  version?: string
  versionCode?: number
}

interface FfcomUpdatePlugin {
  getUpdateReady(): Promise<UpdateInfo>
  canInstall(): Promise<{ granted: boolean }>
  openInstallSettings(): Promise<{ granted: boolean }>
  // Rejeita com code "needs-permission" ou "not-ready".
  install(): Promise<void>
  addListener(event: 'updateReady', callback: (info: UpdateInfo) => void): Promise<PluginListenerHandle>
}

function androidUpdater(): NativeUpdater {
  const plugin = registerPlugin<FfcomUpdatePlugin>('FfcomUpdate')
  return {
    kind: 'android',
    getUpdateReady: async () => {
      try {
        return (await plugin.getUpdateReady()).ready
      } catch {
        return false
      }
    },
    onUpdateReady: (callback) => {
      let removed = false
      let handle: PluginListenerHandle | undefined
      // Num APK da fase 1 (sem o plugin) rodando a parte web nova, o
      // Capacitor rejeita por não implementado: fica sem aviso, sem erro.
      Promise.resolve()
        .then(() =>
          plugin.addListener('updateReady', (info) => {
            if (info.ready) callback()
          }),
        )
        .then((h) => {
          handle = h
          if (removed) void h.remove()
        })
        .catch(() => {})
      return () => {
        removed = true
        void handle?.remove()
      }
    },
    applyUpdate: async () => {
      if (!(await plugin.canInstall()).granted) return 'needs-permission'
      try {
        await plugin.install()
      } catch (err) {
        if ((err as { code?: string }).code === 'needs-permission') return 'needs-permission'
        throw err
      }
      return 'started'
    },
    requestInstallPermission: async () => (await plugin.openInstallSettings()).granted,
  }
}
