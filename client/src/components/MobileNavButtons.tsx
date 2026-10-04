import { useContext } from 'react'
import { useTranslation } from 'react-i18next'
import { MobileNavContext } from './MobileNavContext'
import './MobileNav.css'

// Botão ☰ que abre a gaveta de servidores e canais, no começo do cabeçalho.
export function MobileNavButton() {
  const { isMobile, openNav } = useContext(MobileNavContext)
  const { t } = useTranslation()
  if (!isMobile) return null
  return (
    <button type="button" className="mobile-nav-button" aria-label={t('server.openNav')} onClick={openNav}>
      ☰
    </button>
  )
}

// Botão que abre a gaveta de membros, no fim do cabeçalho.
export function MobileMembersButton() {
  const { isMobile, openMembers } = useContext(MobileNavContext)
  const { t } = useTranslation()
  if (!isMobile || !openMembers) return null
  return (
    <button
      type="button"
      className="mobile-nav-button mobile-members-button"
      aria-label={t('members.openList')}
      onClick={openMembers}
    >
      👥
    </button>
  )
}
