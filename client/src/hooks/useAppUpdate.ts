import { useCallback, useEffect, useRef } from 'react'
import { useRegisterSW } from 'virtual:pwa-register/react'

// De quanto em quanto tempo perguntar ao servidor se há sw.js novo. Sem
// isso o navegador só confere numa navegação nova, e um PWA instalado no
// celular fica dias aberto em segundo plano sem navegar.
const UPDATE_CHECK_INTERVAL_MS = 5 * 60_000

// Prazo para recarregar mesmo que o worker novo não avise que ativou.
const RELOAD_FALLBACK_MS = 3_000

interface UseAppUpdateResult {
  // Uma versão nova já foi baixada e está esperando para ativar.
  updateReady: boolean
  // Ativa a versão nova e recarrega a página.
  applyUpdate: () => void
}

// Registra o service worker do PWA e confere periodicamente se há versão
// nova do client. Chamado no topo do App (antes do login), para registrar
// também para quem ainda não entrou. A troca não é automática para quem
// está logado, porque recarregar derruba uma chamada de voz: o ServerRail
// mostra o botão de atualizar (components/UpdateButton.tsx). Ver
// docs/architecture.md, "Decisão: botão de atualizar o client".
//
// No build Electron o plugin está desligado e o módulo virtual é vazio:
// updateReady nunca vira true.
export function useAppUpdate(): UseAppUpdateResult {
  const registrationRef = useRef<ServiceWorkerRegistration | undefined>(undefined)
  const {
    needRefresh: [updateReady],
    updateServiceWorker,
  } = useRegisterSW({
    onRegisteredSW(_swUrl, registration) {
      registrationRef.current = registration
    },
  })

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
  const applyUpdate = useCallback(() => {
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

  return { updateReady, applyUpdate }
}
