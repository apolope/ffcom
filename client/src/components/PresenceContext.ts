import { createContext, useContext } from 'react'
import type { TFunction } from 'i18next'
import type { AccountSummary } from '../lib/serverCentralApi'
import type { ChosenStatus, PresenceStatus } from '../types'

// O que as listas de membros precisam para desenhar avatar e status sem
// receber amigos, perfil e contas por props em cada nível. App.tsx monta o
// valor. Ver docs/architecture.md, "Decisão: status de presença e avatar nas
// listas de membros".
export interface PresenceContextValue {
  // Status visível de uma conta: o seu próprio, o de um amigo, ou 'offline'
  // para quem não é amigo (status só é visível entre amigos).
  statusOf: (accountId: string | undefined) => PresenceStatus
  // Conta de server-central por trás de um membro de server-channel.
  accountOf: (oidcSubject: string | undefined) => AccountSummary | undefined
}

export const PresenceContext = createContext<PresenceContextValue>({
  statusOf: () => 'offline',
  accountOf: () => undefined,
})

export function usePresence(): PresenceContextValue {
  return useContext(PresenceContext)
}

// Como a própria pessoa aparece: invisível vira offline (é o que os outros
// veem) e online ocioso vira ausente, igual ao cálculo do servidor.
export function visibleOwnStatus(chosen: ChosenStatus | undefined, idle: boolean): PresenceStatus {
  if (chosen === 'invisible') return 'offline'
  if ((chosen ?? 'online') === 'online') return idle ? 'away' : 'online'
  return chosen as PresenceStatus
}

// Rótulo do status no idioma ativo. Recebe o `t` de useTranslation() para
// ser chamado no render e acompanhar a troca de idioma.
export function statusLabel(t: TFunction, status: PresenceStatus | 'invisible'): string {
  switch (status) {
    case 'online':
      return t('presence.online')
    case 'busy':
      return t('presence.busy')
    case 'away':
      return t('presence.away')
    case 'offline':
      return t('presence.offline')
    case 'invisible':
      return t('presence.invisible')
  }
}
