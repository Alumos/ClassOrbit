import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { Camera, ImageUp, X } from 'lucide-react'
import jsQR from 'jsqr'
import { Button } from './ui'

type QRScannerPhase = 'starting' | 'scanning' | 'capture' | 'processing'

export function loginURLFromQR(value: string) {
  try {
    const url = new URL(value, window.location.origin)
    const token = url.searchParams.get('token')
    if (url.origin !== window.location.origin || url.pathname !== '/qr-login' || !token) return null
    return `${url.pathname}?token=${encodeURIComponent(token)}`
  } catch {
    return null
  }
}

function decodeCanvas(canvas: HTMLCanvasElement, source: CanvasImageSource, width: number, height: number) {
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) return null
  canvas.width = width
  canvas.height = height
  context.drawImage(source, 0, 0, width, height)
  const pixels = context.getImageData(0, 0, width, height)
  return jsQR(pixels.data, width, height, { inversionAttempts: 'attemptBoth' })?.data || null
}

function readImage(file: File) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const image = new Image()
    image.onload = () => { URL.revokeObjectURL(url); resolve(image) }
    image.onerror = () => { URL.revokeObjectURL(url); reject(new Error('无法读取这张图片')) }
    image.src = url
  })
}

export function QRScannerDialog({ onClose }: { onClose: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const timerRef = useRef<number>(0)
  const [phase, setPhase] = useState<QRScannerPhase>('starting')
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    const stop = () => {
      window.clearTimeout(timerRef.current)
      streamRef.current?.getTracks().forEach(track => track.stop())
      streamRef.current = null
    }
    const finish = (value: string) => {
      const target = loginURLFromQR(value)
      if (!target) {
        setError('这不是 ClassOrbit 电脑登录二维码，请重新对准二维码。')
        return false
      }
      stop()
      window.location.assign(target)
      return true
    }
    const scan = () => {
      if (!active) return
      const video = videoRef.current
      const canvas = canvasRef.current
      if (video && canvas && video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA && video.videoWidth > 0) {
        const scale = Math.min(1, 800 / video.videoWidth)
        const value = decodeCanvas(canvas, video, Math.round(video.videoWidth * scale), Math.round(video.videoHeight * scale))
        if (value && finish(value)) return
      }
      timerRef.current = window.setTimeout(scan, 180)
    }
    const start = async () => {
      if (!navigator.mediaDevices?.getUserMedia) {
        setPhase('capture')
        setError('当前浏览器无法直接打开相机，请使用下方按钮拍摄二维码。')
        return
      }
      try {
        const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' }, width: { ideal: 1280 }, height: { ideal: 720 } }, audio: false })
        if (!active) { stream.getTracks().forEach(track => track.stop()); return }
        streamRef.current = stream
        const video = videoRef.current
        if (!video) { stop(); return }
        video.srcObject = stream
        await video.play()
        if (!active) return
        setPhase('scanning')
        scan()
      } catch (reason) {
        if (!active) return
        stop()
        setPhase('capture')
        setError(reason instanceof DOMException && reason.name === 'NotAllowedError' ? '相机权限未开启，请允许浏览器使用相机，或使用下方按钮拍摄二维码。' : '无法打开相机，请使用下方按钮拍摄二维码。')
      }
    }
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    document.addEventListener('keydown', closeOnEscape)
    void start()
    return () => { active = false; document.removeEventListener('keydown', closeOnEscape); stop() }
  }, [onClose])

  const capture = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setPhase('processing'); setError('')
    try {
      const image = await readImage(file)
      const canvas = canvasRef.current || document.createElement('canvas')
      const scale = Math.min(1, 1600 / image.naturalWidth)
      const value = decodeCanvas(canvas, image, Math.round(image.naturalWidth * scale), Math.round(image.naturalHeight * scale))
      if (!value) throw new Error('没有识别到二维码，请让二维码完整出现在照片中。')
      const target = loginURLFromQR(value)
      if (!target) throw new Error('这不是 ClassOrbit 电脑登录二维码。')
      window.location.assign(target)
    } catch (reason) {
      setPhase('capture')
      setError((reason as Error).message)
    }
  }

  return <div className="qr-scan-backdrop" role="presentation">
    <section className="qr-scan-dialog" role="dialog" aria-modal="true" aria-labelledby="qr-scan-title">
      <header className="qr-scan-header"><div><span className="setup-kicker">手机扫一扫</span><h2 id="qr-scan-title">扫描电脑登录二维码</h2></div><Button variant="ghost" size="icon" aria-label="关闭扫一扫" onClick={onClose}><X size={18} /></Button></header>
      <div className={`qr-scan-viewport qr-scan-${phase}`}>
        <video ref={videoRef} className="qr-scan-video" autoPlay playsInline muted aria-label="二维码相机取景框" />
        <canvas ref={canvasRef} className="qr-scan-canvas" aria-hidden="true" />
        {phase !== 'scanning' && phase !== 'starting' && <div className="qr-scan-placeholder"><Camera size={30} /><strong>{phase === 'processing' ? '正在识别二维码…' : '请拍摄电脑上的二维码'}</strong></div>}
        {(phase === 'scanning' || phase === 'starting') && <span className="qr-scan-corners" aria-hidden="true" />}
      </div>
      <p className="qr-scan-hint">将电脑端二维码放入取景框，识别后会打开登录确认页。</p>
      {error && <div className="form-error qr-login-error" role="alert">{error}</div>}
      <label className="button button-outline qr-scan-file-button" htmlFor="qr-camera-capture"><ImageUp size={15} />拍照识别二维码</label>
      <input id="qr-camera-capture" className="qr-scan-file-input" type="file" accept="image/*" capture="environment" onChange={event => void capture(event)} />
    </section>
  </div>
}
