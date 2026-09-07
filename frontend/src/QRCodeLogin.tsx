import { useEffect, useState, type FormEvent } from 'react'
import { CheckCircle2, KeyRound, RefreshCw, ScanLine, ShieldCheck, XCircle } from 'lucide-react'
import QRCode from 'qrcode'
import { api, json } from './api'
import type { AuthStatus, SiteSettings } from './types'
import { Button, SiteFooter } from './ui'

type QRStart = { token: string; claimToken: string; expiresAt: number }
type QRStatus = { status: 'pending' | 'scanned' | 'approved' | 'denied' | 'consumed' | 'expired' | 'authenticated'; authenticated?: boolean; username?: string }
type QRChallenge = { deviceName: string; status: QRStatus['status']; expiresAt: number }

function deviceName() {
  const agent = navigator.userAgent
  const browser = /Edg\//.test(agent) ? 'Edge' : /Chrome\//.test(agent) ? 'Chrome' : /Firefox\//.test(agent) ? 'Firefox' : /Safari\//.test(agent) ? 'Safari' : '浏览器'
  const platform = /Macintosh/.test(agent) ? 'macOS' : /Windows/.test(agent) ? 'Windows' : /Android/.test(agent) ? 'Android' : /iPhone|iPad/.test(agent) ? 'iOS' : '电脑'
  return `${browser} · ${platform}`
}

function localOnlyHost() {
  return ['localhost', '127.0.0.1', '0.0.0.0', '::1', '[::]'].includes(window.location.hostname)
}

export function QRCodeLoginPanel({ onLogin }: { onLogin: (auth: AuthStatus) => void }) {
  const [request, setRequest] = useState<QRStart | null>(null)
  const [image, setImage] = useState('')
  const [status, setStatus] = useState<QRStatus['status']>('pending')
  const [remaining, setRemaining] = useState(0)
  const [error, setError] = useState('')

  const start = async () => {
    setRequest(null); setRemaining(0); setError(''); setImage(''); setStatus('pending')
    if (localOnlyHost()) {
      setError('当前地址只能由这台电脑访问。请改用手机可访问的域名或局域网 IP 打开登录页。')
      return
    }
    try {
      const next = await api<QRStart>('/auth/qr/start', json('POST', { deviceName: deviceName() }))
      const loginURL = `${window.location.origin}/qr-login?token=${encodeURIComponent(next.token)}`
      setImage(await QRCode.toDataURL(loginURL, { width: 216, margin: 1, errorCorrectionLevel: 'M', color: { dark: '#18181b', light: '#ffffff' } }))
      setRequest(next)
    } catch (e) { setError((e as Error).message) }
  }

  useEffect(() => { void start() }, [])
  useEffect(() => {
    if (!request) return
    const update = () => setRemaining(Math.max(0, request.expiresAt - Math.floor(Date.now() / 1000)))
    update()
    const timer = window.setInterval(update, 1000)
    return () => window.clearInterval(timer)
  }, [request])
  useEffect(() => {
    if (!request || status === 'denied' || status === 'expired') return
    let active = true
    let timer = 0
    const poll = async () => {
      try {
        const next = await api<QRStatus>('/auth/qr/status', json('POST', { token: request.claimToken }))
        if (!active) return
        if (next.authenticated && next.username) {
          onLogin({ initialized: true, authenticated: true, username: next.username })
          return
        }
        setStatus(next.status)
        if (next.status !== 'denied' && next.status !== 'expired' && next.status !== 'consumed') timer = window.setTimeout(poll, 2000)
      } catch (e) { if (active) setError((e as Error).message) }
    }
    timer = window.setTimeout(poll, 1000)
    return () => { active = false; window.clearTimeout(timer) }
  }, [request, status, onLogin])

  const expired = remaining === 0 && request !== null
  const message = status === 'scanned' || status === 'approved' ? '已扫码，请在手机上确认登录' : status === 'denied' ? '手机已取消本次登录' : status === 'consumed' ? '二维码已经使用' : expired || status === 'expired' ? '二维码已过期' : '请使用已登录 ClassOrbit 的手机扫码'

  return <div className="qr-login-box">
    <div className={`qr-code-frame ${image ? '' : 'qr-code-loading'}`}>{image ? <img src={image} alt="扫码登录二维码" width="216" height="216" /> : <ScanLine size={34} />}</div>
    <strong>{message}</strong>
    {!error && request && !expired && status !== 'denied' && <span>{status === 'scanned' || status === 'approved' ? '确认后电脑将自动登录' : `${remaining} 秒后失效`}</span>}
    {error && <div className="form-error qr-login-error" role="alert">{error}</div>}
    {(expired || status === 'expired' || status === 'denied' || status === 'consumed' || error) && <Button type="button" variant="outline" size="sm" onClick={() => void start()}><RefreshCw size={14} />重新生成</Button>}
  </div>
}

export function QRLoginApprovalPage() {
  const token = new URLSearchParams(window.location.search).get('token') || ''
  const [settings, setSettings] = useState<SiteSettings>({ title: '智创课堂', subtitle: '小学信息科技课' })
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [challenge, setChallenge] = useState<QRChallenge | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [result, setResult] = useState<'approved' | 'denied' | ''>('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = `确认扫码登录 · ${settings.title}`
  }, [settings.title])
  useEffect(() => {
    let active = true
    Promise.all([
      api<AuthStatus>('/auth'),
      api<QRChallenge>(`/auth/qr/challenge?token=${encodeURIComponent(token)}`),
      api<SiteSettings>('/public/settings').catch(() => ({ title: '智创课堂', subtitle: '小学信息科技课' })),
    ]).then(([nextAuth, nextChallenge, nextSettings]) => {
      if (!active) return
      setAuth(nextAuth); setChallenge(nextChallenge); setSettings(nextSettings)
    }).catch(e => { if (active) setError((e as Error).message) })
    return () => { active = false }
  }, [token])

  const login = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try { setAuth(await api<AuthStatus>('/auth', json('POST', { username: username.trim(), password }))) }
    catch (e) { setError((e as Error).message) }
    finally { setBusy(false) }
  }
  const decide = async (approve: boolean) => {
    setBusy(true); setError('')
    try {
      const next = await api<QRChallenge>('/auth/qr/decision', json('POST', { token, approve }))
      setResult(next.status === 'approved' ? 'approved' : 'denied')
    } catch (e) { setError((e as Error).message) }
    finally { setBusy(false) }
  }

  let content
  if (error && !challenge) content = <><XCircle size={42} /><h1>二维码不可用</h1><p>{error}</p></>
  else if (!auth || !challenge) content = <><span className="page-module-loading" /><p>正在读取登录请求…</p></>
  else if (challenge.status === 'expired' || challenge.status === 'consumed' || challenge.status === 'denied') content = <><XCircle size={42} /><h1>二维码已失效</h1><p>请回到电脑登录页重新生成。</p></>
  else if (challenge.status === 'approved') content = <><CheckCircle2 size={42} /><h1>已确认登录</h1><p>电脑端即将自动进入教师后台。</p></>
  else if (result) content = <><CheckCircle2 size={42} /><h1>{result === 'approved' ? '已确认登录' : '已取消登录'}</h1><p>{result === 'approved' ? '电脑端即将自动进入教师后台。' : '电脑端不会登录，可以关闭此页面。'}</p></>
  else if (!auth.authenticated) content = <form className="qr-phone-login" onSubmit={login}><KeyRound size={36} /><h1>先验证教师账号</h1><p>验证后再确认是否允许电脑登录。</p><label className="field"><span>教师账号</span><input className="input" autoFocus autoComplete="username" maxLength={32} value={username} onChange={event => setUsername(event.target.value)} /></label><label className="field"><span>密码</span><input className="input" type="password" autoComplete="current-password" maxLength={72} value={password} onChange={event => setPassword(event.target.value)} /></label>{error && <div className="form-error qr-login-error" role="alert">{error}</div>}<Button disabled={busy || !username.trim() || !password}>{busy ? '正在验证' : '验证账号'}</Button></form>
  else content = <><ShieldCheck size={42} /><span className="setup-kicker">扫码登录确认</span><h1>允许这台电脑登录？</h1><div className="qr-device-card"><span>登录设备</span><strong>{challenge.deviceName}</strong><small>账号：{auth.username}</small></div>{error && <div className="form-error qr-login-error" role="alert">{error}</div>}<div className="qr-approval-actions"><Button variant="outline" disabled={busy} onClick={() => void decide(false)}>取消</Button><Button disabled={busy} onClick={() => void decide(true)}>{busy ? '正在确认' : '确认登录'}</Button></div></>

  return <main className="login-page"><section className="login-panel qr-approval-panel"><span className="brand-mark login-mark"><ScanLine size={21} /></span>{content}</section><SiteFooter /></main>
}
