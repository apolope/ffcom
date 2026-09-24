import { useCallback, useEffect, useState } from 'react'
import {
  assignRole,
  banMember,
  createRole,
  deleteRole,
  fetchBans,
  fetchMembers,
  fetchRoles,
  kickMember,
  removeRole,
  unbanMember,
  type RemoteBan,
  type RemoteMember,
  type RemoteRole,
} from '../lib/serverChannelApi'
import type { Ban, Member, Role } from '../types'

export type ServerMembersStatus = 'loading' | 'ready' | 'error'

interface UseServerMembersResult {
  members: Member[]
  roles: Role[]
  bans: Ban[]
  status: ServerMembersStatus
  error: string | undefined
  createRole: (role: { name: string; color?: string; permissions: number; position: number }) => Promise<void>
  deleteRole: (roleId: string) => Promise<void>
  assignRole: (memberId: string, roleId: string) => Promise<void>
  removeRole: (memberId: string, roleId: string) => Promise<void>
  kickMember: (memberId: string) => Promise<void>
  banMember: (memberId: string, reason?: string) => Promise<void>
  unbanMember: (oidcSubject: string) => Promise<void>
  refresh: () => Promise<void>
}

function toMember(remote: RemoteMember): Member {
  return {
    id: remote.id,
    oidcSubject: remote.oidcSubject,
    // Nome exibido: apelido, senão o nome do perfil do Authentik, senão o
    // começo do id (quem ainda não abriu o servidor com um client que grava
    // o nome). Mesma regra de Member.DisplayName em server-channel.
    nickname: remote.nickname || remote.profileName || remote.id.slice(0, 8),
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

function toBan(remote: RemoteBan): Ban {
  return {
    oidcSubject: remote.oidcSubject,
    displayName: remote.lastNickname ?? remote.oidcSubject.slice(0, 12),
    reason: remote.reason,
    createdAt: remote.createdAt,
  }
}

// Carrega membros e roles de um server-channel (ver docs/architecture.md,
// "Sistema de permissões/roles por servidor e por canal") — usado pela
// lista de membros e pelo painel de administração de roles. Só busca
// quando accessToken existe: chamado tanto pela sidebar (sempre) quanto
// pelo diálogo de administração (só quando aberto).
//
// GET /api/bans exige a permissão BanMembers (ver docs/architecture.md,
// "Decisão: kick/ban de membro") — canViewBans vem de quem chama (App.tsx
// já sabe se o membro atual tem essa permissão) porque um 403 nessa
// chamada, se disparada pra todo mundo, derrubaria o Promise.all inteiro e
// quebraria a lista de membros pra quem não é moderador.
export function useServerMembers(
  serverBaseUrl: string,
  accessToken: string,
  canViewBans: boolean,
): UseServerMembersResult {
  const [remoteMembers, setRemoteMembers] = useState<RemoteMember[]>([])
  const [remoteRoles, setRemoteRoles] = useState<RemoteRole[]>([])
  const [remoteBans, setRemoteBans] = useState<RemoteBan[]>([])
  const [status, setStatus] = useState<ServerMembersStatus>('loading')
  const [error, setError] = useState<string>()

  const load = useCallback(() => {
    if (!serverBaseUrl || !accessToken) return Promise.resolve()
    setStatus('loading')
    setError(undefined)
    return Promise.all([
      fetchMembers(serverBaseUrl, accessToken),
      fetchRoles(serverBaseUrl, accessToken),
      canViewBans ? fetchBans(serverBaseUrl, accessToken) : Promise.resolve([]),
    ])
      .then(([members, roles, bans]) => {
        setRemoteMembers(members)
        setRemoteRoles(roles)
        setRemoteBans(bans)
        setStatus('ready')
      })
      .catch((err) => {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar membros/roles')
      })
  }, [serverBaseUrl, accessToken, canViewBans])

  useEffect(() => {
    void load()
  }, [load])

  return {
    members: remoteMembers.map(toMember),
    roles: remoteRoles.map(toRole),
    bans: remoteBans.map(toBan),
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
    kickMember: async (memberId) => {
      await kickMember(serverBaseUrl, accessToken, memberId)
      await load()
    },
    banMember: async (memberId, reason) => {
      await banMember(serverBaseUrl, accessToken, memberId, reason)
      await load()
    },
    unbanMember: async (oidcSubject) => {
      await unbanMember(serverBaseUrl, accessToken, oidcSubject)
      await load()
    },
    refresh: load,
  }
}
