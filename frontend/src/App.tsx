import { lazy, Suspense, useEffect, useState, type FormEvent } from 'react'
import { BookOpenCheck, GraduationCap, KeyRound, ScanLine } from 'lucide-react'
import { api, json } from './api'
import { Button, SiteFooter } from './ui'
import type { AuthStatus, SiteSettings } from './types'

const CheckInPage = lazy(() => import('./pages/CheckInPage').then(({ CheckInPage }) => ({ default: CheckInPage })))
const StudentNavigationPage = lazy(() => import('./pages/StudentNavigationPage').then(({ StudentNavigationPage }) => ({ default: StudentNavigationPage })))
const TeacherApp = lazy(() => import('./TeacherApp').then(({ TeacherApp }) => ({ default: TeacherApp })))
const QRCodeLoginPanel = lazy(() => import('./QRCodeLogin').then(({ QRCodeLoginPanel }) => ({ default: QRCodeLoginPanel })))
const QRLoginApprovalPage = lazy(() => import('./QRCodeLogin').then(({ QRLoginApprovalPage }) => ({ default: QRLoginApprovalPage })))

export default function App() {
  if (window.location.pathname.startsWith('/checkin')) return <Suspense fallback={<Loading />}><CheckInPage /></Suspense>
  if (window.location.pathname.startsWith('/navigation')) return <Suspense fallback={<Loading />}><StudentNavigationPage /></Suspense>
  if (window.location.pathname.startsWith('/qr-login')) return <Suspense fallback={<Loading />}><QRLoginApprovalPage /></Suspense>
  return <TeacherGate />
}

function Loading() { return <div className="auth-loading"><span className="brand-mark"><GraduationCap size={19} /></span></div> }

function TeacherGate() {
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [settings, setSettings] = useState<SiteSettings>({ title: '智创课堂', subtitle: '小学信息科技课' })
  useEffect(() => {
    let active = true
    api<AuthStatus>('/auth').then(nextAuth => { if (active) setAuth(nextAuth) }).catch(() => { if (active) setAuth({ initialized: true, authenticated: false, username: '' }) })
    api<SiteSettings>('/public/settings').then(site => { if (active) setSettings(site) }).catch(() => undefined)
    return () => { active = false }
  }, [])
  useEffect(() => {
    const handleUnauthorized = () => setAuth(current => current ? { ...current, authenticated: false, username: '' } : current)
    window.addEventListener('classorbit:unauthorized', handleUnauthorized)
    return () => window.removeEventListener('classorbit:unauthorized', handleUnauthorized)
  }, [])
  useEffect(() => { document.title = `${settings.title} · ${settings.subtitle}` }, [settings])
  if (auth === null) return <Loading />
  if (!auth.initialized) return <SetupPage settings={settings} onSetup={setAuth} />
  if (!auth.authenticated) return <LoginPage settings={settings} onLogin={setAuth} />
  return <Suspense fallback={<Loading />}><TeacherApp username={auth.username} settings={settings} onSettingsChange={setSettings} onLogout={() => setAuth({ initialized: true, authenticated: false, username: '' })} /></Suspense>
}

function LoginPage({ settings, onLogin }: { settings: SiteSettings; onLogin: (auth: AuthStatus) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [method, setMethod] = useState<'password' | 'qr'>('password')
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError('')
    try { onLogin(await api<AuthStatus>('/auth', json('POST', { username: username.trim(), password }))) }
    catch (e) { setError((e as Error).message) }
    finally { setBusy(false) }
  }
  return <main className="login-page"><form className="login-panel" onSubmit={method === 'password' ? submit : event => event.preventDefault()}><span className="brand-mark login-mark"><GraduationCap size={21} /></span><div className="login-title"><h1>{settings.title}</h1><p>{settings.subtitle} · 教师后台</p></div><div className="segment segment-full login-methods"><button type="button" className={method === 'password' ? 'active' : ''} onClick={() => setMethod('password')}><KeyRound size={14} />密码登录</button><button type="button" className={method === 'qr' ? 'active' : ''} onClick={() => setMethod('qr')}><ScanLine size={14} />扫码登录</button></div>{method === 'password' ? <><div className="form-stack"><label className="field"><span>教师账号</span><input className="input" autoFocus autoComplete="username" maxLength={32} value={username} onChange={e => setUsername(e.target.value)} placeholder="请输入账号" /></label><label className="field"><span>密码</span><input className="input" type="password" autoComplete="current-password" maxLength={72} value={password} onChange={e => setPassword(e.target.value)} placeholder="请输入密码" /></label></div>{error && <div className="form-error login-error" role="alert">{error}</div>}<Button type="submit" disabled={busy || !username.trim() || !password}>{busy ? '正在登录' : '登录'}</Button></> : <Suspense fallback={<div className="qr-login-box"><div className="qr-code-frame qr-code-loading"><ScanLine size={34} /></div><strong>正在生成登录二维码</strong><span>请稍候</span></div>}><QRCodeLoginPanel onLogin={onLogin} /></Suspense>}<a href="/checkin"><BookOpenCheck size={14} />前往学生自助签到</a></form><SiteFooter /></main>
}

function SetupPage({ settings, onSetup }: { settings: SiteSettings; onSetup: (auth: AuthStatus) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setError('')
    if (password !== confirmPassword) { setError('两次输入的密码不一致'); return }
    setBusy(true)
    try { onSetup(await api<AuthStatus>('/setup', json('POST', { username: username.trim(), password }))) }
    catch (e) { setError((e as Error).message) }
    finally { setBusy(false) }
  }
  return <main className="login-page"><form className="login-panel setup-panel" onSubmit={submit}><span className="brand-mark login-mark"><GraduationCap size={21} /></span><div className="login-title"><span className="setup-kicker">首次使用</span><h1>创建教师账号</h1><p>设置完成后即可进入 {settings.title} 教师后台。</p></div><div className="form-stack"><label className="field"><span>教师账号</span><input className="input" autoFocus autoComplete="username" minLength={2} maxLength={32} value={username} onChange={e => setUsername(e.target.value)} placeholder="2 至 32 个字符" /></label><label className="field"><span>登录密码</span><input className="input" type="password" autoComplete="new-password" minLength={8} maxLength={72} value={password} onChange={e => setPassword(e.target.value)} placeholder="至少 8 位" /></label><label className="field"><span>确认密码</span><input className="input" type="password" autoComplete="new-password" minLength={8} maxLength={72} value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} placeholder="再次输入密码" /></label></div>{error && <div className="form-error login-error" role="alert">{error}</div>}<Button type="submit" disabled={busy || username.trim().length < 2 || password.length < 8 || !confirmPassword}>{busy ? '正在创建' : '创建并进入后台'}</Button></form><SiteFooter /></main>
}
