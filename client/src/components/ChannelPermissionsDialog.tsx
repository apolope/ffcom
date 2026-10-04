import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { errorMessage } from '../lib/apiError'
import { PERMISSIONS } from '../lib/permissions'
import type { ChannelOverwrite } from '../lib/serverChannelApi'
import type { ChannelType, Role } from '../types'
import './Dialog.css'

// Editor de overwrite de canal por role (API GET/PUT/DELETE
// /api/channels/{id}/overwrites[/{roleId}], requer ManageRoles). Ver
// docs/architecture.md, "Decisão: UI de overwrite de canal por role".

type BitState = 'inherit' | 'allow' | 'deny'

// Só os bits que mudam alguma coisa dentro de um canal desse tipo. O
// servidor aceita qualquer bit no overwrite; os que não aparecem aqui são
// preservados como estão ao salvar.
function channelBits(t: TFunction, type: ChannelType): { bit: number; label: string }[] {
  const view = { bit: PERMISSIONS.ViewChannels, label: t('channels.overwrites.viewChannel') }
  switch (type) {
    case 'text':
      return [view, { bit: PERMISSIONS.SendMessages, label: t('channels.overwrites.sendMessages') }]
    case 'forum':
      return [view, { bit: PERMISSIONS.SendMessages, label: t('channels.overwrites.createPosts') }]
    case 'voice':
      return [view, { bit: PERMISSIONS.Voice, label: t('channels.overwrites.joinVoice') }]
  }
}

interface ChannelPermissionsDialogProps {
  channel: { id: string; name: string; type: ChannelType }
  roles: Role[]
  // Permissão base de quem edita: o servidor recusa `allow` de um bit que
  // a pessoa não tem (permissions.Grants), então a opção já vem desligada.
  myPermissions: number
  isOwner: boolean
  onLoad: () => Promise<ChannelOverwrite[]>
  onSet: (roleId: string, overwrite: { allow: number; deny: number }) => Promise<void>
  onDelete: (roleId: string) => Promise<void>
  onSaved: () => void
  onClose: () => void
}

type OverwriteMap = Record<string, { allow: number; deny: number }>

function bitState(o: { allow: number; deny: number } | undefined, bit: number): BitState {
  if (!o) return 'inherit'
  if (o.deny & bit) return 'deny'
  if (o.allow & bit) return 'allow'
  return 'inherit'
}

export function ChannelPermissionsDialog({
  channel,
  roles,
  myPermissions,
  isOwner,
  onLoad,
  onSet,
  onDelete,
  onSaved,
  onClose,
}: ChannelPermissionsDialogProps) {
  const { t } = useTranslation()
  const [original, setOriginal] = useState<OverwriteMap>()
  const [draft, setDraft] = useState<OverwriteMap>({})
  // Erro de carga (fallback de carga) ou de gravação, traduzido no render.
  const [error, setError] = useState<{ err: unknown; loading: boolean }>()
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    onLoad()
      .then((rows) => {
        if (cancelled) return
        const map: OverwriteMap = {}
        for (const o of rows) map[o.roleId] = { allow: o.allow, deny: o.deny }
        setOriginal(map)
        setDraft(map)
      })
      .catch((err) => {
        if (!cancelled) setError({ err, loading: true })
      })
    return () => {
      cancelled = true
    }
  }, [onLoad])

  const isAdmin = isOwner || (myPermissions & PERMISSIONS.Administrator) !== 0
  const canAllow = (bit: number) => isAdmin || (myPermissions & bit) !== 0
  const bits = channelBits(t, channel.type)
  // @everyone primeiro (é o que torna um canal privado), depois pela posição.
  const sortedRoles = [...roles].sort((a, b) =>
    a.isDefault === b.isDefault ? a.position - b.position : a.isDefault ? -1 : 1,
  )

  function change(roleId: string, bit: number, state: BitState) {
    setDraft((prev) => {
      const current = prev[roleId] ?? { allow: 0, deny: 0 }
      const next = { allow: current.allow & ~bit, deny: current.deny & ~bit }
      if (state === 'allow') next.allow |= bit
      if (state === 'deny') next.deny |= bit
      return { ...prev, [roleId]: next }
    })
  }

  const changedRoleIds = original
    ? Object.keys(draft).filter((roleId) => {
        const a = original[roleId] ?? { allow: 0, deny: 0 }
        const b = draft[roleId]
        return a.allow !== b.allow || a.deny !== b.deny
      })
    : []

  async function save() {
    if (!original) return
    setError(undefined)
    setBusy(true)
    try {
      for (const roleId of changedRoleIds) {
        const o = draft[roleId]
        if (o.allow === 0 && o.deny === 0) {
          // Tudo "herdar" = sem overwrite; só apaga se existia no servidor.
          if (original[roleId]) await onDelete(roleId)
        } else {
          await onSet(roleId, o)
        }
      }
      onSaved()
      onClose()
    } catch (err) {
      // Parte pode ter sido salva antes do erro: recarregar ao reabrir
      // mostra o estado real do servidor.
      setError({ err, loading: false })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card channel-permissions-dialog" onClick={(e) => e.stopPropagation()}>
        <h2>{t('channels.overwrites.title', { channel: channel.name })}</h2>
        <p className="dialog-hint">
          {t('channels.overwrites.hint')}
          {!isAdmin && ` ${t('channels.overwrites.hintNotAdmin')}`}
        </p>
        {!original && !error && <p className="dialog-hint">{t('common.loading')}</p>}
        {original && (
          <table className="channel-permissions-table">
            <thead>
              <tr>
                <th scope="col">{t('channels.overwrites.role')}</th>
                {bits.map((b) => (
                  <th scope="col" key={b.bit}>
                    {b.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sortedRoles.map((role) => (
                <tr key={role.id}>
                  <th scope="row">{role.name}</th>
                  {bits.map((b) => {
                    const state = bitState(draft[role.id], b.bit)
                    return (
                      <td key={b.bit}>
                        <select
                          aria-label={t('channels.overwrites.selectLabel', { permission: b.label, role: role.name })}
                          value={state}
                          disabled={busy}
                          onChange={(e) => change(role.id, b.bit, e.target.value as BitState)}
                        >
                          <option value="inherit">{t('channels.overwrites.inherit')}</option>
                          <option value="allow" disabled={!canAllow(b.bit) && state !== 'allow'}>
                            {t('channels.overwrites.allow')}
                          </option>
                          <option value="deny">{t('channels.overwrites.deny')}</option>
                        </select>
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {error && (
          <p className="dialog-error">
            {errorMessage(
              error.err,
              error.loading ? t('channels.overwrites.loadFailed') : t('channels.dialog.saveFailed'),
            )}
          </p>
        )}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </button>
          <button
            type="submit"
            className="dialog-submit"
            onClick={() => void save()}
            disabled={busy || !original || changedRoleIds.length === 0}
          >
            {busy ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </div>
    </div>
  )
}
