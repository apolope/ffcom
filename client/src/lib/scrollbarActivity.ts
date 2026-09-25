// Scrollbar que reage a mouse e rolagem (docs/design-system.md, "Scrollbar"):
// parado, só o polegar fino (2px) aparece, para indicar que a área rola; com
// o mouse em cima da faixa do scrollbar, engrossa um pouco e o trilho cinza
// aparece de leve; rolando, fica na espessura padrão com o trilho inteiro.
//
// Uma espessura só (--sb-size, lida pelo CSS de ::-webkit-scrollbar em
// index.css) comanda tudo; a opacidade do trilho (--sb-track, de 0 a 1) é
// derivada dela. A animação é feita aqui, quadro a quadro, porque o Chrome
// não aplica transition nas partes do scrollbar. A largura reservada (10px)
// nunca muda, só a parte desenhada, então o conteúdo não se mexe.
//
// Só no Chromium/Electron: o Firefox não tem ::-webkit-scrollbar e fica com
// o scrollbar fino padrão dele.

// Espessuras parada (também o fallback de --sb-size no CSS), com o mouse em
// cima e rolando, e a opacidade do trilho com o mouse em cima.
const REST_PX = 2
const HOVER_PX = 4
const FULL_PX = 6
const HOVER_TRACK = 0.4
const GROW_MS = 180
const SHRINK_MS = 400
// Tempo sem evento de scroll para considerar a rolagem terminada.
const IDLE_MS = 700

interface ScrollbarState {
  size: number
  // Para onde a animação em curso está indo (undefined = parada). Só se
  // reinicia a animação quando o destino muda: rolagem contínua dispara um
  // evento por quadro, e reiniciar a cada um travava o crescimento.
  target: number | undefined
  frame: number
  scrolling: boolean
  hovered: boolean
  idle: ReturnType<typeof setTimeout> | undefined
}

const easeOutCubic = (t: number) => 1 - (1 - t) ** 3

// Opacidade do trilho para uma espessura: 0 parado, HOVER_TRACK com o mouse
// em cima, 1 rolando, interpolando entre eles.
function trackFor(size: number) {
  if (size <= REST_PX) return 0
  if (size <= HOVER_PX) return (HOVER_TRACK * (size - REST_PX)) / (HOVER_PX - REST_PX)
  return HOVER_TRACK + ((1 - HOVER_TRACK) * Math.min(size - HOVER_PX, FULL_PX - HOVER_PX)) / (FULL_PX - HOVER_PX)
}

function setSize(el: HTMLElement, state: ScrollbarState, size: number) {
  state.size = size
  el.style.setProperty('--sb-size', `${size}px`)
  el.style.setProperty('--sb-track', trackFor(size).toFixed(3))
}

function animate(el: HTMLElement, state: ScrollbarState, to: number, duration: number, done?: () => void) {
  if (state.target === to || (state.target === undefined && state.size === to)) return
  cancelAnimationFrame(state.frame)
  state.target = to
  const from = state.size
  const start = performance.now()
  const step = (now: number) => {
    // O timestamp do rAF é o início do quadro e pode vir antes de `start`;
    // sem o limite inferior, t negativo fazia a curva devolver um valor
    // abaixo de `from` e a barra encolhia em vez de crescer.
    const t = Math.min(Math.max((now - start) / duration, 0), 1)
    setSize(el, state, from + (to - from) * easeOutCubic(t))
    if (t < 1) {
      state.frame = requestAnimationFrame(step)
    } else {
      state.target = undefined
      done?.()
    }
  }
  state.frame = requestAnimationFrame(step)
}

// Leva a barra para a espessura do estado atual: rolando > mouse em cima >
// parada. Ao voltar ao repouso, limpa os estilos inline (o CSS já desenha o
// repouso pelos fallbacks).
function settle(el: HTMLElement, state: ScrollbarState) {
  if (state.scrolling) {
    animate(el, state, FULL_PX, GROW_MS)
  } else if (state.hovered) {
    animate(el, state, HOVER_PX, state.size < HOVER_PX ? GROW_MS : SHRINK_MS)
  } else {
    animate(el, state, REST_PX, SHRINK_MS, () => {
      el.style.removeProperty('--sb-size')
      el.style.removeProperty('--sb-track')
    })
  }
}

// O ponteiro está sobre a faixa do scrollbar vertical de el? Sobre o
// scrollbar, o alvo do evento é o próprio elemento que rola.
function overVerticalScrollbar(el: HTMLElement, x: number) {
  const scrollbar = el.offsetWidth - el.clientWidth - el.clientLeft * 2
  if (scrollbar <= 0) return false
  const right = el.getBoundingClientRect().right - el.clientLeft
  return x >= right - scrollbar && x <= right
}

export function installScrollbarActivity() {
  if (typeof CSS === 'undefined' || !CSS.supports('selector(::-webkit-scrollbar)')) return

  const states = new WeakMap<HTMLElement, ScrollbarState>()
  const stateOf = (el: HTMLElement) => {
    let state = states.get(el)
    if (!state) {
      state = { size: REST_PX, target: undefined, frame: 0, scrolling: false, hovered: false, idle: undefined }
      states.set(el, state)
    }
    return state
  }

  // Captura: scroll não borbulha, mas passa pela fase de captura no document,
  // então um listener só cobre toda área rolável, inclusive as criadas depois.
  document.addEventListener(
    'scroll',
    (event) => {
      const el = event.target
      if (!(el instanceof HTMLElement)) return
      const state = stateOf(el)

      state.scrolling = true
      el.dataset.scrolling = ''
      settle(el, state)

      clearTimeout(state.idle)
      state.idle = setTimeout(() => {
        state.scrolling = false
        delete el.dataset.scrolling
        settle(el, state)
      }, IDLE_MS)
    },
    { capture: true, passive: true },
  )

  // Mouse sobre a faixa do scrollbar: no máximo uma área por vez.
  let hovered: HTMLElement | undefined
  const unhover = () => {
    if (!hovered) return
    const state = stateOf(hovered)
    state.hovered = false
    settle(hovered, state)
    hovered = undefined
  }

  document.addEventListener(
    'pointermove',
    (event) => {
      const el = event.target
      const over = el instanceof HTMLElement && overVerticalScrollbar(el, event.clientX) ? el : undefined
      if (over === hovered) return
      unhover()
      if (!over) return
      hovered = over
      const state = stateOf(over)
      state.hovered = true
      settle(over, state)
    },
    { capture: true, passive: true },
  )
  document.documentElement.addEventListener('pointerleave', unhover)
  window.addEventListener('blur', unhover)
}
