import { useEffect, useState } from 'react'
import './Dialog.css'
import './ScreenSharePicker.css'

interface ScreenSharePickerProps {
  bridge: FfcomElectronBridge
  onShare: () => void
  onClose: () => void
}

// Seletor de tela do app desktop, no lugar do seletor nativo que o navegador
// abre no getDisplayMedia e o Electron não tem. Guarda a escolha no processo
// main (chooseDisplaySource) e só então chama onShare, que segue o caminho
// normal do setScreenShareEnabled. Ver docs/architecture.md, "Decisão:
// compartilhamento de tela no Electron".
export function ScreenSharePicker({ bridge, onShare, onClose }: ScreenSharePickerProps) {
  const [sources, setSources] = useState<DisplaySource[]>()
  const [selected, setSelected] = useState<string>()
  const [audio, setAudio] = useState(false)
  const [error, setError] = useState<string>()

  useEffect(() => {
    let cancelled = false
    bridge
      .getDisplaySources()
      .then((list) => {
        if (cancelled) return
        setSources(list)
        setSelected(list.find((s) => s.kind === 'screen')?.id)
      })
      .catch(() => {
        if (!cancelled) setError('Não foi possível listar as telas e janelas.')
      })
    return () => {
      cancelled = true
    }
  }, [bridge])

  async function share(id = selected) {
    if (!id) return
    setError(undefined)
    if (!(await bridge.chooseDisplaySource(id, audio))) {
      setError('Essa janela não está mais disponível. Feche e abra o seletor de novo.')
      return
    }
    onShare()
    onClose()
  }

  function renderGroup(kind: DisplaySource['kind'], title: string) {
    const group = sources?.filter((s) => s.kind === kind) ?? []
    if (group.length === 0) return null
    return (
      <section>
        <h3>{title}</h3>
        <div className="screen-picker-grid">
          {group.map((s) => (
            <button
              key={s.id}
              type="button"
              className={s.id === selected ? 'screen-picker-source selected' : 'screen-picker-source'}
              aria-pressed={s.id === selected}
              onClick={() => setSelected(s.id)}
              onDoubleClick={() => share(s.id)}
            >
              {s.thumbnail ? <img src={s.thumbnail} alt="" /> : <span className="screen-picker-blank" />}
              <span className="screen-picker-name">{s.name}</span>
            </button>
          ))}
        </div>
      </section>
    )
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div
        className="dialog-card screen-picker"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
          if (e.key === 'Enter' && e.target instanceof HTMLElement && e.target.tagName !== 'BUTTON') share()
        }}
      >
        <h2>Compartilhar tela</h2>
        {!sources && !error && <p className="dialog-hint">Carregando…</p>}
        {sources && sources.length === 0 && <p className="dialog-hint">Nenhuma tela ou janela encontrada.</p>}
        {renderGroup('screen', 'Telas')}
        {renderGroup('window', 'Janelas')}
        {bridge.canShareSystemAudio ? (
          <label className="screen-picker-audio">
            <input type="checkbox" checked={audio} onChange={() => setAudio(!audio)} />
            Compartilhar áudio do computador
          </label>
        ) : (
          <p className="dialog-hint">Neste sistema o app desktop compartilha só a imagem, sem som.</p>
        )}
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            Cancelar
          </button>
          <button type="button" className="dialog-submit" onClick={() => share()} disabled={!selected} autoFocus>
            Compartilhar
          </button>
        </div>
      </div>
    </div>
  )
}
