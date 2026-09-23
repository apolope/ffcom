// Aviso sonoro de mutar/desmutar o microfone (ver docs/architecture.md,
// "Decisão: som ao mutar e desmutar"). Gerado num AudioContext próprio desta
// página, tocado só nos alto-falantes locais: nunca passa pela track
// publicada no LiveKit, então ninguém na sala recebe o som pela rede.
//
// Dois bipes curtos: descendente ao mutar, ascendente ao desmutar, para dar
// para distinguir sem olhar a tela.

const HIGH_HZ = 660
const LOW_HZ = 440
const NOTE_SECONDS = 0.07
const GAP_SECONDS = 0.03
const PEAK_GAIN = 0.12

let context: AudioContext | undefined

function getContext(): AudioContext | undefined {
  if (context) return context
  try {
    context = new AudioContext()
  } catch {
    // Sem Web Audio (navegador muito antigo): fica sem aviso sonoro.
    return undefined
  }
  return context
}

// Chamar de dentro do handler do clique (ou da tecla): o AudioContext nasce
// suspenso sem gesto do usuário, e o som só toca depois que a troca do
// microfone resolve, quando o gesto pode já ter expirado.
export function primeMicToggleSound(): void {
  const ctx = getContext()
  if (ctx?.state === 'suspended') {
    ctx.resume().catch(() => {})
  }
}

function playNote(ctx: AudioContext, frequency: number, start: number): void {
  const osc = ctx.createOscillator()
  const gain = ctx.createGain()
  osc.type = 'sine'
  osc.frequency.value = frequency
  // Ataque e soltura curtos para não estalar.
  gain.gain.setValueAtTime(0.0001, start)
  gain.gain.exponentialRampToValueAtTime(PEAK_GAIN, start + 0.01)
  gain.gain.exponentialRampToValueAtTime(0.0001, start + NOTE_SECONDS)
  osc.connect(gain).connect(ctx.destination)
  osc.start(start)
  osc.stop(start + NOTE_SECONDS + 0.01)
}

export function playMicToggleSound(micEnabled: boolean): void {
  const ctx = getContext()
  if (!ctx) return
  const [first, second] = micEnabled ? [LOW_HZ, HIGH_HZ] : [HIGH_HZ, LOW_HZ]
  const start = ctx.currentTime + 0.01
  playNote(ctx, first, start)
  playNote(ctx, second, start + NOTE_SECONDS + GAP_SECONDS)
}
