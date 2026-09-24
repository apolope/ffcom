import { useCallback, useEffect, useRef, useState } from 'react'
import './AvatarCropper.css'

// Quadro onde a imagem inteira aparece e o círculo do avatar dentro dele: o
// que fica fora do círculo continua visível, só escurecido, para a pessoa ver
// o que está cortando.
const STAGE = 280
const CROP = 220
const CROP_ORIGIN = (STAGE - CROP) / 2
const MAX_ZOOM = 4
// Lado da imagem enviada: avatar aparece no máximo a 72 px (x2 em tela de
// alta densidade), então 512 sobra e mantém o arquivo pequeno.
const OUTPUT_MAX = 512
const PREVIEW_SIZES = [72, 32]
const KEY_STEP = 10

interface AvatarCropperProps {
  file: File
  busy: boolean
  onCancel: () => void
  onConfirm: (blob: Blob) => void
}

interface View {
  zoom: number
  // Canto superior esquerdo da imagem no quadro, em px de tela.
  x: number
  y: number
}

// Mantém o quadrado do recorte sempre coberto pela imagem.
function clamp(view: View, natural: { w: number; h: number }): View {
  const scale = (CROP / Math.min(natural.w, natural.h)) * view.zoom
  const w = natural.w * scale
  const h = natural.h * scale
  return {
    zoom: view.zoom,
    x: Math.min(CROP_ORIGIN, Math.max(CROP_ORIGIN + CROP - w, view.x)),
    y: Math.min(CROP_ORIGIN, Math.max(CROP_ORIGIN + CROP - h, view.y)),
  }
}

// Recorte circular do avatar antes do envio (ver docs/architecture.md,
// "Decisão: recorte do avatar no client"). Arrastar move, o controle
// deslizante e a roda do mouse dão zoom, setas movem pelo teclado. O
// resultado é um quadrado (o círculo é só como ele é exibido) de até
// OUTPUT_MAX px, em WebP onde o navegador gera, senão PNG.
export function AvatarCropper({ file, busy, onCancel, onConfirm }: AvatarCropperProps) {
  const [src, setSrc] = useState<string>()
  const [natural, setNatural] = useState<{ w: number; h: number }>()
  const [view, setView] = useState<View>({ zoom: 1, x: 0, y: 0 })
  const [error, setError] = useState<string>()
  const imgRef = useRef<HTMLImageElement>(null)
  const stageRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef<{ pointerId: number; startX: number; startY: number; view: View }>(undefined)

  useEffect(() => {
    const url = URL.createObjectURL(file)
    setSrc(url)
    return () => URL.revokeObjectURL(url)
  }, [file])

  const scale = natural ? (CROP / Math.min(natural.w, natural.h)) * view.zoom : 1

  // Zoom mantendo parado o ponto que está no centro do círculo.
  const setZoom = useCallback(
    (zoom: number) => {
      if (!natural) return
      setView((v) => {
        const next = Math.min(MAX_ZOOM, Math.max(1, zoom))
        const base = CROP / Math.min(natural.w, natural.h)
        const center = STAGE / 2
        const ratio = (base * next) / (base * v.zoom)
        return clamp(
          { zoom: next, x: center - (center - v.x) * ratio, y: center - (center - v.y) * ratio },
          natural,
        )
      })
    },
    [natural],
  )

  // Roda do mouse: listener nativo, porque o onWheel do React é passivo e
  // não consegue impedir a página de rolar junto.
  useEffect(() => {
    const stage = stageRef.current
    if (!stage) return
    const onWheel = (event: WheelEvent) => {
      event.preventDefault()
      setZoom(view.zoom * (event.deltaY < 0 ? 1.1 : 1 / 1.1))
    }
    stage.addEventListener('wheel', onWheel, { passive: false })
    return () => stage.removeEventListener('wheel', onWheel)
  }, [setZoom, view.zoom])

  function handleLoad() {
    const img = imgRef.current
    if (!img || !img.naturalWidth || !img.naturalHeight) return
    const n = { w: img.naturalWidth, h: img.naturalHeight }
    const base = CROP / Math.min(n.w, n.h)
    setNatural(n)
    // Começa centralizado, com o lado menor da imagem cobrindo o círculo.
    setView({ zoom: 1, x: (STAGE - n.w * base) / 2, y: (STAGE - n.h * base) / 2 })
  }

  function handlePointerDown(event: React.PointerEvent<HTMLDivElement>) {
    if (!natural) return
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, view }
  }

  function handlePointerMove(event: React.PointerEvent<HTMLDivElement>) {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId || !natural) return
    setView(
      clamp(
        {
          zoom: drag.view.zoom,
          x: drag.view.x + event.clientX - drag.startX,
          y: drag.view.y + event.clientY - drag.startY,
        },
        natural,
      ),
    )
  }

  function handlePointerUp(event: React.PointerEvent<HTMLDivElement>) {
    if (dragRef.current?.pointerId === event.pointerId) dragRef.current = undefined
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (!natural) return
    const moves: Record<string, [number, number]> = {
      ArrowLeft: [KEY_STEP, 0],
      ArrowRight: [-KEY_STEP, 0],
      ArrowUp: [0, KEY_STEP],
      ArrowDown: [0, -KEY_STEP],
    }
    const move = moves[event.key]
    if (move) {
      event.preventDefault()
      setView((v) => clamp({ ...v, x: v.x + move[0], y: v.y + move[1] }, natural))
    } else if (event.key === '+' || event.key === '=') {
      setZoom(view.zoom * 1.1)
    } else if (event.key === '-') {
      setZoom(view.zoom / 1.1)
    }
  }

  function handleConfirm() {
    const img = imgRef.current
    if (!img || !natural) return
    // Recorte em px da imagem original.
    const side = CROP / scale
    const sx = (CROP_ORIGIN - view.x) / scale
    const sy = (CROP_ORIGIN - view.y) / scale
    const out = Math.max(1, Math.round(Math.min(OUTPUT_MAX, side)))
    const canvas = document.createElement('canvas')
    canvas.width = out
    canvas.height = out
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      setError('o navegador não conseguiu gerar a imagem recortada')
      return
    }
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(img, sx, sy, side, side, 0, 0, out, out)
    // Navegador sem WebP no canvas devolve PNG, que o servidor também aceita.
    canvas.toBlob(
      (blob) => {
        if (blob) onConfirm(blob)
        else setError('o navegador não conseguiu gerar a imagem recortada')
      },
      'image/webp',
      0.9,
    )
  }

  const imageStyle = natural
    ? { left: view.x, top: view.y, width: natural.w * scale, height: natural.h * scale }
    : { visibility: 'hidden' as const }

  return (
    <div className="avatar-cropper">
      <div className="avatar-cropper-row">
        <div
          ref={stageRef}
          className="avatar-cropper-stage"
          style={{ width: STAGE, height: STAGE }}
          tabIndex={0}
          role="application"
          aria-label="Área de recorte do avatar: arraste para posicionar, setas movem, + e - dão zoom"
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          onPointerCancel={handlePointerUp}
          onKeyDown={handleKeyDown}
        >
          {src && (
            <img
              ref={imgRef}
              src={src}
              alt=""
              draggable={false}
              onLoad={handleLoad}
              onError={() => setError('não foi possível abrir essa imagem')}
              style={imageStyle}
            />
          )}
          <div
            className="avatar-cropper-circle"
            style={{ left: CROP_ORIGIN, top: CROP_ORIGIN, width: CROP, height: CROP }}
            aria-hidden="true"
          />
        </div>
        <div className="avatar-cropper-previews" aria-label="Como vai ficar">
          {PREVIEW_SIZES.map((size) => {
            const k = size / CROP
            return (
              <div key={size} className="avatar-cropper-preview" style={{ width: size, height: size }}>
                {src && natural && (
                  <img
                    src={src}
                    alt=""
                    draggable={false}
                    style={{
                      left: (view.x - CROP_ORIGIN) * k,
                      top: (view.y - CROP_ORIGIN) * k,
                      width: natural.w * scale * k,
                      height: natural.h * scale * k,
                    }}
                  />
                )}
              </div>
            )
          })}
        </div>
      </div>
      <label className="avatar-cropper-zoom">
        Zoom
        <input
          type="range"
          min={1}
          max={MAX_ZOOM}
          step={0.01}
          value={view.zoom}
          onChange={(e) => setZoom(Number(e.target.value))}
          disabled={!natural}
        />
      </label>
      {error && <p className="dialog-error">{error}</p>}
      <div className="dialog-actions">
        <button type="button" onClick={onCancel} disabled={busy}>
          Cancelar
        </button>
        <button type="button" onClick={handleConfirm} disabled={busy || !natural}>
          {busy ? 'Enviando…' : 'Salvar avatar'}
        </button>
      </div>
    </div>
  )
}
