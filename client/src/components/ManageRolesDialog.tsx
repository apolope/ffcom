import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { errorMessage } from '../lib/apiError'
import { PERMISSIONS } from '../lib/permissions'
import type { Ban, Member, Role } from '../types'
import './Dialog.css'
import './ManageRolesDialog.css'
import { MemberAvatar } from './AvatarWithStatus'

interface ManageRolesDialogProps {
  members: Member[]
  roles: Role[]
  bans: Ban[]
  currentMemberId: string | undefined
  canManageRoles: boolean
  canKick: boolean
  canBan: boolean
  onCreateRole: (role: { name: string; permissions: number; position: number }) => Promise<void>
  onUpdateRole: (
    roleId: string,
    role: { name: string; color?: string; permissions: number; position: number },
  ) => Promise<void>
  onDeleteRole: (roleId: string) => Promise<void>
  onAssignRole: (memberId: string, roleId: string) => Promise<void>
  onRemoveRole: (memberId: string, roleId: string) => Promise<void>
  onKick: (memberId: string) => Promise<void>
  onBan: (memberId: string, reason?: string) => Promise<void>
  onUnban: (oidcSubject: string) => Promise<void>
  onClose: () => void
}

// Rótulos no idioma ativo; os bits (PERMISSIONS) continuam sendo os
// identificadores.
function permissionOptions(t: TFunction): { bit: number; label: string }[] {
  return [
    { bit: PERMISSIONS.ViewChannels, label: t('roles.permissions.viewChannels') },
    { bit: PERMISSIONS.SendMessages, label: t('roles.permissions.sendMessages') },
    { bit: PERMISSIONS.Voice, label: t('roles.permissions.voice') },
    { bit: PERMISSIONS.CreateInvites, label: t('roles.permissions.createInvites') },
    { bit: PERMISSIONS.ManageInvites, label: t('roles.permissions.manageInvites') },
    { bit: PERMISSIONS.ManageRoles, label: t('roles.permissions.manageRoles') },
    { bit: PERMISSIONS.ManageChannels, label: t('roles.permissions.manageChannels') },
    { bit: PERMISSIONS.CreateCategories, label: t('roles.permissions.createCategories') },
    { bit: PERMISSIONS.ReorderCategories, label: t('roles.permissions.reorderCategories') },
    { bit: PERMISSIONS.DeleteCategories, label: t('roles.permissions.deleteCategories') },
    { bit: PERMISSIONS.CreateChannels, label: t('roles.permissions.createChannels') },
    { bit: PERMISSIONS.ReorderChannels, label: t('roles.permissions.reorderChannels') },
    { bit: PERMISSIONS.DeleteChannels, label: t('roles.permissions.deleteChannels') },
    { bit: PERMISSIONS.MoveMembers, label: t('roles.permissions.moveMembers') },
    { bit: PERMISSIONS.KickMembers, label: t('roles.permissions.kickMembers') },
    { bit: PERMISSIONS.BanMembers, label: t('roles.permissions.banMembers') },
    { bit: PERMISSIONS.Administrator, label: t('roles.permissions.administrator') },
  ]
}

function PermissionCheckboxes({ mask, onToggle }: { mask: number; onToggle: (bit: number) => void }) {
  const { t } = useTranslation()
  return (
    <div className="permission-checkboxes">
      {permissionOptions(t).map(({ bit, label }) => (
        <label key={bit}>
          <input type="checkbox" checked={(mask & bit) !== 0} onChange={() => onToggle(bit)} />
          {label}
        </label>
      ))}
    </div>
  )
}

function toggleBit(mask: number, bit: number) {
  return mask & bit ? mask & ~bit : mask | bit
}

// Painel de administração de roles: criar, editar (inclusive a @everyone,
// que é a base de todo membro) e remover roles do servidor e
// atribuí-las a membros (ver docs/architecture.md, "Sistema de
// permissões/roles por servidor e por canal"). Overwrites por canal ficam
// em ChannelPermissionsDialog, aberto pelo cadeado da ChannelSidebar.
export function ManageRolesDialog({
  members,
  roles,
  bans,
  currentMemberId,
  canManageRoles,
  canKick,
  canBan,
  onCreateRole,
  onUpdateRole,
  onDeleteRole,
  onAssignRole,
  onRemoveRole,
  onKick,
  onBan,
  onUnban,
  onClose,
}: ManageRolesDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [permMask, setPermMask] = useState(0)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<unknown>()
  const [actionError, setActionError] = useState<unknown>()
  const [editing, setEditing] = useState<{ roleId: string; mask: number }>()
  const [saving, setSaving] = useState(false)
  const [editError, setEditError] = useState<unknown>()

  const assignableRoles = roles.filter((r) => !r.isDefault)

  // Wrapper comum pras ações de um clique só (remover role, atribuir/tirar
  // de membro): evita promise rejeitada sem handler e mostra o erro, sem
  // precisar de estado de loading por linha.
  function runAction(action: () => Promise<void>) {
    setActionError(undefined)
    action().catch((err) => {
      setActionError(err)
    })
  }

  async function handleCreate(event: FormEvent) {
    event.preventDefault()
    setError(undefined)
    setCreating(true)
    try {
      await onCreateRole({ name: name.trim(), permissions: permMask, position: roles.length })
      setName('')
      setPermMask(0)
    } catch (err) {
      setError(err)
    } finally {
      setCreating(false)
    }
  }

  async function handleSaveEdit(role: Role) {
    if (!editing) return
    setEditError(undefined)
    setSaving(true)
    try {
      await onUpdateRole(role.id, {
        name: role.name,
        color: role.color,
        permissions: editing.mask,
        position: role.position,
      })
      setEditing(undefined)
    } catch (err) {
      setEditError(err)
    } finally {
      setSaving(false)
    }
  }

  // @everyone primeiro: é a role que mais se edita (ex. liberar "Criar
  // convites" para todos) e não aparece na atribuição por membro.
  const editableRoles = [...roles.filter((r) => r.isDefault), ...assignableRoles]

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card manage-roles-card" onClick={(e) => e.stopPropagation()}>
        <h2>{t('roles.manage.title')}</h2>

        {canManageRoles && (
          <section className="manage-roles-section">
            <h3>{t('roles.title')}</h3>
            {editError !== undefined && (
              <p className="dialog-error">{errorMessage(editError, t('roles.manage.saveFailed'))}</p>
            )}
            <ul className="role-list">
              {editableRoles.map((role) => {
                const isEditing = editing?.roleId === role.id
                return (
                  <li key={role.id} className={isEditing ? 'role-editing' : undefined}>
                    <div className="role-row">
                      <span style={role.color ? { color: role.color } : undefined}>
                        {role.isDefault ? '@everyone' : role.name}
                      </span>
                      <div className="role-row-actions">
                        <button
                          type="button"
                          onClick={() => {
                            setEditError(undefined)
                            setEditing(isEditing ? undefined : { roleId: role.id, mask: role.permissions })
                          }}
                        >
                          {isEditing ? t('common.cancel') : t('common.edit')}
                        </button>
                        {!role.isDefault && (
                          <button type="button" onClick={() => runAction(() => onDeleteRole(role.id))}>
                            {t('common.remove')}
                          </button>
                        )}
                      </div>
                    </div>
                    {isEditing && (
                      <div className="role-edit">
                        <PermissionCheckboxes
                          mask={editing.mask}
                          onToggle={(bit) => setEditing({ roleId: role.id, mask: toggleBit(editing.mask, bit) })}
                        />
                        <button
                          type="button"
                          className="dialog-submit"
                          disabled={saving || editing.mask === role.permissions}
                          onClick={() => handleSaveEdit(role)}
                        >
                          {saving ? t('common.saving') : t('common.save')}
                        </button>
                      </div>
                    )}
                  </li>
                )
              })}
              {assignableRoles.length === 0 && (
                <li className="hint">{t('roles.manage.noExtraRoles')}</li>
              )}
            </ul>

            <form onSubmit={handleCreate} className="create-role-form">
              <input
                type="text"
                placeholder={t('roles.manage.newRolePlaceholder')}
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
              <PermissionCheckboxes mask={permMask} onToggle={(bit) => setPermMask((prev) => toggleBit(prev, bit))} />
              {error !== undefined && (
                <p className="dialog-error">{errorMessage(error, t('roles.manage.createFailed'))}</p>
              )}
              <button type="submit" className="dialog-submit" disabled={creating || !name.trim()}>
                {creating ? t('roles.manage.creating') : t('roles.manage.createRole')}
              </button>
            </form>
          </section>
        )}

        <section className="manage-roles-section">
          <h3>{t('members.title')}</h3>
          {actionError !== undefined && (
            <p className="dialog-error">{errorMessage(actionError, t('roles.manage.actionFailed'))}</p>
          )}
          <ul className="member-role-list">
            {members.map((member) => {
              const isSelf = member.id === currentMemberId
              return (
                <li key={member.id}>
                  <span className="member-role-name">
                    <MemberAvatar member={member} size={24} />
                    {member.nickname}
                    {member.isOwner && t('roles.manage.ownerSuffix')}
                  </span>
                  {!member.isOwner && canManageRoles && (
                    <div className="member-role-toggles">
                      {assignableRoles.map((role) => {
                        const has = member.roleIds.includes(role.id)
                        return (
                          <label key={role.id}>
                            <input
                              type="checkbox"
                              checked={has}
                              onChange={() =>
                                runAction(() =>
                                  has ? onRemoveRole(member.id, role.id) : onAssignRole(member.id, role.id),
                                )
                              }
                            />
                            {role.name}
                          </label>
                        )
                      })}
                    </div>
                  )}
                  {!member.isOwner && !isSelf && (canKick || canBan) && (
                    <div className="member-moderation-actions">
                      {canKick && (
                        <button type="button" onClick={() => runAction(() => onKick(member.id))}>
                          {t('roles.manage.kick')}
                        </button>
                      )}
                      {canBan && (
                        <button type="button" onClick={() => runAction(() => onBan(member.id))}>
                          {t('roles.manage.ban')}
                        </button>
                      )}
                    </div>
                  )}
                </li>
              )
            })}
          </ul>
        </section>

        {canBan && (
          <section className="manage-roles-section">
            <h3>{t('roles.manage.banned')}</h3>
            <ul className="member-role-list">
              {bans.map((ban) => (
                <li key={ban.oidcSubject}>
                  <span>
                    {ban.displayName}
                    {ban.reason && ` — ${ban.reason}`}
                  </span>
                  <button type="button" onClick={() => runAction(() => onUnban(ban.oidcSubject))}>
                    {t('roles.manage.unban')}
                  </button>
                </li>
              ))}
              {bans.length === 0 && <li className="hint">{t('roles.manage.noBans')}</li>}
            </ul>
          </section>
        )}

        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </div>
    </div>
  )
}
