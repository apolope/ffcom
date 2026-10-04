import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { SUPPORTED_LANGUAGES, type Language } from '../i18n'
import { useMenuDismiss } from '../hooks/useMenuDismiss'
import type { ChosenStatus } from '../types'
import { statusLabel } from './PresenceContext'
import './RailMenu.css'
import { UserAvatar } from './UserAvatar'
import './StatusMenu.css'

const OPTIONS: ChosenStatus[] = ['online', 'busy', 'away', 'invisible']

// Quem está logado, para o cabeçalho do menu.
export interface AccountIdentity {
  displayName: string
  avatarUrl?: string
  // preferred_username do Authentik; omitido quando igual ao displayName.
  username?: string
  email?: string
  // Apelido no servidor aberto, se houver.
  nickname?: string
  serverName?: string
}

interface StatusMenuProps {
  // O botão do avatar: dá a posição e não conta como "clique fora" (o
  // próprio botão já alterna o menu).
  anchor: HTMLElement
  identity: AccountIdentity
  chosen: ChosenStatus
  onChoose: (status: ChosenStatus) => void
  // Troca o idioma da interface (na hora, sem fechar o menu) e grava na conta.
  onChooseLanguage: (language: Language) => void
  onEditAvatar: () => void
  // Nome de exibição da conta, em server-central.
  onEditDisplayName?: () => void
  // Apelido é por server-channel: só vem com um servidor aberto.
  onEditNickname?: () => void
  onClose: () => void
}

// Menu do próprio avatar no ServerRail: mostra quem está logado (nome,
// usuário e e-mail do Authentik, apelido no servidor aberto), escolhe o
// status e abre os diálogos de nome de exibição (da conta), de apelido (do
// servidor aberto) e de avatar. Posição fixa ao lado do botão, porque o rail
// rola (overflow-y) e cortaria um menu posicionado dentro dele. Fecha com Esc ou
// clique fora. Ver docs/architecture.md, "Decisão: status de presença e
// avatar nas listas de membros".
export function StatusMenu({
  anchor,
  identity,
  chosen,
  onChoose,
  onChooseLanguage,
  onEditAvatar,
  onEditDisplayName,
  onEditNickname,
  onClose,
}: StatusMenuProps) {
  const ref = useRef<HTMLDivElement>(null)
  const { t, i18n } = useTranslation()
  const hints: Partial<Record<ChosenStatus, string>> = {
    busy: t('profile.statusMenu.busyHint'),
    invisible: t('profile.statusMenu.invisibleHint'),
  }
  const serverName = identity.serverName ?? t('profile.statusMenu.thisServer')
  // O nome de cada idioma fica no próprio idioma (igual nos dois arquivos),
  // para quem caiu num idioma que não lê achar o seu.
  const languageNames: Record<Language, string> = {
    'pt-BR': t('settings.languageNames.pt-BR'),
    en: t('settings.languageNames.en'),
  }

  useMenuDismiss(ref, anchor, onClose)
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
  }, [])

  const rect = anchor.getBoundingClientRect()

  return (
    <div
      ref={ref}
      className="rail-menu"
      role="menu"
      aria-label={t('profile.statusMenu.label')}
      style={{ left: rect.right + 8, bottom: Math.max(8, window.innerHeight - rect.bottom) }}
    >
      <div className="status-menu-identity">
        <UserAvatar avatarUrl={identity.avatarUrl} displayName={identity.displayName} size={40} />
        <div className="status-menu-identity-text">
          <strong title={identity.displayName}>{identity.displayName}</strong>
          {identity.username && <span title={identity.username}>@{identity.username}</span>}
          {identity.email && <span title={identity.email}>{identity.email}</span>}
          {identity.nickname && (
            <span title={t('profile.statusMenu.nicknameIn', { server: serverName })}>
              {t('profile.statusMenu.nicknameInValue', { server: serverName, nickname: identity.nickname })}
            </span>
          )}
        </div>
      </div>
      <hr />
      {OPTIONS.map((status) => (
        <button
          key={status}
          type="button"
          role="menuitemradio"
          aria-checked={status === chosen}
          className="rail-menu-item"
          onClick={() => {
            onChoose(status)
            onClose()
          }}
        >
          <span className={`status-menu-dot presence-${status === 'invisible' ? 'offline' : status}`} aria-hidden="true" />
          <span>
            {statusLabel(t, status)}
            {hints[status] && <span className="status-menu-hint">{hints[status]}</span>}
          </span>
        </button>
      ))}
      <hr />
      {onEditDisplayName && (
        <button
          type="button"
          role="menuitem"
          className="rail-menu-item"
          onClick={() => {
            onEditDisplayName()
            onClose()
          }}
        >
          {t('profile.statusMenu.editDisplayName')}
        </button>
      )}
      {onEditNickname && (
        <button
          type="button"
          role="menuitem"
          className="rail-menu-item"
          onClick={() => {
            onEditNickname()
            onClose()
          }}
        >
          {t('profile.statusMenu.editNickname')}
        </button>
      )}
      <button
        type="button"
        role="menuitem"
        className="rail-menu-item"
        onClick={() => {
          onEditAvatar()
          onClose()
        }}
      >
        {t('profile.statusMenu.editAvatar')}
      </button>
      <hr />
      <div role="group" aria-labelledby="status-menu-language">
        <span id="status-menu-language" className="status-menu-section">
          {t('settings.language')}
        </span>
        {SUPPORTED_LANGUAGES.map((language) => (
          <button
            key={language}
            type="button"
            role="menuitemradio"
            aria-checked={i18n.resolvedLanguage === language}
            className="rail-menu-item"
            lang={language}
            onClick={() => onChooseLanguage(language)}
          >
            {languageNames[language]}
          </button>
        ))}
      </div>
      <hr />
      {/* Aviso de licença: o código é AGPL e o bundle leva código de
          terceiros (arquivo gerado no build por
          vite-plugin-third-party-licenses.ts). */}
      <p className="status-menu-legal">
        <a href="https://github.com/apolope/ffcom" target="_blank" rel="noreferrer">
          {t('profile.statusMenu.sourceCode')}
        </a>
        <a href={`${import.meta.env.BASE_URL}third-party-licenses.txt`} target="_blank" rel="noreferrer">
          {t('profile.statusMenu.thirdPartyLicenses')}
        </a>
        {/* Tag do deploy (client-vX.Y.Z), embutida no build pelo workflow;
            ausente em build local. */}
        {import.meta.env.VITE_APP_VERSION && <span>{t('profile.statusMenu.version', { version: import.meta.env.VITE_APP_VERSION })}</span>}
      </p>
    </div>
  )
}
