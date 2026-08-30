import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import {
  BarChart3, BookOpenCheck, CheckCircle2, ChevronRight, ClipboardCheck, GraduationCap,
  Compass, LayoutDashboard, LogOut, Menu, PanelLeftClose, Settings2, Users, X,
} from 'lucide-react'
import { api } from './api'
import { Select, SelectItem } from './select'
import { Button, SiteFooter } from './ui'
import type { AttendanceSuggestion, ClassItem, Dashboard, Notify, SiteSettings } from './types'

const PointsPage = lazy(() => import('./pages/PointsPage').then(({ PointsPage }) => ({ default: PointsPage })))
const OverviewPage = lazy(() => import('./pages/OverviewPage').then(({ OverviewPage }) => ({ default: OverviewPage })))
const ClassesPage = lazy(() => import('./pages/ClassesPage').then(({ ClassesPage }) => ({ default: ClassesPage })))
const AttendancePage = lazy(() => import('./pages/AttendancePage').then(({ AttendancePage }) => ({ default: AttendancePage })))
const NavigationSettingsPage = lazy(() => import('./pages/NavigationSettingsPage').then(({ NavigationSettingsPage }) => ({ default: NavigationSettingsPage })))
const SettingsPage = lazy(() => import('./pages/SettingsPage').then(({ SettingsPage }) => ({ default: SettingsPage })))
const ScheduleWidget = lazy(() => import('./ScheduleWidget').then(({ ScheduleWidget }) => ({ default: ScheduleWidget })))

const nav = [
  { id: 'points', label: '积分台', icon: BarChart3 },
  { id: 'overview', label: '总览', icon: LayoutDashboard },
  { id: 'classes', label: '班级与名单', icon: Users },
  { id: 'attendance', label: '考勤管理', icon: ClipboardCheck },
  { id: 'navigation', label: '学生导航', icon: Compass },
  { id: 'settings', label: '系统设置', icon: Settings2 },
] as const
type Page = typeof nav[number]['id']

export function TeacherApp({ username, settings, onSettingsChange, onLogout }: { username: string; settings: SiteSettings; onSettingsChange: (settings: SiteSettings) => void; onLogout: () => void }) {
  const [page, setPage] = useState<Page>('points')
  const [classes, setClasses] = useState<ClassItem[]>([])
  const [classId, setClassId] = useState(() => Number(localStorage.getItem('classorbit-class') || localStorage.getItem('classpoint-class')) || 0)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [navigationDirty, setNavigationDirty] = useState(false)
  const [attendanceSuggestion, setAttendanceSuggestion] = useState<AttendanceSuggestion | null>(null)
  const [toast, setToast] = useState<{ id: number; message: string; kind: 'success' | 'error' } | null>(null)

  const notify: Notify = useCallback((message, kind = 'success') => setToast({ id: Date.now(), message, kind }), [])
  const refresh = useCallback(async () => {
    try {
      const next = await api<ClassItem[]>('/classes')
      setClasses(next)
      setClassId(current => next.some(item => item.id === current) ? current : next[0]?.id || 0)
    } catch (error) { notify((error as Error).message, 'error') }
  }, [notify])

  useEffect(() => { void refresh() }, [refresh])
  useEffect(() => { if (classId) localStorage.setItem('classorbit-class', String(classId)) }, [classId])
  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 3000)
    return () => window.clearTimeout(timer)
  }, [toast])

  const activeClass = classes.find(item => item.id === classId)
  const dashboard = classes.reduce<Dashboard>((total, item) => ({ classCount: total.classCount + 1, studentCount: total.studentCount + item.studentCount, totalScore: total.totalScore + item.totalScore, activeSessions: total.activeSessions + Number(Boolean(item.activeSessionId)) }), { classCount: 0, studentCount: 0, totalScore: 0, activeSessions: 0 })
  const updateClassScore = useCallback((delta: number) => setClasses(current => current.map(item => item.id === classId ? { ...item, totalScore: item.totalScore + delta } : item)), [classId])
  const switchPage = (next: Page) => {
    if (page === 'navigation' && next !== page && navigationDirty && !window.confirm('导航列表有未保存的修改，确定离开吗？')) return
    setPage(next); setSidebarOpen(false)
  }
  const logout = async () => { try { await api('/auth', { method: 'DELETE' }) } catch { /* The local auth state still needs to be cleared. */ } finally { onLogout() } }

  return <div className="app-shell">
    {sidebarOpen && <button className="sidebar-scrim" aria-label="关闭导航" onClick={() => setSidebarOpen(false)} />}
    <aside className={`sidebar ${sidebarOpen ? 'sidebar-open' : ''}`}>
      <div className="brand"><span className="brand-mark"><GraduationCap size={19} /></span><div><strong>{settings.title}</strong><span>{settings.subtitle}</span></div><Button className="sidebar-close" variant="ghost" size="icon" aria-label="关闭导航" onClick={() => setSidebarOpen(false)}><PanelLeftClose size={17} /></Button></div>
      <nav className="main-nav" aria-label="教师后台导航">
        <span className="nav-label">工作台</span>
        {nav.map(item => <button key={item.id} className={page === item.id ? 'active' : ''} onClick={() => switchPage(item.id)}><item.icon size={16} /><span>{item.label}</span>{page === item.id && <ChevronRight className="nav-chevron" size={14} />}</button>)}
        <span className="nav-label nav-label-secondary">学生入口</span>
        <a href="/checkin" target="_blank" rel="noreferrer"><BookOpenCheck size={16} /><span>自助签到页</span></a>
        <a href="/navigation" target="_blank" rel="noreferrer"><Compass size={16} /><span>学习导航页</span></a>
      </nav>
      <div className="sidebar-footer"><div className="teacher-avatar">{username.slice(0, 1).toUpperCase() || '师'}</div><div><strong>{username || '教师账号'}</strong><span>{classes.length} 个班级</span></div><Button variant="ghost" size="icon" aria-label="退出登录" title="退出登录" onClick={() => void logout()}><LogOut size={15} /></Button></div>
    </aside>
    <main className="workspace">
      <header className="topbar">
        <div className="topbar-title"><Button className="mobile-menu" variant="ghost" size="icon" onClick={() => setSidebarOpen(true)} aria-label="打开导航"><Menu size={18} /></Button><div><span>教师后台</span><strong>{nav.find(item => item.id === page)?.label}</strong></div></div>
        <div className="topbar-actions"><Select value={classId ? String(classId) : undefined} onValueChange={value => setClassId(Number(value))} placeholder="选择班级" className="top-class-select">{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select><a href="/checkin" target="_blank" rel="noreferrer" className="button button-outline button-sm"><BookOpenCheck size={15} />学生签到页</a></div>
      </header>
      <div className="page-wrap"><Suspense fallback={<div className="page-module-loading" aria-label="正在加载页面" />}>
        {page === 'points' && <PointsPage classes={classes} classId={classId} setClassId={setClassId} activeClass={activeClass} notify={notify} onScoreChange={updateClassScore} />}
        {page === 'overview' && <OverviewPage classes={classes} dashboard={dashboard} onNavigate={switchPage} />}
        {page === 'classes' && <ClassesPage classes={classes} notify={notify} onDataChange={refresh} onSelectClass={id => { setClassId(id); setPage('points') }} />}
        {page === 'attendance' && <AttendancePage classes={classes} classId={classId} setClassId={setClassId} notify={notify} onDataChange={refresh} initialSuggestion={attendanceSuggestion} onSuggestionConsumed={() => setAttendanceSuggestion(null)} />}
        {page === 'navigation' && <NavigationSettingsPage notify={notify} onDirtyChange={setNavigationDirty} />}
        {page === 'settings' && <SettingsPage settings={settings} classes={classes} onChange={onSettingsChange} notify={notify} />}
      </Suspense></div>
      <SiteFooter />
    </main>
    <Suspense fallback={null}><ScheduleWidget classes={classes} notify={notify} onStartAttendance={detected => {
      setClassId(detected.classId)
      setAttendanceSuggestion(detected)
      setPage('attendance')
    }} /></Suspense>
    {toast && <div key={toast.id} className={`toast toast-${toast.kind}`} role="status">{toast.kind === 'success' ? <CheckCircle2 size={16} /> : <X size={16} />}<span>{toast.message}</span></div>}
  </div>
}
