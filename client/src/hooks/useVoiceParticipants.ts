import { useEffect, useState } from 'react'
import { fetchVoiceParticipants, type VoiceParticipant } from '../lib/serverChannelApi'

// Mais curto que o poll de estrutura (20s): entrar e sair de sala acontece
// muito mais que criar canal, e quem olha a barra lateral quer saber se tem
// alguém lá agora. Somado aos 5s de cache do servidor, a lista pode atrasar
// até ~15s.
const VOICE_PARTICIPANTS_POLL_INTERVAL_MS = 10_000

// Participantes de cada canal de voz do servidor selecionado, por id do
// canal. enabled=false (servidor sem canal de voz visível) não faz poll
// nenhum. Erro de poll mantém a última lista: é informação de apoio, não
// vale uma tela de erro.
const EMPTY: Record<string, VoiceParticipant[]> = {}

export function useVoiceParticipants(
  serverBaseUrl: string,
  accessToken: string,
  enabled: boolean,
): Record<string, VoiceParticipant[]> {
  // A lista vem marcada com o servidor de onde veio: ao trocar de servidor
  // a do anterior deixa de valer na hora, sem precisar limpar o estado
  // dentro do efeito.
  const [state, setState] = useState<{ serverBaseUrl: string; byChannel: Record<string, VoiceParticipant[]> }>()

  useEffect(() => {
    if (!enabled || !serverBaseUrl || !accessToken) return
    let cancelled = false

    function load() {
      fetchVoiceParticipants(serverBaseUrl, accessToken)
        .then((byChannel) => {
          if (!cancelled) setState({ serverBaseUrl, byChannel })
        })
        .catch(() => {
          /* mantém a última lista, ver comentário do hook */
        })
    }

    load()
    const interval = setInterval(load, VOICE_PARTICIPANTS_POLL_INTERVAL_MS)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [serverBaseUrl, accessToken, enabled])

  return enabled && state?.serverBaseUrl === serverBaseUrl ? state.byChannel : EMPTY
}
