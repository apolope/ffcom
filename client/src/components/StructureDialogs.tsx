import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { errorMessage } from '../lib/apiError'
import type { ChannelType } from '../types'
import './Dialog.css'

// Diálogos de administração da estrutura do servidor (categorias e canais).
// Cada campo só fica editável com a permissão da ação (renomear só com
// ManageChannels; mover e apagar também com os bits granulares, ver
// docs/permissions.md). Ver docs/architecture.md, "Decisão: gerenciar
// categorias e canais".

const CHANNEL_TYPES: ChannelType[] = ['text', 'voice', 'forum']

function channelTypeLabel(t: TFunction, type: ChannelType): string {
  switch (type) {
    case 'text':
      return t('channels.types.text')
    case 'voice':
      return t('channels.types.voice')
    case 'forum':
      return t('channels.types.forum')
  }
}

// Mesmo limite de server-channel (maxStructureNameLength).
const MAX_NAME_LENGTH = 100

interface CategoryDialogProps {
  // Ausente = criar categoria nova.
  category?: { id: string; name: string }
  // Editar o nome de uma categoria existente exige ManageChannels.
  canRename: boolean
  onSave: (name: string) => Promise<void>
  onDelete?: () => Promise<void>
  onClose: () => void
}

export function CategoryDialog({ category, canRename, onSave, onDelete, onClose }: CategoryDialogProps) {
  const { t } = useTranslation()
  const nameEditable = !category || canRename
  const [name, setName] = useState(category?.name ?? '')
  const { error, busy, run } = useAsyncAction(onClose)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <form
        className="dialog-card"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault()
          if (nameEditable && !busy && name.trim()) run(() => onSave(name.trim()))
        }}
      >
        <h2>{category ? t('channels.editCategory') : t('channels.dialog.newCategory')}</h2>
        <label>
          {t('channels.dialog.name')}
          <input
            type="text"
            value={name}
            maxLength={MAX_NAME_LENGTH}
            onChange={(e) => setName(e.target.value)}
            disabled={!nameEditable}
            autoFocus
          />
        </label>
        {category && onDelete && (
          <DeleteButton
            confirming={confirmingDelete}
            busy={busy}
            label={t('channels.dialog.deleteCategory')}
            confirmLabel={t('channels.dialog.confirmDeleteCategory')}
            warning={t('channels.dialog.deleteCategoryWarning')}
            onAsk={() => setConfirmingDelete(true)}
            onConfirm={() => run(onDelete)}
          />
        )}
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={busy}>
            {nameEditable ? t('common.cancel') : t('common.close')}
          </button>
          {nameEditable && (
            <button type="submit" className="dialog-submit" disabled={busy || !name.trim()}>
              {busy ? t('common.saving') : category ? t('common.save') : t('common.create')}
            </button>
          )}
        </div>
      </form>
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
  // Na edição: renomear exige ManageChannels, mover de categoria
  // ReorderChannels (ou ManageChannels). Na criação tudo é editável.
  canRename: boolean
  canMove: boolean
  onSave: (values: ChannelDialogValues) => Promise<void>
  onDelete?: () => Promise<void>
  onClose: () => void
}

export function ChannelDialog({
  channel,
  initialCategoryId,
  categories,
  canRename,
  canMove,
  onSave,
  onDelete,
  onClose,
}: ChannelDialogProps) {
  const { t } = useTranslation()
  const nameEditable = !channel || canRename
  const categoryEditable = !channel || canMove
  const canSave = nameEditable || categoryEditable
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(channel?.type ?? 'text')
  const [categoryId, setCategoryId] = useState(channel ? (channel.categoryId ?? '') : (initialCategoryId ?? ''))
  const { error, busy, run } = useAsyncAction(onClose)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <form
        className="dialog-card"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault()
          if (canSave && !busy && name.trim()) {
            run(() => onSave({ name: name.trim(), type, categoryId: categoryId || undefined }))
          }
        }}
      >
        <h2>{channel ? t('channels.editChannel') : t('channels.dialog.newChannel')}</h2>
        <label>
          {t('channels.dialog.name')}
          <input
            type="text"
            value={name}
            maxLength={MAX_NAME_LENGTH}
            onChange={(e) => setName(e.target.value)}
            disabled={!nameEditable}
            autoFocus
          />
        </label>
        <label>
          {t('channels.dialog.type')}
          {/* O tipo não muda depois de criado (ver server-channel,
              handleUpdateChannel), então na edição só é exibido. */}
          <select value={type} onChange={(e) => setType(e.target.value as ChannelType)} disabled={!!channel}>
            {CHANNEL_TYPES.map((value) => (
              <option key={value} value={value}>
                {channelTypeLabel(t, value)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t('channels.dialog.category')}
          <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)} disabled={!categoryEditable}>
            <option value="">{t('channels.dialog.noCategory')}</option>
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
            label={t('channels.dialog.deleteChannel')}
            confirmLabel={t('channels.dialog.confirmDeleteChannel')}
            warning={t('channels.dialog.deleteChannelWarning')}
            onAsk={() => setConfirmingDelete(true)}
            onConfirm={() => run(onDelete)}
          />
        )}
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={busy}>
            {canSave ? t('common.cancel') : t('common.close')}
          </button>
          {canSave && (
            <button type="submit" className="dialog-submit" disabled={busy || !name.trim()}>
              {busy ? t('common.saving') : channel ? t('common.save') : t('common.create')}
            </button>
          )}
        </div>
      </form>
    </div>
  )
}

// Exclusão em dois cliques dentro do próprio diálogo (sem confirm() do
// navegador): o primeiro mostra o aviso do que se perde, o segundo apaga.
function DeleteButton({
  confirming,
  busy,
  label,
  confirmLabel,
  warning,
  onAsk,
  onConfirm,
}: {
  confirming: boolean
  busy: boolean
  label: string
  confirmLabel: string
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
        {confirmLabel}
      </button>
    </div>
  )
}

// Roda uma ação assíncrona do diálogo, fecha no sucesso e mostra o erro do
// servidor (ex. 403 sem a permissão) sem fechar na falha.
function useAsyncAction(onDone: () => void) {
  const { t } = useTranslation()
  const [error, setError] = useState<unknown>()
  const [busy, setBusy] = useState(false)

  async function run(action: () => Promise<void>) {
    setError(undefined)
    setBusy(true)
    try {
      await action()
      onDone()
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  // Traduzido no render: acompanha a troca de idioma.
  const errorText = error === undefined ? undefined : errorMessage(error, t('channels.dialog.saveFailed'))
  return { error: errorText, busy, run }
}
