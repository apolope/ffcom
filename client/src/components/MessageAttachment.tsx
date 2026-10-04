import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation()
  const { accessToken } = useAuth()
  const [blobUrl, setBlobUrl] = useState<string>()
  const [failed, setFailed] = useState(false)

  // O token fica fora das dependências: a renovação silenciosa (a cada ~1h)
  // não deve baixar de novo todo anexo já na tela nem revogar a object URL
  // que o <img> e o link estão usando.
  const accessTokenRef = useRef(accessToken)
  useEffect(() => {
    accessTokenRef.current = accessToken
  }, [accessToken])

  useEffect(() => {
    let cancelled = false
    let url: string | undefined

    fetchAttachmentBlob(serverBaseUrl, attachment, accessTokenRef.current!)
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
  }, [serverBaseUrl, attachment.id])

  if (failed) {
    return (
      <div className="attachment attachment-error">
        {t('chat.attachmentLoadFailed', { filename: attachment.filename })}
      </div>
    )
  }
  if (!blobUrl) {
    return <div className="attachment attachment-loading">{t('chat.attachmentLoading')}</div>
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
