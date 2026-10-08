import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useRegisterSW } from 'virtual:pwa-register/react'
import { getNativeUpdater, type NativeUpdateKind } from '../lib/nativeUpdater'

// De quanto em quanto tempo perguntar ao servidor se há sw.js novo. Sem
// isso o navegador só confere numa navegação nova, e um PWA instalado no
// celular fica dias aberto em segundo plano sem navegar.
const UPDATE_CHECK_INTERVAL_MS = 5 * 60_000

// Prazo para recarregar mesmo que o worker novo não avise que ativou.
const RELOAD_FALLBACK_MS = 3_000

export type UpdateKind = 'web' | NativeUpdateKind

interface UseAppUpdateResult {
  // Uma versão nova já foi baixada e está esperando para ativar.
  updateReady: boolean
  // De quem é a versão esperando (texto do botão); undefined sem nenhuma.
  updateKind: UpdateKind | undefined
  // Ativa a versão nova: recarrega a página, reinstala o desktop ou abre o
  // instalador do Android.
  applyUpdate: () => void
  // Presente quando dá para aplicar sem ninguém tocar (sem sessão): o
  // service worker e o desktop. O APK do Android nunca, porque abre o
  // instalador na tela.
  applyUpdateSilently: (() => void) | undefined
  // Presente quando o Android pediu "Permitir desta fonte" antes de
  // instalar: o App mostra a explicação (InstallPermissionDialog).
  installPermission: { allow: () => void; dismiss: () => void } | undefined
}

// Registra o service worker do PWA e confere periodicamente se há versão
// nova do client. Chamado no topo do App (antes do login), para registrar
// também para quem ainda não entrou. A troca não é automática para quem
// está logado, porque recarregar derruba uma chamada de voz: o ServerRail
// mostra o botão de atualizar (components/UpdateButton.tsx). Ver
// docs/architecture.md, "Decisão: botão de atualizar o client".
//
// No build Electron o plugin está desligado e o módulo virtual é vazio; lá
// quem baixa a versão nova é o electron-updater no main (electron/main.ts),
// e este hook só repassa o aviso e o clique pela ponte. No app Android as
// duas coisas existem: o service worker atualiza a parte web e o
// UpdatePlugin baixa o APK novo (lib/nativeUpdater.ts); com os dois
// esperando, o clique instala o APK, que ao reabrir já carrega a parte web
// nova. Ver docs/architecture.md, "Decisão: atualização do APK (fase 2)".
export function useAppUpdate(): UseAppUpdateResult {
  const [native] = useState(getNativeUpdater)
  const [nativeUpdateReady, setNativeUpdateReady] = useState(false)
  useEffect(() => {
    if (!native) return
    const unsubscribe = native.onUpdateReady(() => setNativeUpdateReady(true))
    void native.getUpdateReady().then((ready) => {
      if (ready) setNativeUpdateReady(true)
    })
    return unsubscribe
  }, [native])
  const [installPermissionNeeded, setInstallPermissionNeeded] = useState(false)

  const registrationRef = useRef<ServiceWorkerRegistration | undefined>(undefined)
  // O needRefresh do vite-plugin-pwa vem do workbox-window, que para de
  // ouvir "updatefound" depois da primeira atualização vinda de fora do
  // register() (qualquer uma que chegue mais de 1 min depois de abrir a
  // página, ou seja, todas as da checagem periódica). Se essa primeira não
  // chega a "waiting" (instalação que falha no meio de um deploy, rede ruim
  // no celular), as versões seguintes nunca acendem o botão até a página
  // ser recarregada. Por isso o worker esperando também é acompanhado aqui,
  // direto na registration.
  const [waitingFound, setWaitingFound] = useState(false)
  const {
    needRefresh: [needRefresh],
    updateServiceWorker,
  } = useRegisterSW({
    onRegisteredSW(_swUrl, registration) {
      if (!registration) return
      registrationRef.current = registration
      const sync = () => setWaitingFound(!!registration.waiting)
      registration.addEventListener('updatefound', () => {
        registration.installing?.addEventListener('statechange', sync)
      })
      sync()
    },
  })
  const updateReady = needRefresh || waitingFound

  useEffect(() => {
    const check = () => {
      const registration = registrationRef.current
      // Offline, update() rejeita; tenta de novo no próximo ciclo.
      if (registration && navigator.onLine) registration.update().catch(() => {})
    }
    const interval = setInterval(check, UPDATE_CHECK_INTERVAL_MS)
    // Voltar ao app (trocar de aba, desbloquear o celular) também confere:
    // é o momento em que um PWA parado há horas volta a ser usado.
    const onVisibility = () => {
      if (document.visibilityState === 'visible') check()
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      clearInterval(interval)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [])

  // updateServiceWorker(true) só manda SKIP_WAITING: o reload do
  // vite-plugin-pwa depende do evento "controlling" com isUpdate, que não
  // vem numa página aberta sem service worker controlando (Ctrl+F5, ou a
  // primeira visita). Nesse caso a versão nova ativava em silêncio e o
  // clique "não fazia nada". Por isso o reload é feito aqui: quando o
  // worker que estava esperando chega a "activated", com um prazo de
  // segurança; sem nada esperando (já ativou antes), recarrega direto.
  const applyWebUpdate = useCallback(() => {
    const waiting = registrationRef.current?.waiting
    if (!waiting) {
      window.location.reload()
      return
    }
    let reloading = false
    const reload = () => {
      if (reloading) return
      reloading = true
      window.location.reload()
    }
    waiting.addEventListener('statechange', () => {
      if (waiting.state === 'activated') reload()
    })
    setTimeout(reload, RELOAD_FALLBACK_MS)
    void updateServiceWorker(true)
  }, [updateServiceWorker])

  const applyNativeUpdate = useCallback(() => {
    if (!native) return
    native
      .applyUpdate()
      .then((result) => {
        if (result === 'needs-permission') setInstallPermissionNeeded(true)
      })
      .catch((err: unknown) => console.warn('[ffcom] atualização', err))
  }, [native])

  // No desktop o service worker está desligado: o clique é sempre do
  // electron-updater, como antes.
  const nativeFirst = native?.kind === 'desktop' || (native !== undefined && nativeUpdateReady)
  const applyUpdate = nativeFirst ? applyNativeUpdate : applyWebUpdate
  const updateKind: UpdateKind | undefined =
    native && nativeUpdateReady ? native.kind : updateReady ? 'web' : undefined

  let applyUpdateSilently: (() => void) | undefined
  if (native?.kind === 'desktop') {
    if (nativeUpdateReady) applyUpdateSilently = applyNativeUpdate
  } else if (updateReady) {
    applyUpdateSilently = applyWebUpdate
  }

  const installPermission = useMemo(() => {
    if (!installPermissionNeeded || !native?.requestInstallPermission) return undefined
    const request = native.requestInstallPermission
    return {
      // Vai às configurações; voltando com a fonte liberada, já abre o
      // instalador. Se o Android reiniciar o app nesse meio-tempo, o
      // UpdatePlugin acha o APK de novo e o botão volta a aparecer.
      allow: () => {
        setInstallPermissionNeeded(false)
        request()
          .then((granted) => {
            if (granted) applyNativeUpdate()
          })
          .catch((err: unknown) => console.warn('[ffcom] atualização', err))
      },
      dismiss: () => setInstallPermissionNeeded(false),
    }
  }, [installPermissionNeeded, native, applyNativeUpdate])

  return {
    updateReady: updateKind !== undefined,
    updateKind,
    applyUpdate,
    applyUpdateSilently,
    installPermission,
  }
}
