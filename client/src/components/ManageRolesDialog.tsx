import { useState } from 'react'
import type { FormEvent } from 'react'
import { PERMISSIONS } from '../lib/permissions'
import type { Member, Role } from '../types'
import './Dialog.css'
import './ManageRolesDialog.css'

interface ManageRolesDialogProps {
  members: Member[]
  roles: Role[]
  onCreateRole: (role: { name: string; permissions: number; position: number }) => Promise<void>
  onDeleteRole: (roleId: string) => Promise<void>
  onAssignRole: (memberId: string, roleId: string) => Promise<void>
  onRemoveRole: (memberId: string, roleId: string) => Promise<void>
  onClose: () => void
}

const PERMISSION_LABELS: { bit: number; label: string }[] = [
  { bit: PERMISSIONS.ViewChannels, label: 'Ver canais' },
  { bit: PERMISSIONS.SendMessages, label: 'Enviar mensagens' },
  { bit: PERMISSIONS.Voice, label: 'Conectar e falar em voz' },
  { bit: PERMISSIONS.ManageInvites, label: 'Gerenciar convites' },
  { bit: PERMISSIONS.ManageRoles, label: 'Gerenciar roles' },
  { bit: PERMISSIONS.Administrator, label: 'Administrador (ignora tudo acima)' },
]

// Painel de administração de roles: criar/remover roles do servidor e
// atribuí-las a membros (ver docs/architecture.md, "Sistema de
// permissões/roles por servidor e por canal"). Overwrites por canal ainda
// não têm UI — só a API existe por enquanto (ver TODO.md).
export function ManageRolesDialog({
  members,
  roles,
  onCreateRole,
  onDeleteRole,
  onAssignRole,
  onRemoveRole,
  onClose,
}: ManageRolesDialogProps) {
  const [name, setName] = useState('')
  const [permMask, setPermMask] = useState(0)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string>()
  const [actionError, setActionError] = useState<string>()

  const assignableRoles = roles.filter((r) => !r.isDefault)

  // Wrapper comum pras ações de um clique só (remover role, atribuir/tirar
  // de membro): evita promise rejeitada sem handler e mostra o erro, sem
  // precisar de estado de loading por linha.
  function runAction(action: () => Promise<void>) {
    setActionError(undefined)
    action().catch((err) => {
      setActionError(err instanceof Error ? err.message : 'falha ao aplicar mudança')
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
      setError(err instanceof Error ? err.message : 'falha ao criar role')
    } finally {
      setCreating(false)
    }
  }

  function togglePermission(bit: number) {
    setPermMask((prev) => (prev & bit ? prev & ~bit : prev | bit))
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card manage-roles-card" onClick={(e) => e.stopPropagation()}>
        <h2>Gerenciar roles</h2>

        <section className="manage-roles-section">
          <h3>Roles</h3>
          <ul className="role-list">
            {assignableRoles.map((role) => (
              <li key={role.id}>
                <span style={role.color ? { color: role.color } : undefined}>{role.name}</span>
                <button type="button" onClick={() => runAction(() => onDeleteRole(role.id))}>
                  Remover
                </button>
              </li>
            ))}
            {assignableRoles.length === 0 && (
              <li className="hint">Nenhuma role além de "@everyone" ainda.</li>
            )}
          </ul>

          <form onSubmit={handleCreate} className="create-role-form">
            <input
              type="text"
              placeholder="Nome da nova role"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
            <div className="permission-checkboxes">
              {PERMISSION_LABELS.map(({ bit, label }) => (
                <label key={bit}>
                  <input
                    type="checkbox"
                    checked={(permMask & bit) !== 0}
                    onChange={() => togglePermission(bit)}
                  />
                  {label}
                </label>
              ))}
            </div>
            {error && <p className="dialog-error">{error}</p>}
            <button type="submit" className="dialog-submit" disabled={creating || !name.trim()}>
              {creating ? 'Criando…' : 'Criar role'}
            </button>
          </form>
        </section>

        <section className="manage-roles-section">
          <h3>Membros</h3>
          {actionError && <p className="dialog-error">{actionError}</p>}
          <ul className="member-role-list">
            {members.map((member) => (
              <li key={member.id}>
                <span>
                  {member.nickname}
                  {member.isOwner && ' (dono)'}
                </span>
                {!member.isOwner && (
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
              </li>
            ))}
          </ul>
        </section>

        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  )
}
