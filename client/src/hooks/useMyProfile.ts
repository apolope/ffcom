import { useCallback, useEffect, useState } from 'react'
import { invalidateAvatar } from '../lib/avatarCache'
import {
  deleteMyAvatar,
  fetchMyProfile,
  setMyStatus,
  uploadMyAvatar,
  type MyProfile,
} from '../lib/serverCentralApi'
import type { ChosenStatus } from '../types'

interface UseMyProfileResult {
  profile: MyProfile | undefined
  uploadAvatar: (file: File) => Promise<void>
  removeAvatar: () => Promise<void>
  // Troca o status escolhido. Aplica na hora e desfaz se o servidor recusar.
  setStatus: (status: ChosenStatus) => Promise<void>
}

// Perfil da conta autenticada em server-central (nome de exibição, avatar —
// ver docs/architecture.md, "Decisão: upload de avatar de conta"). Não
// confundir com hooks/useMe.ts, que é o "me" de um server-channel específico.
export function useMyProfile(accessToken: string): UseMyProfileResult {
  const [profile, setProfile] = useState<MyProfile>()

  useEffect(() => {
    if (!accessToken) return
    let cancelled = false
    fetchMyProfile(accessToken)
      .then((p) => {
        if (!cancelled) setProfile(p)
      })
      .catch(() => {
        /* sem perfil ainda não é erro -- fica undefined */
      })
    return () => {
      cancelled = true
    }
  }, [accessToken])

  const uploadAvatar = useCallback(
    async (file: File) => {
      const updated = await uploadMyAvatar(accessToken, file)
      // A URL do avatar é fixa por conta: sem invalidar, o cache
      // continuaria mostrando a imagem antiga.
      if (updated.avatarUrl) invalidateAvatar(updated.avatarUrl)
      setProfile(updated)
    },
    [accessToken],
  )

  const removeAvatar = useCallback(async () => {
    const previous = profile?.avatarUrl
    const updated = await deleteMyAvatar(accessToken)
    if (previous) invalidateAvatar(previous)
    setProfile(updated)
  }, [accessToken, profile?.avatarUrl])

  const setStatus = useCallback(
    async (status: ChosenStatus) => {
      const previous = profile?.status
      setProfile((p) => (p ? { ...p, status } : p))
      try {
        await setMyStatus(accessToken, status)
      } catch (err) {
        setProfile((p) => (p ? { ...p, status: previous } : p))
        throw err
      }
    },
    [accessToken, profile?.status],
  )

  return { profile, uploadAvatar, removeAvatar, setStatus }
}
