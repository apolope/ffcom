import { useTranslation } from 'react-i18next'
import { formatNumber } from '../lib/format'
import { DEFAULT_VOLUME, MAX_VOLUME, type ParticipantAudio } from '../lib/participantAudio'
import './VoiceIcons.css'

// Ícones de estado da voz em SVG (currentColor), porque não há emoji de
// microfone cortado e o 🔇 (alto-falante cortado) dava a entender "eu
// silenciei essa pessoa". Convenção:
//   microfone cortado      = a pessoa fechou o próprio microfone;
//   alto-falante cortado   = você silenciou a pessoa para você;
//   alto-falante com seta  = você mudou o volume dela (seta maior = mais
//                            longe de 100%, até 0% ou 200%).

interface MicIconProps {
  off?: boolean
}

export function MicIcon({ off = false }: MicIconProps) {
  return (
    <svg className={'voice-icon' + (off ? ' mic-off' : '')} viewBox="0 0 24 24" aria-hidden="true">
      <rect x="9" y="2.5" width="6" height="11.5" rx="3" />
      <path d="M5.5 11a6.5 6.5 0 0 0 13 0M12 17.5V21M8.5 21h7" />
      {off && <path className="voice-icon-slash" d="M4 3.5 20 20.5" />}
    </svg>
  )
}

// Alto-falante; a seta (direction) cresce com amount, de 0 (perto de 100%)
// a 1 (0% ou 200%).
export function SpeakerIcon({ slash, direction, amount = 0 }: { slash?: boolean; direction?: 'up' | 'down'; amount?: number }) {
  const length = 6 + 12 * amount
  const top = 12 - length / 2
  const bottom = 12 + length / 2
  const arrow =
    direction === 'up'
      ? `M19 ${bottom}V${top}M16 ${top + 3}l3-3 3 3`
      : `M19 ${top}V${bottom}M16 ${bottom - 3}l3 3 3-3`
  return (
    <svg className="voice-icon" viewBox="0 0 24 24" aria-hidden="true">
      <path className="voice-icon-fill" d="M3 9h3.5L11 5v14l-4.5-4H3z" />
      {direction && <path d={arrow} />}
      {slash && <path className="voice-icon-slash" d="M3 3.5 20 20.5" />}
    </svg>
  )
}

// Marca do que você fez com o áudio da pessoa (lib/participantAudio.ts):
// silenciada para você ou volume da voz fora de 100%. Nada no padrão.
export function ParticipantAudioIcon({ audio }: { audio: ParticipantAudio }) {
  const { t } = useTranslation()
  if (audio.muted) {
    const label = t('voice.mutedForYou')
    return (
      <span className="voice-icon-wrap" role="img" aria-label={label} title={label}>
        <SpeakerIcon slash />
      </span>
    )
  }
  if (audio.voice === DEFAULT_VOLUME) return null
  const up = audio.voice > DEFAULT_VOLUME
  const amount = up
    ? (audio.voice - DEFAULT_VOLUME) / (MAX_VOLUME - DEFAULT_VOLUME)
    : (DEFAULT_VOLUME - audio.voice) / DEFAULT_VOLUME
  const label = t('voice.volumeForYou', {
    percent: formatNumber(audio.voice, { style: 'percent', maximumFractionDigits: 0 }),
  })
  return (
    <span className="voice-icon-wrap" role="img" aria-label={label} title={label}>
      <SpeakerIcon direction={up ? 'up' : 'down'} amount={amount} />
    </span>
  )
}
