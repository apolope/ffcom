// Aviso sonoro de alguém entrando ou saindo da chamada, inclusive a própria
// pessoa (ver docs/architecture.md, "Decisão: som de entrada e saída da
// chamada"). Mesmo AudioContext do som de mutar (lib/micToggleSound.ts),
// tocado só nos alto-falantes locais.
//
// Três notas em arpejo (dó, mi, sol), subindo ao entrar e descendo ao sair,
// em onda triangular e um pouco mais longas que os bipes de mutar, para não
// confundir com eles.

import { getContext, playNote } from './micToggleSound'

const ARPEGGIO_HZ = [523.25, 659.25, 783.99]
const NOTE_SECONDS = 0.09
const GAP_SECONDS = 0.02
const PEAK_GAIN = 0.1
// Várias entradas ou saídas juntas (a sala enchendo, um reconectar que
// escapou do filtro do hook) tocam uma vez só em vez de embolar os arpejos.
const MIN_INTERVAL_MS = 400

let lastPlayedAt = 0

export function playPresenceSound(joined: boolean): void {
  const now = Date.now()
  if (now - lastPlayedAt < MIN_INTERVAL_MS) return
  const ctx = getContext()
  if (!ctx) return
  // Sem gesto recente o contexto pode estar suspenso; o resume só funciona
  // se a página já teve algum gesto, senão fica sem som (sem erro).
  if (ctx.state === 'suspended') ctx.resume().catch(() => {})
  lastPlayedAt = now
  const notes = joined ? ARPEGGIO_HZ : [...ARPEGGIO_HZ].reverse()
  const start = ctx.currentTime + 0.01
  notes.forEach((hz, i) => {
    playNote(ctx, hz, start + i * (NOTE_SECONDS + GAP_SECONDS), NOTE_SECONDS, PEAK_GAIN, 'triangle')
  })
}
