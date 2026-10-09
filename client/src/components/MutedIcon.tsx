import { useTranslation } from 'react-i18next'
import './MutedIcon.css'

interface MutedIconProps {
  // 'inline' depois de um nome (canal, cabeçalho do servidor); 'badge' no
  // canto do ícone do servidor no rail.
  variant?: 'inline' | 'badge'
}

// Marca de "notificações silenciadas" do push do app Android (hooks/
// useAndroidPush.ts): o mesmo 🔕 do sino no cabeçalho do canal. Só aparece
// onde há push, porque quem a desenha recebe o estado de silêncio só lá.
export function MutedIcon({ variant = 'inline' }: MutedIconProps) {
  const { t } = useTranslation()
  const label = t('push.mute.mutedLabel')
  return (
    <span className={`muted-icon ${variant}`} role="img" aria-label={label} title={label}>
      🔕
    </span>
  )
}
