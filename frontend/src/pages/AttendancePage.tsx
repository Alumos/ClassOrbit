import { useCallback, useEffect, useState } from 'react'
import { ArchiveRestore, CalendarDays, CheckCircle2, ClipboardCheck, Clock3, Play, RotateCcw, Square, Trash2, UserCheck, UserX } from 'lucide-react'
import { api, json } from '../api'
import { Dialog } from '../dialog'
import { Select, SelectItem } from '../select'
import { Button, EmptyState, Input, formatTime } from '../ui'
import type { Attendance, AttendancePageData, AttendanceRecord, AttendanceSuggestion, ClassItem, Notify } from '../types'

const statusLabels: Record<AttendanceRecord['status'], string> = { present: '已到', absent: '缺席', late: '迟到', leave: '请假' }

type Props = {
  classes: ClassItem[]
  classId: number
  setClassId: (id: number) => void
  notify: Notify
  onDataChange: () => Promise<void>
  initialSuggestion?: AttendanceSuggestion | null
  onSuggestionConsumed?: () => void
}

export function AttendancePage({ classes, classId, setClassId, notify, onDataChange, initialSuggestion, onSuggestionConsumed }: Props) {
  const [sessions, setSessions] = useState<Attendance[]>([])
  const [selectedId, setSelectedId] = useState(0)
  const [nextCursor, setNextCursor] = useState(0)
  const [startOpen, setStartOpen] = useState(false)
  const [suggestion, setSuggestion] = useState<AttendanceSuggestion | null>(null)
  const [deleting, setDeleting] = useState<Attendance | null>(null)
  const [purging, setPurging] = useState<Attendance | null>(null)
  const [busy, setBusy] = useState(false)
  const [filterClassId, setFilterClassId] = useState(classId)
  const [date, setDate] = useState('')
  const [trash, setTrash] = useState(false)
  useEffect(() => { if (classId) setFilterClassId(classId) }, [classId])
  const load = useCallback(async (cursor = 0, append = false) => {
    const params = new URLSearchParams({ limit: '30' })
    if (filterClassId) params.set('class_id', String(filterClassId))
    if (date) params.set('date', date)
    if (trash) params.set('trash', 'true')
    if (cursor) params.set('cursor', String(cursor))
    try {
      const page = await api<AttendancePageData>(`/attendance?${params}`)
      setSessions(current => append ? [...current, ...page.items] : page.items)
      setNextCursor(page.nextCursor)
      if (!append) setSelectedId(current => page.items.some(item => item.id === current) ? current : page.items[0]?.id || 0)
    }
    catch (error) { notify((error as Error).message, 'error') }
  }, [filterClassId, date, trash, notify])
  useEffect(() => { void load() }, [load])
  const selected = sessions.find(item => item.id === selectedId)
  useEffect(() => {
    if (!selectedId) return
    let cancelled = false
    api<Attendance>(`/attendance/${selectedId}`).then(detail => {
      if (!cancelled) setSessions(current => current.map(item => item.id === detail.id ? detail : item))
    }).catch(error => notify((error as Error).message, 'error'))
    return () => { cancelled = true }
  }, [selectedId, notify])
  useEffect(() => {
    if (selected?.status !== 'active' || trash) return
    const timer = window.setInterval(() => {
      api<Attendance>(`/attendance/${selected.id}`).then(next => setSessions(current => current.map(item => item.id === next.id ? next : item))).catch(error => notify((error as Error).message, 'error'))
    }, 5000)
    return () => window.clearInterval(timer)
  }, [selected?.id, selected?.status, trash, notify])

  useEffect(() => {
    if (!initialSuggestion) return
    setSuggestion(initialSuggestion)
    if (initialSuggestion.detected) {
      setFilterClassId(initialSuggestion.classId)
      setClassId(initialSuggestion.classId)
    }
    setStartOpen(true)
    onSuggestionConsumed?.()
  }, [initialSuggestion, onSuggestionConsumed, setClassId])

  const openStart = async () => {
    try {
      const detected = await api<AttendanceSuggestion>('/schedule/current')
      setSuggestion(detected)
      if (detected.detected) {
        setFilterClassId(detected.classId)
        setClassId(detected.classId)
      }
    } catch (error) {
      notify((error as Error).message, 'error')
      setSuggestion(null)
    }
    setStartOpen(true)
  }
  const updateSelected = (next: Attendance) => setSessions(current => current.map(item => item.id === next.id ? next : item))
  const start = async (input: { course: string; sessionAt: string }) => {
    const targetClassID = suggestion?.detected ? suggestion.classId : filterClassId
    setBusy(true)
    try {
      const next = await api<Attendance>('/attendance', json('POST', { classId: targetClassID, ...input }))
      setTrash(false); setFilterClassId(targetClassID); setSessions(current => [next, ...current]); setSelectedId(next.id); setStartOpen(false)
      notify('签到已开放，学生现在可以自助签到'); await onDataChange()
    }
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
      notify(`${deleting.title} 已移入回收站`)
      await Promise.all([load(), onDataChange()])
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const restore = async () => {
    if (!selected) return
    setBusy(true)
    try { await api(`/attendance/${selected.id}/restore`, json('POST')); notify('考勤记录已恢复'); await load() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const purge = async () => {
    if (!purging) return
    setBusy(true)
    try { await api(`/attendance/${purging.id}/permanent`, { method: 'DELETE' }); setPurging(null); notify('考勤记录已永久删除'); await load() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const setStatus = async (record: AttendanceRecord, status: AttendanceRecord['status']) => {
    if (!selected) return
    try { updateSelected(await api<Attendance>(`/attendance/${selected.id}/records/${record.studentId}`, json('PATCH', { status }))) }
    catch (error) { notify((error as Error).message, 'error') }
  }

  const hasAnyActive = classes.some(item => item.activeSessionId)
  const targetClassName = suggestion?.detected ? suggestion.className : classes.find(item => item.id === filterClassId)?.name || ''

  return <>
    <div className="page-heading">
      <div><h1>考勤管理</h1><p>按需加载历史明细，删除记录可从回收站恢复。</p></div>
      <div className="heading-actions">
        <Button variant="outline" onClick={() => setTrash(value => !value)}>{trash ? <RotateCcw size={15} /> : <ArchiveRestore size={15} />}{trash ? '返回考勤' : '回收站'}</Button>
        {!trash && <Button disabled={!classes.length || hasAnyActive} onClick={() => void openStart()}><Play size={15} />智能发起点名</Button>}
      </div>
    </div>
    <section className="panel filter-panel">
      <div className="filter-title"><ClipboardCheck size={16} /><div><strong>{trash ? '考勤回收站' : '考勤场次'}</strong><span>已加载 {sessions.length} 条</span></div></div>
      <div className="filter-controls attendance-filters">
        <Select value={filterClassId ? String(filterClassId) : 'all'} onValueChange={value => { const next = value === 'all' ? 0 : Number(value); setFilterClassId(next); if (next) setClassId(next) }} placeholder="选择班级"><SelectItem value="all">全部班级</SelectItem>{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select>
        <label className="date-filter"><CalendarDays size={15} /><Input type="date" value={date} onChange={event => setDate(event.target.value)} aria-label="按日期筛选" /></label>
        {sessions.length > 0 && <Select value={String(selectedId)} onValueChange={value => setSelectedId(Number(value))} placeholder="选择场次">{sessions.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.status === 'active' ? '进行中 · ' : ''}{item.title}</SelectItem>)}</Select>}
      </div>
    </section>
    {selected ? <>
      <section className="attendance-summary">
        <div><span className="summary-icon green"><UserCheck size={18} /></span><div><span>已到 / 迟到</span><strong>{selected.presentCount}</strong></div></div>
        <div><span className="summary-icon red"><UserX size={18} /></span><div><span>当前缺席</span><strong>{selected.absentCount}</strong></div></div>
        <div><span className="summary-icon"><Clock3 size={18} /></span><div><span>上课时间</span><strong className="summary-time">{formatTime(selected.sessionAt)}</strong></div></div>
        <div className="summary-session"><span className={`status ${selected.status === 'active' ? 'status-active' : ''}`}>{trash ? '回收站' : selected.status === 'active' ? '签到进行中' : '已结束'}</span>
          {trash ? <><Button variant="outline" size="sm" disabled={busy} onClick={() => void restore()}><RotateCcw size={13} />恢复</Button><Button variant="danger" size="sm" disabled={busy} onClick={() => setPurging(selected)}><Trash2 size={13} />永久删除</Button></> : <>{selected.status === 'active' && <Button variant="outline" size="sm" disabled={busy} onClick={() => void close()}><Square size={13} />结束点名</Button>}<Button variant="danger" size="sm" disabled={busy} onClick={() => setDeleting(selected)}><Trash2 size={13} />删除记录</Button></>}</div>
      </section>
      <section className="panel table-panel">
        <div className="panel-header"><div><h2>{selected.title}</h2><p>{selected.className} · {selected.course} · {selected.records.length || selected.presentCount + selected.absentCount} 名学生</p></div>{selected.status === 'active' && !trash && <span className="live-indicator"><i />每 5 秒自动刷新</span>}</div>
        <div className="table-scroll"><table><thead><tr><th>学号</th><th>姓名</th><th>状态</th><th>签到时间</th><th>签到方式</th><th className="cell-action">修正状态</th></tr></thead><tbody>{selected.records.map(record => <tr key={record.studentId}><td className="mono">{record.studentNo}</td><td><strong>{record.name}</strong></td><td><span className={`attendance-status attendance-${record.status}`}>{statusLabels[record.status]}</span></td><td>{formatTime(record.checkedAt)}</td><td>{record.method === 'self' ? '学生自助' : record.method === 'teacher' ? '教师修正' : '—'}</td><td className="cell-action">{trash ? '—' : <Select value={record.status} onValueChange={value => void setStatus(record, value as AttendanceRecord['status'])} className="status-select"><SelectItem value="present">已到</SelectItem><SelectItem value="late">迟到</SelectItem><SelectItem value="leave">请假</SelectItem><SelectItem value="absent">缺席</SelectItem></Select>}</td></tr>)}</tbody></table></div>
      </section>
      {nextCursor > 0 && <div className="load-more"><Button variant="outline" onClick={() => void load(nextCursor, true)}>加载更多历史记录</Button></div>}
    </> : <section className="panel"><EmptyState icon={<ClipboardCheck size={22} />} title={trash ? '回收站为空' : '没有符合条件的考勤记录'} detail={trash ? '删除的考勤记录会出现在这里。' : '可以智能识别当前课时发起点名，或清除日期筛选。'} action={!trash && classes.length ? <Button onClick={() => void openStart()}><Play size={15} />智能发起点名</Button> : undefined} /></section>}
    <StartDialog open={startOpen} busy={busy} onOpenChange={setStartOpen} onStart={start} className={targetClassName} suggestion={suggestion} />
    <Dialog open={!!deleting} onOpenChange={open => !open && !busy && setDeleting(null)} title="删除考勤记录" description="记录会进入回收站，之后可以恢复。" footer={<><Button variant="outline" disabled={busy} onClick={() => setDeleting(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}><Trash2 size={14} />{busy ? '正在删除' : '移入回收站'}</Button></>}><div className="danger-box">确定删除 <strong>{deleting?.title}</strong> 吗？{deleting?.status === 'active' && <span>删除后该班将立即停止签到。</span>}</div></Dialog>
    <Dialog open={!!purging} onOpenChange={open => !open && !busy && setPurging(null)} title="永久删除考勤记录" description="此操作会清除场次和全部考勤明细，无法恢复。" footer={<><Button variant="outline" disabled={busy} onClick={() => setPurging(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void purge()}><Trash2 size={14} />确认永久删除</Button></>}><div className="danger-box">永久删除 <strong>{purging?.title}</strong>？建议先下载数据库备份。</div></Dialog>
  </>
}

function StartDialog({ open, busy, onOpenChange, onStart, className, suggestion }: { open: boolean; busy: boolean; onOpenChange: (v: boolean) => void; onStart: (input: { course: string; sessionAt: string }) => Promise<void>; className: string; suggestion: AttendanceSuggestion | null }) {
  const [course, setCourse] = useState('信息课')
  const [sessionAt, setSessionAt] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  useEffect(() => {
    if (!open) return
    const now = new Date()
    now.setMinutes(now.getMinutes() - now.getTimezoneOffset())
    setCourse(suggestion?.detected ? suggestion.course : '信息课')
    setSessionAt(suggestion?.sessionAt || now.toISOString().slice(0, 16))
    setConfirmed(false)
  }, [open, suggestion])
  const generatedTitle = sessionAt ? `${className} · ${sessionAt.replace('T', ' ')}` : `${className} · 日期 时间`
  return <Dialog open={open} onOpenChange={onOpenChange} title="确认课堂点名" description={suggestion?.message || '请人工核对班级、课程和上课时间。'} footer={<><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={busy || !confirmed || !className || !course.trim() || !sessionAt} onClick={() => void onStart({ course: course.trim(), sessionAt })}><Play size={15} />确认并开放签到</Button></>}>
    <div className="form-stack">
      <div className="class-name-preview"><span>场次名称</span><strong>{generatedTitle}</strong></div>
      {suggestion?.detected && <div className="info-box"><Clock3 size={17} /><div><strong>服务器识别：第 {suggestion.period} 节 · {suggestion.startTime}-{suggestion.endTime}</strong><span>{suggestion.source === 'rescheduled' ? '已应用临时换课记录' : '来自本学期常规课表'} · 仍需教师确认</span></div></div>}
      <div className="class-fields"><div className="field"><label htmlFor="attendance-course">课程</label><Input id="attendance-course" value={course} onChange={e => setCourse(e.target.value)} /></div><div className="field"><label htmlFor="attendance-date">上课日期时间</label><Input id="attendance-date" type="datetime-local" value={sessionAt} onChange={e => setSessionAt(e.target.value)} /></div></div>
      <label className="confirm-check"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} /><span><strong>我已人工核对以上信息</strong><small>确认后会立即锁定当前班级名单并开放学生签到。</small></span></label>
      <div className="info-box"><CheckCircle2 size={17} /><div><strong>历史名单使用快照保存</strong><span>之后修改或删除学生，不会改变本次考勤中的姓名和学号。</span></div></div>
    </div>
  </Dialog>
}
