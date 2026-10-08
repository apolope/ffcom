import { useTranslation } from 'react-i18next'
import type { UpdateKind } from '../hooks/useAppUpdate'

interface UpdateButtonProps {
  // De quem é a versão esperando: muda só o texto (o que acontece ao clicar).
  kind: UpdateKind
  onUpdate: () => void
}

const TITLE_KEYS = {
  web: 'update.availableWeb',
  desktop: 'update.availableDesktop',
  android: 'update.availableAndroid',
} as const satisfies Record<UpdateKind, string>

// Botão de atualizar o client no ServerRail, como o do Discord: só aparece
// quando hooks/useAppUpdate.ts avisa que há versão nova esperando, seja do
// service worker, do electron-updater ou do APK do Android.
export function UpdateButton({ kind, onUpdate }: UpdateButtonProps) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      className="update-button"
      title={t(TITLE_KEYS[kind])}
      aria-label={t('update.label')}
      onClick={onUpdate}
    >
      <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true">
        <path d="M11 4h2v8.2l3.3-3.3 1.4 1.4L12 16l-5.7-5.7 1.4-1.4 3.3 3.3V4zM5 18h14v2H5v-2z" />
      </svg>
    </button>
  )
}
