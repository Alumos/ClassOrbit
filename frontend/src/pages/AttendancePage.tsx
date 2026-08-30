import { useCallback, useEffect, useState } from 'react'
import { CalendarDays, CheckCircle2, ClipboardCheck, Clock3, Play, Square, Trash2, UserCheck, UserX } from 'lucide-react'
import { api, json } from '../api'
import { Dialog } from '../dialog'
import { Select, SelectItem } from '../select'
import { Button, EmptyState, Input, formatTime } from '../ui'
import type { Attendance, AttendanceRecord, ClassItem } from '../types'
import type { Notify } from '../types'

const statusLabels: Record<AttendanceRecord['status'], string> = { present: '已到', absent: '缺席', late: '迟到', leave: '请假' }

export function AttendancePage({ classes, classId, setClassId, notify, onDataChange }: { classes: ClassItem[]; classId: number; setClassId: (id: number) => void; notify: Notify; onDataChange: () => Promise<void> }) {
  const [sessions, setSessions] = useState<Attendance[]>([])
  const [selectedId, setSelectedId] = useState(0)
  const [startOpen, setStartOpen] = useState(false)
  const [deleting, setDeleting] = useState<Attendance | null>(null)
  const [busy, setBusy] = useState(false)
  const [filterClassId, setFilterClassId] = useState(classId)
  const [date, setDate] = useState('')
  useEffect(() => { if (classId) setFilterClassId(classId) }, [classId])
  const load = useCallback(async () => {
    const params = new URLSearchParams()
    if (filterClassId) params.set('class_id', String(filterClassId))
    if (date) params.set('date', date)
    try { const data = await api<Attendance[]>(`/attendance?${params}`); setSessions(data); setSelectedId(current => data.some(s => s.id === current) ? current : data[0]?.id || 0) }
    catch (error) { notify((error as Error).message, 'error') }
  }, [filterClassId, date, notify])
  useEffect(() => { void load() }, [load])
  const selected = sessions.find(item => item.id === selectedId)
  const active = sessions.find(item => item.status === 'active')
  useEffect(() => {
    if (selected?.status !== 'active') return
    const timer = window.setInterval(() => {
      api<Attendance>(`/attendance/${selected.id}`).then(next => setSessions(current => current.map(item => item.id === next.id ? next : item))).catch(error => notify((error as Error).message, 'error'))
    }, 5000)
    return () => window.clearInterval(timer)
  }, [selected?.id, selected?.status, notify])
  const updateSelected = (next: Attendance) => setSessions(current => current.map(item => item.id === next.id ? next : item))
  const start = async (input: { course: string; sessionAt: string }) => {
    setBusy(true)
    try { const next = await api<Attendance>('/attendance', json('POST', { classId: filterClassId, ...input })); setSessions(current => [next, ...current]); setSelectedId(next.id); setStartOpen(false); notify('签到已开放，学生现在可以自助签到'); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const close = async () => {
    if (!selected) return
    setBusy(true)
    try { updateSelected(await api<Attendance>(`/attendance/${selected.id}/close`, json('POST'))); notify('本次点名已结束，缺席名单已确认'); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const remove = async () => {
    if (!deleting) return
    setBusy(true)
    try {
      await api(`/attendance/${deleting.id}`, { method: 'DELETE' })
      setDeleting(null)
      notify(`${deleting.title} 已删除`)
      await Promise.all([load(), onDataChange()])
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const setStatus = async (record: AttendanceRecord, status: AttendanceRecord['status']) => {
    if (!selected) return
    try { updateSelected(await api<Attendance>(`/attendance/${selected.id}/records/${record.studentId}`, json('PATCH', { status }))) }
    catch (error) { notify((error as Error).message, 'error') }
  }

  return <>
    <div className="page-heading"><div><h1>考勤管理</h1><p>开放学生自助签到，并实时核对缺席和异常状态。</p></div><Button disabled={!filterClassId || !!active} onClick={() => setStartOpen(true)}><Play size={15} />发起课堂点名</Button></div>
    <section className="panel filter-panel"><div className="filter-title"><ClipboardCheck size={16} /><div><strong>考勤场次</strong><span>{sessions.length} 条记录</span></div></div><div className="filter-controls attendance-filters"><Select value={filterClassId ? String(filterClassId) : 'all'} onValueChange={value => { const next = value === 'all' ? 0 : Number(value); setFilterClassId(next); if (next) setClassId(next) }} placeholder="选择班级"><SelectItem value="all">全部班级</SelectItem>{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select><label className="date-filter"><CalendarDays size={15} /><Input type="date" value={date} onChange={event => setDate(event.target.value)} aria-label="按日期筛选" /></label>{sessions.length > 0 && <Select value={String(selectedId)} onValueChange={value => setSelectedId(Number(value))} placeholder="选择场次">{sessions.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.status === 'active' ? '进行中 · ' : ''}{item.title}</SelectItem>)}</Select>}</div></section>
    {selected ? <>
      <section className="attendance-summary"><div><span className="summary-icon green"><UserCheck size={18} /></span><div><span>已到 / 迟到</span><strong>{selected.presentCount}</strong></div></div><div><span className="summary-icon red"><UserX size={18} /></span><div><span>当前缺席</span><strong>{selected.absentCount}</strong></div></div><div><span className="summary-icon"><Clock3 size={18} /></span><div><span>上课时间</span><strong className="summary-time">{formatTime(selected.sessionAt)}</strong></div></div><div className="summary-session"><span className={`status ${selected.status === 'active' ? 'status-active' : ''}`}>{selected.status === 'active' ? '签到进行中' : '已结束'}</span>{selected.status === 'active' && <Button variant="outline" size="sm" disabled={busy} onClick={() => void close()}><Square size={13} />结束点名</Button>}<Button variant="danger" size="sm" disabled={busy} onClick={() => setDeleting(selected)}><Trash2 size={13} />删除记录</Button></div></section>
      <section className="panel table-panel"><div className="panel-header"><div><h2>{selected.title}</h2><p>{selected.className} · {selected.course} · {selected.records.length} 名学生</p></div>{selected.status === 'active' && <span className="live-indicator"><i />每 5 秒自动刷新</span>}</div><div className="table-scroll"><table><thead><tr><th>学号</th><th>姓名</th><th>状态</th><th>签到时间</th><th>签到方式</th><th className="cell-action">修正状态</th></tr></thead><tbody>{selected.records.map(record => <tr key={record.studentId}><td className="mono">{record.studentNo}</td><td><strong>{record.name}</strong></td><td><span className={`attendance-status attendance-${record.status}`}>{statusLabels[record.status]}</span></td><td>{formatTime(record.checkedAt)}</td><td>{record.method === 'self' ? '学生自助' : record.method === 'teacher' ? '教师修正' : '—'}</td><td className="cell-action"><Select value={record.status} onValueChange={value => void setStatus(record, value as AttendanceRecord['status'])} className="status-select"><SelectItem value="present">已到</SelectItem><SelectItem value="late">迟到</SelectItem><SelectItem value="leave">请假</SelectItem><SelectItem value="absent">缺席</SelectItem></Select></td></tr>)}</tbody></table></div></section>
    </> : <section className="panel"><EmptyState icon={<ClipboardCheck size={22} />} title="没有符合条件的考勤记录" detail={filterClassId ? '可以发起新场次，或清除日期筛选查看历史记录。' : '请选择具体班级后发起课堂点名。'} action={filterClassId ? <Button onClick={() => setStartOpen(true)}><Play size={15} />发起课堂点名</Button> : undefined} /></section>}
    <StartDialog open={startOpen} busy={busy} onOpenChange={setStartOpen} onStart={start} className={classes.find(item => item.id === filterClassId)?.name || ''} />
    <Dialog open={!!deleting} onOpenChange={open => !open && !busy && setDeleting(null)} title="删除考勤记录" description="该场次和全体学生的考勤明细将一并删除，且无法恢复。" footer={<><Button variant="outline" disabled={busy} onClick={() => setDeleting(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}><Trash2 size={14} />{busy ? '正在删除' : '确认删除'}</Button></>}><div className="danger-box">确定删除 <strong>{deleting?.title}</strong> 吗？{deleting?.status === 'active' && <span>删除后该班将立即停止签到。</span>}</div></Dialog>
  </>
}

function StartDialog({ open, busy, onOpenChange, onStart, className }: { open: boolean; busy: boolean; onOpenChange: (v: boolean) => void; onStart: (input: { course: string; sessionAt: string }) => Promise<void>; className: string }) {
  const [course, setCourse] = useState('信息课')
  const [sessionAt, setSessionAt] = useState('')
  useEffect(() => { if (open) { const now = new Date(); now.setMinutes(now.getMinutes() - now.getTimezoneOffset()); setCourse('信息课'); setSessionAt(now.toISOString().slice(0, 16)) } }, [open])
  const generatedTitle = sessionAt ? `${className} · ${sessionAt.replace('T', ' ')}` : `${className} · 日期 时间`
  return <Dialog open={open} onOpenChange={onOpenChange} title="发起课堂点名" description={`${className} · 开放后学生即可进入签到页`} footer={<><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={busy || !course.trim() || !sessionAt} onClick={() => void onStart({ course: course.trim(), sessionAt })}><Play size={15} />开放签到</Button></>}><div className="form-stack"><div className="class-name-preview"><span>场次名称</span><strong>{generatedTitle}</strong></div><div className="class-fields"><div className="field"><label htmlFor="attendance-course">课程</label><Input id="attendance-course" value={course} onChange={e => setCourse(e.target.value)} /></div><div className="field"><label htmlFor="attendance-date">上课日期时间</label><Input id="attendance-date" type="datetime-local" value={sessionAt} onChange={e => setSessionAt(e.target.value)} /></div></div><div className="info-box"><CheckCircle2 size={17} /><div><strong>名单会立即锁定</strong><span>系统先将全班标记为缺席，学生签到后自动更新为已到；教师可随时修正。</span></div></div></div></Dialog>
}
