import { describe, expect, it } from 'vitest'
import { buildOccurrences, semesterWeek } from './ScheduleWidget'
import type { ScheduleData } from './types'

const data: ScheduleData = {
  settings: {
    semesterStart: '2026-09-07',
    semesterEnd: '2027-01-20',
    periods: [{ period: 1, startTime: '08:00', endTime: '08:40' }],
  },
  lessons: [{
    id: 1,
    classId: 10,
    className: '三 1 班',
    course: '信息科技',
    weekday: 1,
    period: 1,
    startTime: '08:00',
    endTime: '08:40',
    locationOdd: '机房 1',
    locationEven: '机房 2',
  }],
  changes: [],
}

describe('schedule occurrences', () => {
  it('calculates semester week and odd/even location', () => {
    expect(semesterWeek(data.settings, '2026-09-07')).toBe(1)
    expect(semesterWeek(data.settings, '2026-09-14')).toBe(2)
    const first = buildOccurrences(data, new Date(2026, 8, 7, 12), 8)
    expect(first[0].location).toBe('机房 1')
    expect(first[1].location).toBe('机房 2')
  })

  it('keeps the original occurrence marked and creates a moved occurrence', () => {
    const changed: ScheduleData = {
      ...data,
      changes: [{
        id: 2,
        lessonId: 1,
        date: '2026-09-07',
        status: 'rescheduled',
        newDate: '2026-09-08',
        newStartTime: '10:00',
        newEndTime: '10:40',
        newClassId: 10,
        newClassName: '三 1 班',
        note: '临时换课',
      }],
    }
    const items = buildOccurrences(changed, new Date(2026, 8, 7, 12), 3)
    expect(items).toHaveLength(2)
    expect(items[0].change?.status).toBe('rescheduled')
    expect(items[1].movedHere).toBe(true)
    expect(items[1].date).toBe('2026-09-08')
  })
})
