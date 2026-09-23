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
} as const

export function hasPermission(effective: number, bit: number): boolean {
  return (effective & PERMISSIONS.Administrator) !== 0 || (effective & bit) !== 0
}
