// Atalho de teclado no formato de "accelerator" do Electron (ex.
// "Ctrl+Shift+M"), usado tanto para comparar com o keydown no web/PWA quanto
// para registrar direto no globalShortcut do Electron (electron/main.ts).
// Ver docs/architecture.md, "Decisão: atalho de teclado para mutar".

const MODIFIERS = ['Ctrl', 'Alt', 'Shift', 'Super'] as const

// Tecla pelo event.code (posição física), não pelo event.key: com Shift ou
// Alt o key muda ("m" vira "M", ou um símbolo no AltGr), e o code é o que
// o globalShortcut do Electron também enxerga. Só letras, dígitos, F1-F24,
// espaço e teclado numérico: o suficiente para um atalho e todas com nome
// garantido no formato de accelerator.
function keyFromCode(code: string): string | undefined {
  let m = /^Key([A-Z])$/.exec(code)
  if (m) return m[1]
  m = /^Digit(\d)$/.exec(code)
  if (m) return m[1]
  m = /^Numpad(\d)$/.exec(code)
  if (m) return `num${m[1]}`
  if (/^F([1-9]|1\d|2[0-4])$/.test(code)) return code
  if (code === 'Space') return 'Space'
  return undefined
}

// undefined quando a tecla não é suportada ou só modificadores estão
// apertados (a pessoa ainda está montando a combinação).
export function shortcutFromEvent(e: KeyboardEvent): string | undefined {
  const key = keyFromCode(e.code)
  if (!key) return undefined
  const parts: string[] = []
  if (e.ctrlKey) parts.push('Ctrl')
  if (e.altKey) parts.push('Alt')
  if (e.shiftKey) parts.push('Shift')
  if (e.metaKey) parts.push('Super')
  parts.push(key)
  return parts.join('+')
}

// Exige Ctrl, Alt ou Super (Shift sozinho só troca maiúscula), a não ser
// numa tecla F: no Electron o atalho é global, e uma letra sem modificador
// seria engolida em qualquer programa enquanto a pessoa estiver na voz.
export function isUsableShortcut(shortcut: string): boolean {
  const parts = shortcut.split('+')
  const key = parts[parts.length - 1]
  if (/^F\d+$/.test(key)) return true
  return parts.slice(0, -1).some((p) => p === 'Ctrl' || p === 'Alt' || p === 'Super')
}

// Mesmo formato gerado por shortcutFromEvent; confere o que vem do
// localStorage antes de chegar ao globalShortcut.
export function isShortcutFormat(value: string): boolean {
  const parts = value.split('+')
  const key = parts.pop() ?? ''
  const validKey = /^([A-Z0-9]|num\d|F([1-9]|1\d|2[0-4])|Space)$/.test(key)
  return validKey && parts.every((p) => (MODIFIERS as readonly string[]).includes(p))
}

// Digitando num campo de texto o atalho não dispara (a combinação pode ser
// uma que a pessoa usa para escrever, ex. AltGr no teclado ABNT, que chega
// como Ctrl+Alt).
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    (target instanceof HTMLInputElement && !['checkbox', 'radio', 'button', 'submit'].includes(target.type))
  )
}

// Rótulo para a UI: no Windows/Linux a tecla Super é a do Windows.
export function formatShortcut(shortcut: string): string {
  return shortcut.replace('Super', 'Win').replace(/num(\d)/, 'Num $1')
}
