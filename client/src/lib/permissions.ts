// Espelha os bits definidos em server-channel/internal/permissions —
// usado só para decidir se a UI mostra ações de administração (o servidor
// sempre reforça a permissão de fato em cada rota, ver
// docs/architecture.md, "Sistema de permissões/roles por servidor e por
// canal"). Bitwise em number (não BigInt) é seguro aqui porque os bits
// definidos hoje cabem folgados em 32 bits.
export const PERMISSIONS = {
  ViewChannels: 1,
  SendMessages: 2,
  Voice: 4,
  ManageInvites: 8,
  ManageRoles: 16,
  Administrator: 32,
  KickMembers: 64,
  BanMembers: 128,
  ManageChannels: 256,
  CreateInvites: 512,
  // Fatias de ManageChannels (que continua valendo tudo, inclusive
  // renomear). Ver docs/permissions.md.
  CreateChannels: 1024,
  ReorderChannels: 2048,
  DeleteChannels: 4096,
  CreateCategories: 8192,
  ReorderCategories: 16384,
  DeleteCategories: 32768,
} as const

export function hasPermission(effective: number, bit: number): boolean {
  return (effective & PERMISSIONS.Administrator) !== 0 || (effective & bit) !== 0
}

// O que a pessoa pode fazer na estrutura (categorias e canais) do servidor
// aberto. Cada ação aceita ManageChannels ou o bit granular dela, igual ao
// servidor (server-channel/internal/httpapi/channels_admin.go); renomear só
// com ManageChannels.
export interface StructurePermissions {
  rename: boolean
  createChannels: boolean
  reorderChannels: boolean
  deleteChannels: boolean
  createCategories: boolean
  reorderCategories: boolean
  deleteCategories: boolean
}

export function structurePermissionsOf(effective: number, isOwner: boolean): StructurePermissions {
  const can = (bit: number) => isOwner || hasPermission(effective, PERMISSIONS.ManageChannels | bit)
  return {
    rename: can(0),
    createChannels: can(PERMISSIONS.CreateChannels),
    reorderChannels: can(PERMISSIONS.ReorderChannels),
    deleteChannels: can(PERMISSIONS.DeleteChannels),
    createCategories: can(PERMISSIONS.CreateCategories),
    reorderCategories: can(PERMISSIONS.ReorderCategories),
    deleteCategories: can(PERMISSIONS.DeleteCategories),
  }
}

export const NO_STRUCTURE_PERMISSIONS: StructurePermissions = structurePermissionsOf(0, false)
