import { describe, expect, it } from 'vitest'
import { buildWeekGrid, changeNote, findWeekCell, mondayOf, nearestPeriod, periodForTime, planDrop } from './scheduleModel'
import type { ScheduleData } from './types'

const periods = [
  { period: 1, startTime: '08:00', endTime: '08:40' },
  { period: 2, startTime: '08:50', endTime: '09:30' },
  { period: 3, startTime: '09:40', endTime: '10:20' },
]

const data: ScheduleData = {
  settings: { semesterStart: '2026-09-07', semesterEnd: '2027-01-20', periods },
  lessons: [
    { id: 1, classId: 10, className: '三 1 班', course: '信息科技', weekday: 1, period: 1, startTime: '08:00', endTime: '08:40', locationOdd: '机房 1', locationEven: '机房 2' },
    { id: 2, classId: 20, className: '四 2 班', course: '人工智能', weekday: 2, period: 2, startTime: '08:50', endTime: '09:30', locationOdd: '教室', locationEven: '教室' },
  ],
  changes: [],
}

// 2026-09-07 是周一
const monday = new Date(2026, 8, 7)
const at = (date: string, period: number) => ({ date, period })

describe('week grid', () => {
  it('derives the Monday of the week, keeping Sunday in the previous week', () => {
    const key = (value: Date) => `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`
    expect(key(mondayOf(new Date(2026, 8, 9)))).toBe('2026-09-07')
    expect(key(mondayOf(new Date(2026, 8, 7)))).toBe('2026-09-07')
    expect(key(mondayOf(new Date(2026, 8, 13)))).toBe('2026-09-07')
  })

  it('maps clock times onto the nearest period', () => {
    expect(periodForTime(periods, '08:50')).toBe(2)
    expect(periodForTime(periods, '08:55')).toBeUndefined()
    expect(nearestPeriod(periods, '10:05')).toBe(3)
    expect(nearestPeriod(periods, '07:50')).toBe(1)
  })

  it('places lessons, occupied marks and moved lessons into the right cells', () => {
    const grid = buildWeekGrid(data, monday)
    expect(grid.days).toHaveLength(5)
    expect(grid.days[0]).toMatchObject({ date: '2026-09-07', weekday: 1, inSemester: true })
    expect(findWeekCell(grid, at('2026-09-07', 1))?.state).toBe('lesson')
    expect(findWeekCell(grid, at('2026-09-08', 2))?.occurrence?.className).toBe('四 2 班')
    expect(findWeekCell(grid, at('2026-09-07', 2))?.state).toBe('empty')
  })

  it('keeps occupied slots and empties the original slot of a moved lesson', () => {
    const grid = buildWeekGrid({
      ...data,
      changes: [
        { id: 1, lessonId: 1, date: '2026-09-07', status: 'occupied', newDate: '', newStartTime: '', newEndTime: '', newClassId: 0, newClassName: '', note: '开会' },
        { id: 2, lessonId: 2, date: '2026-09-08', status: 'rescheduled', newDate: '2026-09-10', newStartTime: '09:40', newEndTime: '10:20', newClassId: 20, newClassName: '四 2 班', note: '调课' },
      ],
    }, monday)
    expect(findWeekCell(grid, at('2026-09-07', 1))?.state).toBe('occupied')
    expect(findWeekCell(grid, at('2026-09-08', 2))?.state).toBe('empty')
    const moved = findWeekCell(grid, at('2026-09-10', 3))
    expect(moved?.state).toBe('moved')
    expect(moved?.occurrence?.movedHere).toBe(true)
  })

  it('flags days outside the semester', () => {
    const grid = buildWeekGrid({ ...data, settings: { ...data.settings, semesterStart: '2026-09-07', semesterEnd: '2026-09-11' } }, new Date(2026, 8, 14))
    expect(grid.days.every(day => !day.inSemester)).toBe(true)
    expect(grid.cells.every(cell => !cell.occurrence)).toBe(true)
  })
})

describe('drag and drop planning', () => {
  it('moves a lesson into an empty slot with the target period times', () => {
    const grid = buildWeekGrid(data, monday)
    const plan = planDrop(grid, at('2026-09-07', 1), at('2026-09-10', 3))
    expect(plan.kind).toBe('move')
    expect(plan.changes).toEqual([{
      lessonId: 1,
      date: '2026-09-07',
      status: 'rescheduled',
      newDate: '2026-09-10',
      newStartTime: '09:40',
      newEndTime: '10:20',
      newClassId: 10,
      note: '',
    }])
  })

  it('swaps two lessons and keeps each original date and class', () => {
    const grid = buildWeekGrid(data, monday)
    const plan = planDrop(grid, at('2026-09-07', 1), at('2026-09-08', 2))
    expect(plan.kind).toBe('swap')
    expect(plan.changes).toHaveLength(2)
    expect(plan.changes[0]).toMatchObject({ lessonId: 1, date: '2026-09-07', newDate: '2026-09-08', newStartTime: '08:50', newClassId: 10 })
    expect(plan.changes[1]).toMatchObject({ lessonId: 2, date: '2026-09-08', newDate: '2026-09-07', newStartTime: '08:00', newClassId: 20 })
  })

  it('refuses invalid drops', () => {
    const grid = buildWeekGrid(data, monday)
    expect(planDrop(grid, at('2026-09-07', 1), at('2026-09-07', 1)).kind).toBe('none')
    const emptySource = planDrop(grid, at('2026-09-07', 2), at('2026-09-09', 2))
    expect(emptySource.kind).toBe('none')
    expect(emptySource.reason).toBeTruthy()
    const occupied = buildWeekGrid({
      ...data,
      changes: [{ id: 3, lessonId: 1, date: '2026-09-07', status: 'occupied', newDate: '', newStartTime: '', newEndTime: '', newClassId: 0, newClassName: '', note: '' }],
    }, monday)
    const blocked = planDrop(occupied, at('2026-09-08', 2), at('2026-09-07', 1))
    expect(blocked.kind).toBe('none')
    expect(blocked.reason).toBeTruthy()
  })

  it('re-drags a moved lesson against its original date', () => {
    const grid = buildWeekGrid({
      ...data,
      changes: [{ id: 4, lessonId: 1, date: '2026-09-07', status: 'rescheduled', newDate: '2026-09-09', newStartTime: '09:40', newEndTime: '10:20', newClassId: 10, newClassName: '三 1 班', note: '' }],
    }, monday)
    expect(findWeekCell(grid, at('2026-09-09', 3))?.state).toBe('moved')
    const plan = planDrop(grid, at('2026-09-09', 3), at('2026-09-11', 1))
    expect(plan.kind).toBe('move')
    expect(plan.changes[0].date).toBe('2026-09-07')
    expect(plan.changes[0].newDate).toBe('2026-09-11')
  })

  it('labels drag reasons', () => {
    expect(changeNote('swap')).toBe('拖动互换')
    expect(changeNote('move')).toBe('拖动调课')
  })
})
