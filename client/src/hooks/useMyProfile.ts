import { useCallback, useEffect, useState } from 'react'
import {
  deleteMyAvatar,
  fetchMyProfile,
  uploadMyAvatar,
  type MyProfile,
} from '../lib/serverCentralApi'

interface UseMyProfileResult {
  profile: MyProfile | undefined
  uploadAvatar: (file: File) => Promise<void>
  removeAvatar: () => Promise<void>
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
      setProfile(updated)
    },
    [accessToken],
  )

  const removeAvatar = useCallback(async () => {
    const updated = await deleteMyAvatar(accessToken)
    setProfile(updated)
  }, [accessToken])

  return { profile, uploadAvatar, removeAvatar }
}
