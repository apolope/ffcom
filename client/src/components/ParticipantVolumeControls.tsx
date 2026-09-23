import { useId } from 'react'
import { DEFAULT_PARTICIPANT_AUDIO, MAX_VOLUME, type ParticipantAudio } from '../lib/participantAudio'

interface ParticipantVolumeControlsProps {
  name: string
  audio: ParticipantAudio
  // A pessoa está compartilhando tela com áudio agora.
  screenShareAudio: boolean
  onChange: (change: Partial<ParticipantAudio>) => void
}

// Painel de volume de uma pessoa na lista do canal de voz: voz e áudio da
// tela separados, de 0% a 200%, e "silenciar para mim". Vale só neste
// dispositivo (lib/participantAudio.ts).
export function ParticipantVolumeControls({ name, audio, screenShareAudio, onChange }: ParticipantVolumeControlsProps) {
  const id = useId()
  // O controle da tela só aparece quando há o que ajustar: a pessoa está
  // mandando áudio de tela ou já tem um volume de tela escolhido antes.
  const showScreen = screenShareAudio || audio.screen !== DEFAULT_PARTICIPANT_AUDIO.screen
  const isDefault =
    audio.voice === DEFAULT_PARTICIPANT_AUDIO.voice && audio.screen === DEFAULT_PARTICIPANT_AUDIO.screen && !audio.muted

  return (
    <div className="voice-volume-panel" role="group" aria-label={`Volume de ${name}`}>
      <VolumeSlider
        id={`${id}-voice`}
        label="Voz"
        value={audio.voice}
        disabled={audio.muted}
        onChange={(voice) => onChange({ voice })}
      />
      {showScreen && (
        <VolumeSlider
          id={`${id}-screen`}
          label="Áudio da tela"
          value={audio.screen}
          disabled={audio.muted}
          onChange={(screen) => onChange({ screen })}
        />
      )}
      <div className="voice-volume-actions">
        <label className="voice-pref">
          <input type="checkbox" checked={audio.muted} onChange={() => onChange({ muted: !audio.muted })} />
          Silenciar para mim
        </label>
        {!isDefault && (
          <button type="button" onClick={() => onChange(DEFAULT_PARTICIPANT_AUDIO)}>
            Restaurar
          </button>
        )}
      </div>
    </div>
  )
}

interface VolumeSliderProps {
  id: string
  label: string
  value: number
  disabled: boolean
  onChange: (value: number) => void
}

function VolumeSlider({ id, label, value, disabled, onChange }: VolumeSliderProps) {
  const percent = Math.round(value * 100)
  return (
    <div className="voice-volume-slider">
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        type="range"
        min={0}
        max={MAX_VOLUME * 100}
        step={5}
        value={percent}
        disabled={disabled}
        onChange={(event) => onChange(Number(event.target.value) / 100)}
      />
      <output htmlFor={id}>{percent}%</output>
    </div>
  )
}
