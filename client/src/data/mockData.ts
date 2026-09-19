import type { KnownServer, ServerDetail } from '../types'

// Dados estáticos de exemplo até a API de server-central (diretório de
// servidores) e server-channel (categorias/canais/membros) serem integradas.
export const mockServers: KnownServer[] = [
  { id: 'srv-1', name: 'A3S Community', initials: 'A3' },
  { id: 'srv-2', name: 'Retro Games', initials: 'RG' },
]

export const mockServerDetails: Record<string, ServerDetail> = {
  'srv-1': {
    categories: [
      {
        id: 'cat-1',
        name: 'Geral',
        channels: [
          { id: 'ch-1', name: 'geral', type: 'text' },
          { id: 'ch-2', name: 'avisos', type: 'text' },
          { id: 'ch-3', name: 'Sala de voz', type: 'voice' },
        ],
      },
      {
        id: 'cat-2',
        name: 'Projetos',
        channels: [
          { id: 'ch-4', name: 'ffcom-dev', type: 'text' },
          { id: 'ch-5', name: 'sugestões', type: 'forum' },
        ],
      },
    ],
    members: [
      { id: 'mem-1', nickname: 'apolonio', online: true },
      { id: 'mem-2', nickname: 'maria', online: true },
      { id: 'mem-3', nickname: 'joao', online: false },
    ],
  },
  'srv-2': {
    categories: [
      {
        id: 'cat-3',
        name: 'Geral',
        channels: [
          { id: 'ch-6', name: 'geral', type: 'text' },
          { id: 'ch-7', name: 'Sala de voz', type: 'voice' },
        ],
      },
    ],
    members: [
      { id: 'mem-4', nickname: 'gamer01', online: true },
      { id: 'mem-5', nickname: 'gamer02', online: false },
    ],
  },
}
