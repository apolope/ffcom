import { useRef, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

interface AttachFileButtonProps {
  onPick: (file: File) => void
  disabled?: boolean
}

// Clipe que abre o seletor de arquivo, usado pelo canal de texto e pelo
// fórum. O input nativo fica escondido: o texto dele ("Nenhum ficheiro
// selecionado", no idioma do navegador) era cortado pela largura do
// formulário, e quem usa já mostra o nome do arquivo escolhido em
// .pending-attachment. O valor é limpo a cada escolha para a mesma
// seleção disparar onChange de novo depois de remover o anexo.
export function AttachFileButton({ onPick, disabled }: AttachFileButtonProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)

  function handleChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (file) onPick(file)
  }

  return (
    <>
      <input type="file" ref={inputRef} onChange={handleChange} disabled={disabled} hidden />
      <button
        type="button"
        className="message-attach-button"
        onClick={() => inputRef.current?.click()}
        disabled={disabled}
        title={t('chat.attachFile')}
        aria-label={t('chat.attachFile')}
      >
        <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true">
          <path d="M16.5 6.5v10a4.5 4.5 0 0 1-9 0V5a3 3 0 0 1 6 0v10.5a1.5 1.5 0 0 1-3 0V6.5H9v9a3 3 0 0 0 6 0V5a4.5 4.5 0 0 0-9 0v11.5a6 6 0 0 0 12 0v-10h-1.5z" />
        </svg>
      </button>
    </>
  )
}
