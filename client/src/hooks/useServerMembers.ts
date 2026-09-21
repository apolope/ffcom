import { useCallback, useEffect, useState } from 'react'
import {
  assignRole,
  createRole,
  deleteRole,
  fetchMembers,
  fetchRoles,
  removeRole,
  type RemoteMember,
  type RemoteRole,
} from '../lib/serverChannelApi'
import type { Member, Role } from '../types'

export type ServerMembersStatus = 'loading' | 'ready' | 'error'

interface UseServerMembersResult {
  members: Member[]
  roles: Role[]
  status: ServerMembersStatus
  error: string | undefined
  createRole: (role: { name: string; color?: string; permissions: number; position: number }) => Promise<void>
  deleteRole: (roleId: string) => Promise<void>
  assignRole: (memberId: string, roleId: string) => Promise<void>
  removeRole: (memberId: string, roleId: string) => Promise<void>
  refresh: () => Promise<void>
}

function toMember(remote: RemoteMember): Member {
  return {
    id: remote.id,
    nickname: remote.nickname ?? remote.id.slice(0, 8),
    isOwner: remote.isOwner ?? false,
    roleIds: remote.roleIds ?? [],
  }
}

function toRole(remote: RemoteRole): Role {
  return {
    id: remote.id,
    name: remote.name,
    color: remote.color,
    permissions: remote.permissions,
    position: remote.position,
    isDefault: remote.isDefault,
  }
}

// Carrega membros e roles de um server-channel (ver docs/architecture.md,
// "Sistema de permissões/roles por servidor e por canal") — usado pela
// lista de membros e pelo painel de administração de roles. Só busca
// quando accessToken existe: chamado tanto pela sidebar (sempre) quanto
// pelo diálogo de administração (só quando aberto).
export function useServerMembers(serverBaseUrl: string, accessToken: string): UseServerMembersResult {
  const [remoteMembers, setRemoteMembers] = useState<RemoteMember[]>([])
  const [remoteRoles, setRemoteRoles] = useState<RemoteRole[]>([])
  const [status, setStatus] = useState<ServerMembersStatus>('loading')
  const [error, setError] = useState<string>()

  const load = useCallback(() => {
    if (!serverBaseUrl || !accessToken) return Promise.resolve()
    setStatus('loading')
    setError(undefined)
    return Promise.all([fetchMembers(serverBaseUrl, accessToken), fetchRoles(serverBaseUrl, accessToken)])
      .then(([members, roles]) => {
        setRemoteMembers(members)
        setRemoteRoles(roles)
        setStatus('ready')
      })
      .catch((err) => {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar membros/roles')
      })
  }, [serverBaseUrl, accessToken])

  useEffect(() => {
    void load()
  }, [load])

  return {
    members: remoteMembers.map(toMember),
    roles: remoteRoles.map(toRole),
    status,
    error,
    createRole: async (role) => {
      await createRole(serverBaseUrl, accessToken, role)
      await load()
    },
    deleteRole: async (roleId) => {
      await deleteRole(serverBaseUrl, accessToken, roleId)
      await load()
    },
    assignRole: async (memberId, roleId) => {
      await assignRole(serverBaseUrl, accessToken, memberId, roleId)
      await load()
    },
    removeRole: async (memberId, roleId) => {
      await removeRole(serverBaseUrl, accessToken, memberId, roleId)
      await load()
    },
    refresh: load,
  }
}
