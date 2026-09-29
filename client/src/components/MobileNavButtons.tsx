import { useContext } from 'react'
import { MobileNavContext } from './MobileNavContext'
import './MobileNav.css'

// Botão ☰ que abre a gaveta de servidores e canais, no começo do cabeçalho.
export function MobileNavButton() {
  const { isMobile, openNav } = useContext(MobileNavContext)
  if (!isMobile) return null
  return (
    <button type="button" className="mobile-nav-button" aria-label="Abrir servidores e canais" onClick={openNav}>
      ☰
    </button>
  )
}

// Botão que abre a gaveta de membros, no fim do cabeçalho.
export function MobileMembersButton() {
  const { isMobile, openMembers } = useContext(MobileNavContext)
  if (!isMobile || !openMembers) return null
  return (
    <button
      type="button"
      className="mobile-nav-button mobile-members-button"
      aria-label="Abrir lista de membros"
      onClick={openMembers}
    >
      👥
    </button>
  )
}
