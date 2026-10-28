import type { ScheduleChange, ScheduleData, ScheduleLesson, SchedulePeriod, ScheduleSettings } from './types'

export type Occurrence = {
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

const pad = (value: number) => String(value).padStart(2, '0')
export const dateKey = (date: Date) => `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
export const dateAt = (date: string, time = '12:00') => new Date(`${date}T${time}:00`)
export const weekdayOf = (date: Date) => ((date.getDay() + 6) % 7) + 1

export function semesterWeek(settings: ScheduleSettings, date: string) {
  if (!settings.semesterStart || date < settings.semesterStart || date > settings.semesterEnd) return undefined
  return Math.floor((dateAt(date).getTime() - dateAt(settings.semesterStart).getTime()) / 604800000) + 1
}

export function lessonLocation(lesson: ScheduleLesson, week?: number) {
  return week && week % 2 === 0 ? lesson.locationEven : lesson.locationOdd
}

export function buildOccurrences(data: ScheduleData, from: Date, days: number) {
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

/** 本周周一 00:00。周日归属前一个周一至周五。 */
export function mondayOf(date: Date) {
  const start = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  start.setDate(start.getDate() - (weekdayOf(start) - 1))
  return start
}

const minutesOf = (time: string) => {
  const [hour, minute] = time.split(':').map(Number)
  return (Number.isFinite(hour) ? hour : 0) * 60 + (Number.isFinite(minute) ? minute : 0)
}

export function periodForTime(periods: SchedulePeriod[], time: string) {
  return periods.find(period => period.startTime === time)?.period
}

/** 手工填写的换课时间可能不在整点上，落格时取开始时间最近的一节。 */
export function nearestPeriod(periods: SchedulePeriod[], time: string) {
  let best: SchedulePeriod | undefined
  let distance = Number.POSITIVE_INFINITY
  const target = minutesOf(time)
  for (const period of periods) {
    const current = Math.abs(minutesOf(period.startTime) - target)
    if (current < distance) { best = period; distance = current }
  }
  return best?.period
}

export type WeekDay = { date: string; weekday: number; week?: number; inSemester: boolean }
export type WeekCellState = 'lesson' | 'moved' | 'occupied' | 'empty'
export type WeekCell = { date: string; weekday: number; period: number; state: WeekCellState; occurrence?: Occurrence }
export type WeekGrid = { days: WeekDay[]; periods: SchedulePeriod[]; cells: WeekCell[] }

export const weekCellKey = (date: string, period: number) => `${date}-${period}`

export function findWeekCell(grid: WeekGrid, target: { date: string; period: number }) {
  return grid.cells.find(cell => cell.date === target.date && cell.period === target.period)
}

export function buildWeekGrid(data: ScheduleData, monday: Date): WeekGrid {
  const days: WeekDay[] = []
  for (let offset = 0; offset < 5; offset++) {
    const day = new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + offset)
    const date = dateKey(day)
    const inSemester = !data.settings.semesterStart || (date >= data.settings.semesterStart && date <= data.settings.semesterEnd)
    days.push({ date, weekday: weekdayOf(day), week: semesterWeek(data.settings, date), inSemester })
  }
  const periods = data.settings.periods
  const cells: WeekCell[] = []
  const bySlot = new Map<string, Occurrence>()
  for (const occurrence of buildOccurrences(data, monday, 5)) {
    // 已经调到别处的课程，原时段留空；被占课仍然占用原时段。
    if (occurrence.change?.status === 'rescheduled' && !occurrence.movedHere) continue
    const period = periodForTime(periods, occurrence.startTime) ?? nearestPeriod(periods, occurrence.startTime)
    if (!period) continue
    const key = weekCellKey(occurrence.date, period)
    if (!bySlot.has(key)) bySlot.set(key, occurrence)
  }
  for (const day of days) {
    for (const period of periods) {
      const occurrence = bySlot.get(weekCellKey(day.date, period.period))
      const state: WeekCellState = !occurrence
        ? 'empty'
        : occurrence.change?.status === 'occupied' ? 'occupied' : occurrence.movedHere ? 'moved' : 'lesson'
      cells.push({ date: day.date, weekday: day.weekday, period: period.period, state, occurrence })
    }
  }
  return { days, periods, cells }
}

export type ChangeRequest = {
  lessonId: number
  date: string
  status: 'rescheduled'
  newDate: string
  newStartTime: string
  newEndTime: string
  newClassId: number
  note: string
}
export type DropPlan = { kind: 'none' | 'move' | 'swap'; reason?: string; changes: ChangeRequest[] }

const originalDateOf = (occurrence: Occurrence) => occurrence.change?.date ?? occurrence.date

/**
 * 拖动结果：拖到空格子＝当天临时调课；拖到有课格子＝两节课当天互换。
 * 请求统一以课程原始日期为键，重复拖动只更新同一条变动记录。
 */
export function planDrop(grid: WeekGrid, from: { date: string; period: number }, to: { date: string; period: number }): DropPlan {
  if (from.date === to.date && from.period === to.period) return { kind: 'none', changes: [] }
  const source = findWeekCell(grid, from)
  const target = findWeekCell(grid, to)
  if (!source?.occurrence || (source.state !== 'lesson' && source.state !== 'moved')) {
    return { kind: 'none', reason: '该格子没有可移动的课程', changes: [] }
  }
  if (target?.state === 'occupied') return { kind: 'none', reason: '该时段已被占课，请先撤销占课标记', changes: [] }
  const sourcePeriod = grid.periods.find(period => period.period === from.period)
  const targetPeriod = grid.periods.find(period => period.period === to.period)
  if (!sourcePeriod || !targetPeriod) return { kind: 'none', reason: '节次不存在', changes: [] }
  const moveTo = (occurrence: Occurrence, date: string, period: SchedulePeriod): ChangeRequest => ({
    lessonId: occurrence.lesson.id,
    date: originalDateOf(occurrence),
    status: 'rescheduled',
    newDate: date,
    newStartTime: period.startTime,
    newEndTime: period.endTime,
    newClassId: occurrence.classId,
    note: '',
  })
  const changes = [moveTo(source.occurrence, to.date, targetPeriod)]
  if (target?.occurrence) {
    const sameOccurrence = target.occurrence.lesson.id === source.occurrence.lesson.id
      && originalDateOf(target.occurrence) === originalDateOf(source.occurrence)
    if (sameOccurrence) return { kind: 'none', changes: [] }
    changes.push(moveTo(target.occurrence, from.date, sourcePeriod))
  }
  return { kind: target?.occurrence ? 'swap' : 'move', changes }
}

export function changeNote(kind: 'move' | 'swap') {
  return kind === 'swap' ? '拖动互换' : '拖动调课'
}
