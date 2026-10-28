import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  data: {
    settings: {
      semesterStart: '2020-01-01',
      semesterEnd: '2035-12-31',
      periods: [
        { period: 1, startTime: '08:00', endTime: '08:40' },
        { period: 2, startTime: '08:50', endTime: '09:30' },
        { period: 3, startTime: '09:40', endTime: '10:20' },
      ],
    },
    lessons: [
      { id: 1, classId: 10, className: '三 1 班', course: '信息科技', weekday: 1, period: 1, startTime: '08:00', endTime: '08:40', locationOdd: '机房 1', locationEven: '机房 1' },
      { id: 2, classId: 20, className: '四 2 班', course: '人工智能', weekday: 2, period: 2, startTime: '08:50', endTime: '09:30', locationOdd: '教室', locationEven: '教室' },
    ],
    changes: [] as unknown[],
  },
  calls: [] as Array<{ path: string; init?: { method?: string; body?: unknown } }>,
}))

vi.mock('./api', () => ({
  api: vi.fn(async (path: string, init?: { method?: string; body?: unknown }) => {
    state.calls.push({ path, init })
    if (path === '/schedule') return state.data
    if (path === '/schedule/current') return { detected: false, serverTime: new Date().toISOString(), sessionAt: '', classId: 0, className: '', course: '', period: 0, startTime: '', endTime: '', source: '', message: '' }
    return { ok: true }
  }),
  json: (method: string, body?: unknown) => ({ method, body: body === undefined ? undefined : JSON.stringify(body) }),
}))

import { ScheduleWidget } from './ScheduleWidget'

afterEach(cleanup)

const keyOf = (value: Date) => `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`
const today = new Date()
const monday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - ((today.getDay() + 6) % 7))
const mondayKey = keyOf(monday)
const thursday = new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + 3)
const thursdayKey = keyOf(thursday)
const tuesdayKey = keyOf(new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + 1))

const openWeekBoard = async () => {
  const view = render(<ScheduleWidget classes={[]} notify={() => {}} onStartAttendance={() => {}} />)
  fireEvent.click(await screen.findByLabelText('展开课程提醒'))
  fireEvent.click(await screen.findByLabelText('管理课表'))
  await screen.findByText('本周课表')
  await waitFor(() => expect(view.container.ownerDocument.querySelector(`[data-date="${mondayKey}"][data-period="1"]`)).toBeTruthy())
  return view
}

describe('week timetable drag and drop', () => {
  beforeEach(() => {
    localStorage.clear()
    state.calls.length = 0
    state.data.changes = []
  })

  it('renders the current week with draggable lesson cards', async () => {
    await openWeekBoard()
    expect(screen.getByText('拖动课程卡片到其他格子即可调课；拖到已有课程的格子会互换，只影响这一天')).toBeTruthy()
    const cell = document.querySelector(`[data-date="${mondayKey}"][data-period="1"]`) as HTMLElement
    expect(cell.className).toContain('has-course')
    expect(cell.textContent).toContain('三 1 班')
    expect(cell.querySelector('[draggable="true"]')).toBeTruthy()
  })
})

describe('week timetable drop behaviour', () => {
  beforeEach(() => {
    localStorage.clear()
    state.calls.length = 0
    state.data.changes = []
  })

  const drop = (fromDate: string, fromPeriod: number, toDate: string, toPeriod: number) => {
    const source = document.querySelector(`[data-date="${fromDate}"][data-period="${fromPeriod}"]`) as HTMLElement
    const target = document.querySelector(`[data-date="${toDate}"][data-period="${toPeriod}"]`) as HTMLElement
    const card = source.querySelector('[draggable="true"]') as HTMLElement
    const dataTransfer = { setData: () => {}, effectAllowed: '' }
    fireEvent.dragStart(card, { dataTransfer })
    fireEvent.dragOver(target, { dataTransfer })
    fireEvent.drop(target, { dataTransfer })
  }

  it('writes one one-off change when dropping on an empty slot', async () => {
    await openWeekBoard()
    drop(mondayKey, 1, thursdayKey, 3)
    await waitFor(() => expect(state.calls.filter(call => call.path === '/schedule/1/changes')).toHaveLength(1))
    const call = state.calls.find(item => item.path === '/schedule/1/changes')!
    expect(call.init?.method).toBe('PUT')
    expect(JSON.parse(String(call.init?.body))).toEqual({
      lessonId: 1, date: mondayKey, status: 'rescheduled', newDate: thursdayKey,
      newStartTime: '09:40', newEndTime: '10:20', newClassId: 10, note: '拖动调课',
    })
  })

  it('writes two changes when swapping two lessons', async () => {
    await openWeekBoard()
    drop(mondayKey, 1, tuesdayKey, 2)
    await waitFor(() => expect(state.calls.filter(call => call.path === '/schedule/2/changes')).toHaveLength(1))
    const bodies = state.calls.filter(call => call.path.includes('/changes')).map(call => JSON.parse(String(call.init?.body)))
    expect(bodies).toHaveLength(2)
    expect(bodies).toContainEqual({ lessonId: 1, date: mondayKey, status: 'rescheduled', newDate: tuesdayKey, newStartTime: '08:50', newEndTime: '09:30', newClassId: 10, note: '拖动互换' })
    expect(bodies).toContainEqual({ lessonId: 2, date: tuesdayKey, status: 'rescheduled', newDate: mondayKey, newStartTime: '08:00', newEndTime: '08:40', newClassId: 20, note: '拖动互换' })
  })

  it('refuses to drop onto an occupied slot', async () => {
    state.data.changes = [{ id: 9, lessonId: 2, date: tuesdayKey, status: 'occupied', newDate: '', newStartTime: '', newEndTime: '', newClassId: 0, newClassName: '', note: '开会' }]
    await openWeekBoard()
    expect(document.querySelector(`[data-date="${tuesdayKey}"][data-period="2"]`)?.className).toContain('is-occupied')
    drop(mondayKey, 1, tuesdayKey, 2)
    await new Promise(resolve => setTimeout(resolve, 20))
    expect(state.calls.filter(call => call.path.includes('/changes'))).toHaveLength(0)
  })
})
