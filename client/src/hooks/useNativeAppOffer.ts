import { useCallback, useEffect, useState } from 'react'
import { isNativeAppInstalled, nativeAppForVisitor, startNativeAppDownload, type NativeApp } from '../lib/nativeApps'

// "Agora não" no botão de instalar esconde o botão por 30 dias neste
// navegador. O item do menu do avatar continua lá.
const DISMISS_KEY = 'ffcom.installApp.dismissedUntil'
const DISMISS_MS = 30 * 24 * 60 * 60_000

function readDismissed(): boolean {
  try {
    const until = Number(localStorage.getItem(DISMISS_KEY))
    return Number.isFinite(until) && until > Date.now()
  } catch {
    return false
  }
}

function writeDismissed(): void {
  try {
    localStorage.setItem(DISMISS_KEY, String(Date.now() + DISMISS_MS))
  } catch {
    // Sem localStorage (modo privado restrito): esconde só nesta sessão.
  }
}

export interface NativeAppOffer {
  app: NativeApp
  // O botão do rail aparece (não dispensado e app não detectado no aparelho).
  showButton: boolean
  // Diálogo de explicação aberto (antes do download no Android, junto com o
  // download no Windows).
  dialogOpen: boolean
  // Clique no botão ou no item do menu.
  start: () => void
  // Baixa a partir do diálogo de confirmação.
  download: () => void
  closeDialog: () => void
  // "Agora não": esconde o botão por 30 dias.
  dismiss: () => void
}

// Oferta do app nativo para quem usa o FFCom no navegador. undefined dentro
// dos apps nativos e em sistema sem app (lib/nativeApps.ts). Ver
// docs/architecture.md, "Decisão: botão de instalar o app nativo pelo
// navegador".
export function useNativeAppOffer(): NativeAppOffer | undefined {
  const [app] = useState(nativeAppForVisitor)
  const [dismissed, setDismissed] = useState(readDismissed)
  const [installed, setInstalled] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)

  useEffect(() => {
    if (!app) return
    let cancelled = false
    void isNativeAppInstalled(app).then((value) => {
      if (!cancelled) setInstalled(value)
    })
    return () => {
      cancelled = true
    }
  }, [app])

  const start = useCallback(() => {
    if (!app) return
    if (app.flow === 'direct') startNativeAppDownload(app)
    setDialogOpen(true)
  }, [app])

  const download = useCallback(() => {
    if (!app) return
    startNativeAppDownload(app)
    setDialogOpen(false)
  }, [app])

  const closeDialog = useCallback(() => setDialogOpen(false), [])

  const dismiss = useCallback(() => {
    writeDismissed()
    setDismissed(true)
    setDialogOpen(false)
  }, [])

  if (!app) return undefined
  return {
    app,
    showButton: !dismissed && !installed,
    dialogOpen,
    start,
    download,
    closeDialog,
    dismiss,
  }
}
