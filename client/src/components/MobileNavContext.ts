import { createContext } from 'react'

// Layout móvel (docs/architecture.md, "Decisão: layout móvel com gavetas"):
// abaixo de MOBILE_QUERY o painel principal ocupa a tela inteira e
// servidores + canais (à esquerda) e membros (à direita) viram gavetas.
// App.tsx monta o valor; os cabeçalhos dos painéis mostram os botões.
export const MOBILE_QUERY = '(max-width: 768px)'

export interface MobileNavValue {
  isMobile: boolean
  openNav: () => void
  // Ausente quando a tela aberta não tem lista de membros (Amigos/DM).
  openMembers?: () => void
}

export const MobileNavContext = createContext<MobileNavValue>({
  isMobile: false,
  openNav: () => {},
})
