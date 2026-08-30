import { useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import {
  ArrowLeft, ArrowLeftRight, CalendarClock, CalendarDays, ChevronDown, FileSpreadsheet,
  MapPin, Pencil, Plus, Settings2, Trash2, UserRoundX, X,
} from 'lucide-react'
import { api, json } from './api'
import { Dialog } from './dialog'
import { Select, SelectItem } from './select'
import { Button, Input } from './ui'
import type { ClassItem, Notify, ScheduleChange, ScheduleData, ScheduleLesson, SchedulePeriod, ScheduleSettings } from './types'

const weekdays = ['周一', '周二', '周三', '周四', '周五']
const locations = ['机房 1', '机房 2', '教室']
const defaultPeriods: SchedulePeriod[] = [
  ['08:00', '08:40'], ['08:50', '09:30'], ['09:40', '10:20'],
  ['13:30', '14:10'], ['14:20', '15:00'], ['15:10', '15:50'], ['16:00', '16:40'],
].map(([startTime, endTime], index) => ({ period: index + 1, startTime, endTime }))
const emptyData: ScheduleData = { lessons: [], changes: [], settings: { semesterStart: '', semesterEnd: '', periods: defaultPeriods } }
const emptyLessonForm = { classId: '', course: '信息科技', locationOdd: '机房 1', locationEven: '机房 1' }

type Occurrence = {
  key: string
  lesson: ScheduleLesson
  date: string
  startTime: string
  endTime: string
  classId: number
  className: string
  location: string
  week?: number
  change?: ScheduleChange
  movedHere?: boolean
}

type EditingCell = { weekday: number; period: number; lesson?: ScheduleLesson }

const pad = (value: number) => String(value).padStart(2, '0')
const dateKey = (date: Date) => `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
const dateAt = (date: string, time = '12:00') => new Date(`${date}T${time}:00`)
const weekdayOf = (date: Date) => ((date.getDay() + 6) % 7) + 1
const shortDate = (date: string) => date ? date.replaceAll('-', '.') : ''

function semesterWeek(settings: ScheduleSettings, date: string) {
  if (!settings.semesterStart || date < settings.semesterStart || date > settings.semesterEnd) return undefined
  return Math.floor((dateAt(date).getTime() - dateAt(settings.semesterStart).getTime()) / 604800000) + 1
}

function lessonLocation(lesson: ScheduleLesson, week?: number) {
  return week && week % 2 === 0 ? lesson.locationEven : lesson.locationOdd
}

function buildOccurrences(data: ScheduleData, from: Date, days: number) {
  const occurrences: Occurrence[] = []
  const changes = new Map(data.changes.map(change => [`${change.lessonId}-${change.date}`, change]))
  const lessons = new Map<number, ScheduleLesson[]>()
  const lessonsByID = new Map(data.lessons.map(lesson => [lesson.id, lesson]))
  data.lessons.forEach(lesson => {
    const day = lessons.get(lesson.weekday) || []
    day.push(lesson); lessons.set(lesson.weekday, day)
  })
  for (let offset = 0; offset < days; offset++) {
    const day = new Date(from.getFullYear(), from.getMonth(), from.getDate() + offset)
    const date = dateKey(day)
    if (data.settings.semesterStart && (date < data.settings.semesterStart || date > data.settings.semesterEnd)) continue
    for (const lesson of lessons.get(weekdayOf(day)) || []) {
      const week = semesterWeek(data.settings, date)
      occurrences.push({
        key: `${lesson.id}-${date}`, lesson, date, startTime: lesson.startTime, endTime: lesson.endTime,
        classId: lesson.classId, className: lesson.className, location: lessonLocation(lesson, week), week,
        change: changes.get(`${lesson.id}-${date}`),
      })
    }
  }
  for (const change of data.changes.filter(item => item.status === 'rescheduled' && item.newDate)) {
    const lesson = lessonsByID.get(change.lessonId)
    if (!lesson) continue
    const rangeStart = new Date(from.getFullYear(), from.getMonth(), from.getDate())
    const target = dateAt(change.newDate, change.newStartTime)
    if (target < rangeStart || target >= new Date(rangeStart.getFullYear(), rangeStart.getMonth(), rangeStart.getDate() + days)) continue
    const week = semesterWeek(data.settings, change.newDate)
    occurrences.push({
      key: `${lesson.id}-${change.date}-moved`, lesson, date: change.newDate,
      startTime: change.newStartTime, endTime: change.newEndTime,
      classId: change.newClassId || lesson.classId, className: change.newClassName || lesson.className,
      location: lessonLocation(lesson, week), week, change, movedHere: true,
    })
  }
  return occurrences.sort((a, b) => `${a.date} ${a.startTime}`.localeCompare(`${b.date} ${b.startTime}`))
}

function relativeLabel(target: Date, now: Date) {
  const minutes = Math.max(0, Math.ceil((target.getTime() - now.getTime()) / 60000))
  if (minutes < 60) return `${minutes} 分钟后`
  if (minutes < 24 * 60) return `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分后`
  const days = Math.round((dateAt(dateKey(target)).getTime() - dateAt(dateKey(now)).getTime()) / 86400000)
  return days === 1 ? '明天' : weekdays[weekdayOf(target) - 1] || target.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric' })
}

export function ScheduleWidget({ classes, notify }: { classes: ClassItem[]; notify: Notify }) {
  const [data, setData] = useState<ScheduleData>(emptyData)
  const [collapsed, setCollapsed] = useState(() => (localStorage.getItem('classorbit-schedule-collapsed') || localStorage.getItem('classpoint-schedule-collapsed')) !== 'false')
  const [manageOpen, setManageOpen] = useState(false)
  const [configOpen, setConfigOpen] = useState(false)
  const [editing, setEditing] = useState<EditingCell | null>(null)
  const [lessonForm, setLessonForm] = useState(emptyLessonForm)
  const [settingsDraft, setSettingsDraft] = useState<ScheduleSettings>(emptyData.settings)
  const [changeTarget, setChangeTarget] = useState<Occurrence | null>(null)
  const [changeMode, setChangeMode] = useState<'occupied' | 'rescheduled'>('occupied')
  const [changeForm, setChangeForm] = useState({ newDate: '', newStartTime: '', newEndTime: '', newClassId: '', note: '' })
  const [busy, setBusy] = useState(false)
  const [now, setNow] = useState(() => new Date())
  const fileRef = useRef<HTMLInputElement>(null)

  const refresh = async () => {
    try {
      const next = await api<ScheduleData>('/schedule')
      setData(next)
      setSettingsDraft(next.settings)
    } catch (error) { notify((error as Error).message, 'error') }
  }
  useEffect(() => { void refresh() }, [])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 30000)
    return () => window.clearInterval(timer)
  }, [])
  useEffect(() => { localStorage.setItem('classorbit-schedule-collapsed', String(collapsed)) }, [collapsed])

  const today = dateKey(now)
  const occurrences = useMemo(() => buildOccurrences(data, dateAt(today), 28), [data, today])
  const lessonsByCell = useMemo(() => new Map(data.lessons.map(lesson => [`${lesson.weekday}-${lesson.period}`, lesson])), [data.lessons])
  const todayItems = occurrences.filter(item => item.date === today)
  const available = occurrences.filter(item => (!item.change || item.movedHere) && dateAt(item.date, item.endTime) > now)
  const current = available.find(item => dateAt(item.date, item.startTime) <= now && dateAt(item.date, item.endTime) > now)
  const next = current || available.find(item => dateAt(item.date, item.startTime) >= now)
  const currentWeek = semesterWeek(data.settings, today)

  const openManager = () => { setEditing(null); setConfigOpen(false); setManageOpen(true) }
  const openCell = (weekday: number, period: number, lesson?: ScheduleLesson) => {
    setEditing({ weekday, period, lesson })
    setLessonForm(lesson ? { classId: String(lesson.classId), course: lesson.course, locationOdd: lesson.locationOdd, locationEven: lesson.locationEven } : { ...emptyLessonForm, classId: classes[0] ? String(classes[0].id) : '' })
  }

  const saveLesson = async () => {
    if (!editing || !lessonForm.classId || !lessonForm.course.trim()) return
    setBusy(true)
    try {
      const body = { ...lessonForm, classId: Number(lessonForm.classId), course: lessonForm.course.trim(), weekday: editing.weekday, period: editing.period }
      if (editing.lesson) await api(`/schedule/${editing.lesson.id}`, json('PATCH', body))
      else await api('/schedule', json('POST', body))
      await refresh(); setEditing(null); notify(editing.lesson ? '课程已更新' : '课程已添加')
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }

  const deleteLesson = async () => {
    if (!editing?.lesson || !window.confirm(`确认删除 ${weekdays[editing.weekday - 1]}第 ${editing.period} 节课程？`)) return
    try { await api(`/schedule/${editing.lesson.id}`, { method: 'DELETE' }); await refresh(); setEditing(null); notify('课程已删除') }
    catch (error) { notify((error as Error).message, 'error') }
  }

  const saveSettings = async () => {
    setBusy(true)
    try {
      const settings = await api<ScheduleSettings>('/schedule/settings', json('PUT', settingsDraft))
      setData(currentData => ({ ...currentData, settings, lessons: currentData.lessons.map(lesson => {
        const period = settings.periods.find(item => item.period === lesson.period)
        return period ? { ...lesson, startTime: period.startTime, endTime: period.endTime } : lesson
      }) }))
      setSettingsDraft(settings); setConfigOpen(false); notify('本学期校历与节次已保存')
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }

  const openChange = (item: Occurrence, mode: 'occupied' | 'rescheduled') => {
    const nextDay = dateAt(item.date, item.startTime); nextDay.setDate(nextDay.getDate() + 1)
    setChangeTarget(item); setChangeMode(mode)
    setChangeForm({ newDate: dateKey(nextDay), newStartTime: item.startTime, newEndTime: item.endTime, newClassId: String(item.classId), note: '' })
  }

  const saveChange = async () => {
    if (!changeTarget) return
    setBusy(true)
    try {
      const body = changeMode === 'occupied'
        ? { date: changeTarget.change?.date || changeTarget.date, status: 'occupied', note: changeForm.note }
        : { date: changeTarget.change?.date || changeTarget.date, status: 'rescheduled', ...changeForm, newClassId: Number(changeForm.newClassId) || 0 }
      await api(`/schedule/${changeTarget.lesson.id}/changes`, json('PUT', body))
      await refresh(); setChangeTarget(null); notify(changeMode === 'occupied' ? '占课标记已保存' : '换课安排已保存')
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }

  const undoChange = async (item: Occurrence) => {
    try {
      await api(`/schedule/${item.lesson.id}/changes?date=${item.change?.date || item.date}`, { method: 'DELETE' })
      await refresh(); notify('已恢复原课程')
    } catch (error) { notify((error as Error).message, 'error') }
  }

  const importFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    const body = new FormData(); body.append('file', file); setBusy(true)
    try {
      const result = await api<{ added: number; skipped: number }>('/schedule/import', { method: 'POST', body })
      await refresh(); notify(`已导入 ${result.added} 节课程${result.skipped ? `，跳过 ${result.skipped} 个已占用格子` : ''}`)
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false); event.target.value = '' }
  }

  const nextDateLabel = next ? (next.date === today ? '今天' : next.date === dateKey(new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)) ? '明天' : weekdays[weekdayOf(dateAt(next.date)) - 1] || next.date.slice(5)) : ''
  const editPeriod = data.settings.periods.find(item => item.period === editing?.period)

  return <>
    {collapsed ? <button className="schedule-fab" title="展开课程提醒" aria-label="展开课程提醒" onClick={() => setCollapsed(false)}><CalendarClock size={22} />{next && <span />}</button> : <aside className="schedule-widget" aria-label="课程提醒">
      <header className="schedule-head">
        <button className="schedule-head-toggle" onClick={() => setCollapsed(true)} title="收起课程提醒"><span className="schedule-head-icon"><CalendarClock size={17} /></span><div><strong>课程提醒</strong><small>{currentWeek ? `第 ${currentWeek} 周 · ${currentWeek % 2 ? '单周' : '双周'}` : `${now.getMonth() + 1} 月 ${now.getDate()} 日`}</small></div></button>
        <div><Button variant="ghost" size="icon" title="管理课表" aria-label="管理课表" onClick={openManager}><CalendarDays size={16} /></Button><Button variant="ghost" size="icon" title="收起" aria-label="折叠课程提醒" onClick={() => setCollapsed(true)}><ChevronDown size={17} /></Button></div>
      </header>
      <div className="schedule-body">
        {next ? <section className={`schedule-next ${current ? 'schedule-current' : ''}`}>
          <div className="schedule-next-label"><span>{current ? '正在上课' : '下节课'}</span><strong>{current ? `${next.endTime} 下课` : relativeLabel(dateAt(next.date, next.startTime), now)}</strong></div>
          <div className="schedule-next-main"><div><strong>{next.className}</strong><span>{next.lesson.course}{next.movedHere ? ' · 换课' : ''}</span><small><MapPin size={11} />{next.location}</small></div><time>{nextDateLabel}<b>{next.startTime}</b></time></div>
        </section> : <button className="schedule-empty" onClick={openManager}><CalendarDays size={22} /><span><strong>{data.lessons.length ? '近期没有课程' : '录入本学期课表'}</strong><small>{data.settings.semesterEnd && today > data.settings.semesterEnd ? '本学期已经结束' : '设置校历后即可获得课程提醒'}</small></span></button>}
        <div className="schedule-today-title"><strong>今天</strong><span>{todayItems.length} 节课</span></div>
        <div className="schedule-timeline">
          {todayItems.map(item => {
            const changedAway = item.change?.status === 'rescheduled' && !item.movedHere
            const occupied = item.change?.status === 'occupied'
            return <div className={`schedule-row ${occupied || changedAway ? 'schedule-row-muted' : ''}`} key={item.key}>
              <time>{item.startTime}<small>{item.endTime}</small></time><i />
              <div className="schedule-row-copy"><strong>{item.className}</strong><span>{item.lesson.course} · {item.location}{occupied ? ' · 被占课' : changedAway ? ` · 换至 ${item.change?.newDate.slice(5)} ${item.change?.newStartTime}` : item.movedHere ? ' · 换课' : ''}</span>{item.change?.note && <small>{item.change.note}</small>}</div>
              <div className="schedule-row-actions">{item.change ? <Button variant="ghost" size="icon" title="撤销变动" aria-label="撤销变动" onClick={() => void undoChange(item)}><X size={14} /></Button> : <><Button variant="ghost" size="icon" title="标记占课" aria-label="标记占课" onClick={() => openChange(item, 'occupied')}><UserRoundX size={14} /></Button><Button variant="ghost" size="icon" title="标记换课" aria-label="标记换课" onClick={() => openChange(item, 'rescheduled')}><ArrowLeftRight size={14} /></Button></>}</div>
            </div>
          })}
          {todayItems.length === 0 && <p className="schedule-no-today">今天没有安排课程</p>}
        </div>
      </div>
      <footer className="schedule-footer"><button onClick={openManager}><CalendarDays size={14} />本学期课表</button></footer>
    </aside>}

    {manageOpen && <Dialog open onOpenChange={setManageOpen} title="本学期课表" description="点击课表格子添加或编辑课程，周一至周五共 7 节课。" width="schedule">
      {editing ? <div className="schedule-form form-stack">
        <button className="schedule-back" onClick={() => setEditing(null)}><ArrowLeft size={14} />返回课表</button>
        <div className="schedule-cell-summary"><span>{weekdays[editing.weekday - 1]} · 第 {editing.period} 节</span><strong>{editPeriod?.startTime} - {editPeriod?.endTime}</strong></div>
        <div className="class-fields"><div className="field"><label>班级</label><Select value={lessonForm.classId || undefined} onValueChange={value => setLessonForm(currentForm => ({ ...currentForm, classId: value }))} placeholder="选择班级">{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select></div><div className="field"><label>课程</label><Input value={lessonForm.course} maxLength={20} onChange={event => setLessonForm(currentForm => ({ ...currentForm, course: event.target.value }))} /></div></div>
        <div className="class-fields"><div className="field"><label>单周上课地点</label><Select value={lessonForm.locationOdd} onValueChange={value => setLessonForm(currentForm => ({ ...currentForm, locationOdd: value }))}>{locations.map(item => <SelectItem key={item} value={item}>{item}</SelectItem>)}</Select></div><div className="field"><label>双周上课地点</label><Select value={lessonForm.locationEven} onValueChange={value => setLessonForm(currentForm => ({ ...currentForm, locationEven: value }))}>{locations.map(item => <SelectItem key={item} value={item}>{item}</SelectItem>)}</Select></div></div>
        <div className="schedule-form-actions">{editing.lesson && <Button variant="danger" onClick={() => void deleteLesson()}><Trash2 size={14} />删除课程</Button>}<span /><Button variant="outline" onClick={() => setEditing(null)}>取消</Button><Button onClick={() => void saveLesson()} disabled={busy || !lessonForm.classId || !lessonForm.course.trim()}>{busy ? '保存中' : '保存课程'}</Button></div>
      </div> : <>
        <div className="schedule-calendar-bar">
          <div><span className="schedule-calendar-icon"><CalendarDays size={17} /></span><span><strong>{data.settings.semesterStart ? `${shortDate(data.settings.semesterStart)} - ${shortDate(data.settings.semesterEnd)}` : '尚未设置本学期校历'}</strong><small>{currentWeek ? `当前第 ${currentWeek} 周 · ${currentWeek % 2 ? '单周' : '双周'}` : '校历决定周次、单双周地点和提醒范围'}</small></span></div>
          <div><Button variant="outline" size="sm" onClick={() => setConfigOpen(open => !open)}><Settings2 size={14} />校历与节次</Button><Button variant="outline" size="sm" disabled={busy} onClick={() => fileRef.current?.click()}><FileSpreadsheet size={14} />Excel 导入</Button><input ref={fileRef} type="file" accept=".xlsx,.xlsm" onChange={event => void importFile(event)} /></div>
        </div>
        {configOpen && <div className="schedule-config">
          <div className="schedule-config-dates"><div><strong>本学期校历</strong><small>起始日作为第 1 周的第一天</small></div><div className="field"><label>开始日期</label><Input type="date" value={settingsDraft.semesterStart} onChange={event => setSettingsDraft(current => ({ ...current, semesterStart: event.target.value }))} /></div><div className="field"><label>结束日期</label><Input type="date" value={settingsDraft.semesterEnd} onChange={event => setSettingsDraft(current => ({ ...current, semesterEnd: event.target.value }))} /></div></div>
          <div className="schedule-period-settings"><div><strong>每日节次</strong><small>上午 3 节 · 下午 4 节</small></div>{settingsDraft.periods.map((period, index) => <div className="schedule-period-setting" key={period.period}><span>第 {period.period} 节</span><Input aria-label={`第 ${period.period} 节开始时间`} type="time" value={period.startTime} onChange={event => setSettingsDraft(current => ({ ...current, periods: current.periods.map((item, itemIndex) => itemIndex === index ? { ...item, startTime: event.target.value } : item) }))} /><i>至</i><Input aria-label={`第 ${period.period} 节结束时间`} type="time" value={period.endTime} onChange={event => setSettingsDraft(current => ({ ...current, periods: current.periods.map((item, itemIndex) => itemIndex === index ? { ...item, endTime: event.target.value } : item) }))} /></div>)}</div>
          <div className="schedule-config-actions"><Button variant="outline" onClick={() => { setSettingsDraft(data.settings); setConfigOpen(false) }}>取消</Button><Button disabled={busy || !settingsDraft.semesterStart || !settingsDraft.semesterEnd} onClick={() => void saveSettings()}>{busy ? '保存中' : '保存设置'}</Button></div>
        </div>}
        <div className="schedule-grid-scroll"><div className="schedule-grid">
          <div className="schedule-grid-corner"><span>节次</span><small>时间</small></div>{weekdays.map(day => <div className="schedule-grid-day" key={day}>{day}</div>)}
          {data.settings.periods.map(period => <div className="schedule-grid-row" key={period.period}>
            <div className="schedule-period-cell"><strong>第 {period.period} 节</strong><span>{period.period <= 3 ? '上午' : '下午'}</span><small>{period.startTime} - {period.endTime}</small></div>
            {weekdays.map((_, dayIndex) => {
              const lesson = lessonsByCell.get(`${dayIndex + 1}-${period.period}`)
              return <button className={`schedule-course-cell ${lesson ? 'has-course' : ''}`} key={dayIndex} onClick={() => openCell(dayIndex + 1, period.period, lesson)}>{lesson ? <><strong>{lesson.className}</strong><span>{lesson.course}</span><small><MapPin size={11} />{lesson.locationOdd === lesson.locationEven ? lesson.locationOdd : `单 ${lesson.locationOdd} · 双 ${lesson.locationEven}`}</small><Pencil size={12} /></> : <><Plus size={15} /><span>添加课程</span></>}</button>
            })}
          </div>)}
        </div></div>
      </>}
    </Dialog>}

    {changeTarget && <Dialog open onOpenChange={open => { if (!open) setChangeTarget(null) }} title="标记课程变动" description={`${changeTarget.date} ${changeTarget.startTime} · ${changeTarget.className}`} footer={<><Button variant="outline" onClick={() => setChangeTarget(null)}>取消</Button><Button disabled={busy || (changeMode === 'rescheduled' && !changeForm.newDate)} onClick={() => void saveChange()}>{changeMode === 'occupied' ? <UserRoundX size={15} /> : <ArrowLeftRight size={15} />}{changeMode === 'occupied' ? '保存占课标记' : '确认换课'}</Button></>}>
      <div className="form-stack"><div className="segment segment-full"><button className={changeMode === 'occupied' ? 'active' : ''} onClick={() => setChangeMode('occupied')}><UserRoundX size={14} />被占课</button><button className={changeMode === 'rescheduled' ? 'active' : ''} onClick={() => setChangeMode('rescheduled')}><ArrowLeftRight size={14} />换课</button></div>{changeMode === 'rescheduled' && <><div className="field"><label>新的日期</label><Input type="date" value={changeForm.newDate} onChange={event => setChangeForm(current => ({ ...current, newDate: event.target.value }))} /></div><div className="class-fields"><div className="field"><label>上课</label><Input type="time" value={changeForm.newStartTime} onChange={event => setChangeForm(current => ({ ...current, newStartTime: event.target.value }))} /></div><div className="field"><label>下课</label><Input type="time" value={changeForm.newEndTime} onChange={event => setChangeForm(current => ({ ...current, newEndTime: event.target.value }))} /></div></div><div className="field"><label>班级 <span>可同时换班</span></label><Select value={changeForm.newClassId || undefined} onValueChange={value => setChangeForm(current => ({ ...current, newClassId: value }))} placeholder="选择班级">{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select></div></>}<div className="field"><label>备注 <span>选填</span></label><Input maxLength={50} placeholder={changeMode === 'occupied' ? '例如：王老师临时使用教室' : '例如：与王老师调课'} value={changeForm.note} onChange={event => setChangeForm(current => ({ ...current, note: event.target.value }))} /></div></div>
    </Dialog>}
  </>
}
