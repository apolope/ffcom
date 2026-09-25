// Grava um valor no servidor tentando até conseguir: espera de 2s entre as
// tentativas, dobrando até 30s, sem limite de tentativas. Usado para a ordem
// arrastada (categorias e canais em hooks/useServerStructure.ts, servidores
// do rail em hooks/useKnownServers.ts).
//
// - schedule(valor) troca o que vai ser gravado. Se uma tentativa falhou e o
//   job está esperando, tenta de novo na hora com o valor novo.
// - onFirstFailure é chamado só na primeira falha de uma sequência (para
//   avisar a pessoa uma vez, não a cada tentativa).
// - onSaved(valor, resultado) só é chamado se ninguém agendou outro valor
//   durante a gravação; se agendou, a próxima volta grava o mais novo.
// - cancel() mata o job (troca de servidor/sessão). Não existe persistência:
//   um reload da página também mata, e o que não foi gravado se perde.
export interface RetryingSaver<T> {
  schedule: (value: T) => void
  cancel: () => void
}

const RETRY_BASE_MS = 2_000
const RETRY_MAX_MS = 30_000

export function createRetryingSaver<T, R>(options: {
  label: string
  save: (value: T) => Promise<R>
  onSaved: (value: T, result: R) => void
  onFirstFailure?: () => void
}): RetryingSaver<T> {
  let latest: { value: T } | undefined
  let running = false
  let cancelled = false
  let wake: (() => void) | undefined

  async function loop() {
    running = true
    let failures = 0
    try {
      while (latest && !cancelled) {
        const current = latest
        try {
          const result = await options.save(current.value)
          if (cancelled) return
          failures = 0
          if (latest === current) {
            latest = undefined
            options.onSaved(current.value, result)
          }
        } catch (err) {
          if (cancelled) return
          failures += 1
          console.warn(`ffcom: falha ao salvar ${options.label} (tentativa ${failures})`, err)
          if (failures === 1) options.onFirstFailure?.()
          const delay = Math.min(RETRY_BASE_MS * 2 ** (failures - 1), RETRY_MAX_MS)
          await new Promise<void>((resolve) => {
            const timer = setTimeout(resolve, delay)
            wake = () => {
              clearTimeout(timer)
              resolve()
            }
          })
          wake = undefined
        }
      }
    } finally {
      running = false
    }
  }

  return {
    schedule(value) {
      if (cancelled) return
      latest = { value }
      if (running) wake?.()
      else void loop()
    },
    cancel() {
      cancelled = true
      latest = undefined
      wake?.()
    },
  }
}
