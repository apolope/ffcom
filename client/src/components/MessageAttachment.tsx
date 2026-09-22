import { useEffect, useState } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { fetchAttachmentBlob, type RemoteAttachment } from '../lib/serverChannelApi'
import './MessageAttachment.css'

interface MessageAttachmentProps {
  serverBaseUrl: string
  attachment: RemoteAttachment
}

// Anexo exige o mesmo Bearer token de qualquer outra rota (ver
// docs/architecture.md, "Decisão: upload de anexo em mensagem"), então não
// dá pra apontar um <img src="..."> direto pra URL do servidor — busca como
// Blob e gera uma object URL local, revogada quando o componente desmonta.
export function MessageAttachment({ serverBaseUrl, attachment }: MessageAttachmentProps) {
  const { accessToken } = useAuth()
  const [blobUrl, setBlobUrl] = useState<string>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    let url: string | undefined

    fetchAttachmentBlob(serverBaseUrl, attachment, accessToken!)
      .then((blob) => {
        if (cancelled) return
        url = URL.createObjectURL(blob)
        setBlobUrl(url)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })

    return () => {
      cancelled = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [serverBaseUrl, attachment.id, accessToken])

  if (failed) {
    return <div className="attachment attachment-error">falha ao carregar anexo: {attachment.filename}</div>
  }
  if (!blobUrl) {
    return <div className="attachment attachment-loading">carregando anexo…</div>
  }
  if (attachment.contentType.startsWith('image/')) {
    return (
      <a href={blobUrl} target="_blank" rel="noreferrer" className="attachment attachment-image">
        <img src={blobUrl} alt={attachment.filename} />
      </a>
    )
  }
  return (
    <a href={blobUrl} download={attachment.filename} className="attachment attachment-file">
      {attachment.filename}
    </a>
  )
}
