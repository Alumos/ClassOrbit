import { useEffect, useMemo, useState } from 'react'
import { ArrowRight, BookOpenCheck, CheckCircle2, GraduationCap, Search, ShieldCheck } from 'lucide-react'
import { api, json } from '../api'
import { Select, SelectItem } from '../select'
import { Button, EmptyState, Input, SiteFooter } from '../ui'
import type { ClassItem, SiteSettings } from '../types'

type PublicStudent = { id: number; studentNo: string; name: string; checkedIn: boolean }
type Roster = { sessionId: number; title: string; course: string; sessionAt: string; students: PublicStudent[] }
const NAVIGATION_COUNTDOWN_SECONDS = 5

export function CheckInPage() {
  const [classes, setClasses] = useState<ClassItem[]>([])
  const [classId, setClassId] = useState(0)
  const [roster, setRoster] = useState<Roster | null>(null)
  const [rosterLoading, setRosterLoading] = useState(false)
  const [selected, setSelected] = useState<PublicStudent | null>(null)
  const [query, setQuery] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ name: string; message: string } | null>(null)
  const [countdown, setCountdown] = useState(NAVIGATION_COUNTDOWN_SECONDS)
  const [error, setError] = useState('')
  const [settings, setSettings] = useState<SiteSettings>({ title: '智创课堂', subtitle: '小学信息科技课' })
  const [pinyinIndex, setPinyinIndex] = useState<Map<number, string>>(new Map())

  useEffect(() => {
    let active = true
    api<ClassItem[]>('/public/classes').then(data => { if (!active) return; setClasses(data); if (data.length === 1) setClassId(data[0].id) }).catch(e => { if (active) setError(e.message) })
    api<SiteSettings>('/public/settings').then(site => { if (!active) return; setSettings(site); document.title = `${site.title} · 学生签到` }).catch(() => undefined)
    return () => { active = false }
  }, [])
  useEffect(() => {
    if (!classId) { setRoster(null); setRosterLoading(false); return }
    const controller = new AbortController()
    let active = true
    setError(''); setSelected(null); setQuery(''); setRoster(null); setRosterLoading(true)
    api<Roster>(`/public/classes/${classId}/students`, { signal: controller.signal }).then(next => { if (!active) return; setRoster(next); setRosterLoading(false) }).catch(e => { if (!active || e.name === 'AbortError') return; setRoster(null); setRosterLoading(false); setError(e.message) })
    return () => { active = false; controller.abort() }
  }, [classId])
  useEffect(() => setPinyinIndex(new Map()), [roster?.sessionId])
  useEffect(() => {
    if (!result) return
    setCountdown(NAVIGATION_COUNTDOWN_SECONDS)
    const tick = window.setInterval(() => setCountdown(current => Math.max(0, current - 1)), 1000)
    const redirect = window.setTimeout(() => window.location.replace('/navigation'), NAVIGATION_COUNTDOWN_SECONDS * 1000)
    return () => { window.clearInterval(tick); window.clearTimeout(redirect) }
  }, [result])
  const needsPinyin = /[a-z]/i.test(query)
  useEffect(() => {
    if (!roster || !needsPinyin || pinyinIndex.size) return
    let active = true
    void import('pinyin-pro').then(({ pinyin }) => {
      if (!active) return
      setPinyinIndex(new Map(roster.students.map(student => {
        const syllables = pinyin(student.name, { toneType: 'none', type: 'array' })
        return [student.id, `${syllables.map(item => item[0]).join('')} ${syllables.join('')}`.toLowerCase()]
      })))
    })
    return () => { active = false }
  }, [roster, needsPinyin, pinyinIndex.size])
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase().replace(/\s+/g, '')
    return roster?.students.filter(item => !q || item.name.toLowerCase().includes(q) || item.studentNo.toLowerCase().includes(q) || pinyinIndex.get(item.id)?.replace(' ', '').includes(q)) || []
  }, [roster, query, pinyinIndex])
  const checkIn = async () => {
    if (!selected) return
    setBusy(true); setError('')
    try { const data = await api<{ name: string; message: string }>('/public/check-in', json('POST', { classId, studentId: selected.id })); setResult(data); setRoster(current => current ? { ...current, students: current.students.map(item => item.id === selected.id ? { ...item, checkedIn: true } : item) } : current) }
    catch (e) { setError((e as Error).message) }
    finally { setBusy(false) }
  }
  const enterNavigation = () => window.location.replace('/navigation')

  return <main className="checkin-page">
    <header className="checkin-header"><a href="/" className="checkin-brand"><span className="brand-mark"><GraduationCap size={19} /></span><div><strong>{settings.title}</strong><span>{settings.subtitle}</span></div></a><span className="secure-note"><ShieldCheck size={15} />仅用于课堂签到</span></header>
    <div className="checkin-shell">
      {result ? <section className="checkin-result"><span className="success-ring"><CheckCircle2 size={38} /></span><p>签到成功</p><h1>{result.name}</h1><span>系统已记录你的到课时间</span><div className="checkin-countdown" role="timer" aria-live="polite"><strong>{countdown}</strong><span>秒后进入学习导航</span></div><Button onClick={enterNavigation}>立即进入<ArrowRight size={15} /></Button></section> : <>
        <div className="checkin-heading"><span className="checkin-symbol"><BookOpenCheck size={22} /></span><div><h1>课堂签到</h1><p>选择你的班级和姓名完成签到。</p></div></div>
        <section className="checkin-panel">
          <div className="checkin-fields"><div className="field"><label>班级</label>{classes.length ? <Select value={classId ? String(classId) : undefined} onValueChange={value => setClassId(Number(value))} placeholder="选择正在上课的班级">{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select> : <div className="disabled-field">当前没有开放签到的班级</div>}</div>{roster && <div className="field"><label>查找姓名</label><label className="search-box checkin-search"><Search size={15} /><Input value={query} onChange={e => setQuery(e.target.value)} placeholder="姓名、拼音首字母或部分学号" /></label></div>}</div>
          {rosterLoading && <div className="checkin-roster-loading" role="status"><span className="page-module-loading" /><span>正在加载学生名单</span></div>}
          {roster && <><div className="session-strip"><div><span>当前场次 · {roster.course}</span><strong>{roster.title}</strong></div><span>{roster.students.filter(item => item.checkedIn).length} / {roster.students.length} 已签到</span></div><div className="name-grid">{visible.map(student => <button key={student.id} disabled={student.checkedIn} aria-pressed={selected?.id === student.id} className={`${selected?.id === student.id ? 'selected' : ''} ${student.checkedIn ? 'checked' : ''}`} onClick={() => setSelected(student)}><span aria-hidden="true">{student.name.slice(-1)}</span><div><strong>{student.name}</strong><small>{student.studentNo}</small></div>{student.checkedIn && <CheckCircle2 size={16} aria-hidden="true" />}</button>)}</div>{visible.length === 0 && <EmptyState icon={<Search size={21} />} title="没有找到学生" detail="请检查姓名拼音首字母、学号或班级。" />}</>}
          {error && <div className="form-error" role="alert">{error}</div>}
          {selected && <div className="checkin-confirm"><div><span>你选择了</span><strong>{selected.name} · {selected.studentNo}</strong></div><Button disabled={busy} onClick={() => void checkIn()}><CheckCircle2 size={16} />{busy ? '正在签到' : '确认是我'}</Button></div>}
          {!roster && classes.length === 0 && <EmptyState icon={<BookOpenCheck size={22} />} title="暂无开放签到" detail="请等待老师发起课堂点名后刷新页面。" action={<Button variant="outline" onClick={() => window.location.reload()}>刷新页面</Button>} />}
        </section>
      </>}
    </div>
    <SiteFooter note="请选择本人信息，避免代签或误签。" />
  </main>
}
