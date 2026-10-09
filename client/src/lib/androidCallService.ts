import { registerPlugin, type PluginListenerHandle } from '@capacitor/core'
import { isAndroidApp } from './platform'

// Chamada em segundo plano no app Android: enquanto há chamada, o CallPlugin
// (client/android/.../CallPlugin.java, "FfcomCall") mantém o CallService,
// um serviço em primeiro plano com a notificação "Em chamada" e as ações
// "Mutar"/"Desmutar" e "Sair". Sem ele, com a tela apagada ou em outro app,
// o Android corta o microfone e acaba matando a chamada. Quem decide é o
// hooks/useVoiceChannel.ts, ao lado do setVoiceConnected; fora do app
// Android tudo aqui é no-op. Ver docs/architecture.md, "Decisão: chamada em
// segundo plano no Android (fase 5)".

// Textos já traduzidos: o nativo não tem i18n próprio.
export interface CallNotification {
  // Nome do canal de notificação nas configurações do Android.
  channelName: string
  title: string
  text: string
  // Ausente esconde a ação (ainda conectando).
  toggleMicLabel?: string
  leaveLabel: string
}

export type CallNotificationAction = 'toggleMic' | 'leave'

// Contrato com o CallPlugin.java (@CapacitorPlugin(name = "FfcomCall")).
interface FfcomCallPlugin {
  // Liga o serviço na primeira vez, atualiza a notificação nas seguintes.
  // Rejeita com code "not-allowed" se o Android recusou ligar.
  show(notification: CallNotification): Promise<void>
  stop(): Promise<void>
  addListener(
    event: 'action',
    callback: (data: { action: CallNotificationAction }) => void,
  ): Promise<PluginListenerHandle>
}

let plugin: FfcomCallPlugin | undefined

function callPlugin(): FfcomCallPlugin {
  plugin ??= registerPlugin<FfcomCallPlugin>('FfcomCall')
  return plugin
}

// O que foi pedido por último ('' = parado), para não chamar o nativo a cada
// render com a mesma notificação.
let current = ''

function warn(err: unknown) {
  // APK antigo (sem o plugin) rodando a parte web nova: o Capacitor rejeita
  // com UNIMPLEMENTED, e a chamada segue como no navegador.
  if ((err as { code?: string }).code === 'UNIMPLEMENTED') return
  console.warn('[ffcom] serviço da chamada', err)
}

// Mostra (liga o serviço) ou, com undefined, tira a notificação (para o
// serviço). A primeira chamada com notificação precisa sair com o app
// visível (exigência do Android 14 para o microfone), o que o join garante:
// ela roda logo depois do clique em entrar.
export function syncCallNotification(notification: CallNotification | undefined): void {
  if (!isAndroidApp()) return
  const next = notification ? JSON.stringify(notification) : ''
  if (next === current) return
  current = next
  const p = callPlugin()
  void (notification ? p.show(notification) : p.stop()).catch(warn)
}

// Ações da notificação. Devolve a função que remove o listener.
export function onCallNotificationAction(callback: (action: CallNotificationAction) => void): () => void {
  if (!isAndroidApp()) return () => {}
  let removed = false
  let handle: PluginListenerHandle | undefined
  Promise.resolve()
    .then(() => callPlugin().addListener('action', (data) => callback(data.action)))
    .then((h) => {
      handle = h
      if (removed) void h.remove()
    })
    .catch(() => {})
  return () => {
    removed = true
    void handle?.remove()
  }
}
