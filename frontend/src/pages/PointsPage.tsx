import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BarChart3, Clock3, Minus, Plus, RotateCcw, Search, Shuffle, SlidersHorizontal, UserRound, Users } from 'lucide-react'
import { api, json } from '../api'
import { Dialog } from '../dialog'
import { Select, SelectItem } from '../select'
import { Button, EmptyState, Input, SkeletonGrid, formatTime } from '../ui'
import type { ClassItem, ScoreEvent, Student } from '../types'
import type { Notify } from '../types'

export function PointsPage({ classes, classId, setClassId, activeClass, notify, onScoreChange }: { classes: ClassItem[]; classId: number; setClassId: (id: number) => void; activeClass?: ClassItem; notify: Notify; onScoreChange: (delta: number) => void }) {
  const [students, setStudents] = useState<Student[]>([])
  const [sort, setSort] = useState('student_no')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [adjusting, setAdjusting] = useState<Student | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

  const load = useCallback(async () => {
    if (!classId) { setStudents([]); return }
    setLoading(true)
    try { setStudents(await api<Student[]>(`/classes/${classId}/students?sort=${sort}`)) }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setLoading(false) }
  }, [classId, sort, notify])
  useEffect(() => { void load() }, [load])

  const changeScore = async (student: Student, delta: number, reason = '') => {
    setStudents(current => current.map(item => item.id === student.id ? { ...item, score: item.score + delta } : item))
    try {
      const updated = await api<Student>(`/students/${student.id}/score`, json('POST', { delta, reason }))
      setStudents(current => current.map(item => item.id === updated.id ? updated : item))
      onScoreChange(delta)
    } catch (error) {
      setStudents(current => current.map(item => item.id === student.id ? student : item))
      notify((error as Error).message, 'error')
    }
  }
  const undoScore = async (event: ScoreEvent) => {
    if (!adjusting || !event.reversible) return false
    try {
      const updated = await api<Student>(`/score-events/${event.id}/undo`, json('POST'))
      setStudents(current => current.map(item => item.id === updated.id ? updated : item))
      setAdjusting(updated)
      onScoreChange(-event.delta)
      notify('已撤销该笔积分变更')
      return true
    } catch (error) {
      notify((error as Error).message, 'error')
      return false
    }
  }
  const visible = useMemo(() => {
    const q = search.trim().toLowerCase()
    return q ? students.filter(st => st.name.toLowerCase().includes(q) || st.studentNo.toLowerCase().includes(q)) : students
  }, [students, search])

  return <>
    <div className="page-heading"><div><h1>{activeClass?.name || '班级积分台'}</h1><p>课堂表现即时记录，所有变更均保留积分流水。</p></div><Button variant="outline" disabled={!students.length} onClick={() => setPickerOpen(true)}><Shuffle size={16} />随机点名</Button></div>
    <section className="panel filter-panel">
      <div className="filter-title"><SlidersHorizontal size={16} /><div><strong>筛选与排序</strong><span>{visible.length} 名学生</span></div></div>
      <div className="filter-controls">
        <Select value={classId ? String(classId) : undefined} onValueChange={value => setClassId(Number(value))} placeholder="选择班级">{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name} · {item.studentCount}人</SelectItem>)}</Select>
        <label className="search-box"><Search size={15} /><Input value={search} onChange={e => setSearch(e.target.value)} placeholder="按姓名或学号搜索" /></label>
        <div className="segment" aria-label="排序方式"><button className={sort === 'student_no' ? 'active' : ''} onClick={() => setSort('student_no')}>学号</button><button className={sort === 'score_desc' ? 'active' : ''} onClick={() => setSort('score_desc')}>积分高</button><button className={sort === 'score_asc' ? 'active' : ''} onClick={() => setSort('score_asc')}>积分低</button></div>
      </div>
    </section>
    {loading ? <SkeletonGrid /> : visible.length > 0 ? <div className="student-grid">{visible.map(student => <article className="student-card" key={student.id}>
      <div className="student-card-head"><span className="student-avatar">{student.name.slice(-1)}</span><div><strong>{student.name}</strong><span>学号 {student.studentNo}</span></div><button className={`score-pill ${student.score < 0 ? 'negative' : student.score > 0 ? 'positive' : ''}`} onClick={() => setAdjusting(student)} aria-label={`调整${student.name}的积分`}>{student.score > 0 ? '+' : ''}{student.score}</button></div>
      <div className="quick-score"><Button variant="outline" onClick={() => void changeScore(student, -1)} aria-label={`${student.name}扣一分`}><Minus size={17} /><span>扣 1</span></Button><Button onClick={() => void changeScore(student, 1)} aria-label={`${student.name}加一分`}><Plus size={17} /><span>加 1</span></Button></div>
    </article>)}</div> : <section className="panel"><EmptyState icon={<Users size={22} />} title={classId ? '暂无匹配学生' : '先选择一个班级'} detail={classId ? (students.length ? '换一个关键词试试。' : '请在“班级与名单”中导入 Excel 学生名单。') : '创建班级并导入名单后即可开始积分。'} /></section>}
    <AdjustDialog student={adjusting} onClose={() => setAdjusting(null)} onChange={async (delta, reason) => { if (adjusting) await changeScore(adjusting, delta, reason); setAdjusting(null) }} onUndo={undoScore} />
    <RandomPicker open={pickerOpen} onOpenChange={setPickerOpen} students={students} className={activeClass?.name || ''} onAdjust={(student, delta) => void changeScore(student, delta, '随机点名')} />
  </>
}

function AdjustDialog({ student, onClose, onChange, onUndo }: { student: Student | null; onClose: () => void; onChange: (delta: number, reason: string) => Promise<void>; onUndo: (event: ScoreEvent) => Promise<boolean> }) {
  const [amount, setAmount] = useState('1')
  const [direction, setDirection] = useState<'add' | 'minus'>('add')
  const [reason, setReason] = useState('')
  const [events, setEvents] = useState<ScoreEvent[]>([])
  useEffect(() => { if (student) { setAmount('1'); setDirection('add'); setReason(''); api<ScoreEvent[]>(`/students/${student.id}/events`).then(setEvents).catch(() => setEvents([])) } }, [student])
  const submit = () => { const value = Math.max(1, Math.min(100, Number(amount) || 1)); void onChange(direction === 'add' ? value : -value, reason) }
  return <Dialog open={!!student} onOpenChange={open => !open && onClose()} title={student ? `调整 ${student.name} 的积分` : '调整积分'} description={student ? `当前积分 ${student.score} · 学号 ${student.studentNo}` : ''} width="wide" footer={<><Button variant="outline" onClick={onClose}>取消</Button><Button onClick={submit}>确认调整</Button></>}>
    <div className="adjust-layout"><div className="form-stack"><div className="field"><label>变更方向</label><div className="segment segment-full"><button className={direction === 'add' ? 'active' : ''} onClick={() => setDirection('add')}><Plus size={14} />加分</button><button className={direction === 'minus' ? 'active danger-active' : ''} onClick={() => setDirection('minus')}><Minus size={14} />扣分</button></div></div><div className="field"><label htmlFor="score-amount">分值</label><Input id="score-amount" type="number" min="1" max="100" value={amount} onChange={e => setAmount(e.target.value)} /></div><div className="field"><label htmlFor="score-reason">原因</label><Input id="score-reason" value={reason} onChange={e => setReason(e.target.value)} placeholder={direction === 'add' ? '如：积极回答问题' : '如：课堂纪律'} /></div></div>
      <div className="history-list"><div className="history-title"><Clock3 size={15} /><strong>最近流水</strong></div>{events.length ? events.slice(0, 6).map(event => <div className="history-item" key={event.id}><span className={event.delta > 0 ? 'delta-add' : 'delta-minus'}>{event.delta > 0 ? '+' : ''}{event.delta}</span><div><strong>{event.reason}</strong><small>{formatTime(event.createdAt)}{event.reversedAt ? ' · 已撤销' : ''}</small></div>{event.reversible && <Button variant="ghost" size="icon" title="撤销这笔积分" onClick={async () => { if (await onUndo(event)) setEvents(current => current.map(item => item.id === event.id ? { ...item, reversible: false, reversedAt: new Date().toISOString() } : item)) }}><RotateCcw size={13} /></Button>}</div>) : <p className="muted">暂无积分流水</p>}</div></div>
  </Dialog>
}

function RandomPicker({ open, onOpenChange, students, className, onAdjust }: { open: boolean; onOpenChange: (v: boolean) => void; students: Student[]; className: string; onAdjust: (student: Student, delta: number) => void }) {
  const [selected, setSelected] = useState<Student[]>([])
  const [pickCount, setPickCount] = useState(1)
  const [rolling, setRolling] = useState(false)
  const timerRef = useRef<number | null>(null)
  const sample = (count: number) => {
    const pool = [...students]
    for (let i = pool.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1)); [pool[i], pool[j]] = [pool[j], pool[i]]
    }
    return pool.slice(0, Math.min(count, pool.length))
  }
  const pick = () => {
    if (!students.length || rolling) return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) { setSelected(sample(pickCount)); return }
    setRolling(true)
    let count = 0
    timerRef.current = window.setInterval(() => { setSelected(sample(pickCount)); count++; if (count >= 12 && timerRef.current !== null) { window.clearInterval(timerRef.current); timerRef.current = null; setRolling(false) } }, 65)
  }
  useEffect(() => {
    if (open) { setSelected([]); setPickCount(current => Math.min(Math.max(1, current), Math.max(1, students.length))); setRolling(false) }
    return () => { if (timerRef.current !== null) { window.clearInterval(timerRef.current); timerRef.current = null } }
  }, [open, students.length])
  const changeCount = (next: number) => { setPickCount(Math.min(Math.max(1, next), Math.max(1, students.length))); setSelected([]) }
  return <Dialog open={open} onOpenChange={onOpenChange} title="随机点名" description={`${className} · 共 ${students.length} 名学生`} width="wide" footer={<><Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button><Button onClick={pick} disabled={rolling}><Shuffle size={16} />{selected.length ? '重新抽取' : '开始抽取'}</Button></>}>
    <div className="random-count"><div><strong>点名人数</strong><span>本次不重复抽取</span></div><div className="number-stepper"><Button variant="outline" size="icon" disabled={pickCount <= 1} onClick={() => changeCount(pickCount - 1)} aria-label="减少点名人数"><Minus size={15} /></Button><Input type="number" min="1" max={Math.max(1, students.length)} value={pickCount} onChange={event => changeCount(Number(event.target.value) || 1)} aria-label="点名人数" /><Button variant="outline" size="icon" disabled={pickCount >= students.length} onClick={() => changeCount(pickCount + 1)} aria-label="增加点名人数"><Plus size={15} /></Button></div></div>
    <div className={`random-stage ${rolling ? 'rolling' : ''} ${selected.length > 1 ? 'random-stage-multiple' : ''}`}>{selected.length ? <div className="random-results">{selected.map(student => <div className="random-result" key={student.id}><span className="random-avatar">{student.name.slice(-1)}</span><div className="random-student"><strong>{student.name}</strong><p>学号 {student.studentNo}</p></div><div className="random-actions"><Button variant="outline" size="icon" onClick={() => onAdjust(student, -1)} aria-label={`${student.name}扣一分`}><Minus size={14} /></Button><Button size="icon" onClick={() => onAdjust(student, 1)} aria-label={`${student.name}加一分`}><Plus size={14} /></Button></div></div>)}</div> : <><span className="random-placeholder"><UserRound size={34} /></span><strong>准备点名</strong><p>设置人数后开始随机抽取</p></>}</div>
  </Dialog>
}
