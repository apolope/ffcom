import { useEffect, useState } from 'react'

// Folga para arredondamento de subpixel: com zoom ou DPI fracionário,
// scrollTop + clientHeight pode ficar a menos de 1px de scrollHeight mesmo
// no fim.
const EDGE_TOLERANCE_PX = 1

export interface ScrollEdges {
  // Ainda há conteúdo escondido acima / abaixo da área visível.
  up: boolean
  down: boolean
}

const NO_EDGES: ScrollEdges = { up: false, down: false }

// Acompanha se uma área com overflow ainda pode rolar para cima ou para
// baixo. Reage à rolagem e a mudança de tamanho da área ou do conteúdo
// (item adicionado ou removido, janela redimensionada).
//
// Devolve um ref de callback (passar em `ref={...}`) em vez de receber um
// useRef: o efeito precisa rodar de novo quando o elemento é montado outra
// vez, e um useRef não avisa isso.
export function useScrollEdges(): [(el: HTMLElement | null) => void, ScrollEdges, HTMLElement | null] {
  const [el, setEl] = useState<HTMLElement | null>(null)
  const [edges, setEdges] = useState<ScrollEdges>(NO_EDGES)

  useEffect(() => {
    if (!el) return

    const update = () => {
      const up = el.scrollTop > EDGE_TOLERANCE_PX
      const down = el.scrollTop + el.clientHeight < el.scrollHeight - EDGE_TOLERANCE_PX
      setEdges((prev) => (prev.up === up && prev.down === down ? prev : { up, down }))
    }

    update()
    el.addEventListener('scroll', update, { passive: true })
    const resize = new ResizeObserver(update)
    resize.observe(el)
    for (const child of el.children) resize.observe(child)
    return () => {
      el.removeEventListener('scroll', update)
      resize.disconnect()
    }
  }, [el])

  // Sem elemento montado não há o que rolar, mesmo que o último estado
  // calculado diga o contrário.
  return [setEl, el ? edges : NO_EDGES, el]
}
