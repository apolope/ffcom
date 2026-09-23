import { useEffect, useState } from 'react'
import { formatShortcut, isUsableShortcut, shortcutFromEvent } from '../lib/shortcut'

interface MuteShortcutSettingProps {
  shortcut: string | undefined
  onChange: (shortcut: string | undefined) => void
  // Enquanto grava, o atalho atual fica desligado (useMuteShortcut), para
  // apertar a combinação antiga não alternar o microfone.
  recording: boolean
  onRecordingChange: (recording: boolean) => void
  // Texto antes da tecla; padrão "Atalho para mutar".
  label?: string
  // Aceita tecla sozinha, sem Ctrl/Alt/Win. Serve ao push-to-talk, que só
  // funciona com a janela em foco e por isso não prende a tecla nos outros
  // programas.
  allowSingleKey?: boolean
}

// Grava um atalho de voz (mutar ou push-to-talk): clicar em "Definir atalho"
// e apertar a combinação. Esc cancela.
export function MuteShortcutSetting({
  shortcut,
  onChange,
  recording,
  onRecordingChange,
  label = 'Atalho para mutar',
  allowSingleKey = false,
}: MuteShortcutSettingProps) {
  const [hint, setHint] = useState<string>()

  useEffect(() => {
    if (!recording) return
    const onKeyDown = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (e.key === 'Escape') {
        setHint(undefined)
        onRecordingChange(false)
        return
      }
      const next = shortcutFromEvent(e)
      // Só modificadores apertados até agora: espera a tecla principal.
      if (!next) {
        if (!['Control', 'Alt', 'Shift', 'Meta', 'AltGraph'].includes(e.key)) {
          setHint('Tecla não suportada. Use uma letra, um número, F1-F24 ou espaço.')
        }
        return
      }
      if (!allowSingleKey && !isUsableShortcut(next)) {
        setHint('Use Ctrl, Alt ou Win junto com a tecla (ou uma tecla F sozinha).')
        return
      }
      setHint(undefined)
      onChange(next)
      onRecordingChange(false)
    }
    // Captura: o listener do atalho e o resto da página não veem a tecla.
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [recording, onChange, onRecordingChange, allowSingleKey])

  return (
    <div className="voice-pref voice-shortcut">
      {recording ? (
        <>
          <span>{allowSingleKey ? 'Aperte a tecla' : 'Aperte a combinação'} (Esc cancela)</span>
          <button type="button" onClick={() => onRecordingChange(false)}>
            Cancelar
          </button>
        </>
      ) : (
        <>
          <span>
            {label}: {shortcut ? <kbd>{formatShortcut(shortcut)}</kbd> : 'nenhum'}
          </span>
          <button type="button" onClick={() => onRecordingChange(true)}>
            {shortcut ? 'Trocar' : 'Definir atalho'}
          </button>
          {shortcut && (
            <button type="button" onClick={() => onChange(undefined)}>
              Remover
            </button>
          )}
        </>
      )}
      {hint && <span className="voice-shortcut-hint">{hint}</span>}
    </div>
  )
}
