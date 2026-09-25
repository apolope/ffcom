import { useEffect, type RefObject } from 'react'

// Fecha um menu flutuante com Esc ou clique fora dele. O anchor (o botão que
// abriu o menu) não conta como "fora": o próprio botão já alterna o menu.
export function useMenuDismiss(ref: RefObject<HTMLElement | null>, anchor: HTMLElement, onClose: () => void) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    const onPointer = (event: PointerEvent) => {
      const target = event.target as Node
      if (!ref.current?.contains(target) && !anchor.contains(target)) onClose()
    }
    window.addEventListener('keydown', onKey)
    // Captura: o clique que abriu o menu já passou, e o próximo fora dele
    // fecha antes de chegar a outro botão.
    window.addEventListener('pointerdown', onPointer, true)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('pointerdown', onPointer, true)
    }
  }, [ref, anchor, onClose])
}
