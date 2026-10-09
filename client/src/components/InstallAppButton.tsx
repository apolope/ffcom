import type { FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { NativeAppOffer } from '../hooks/useNativeAppOffer'
import './Dialog.css'

interface InstallAppButtonProps {
  offer: NativeAppOffer
}

// Botão de instalar o app nativo no ServerRail, logo abaixo do de atualizar
// (components/UpdateButton.tsx): mesma forma, ícone de celular com seta e a
// cor do acento, para não confundir com a atualização verde. O "x" no canto
// esconde por 30 dias (hooks/useNativeAppOffer.ts); o menu do avatar mantém
// a entrada. Ver docs/architecture.md, "Decisão: botão de instalar o app
// nativo pelo navegador".
export function InstallAppButton({ offer }: InstallAppButtonProps) {
  const { t } = useTranslation()
  const title = t('installApp.button', { platform: offer.app.label })
  return (
    <div className="install-app-slot">
      <button type="button" className="install-app-button" title={title} aria-label={title} onClick={offer.start}>
        <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true">
          <path d="M7 2h10a2 2 0 0 1 2 2v16a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zm0 2v16h10V4H7zm4 2h2v6.2l2.3-2.3 1.4 1.4L12 16l-4.7-4.7 1.4-1.4 2.3 2.3V6zm-1 12h4v1h-4v-1z" />
        </svg>
      </button>
      <button
        type="button"
        className="install-app-dismiss"
        title={t('installApp.dismissHint')}
        aria-label={t('installApp.dismissHint')}
        onClick={offer.dismiss}
      >
        <svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor" aria-hidden="true">
          <path d="M6.4 5 12 10.6 17.6 5 19 6.4 13.4 12l5.6 5.6-1.4 1.4-5.6-5.6L6.4 19 5 17.6l5.6-5.6L5 6.4z" />
        </svg>
      </button>
    </div>
  )
}

interface InstallAppDialogProps {
  offer: NativeAppOffer
}

// Explicação do que o sistema vai pedir. No Android aparece antes do
// download (o APK vem de fora da Play Store); no Windows, junto com o
// download já iniciado (aviso do SmartScreen). Vai para o body por portal:
// no celular o rail fica numa gaveta que some (visibility: hidden) ao fechar.
export function InstallAppDialog({ offer }: InstallAppDialogProps) {
  const { t } = useTranslation()
  const { app } = offer
  const confirm = app.flow === 'confirm'

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (confirm) offer.download()
    else offer.closeDialog()
  }

  return createPortal(
    <div className="dialog-overlay" onClick={offer.closeDialog}>
      <form className="dialog-card install-app-dialog" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <h2>{t(app.titleKey)}</h2>
        {t(app.noteKey)
          .split('\n\n')
          .map((paragraph) => (
            <p key={paragraph} className="dialog-hint">
              {paragraph}
            </p>
          ))}
        {!confirm && (
          <p className="dialog-hint">
            <a href={app.url}>{t('installApp.retryDownload')}</a>
          </p>
        )}
        <div className="dialog-actions">
          {confirm ? (
            <>
              <button type="button" onClick={offer.dismiss}>
                {t('installApp.notNow')}
              </button>
              <button type="submit" className="dialog-submit" autoFocus>
                {t('installApp.download')}
              </button>
            </>
          ) : (
            <button type="submit" className="dialog-submit" autoFocus>
              {t('installApp.gotIt')}
            </button>
          )}
        </div>
      </form>
    </div>,
    document.body,
  )
}
