import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  label,
  allowSingleKey = false,
}: MuteShortcutSettingProps) {
  const { t } = useTranslation()
  // Qual aviso mostrar; o texto sai do arquivo de idioma no render.
  const [hint, setHint] = useState<'unsupported' | 'modifier'>()

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
          setHint('unsupported')
        }
        return
      }
      if (!allowSingleKey && !isUsableShortcut(next)) {
        setHint('modifier')
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
          <span>{allowSingleKey ? t('voice.shortcut.pressKey') : t('voice.shortcut.pressCombo')}</span>
          <button type="button" onClick={() => onRecordingChange(false)}>
            {t('common.cancel')}
          </button>
        </>
      ) : (
        <>
          <span>
            {label ?? t('voice.shortcut.muteLabel')}: {shortcut ? <kbd>{formatShortcut(shortcut)}</kbd> : t('voice.shortcut.none')}
          </span>
          <button type="button" onClick={() => onRecordingChange(true)}>
            {shortcut ? t('voice.shortcut.change') : t('voice.shortcut.set')}
          </button>
          {shortcut && (
            <button type="button" onClick={() => onChange(undefined)}>
              {t('common.remove')}
            </button>
          )}
        </>
      )}
      {hint && (
        <span className="voice-shortcut-hint">
          {hint === 'unsupported' ? t('voice.shortcut.unsupportedKey') : t('voice.shortcut.needsModifier')}
        </span>
      )}
    </div>
  )
}
