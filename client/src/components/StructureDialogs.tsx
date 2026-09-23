import { useState } from 'react'
import type { ChannelType } from '../types'
import './Dialog.css'

// Diálogos de administração da estrutura do servidor (categorias e canais),
// só abertos para quem tem ManageChannels ou é dono. Ver
// docs/architecture.md, "Decisão: gerenciar categorias e canais".

const CHANNEL_TYPE_LABEL: Record<ChannelType, string> = {
  text: 'Texto',
  voice: 'Voz',
  forum: 'Fórum',
}

// Mesmo limite de server-channel (maxStructureNameLength).
const MAX_NAME_LENGTH = 100

interface CategoryDialogProps {
  // Ausente = criar categoria nova.
  category?: { id: string; name: string }
  onSave: (name: string) => Promise<void>
  onDelete?: () => Promise<void>
  onClose: () => void
}

export function CategoryDialog({ category, onSave, onDelete, onClose }: CategoryDialogProps) {
  const [name, setName] = useState(category?.name ?? '')
  const { error, busy, run } = useAsyncAction(onClose)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>{category ? 'Editar categoria' : 'Nova categoria'}</h2>
        <label>
          Nome
          <input
            type="text"
            value={name}
            maxLength={MAX_NAME_LENGTH}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
        </label>
        {category && onDelete && (
          <DeleteButton
            confirming={confirmingDelete}
            busy={busy}
            label="Apagar categoria"
            warning="Os canais desta categoria não são apagados: ficam sem categoria."
            onAsk={() => setConfirmingDelete(true)}
            onConfirm={() => run(onDelete)}
          />
        )}
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={busy}>
            Cancelar
          </button>
          <button
            type="submit"
            className="dialog-submit"
            onClick={() => run(() => onSave(name.trim()))}
            disabled={busy || !name.trim()}
          >
            {busy ? 'Salvando…' : category ? 'Salvar' : 'Criar'}
          </button>
        </div>
      </div>
    </div>
  )
}

export interface ChannelDialogValues {
  name: string
  type: ChannelType
  // Ausente = sem categoria.
  categoryId?: string
}

interface ChannelDialogProps {
  // Ausente = criar canal novo.
  channel?: { id: string; name: string; type: ChannelType; categoryId?: string }
  initialCategoryId?: string
  categories: { id: string; name: string }[]
  onSave: (values: ChannelDialogValues) => Promise<void>
  onDelete?: () => Promise<void>
  onClose: () => void
}

export function ChannelDialog({ channel, initialCategoryId, categories, onSave, onDelete, onClose }: ChannelDialogProps) {
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(channel?.type ?? 'text')
  const [categoryId, setCategoryId] = useState(channel ? (channel.categoryId ?? '') : (initialCategoryId ?? ''))
  const { error, busy, run } = useAsyncAction(onClose)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>{channel ? 'Editar canal' : 'Novo canal'}</h2>
        <label>
          Nome
          <input
            type="text"
            value={name}
            maxLength={MAX_NAME_LENGTH}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
        </label>
        <label>
          Tipo
          {/* O tipo não muda depois de criado (ver server-channel,
              handleUpdateChannel), então na edição só é exibido. */}
          <select value={type} onChange={(e) => setType(e.target.value as ChannelType)} disabled={!!channel}>
            {(Object.keys(CHANNEL_TYPE_LABEL) as ChannelType[]).map((t) => (
              <option key={t} value={t}>
                {CHANNEL_TYPE_LABEL[t]}
              </option>
            ))}
          </select>
        </label>
        <label>
          Categoria
          <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)}>
            <option value="">(sem categoria)</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        {channel && onDelete && (
          <DeleteButton
            confirming={confirmingDelete}
            busy={busy}
            label="Apagar canal"
            warning="Todas as mensagens, threads e anexos deste canal serão apagados. Não dá para desfazer."
            onAsk={() => setConfirmingDelete(true)}
            onConfirm={() => run(onDelete)}
          />
        )}
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={busy}>
            Cancelar
          </button>
          <button
            type="submit"
            className="dialog-submit"
            onClick={() => run(() => onSave({ name: name.trim(), type, categoryId: categoryId || undefined }))}
            disabled={busy || !name.trim()}
          >
            {busy ? 'Salvando…' : channel ? 'Salvar' : 'Criar'}
          </button>
        </div>
      </div>
    </div>
  )
}

// Exclusão em dois cliques dentro do próprio diálogo (sem confirm() do
// navegador): o primeiro mostra o aviso do que se perde, o segundo apaga.
function DeleteButton({
  confirming,
  busy,
  label,
  warning,
  onAsk,
  onConfirm,
}: {
  confirming: boolean
  busy: boolean
  label: string
  warning: string
  onAsk: () => void
  onConfirm: () => void
}) {
  if (!confirming) {
    return (
      <button type="button" className="dialog-danger-link" onClick={onAsk} disabled={busy}>
        {label}
      </button>
    )
  }
  return (
    <div className="dialog-danger-zone">
      <p>{warning}</p>
      <button type="button" className="dialog-danger" onClick={onConfirm} disabled={busy}>
        Confirmar: {label.toLowerCase()}
      </button>
    </div>
  )
}

// Roda uma ação assíncrona do diálogo, fecha no sucesso e mostra o erro do
// servidor (ex. 403 sem ManageChannels) sem fechar na falha.
function useAsyncAction(onDone: () => void) {
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  async function run(action: () => Promise<void>) {
    setError(undefined)
    setBusy(true)
    try {
      await action()
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao salvar')
    } finally {
      setBusy(false)
    }
  }

  return { error, busy, run }
}
