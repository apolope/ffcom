interface UpdateButtonProps {
  onUpdate: () => void
}

// Botão de atualizar o client no ServerRail, como o do Discord: só aparece
// quando hooks/useAppUpdate.ts avisa que há versão nova esperando.
export function UpdateButton({ onUpdate }: UpdateButtonProps) {
  return (
    <button
      type="button"
      className="update-button"
      title="Nova versão disponível. Clique para atualizar (recarrega a página e sai da chamada de voz)."
      aria-label="Atualizar o FFCom para a nova versão"
      onClick={onUpdate}
    >
      <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true">
        <path d="M11 4h2v8.2l3.3-3.3 1.4 1.4L12 16l-5.7-5.7 1.4-1.4 3.3 3.3V4zM5 18h14v2H5v-2z" />
      </svg>
    </button>
  )
}
