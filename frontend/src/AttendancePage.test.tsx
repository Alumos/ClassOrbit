import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AttendancePage } from './pages/AttendancePage'
import type { Attendance, ClassItem } from './types'

const classes: ClassItem[] = [{
  id: 7,
  name: '三 1 班',
  grade: '三',
  classNo: '1',
  studentCount: 35,
  totalScore: 0,
  activeSessionId: null,
  createdAt: '2026-09-01 08:00:00',
}]

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('AttendancePage', () => {
  it.each([7, 8])('can start the detected lesson while class 8 has active attendance and class %i is selected', async (selectedClassId) => {
    const otherClass = { ...classes[0], id: 8, name: '三 2 班', classNo: '2', activeSessionId: 20 }
    const selectedClass = selectedClassId === 7 ? classes[0] : otherClass
    const history: Attendance = {
      id: 10, classId: selectedClassId, className: selectedClass.name,
      title: `${selectedClass.name} · 2026-09-03 08:00`, course: '信息科技',
      status: 'closed', startedAt: '2026-09-03 08:00:00', sessionAt: '2026-09-03T08:00',
      endedAt: '2026-09-03 08:40:00', deletedAt: null,
      presentCount: 35, absentCount: 0, records: [],
    }
    const next: Attendance = {
      ...history, id: 21, classId: 7, className: classes[0].name,
      title: '三 1 班 · 2026-09-10 08:00', status: 'active',
      startedAt: '2026-09-10 08:00:00', sessionAt: '2026-09-10T08:00', endedAt: null,
    }
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path.startsWith('/api/attendance?')) return new Response(JSON.stringify({ items: [history], nextCursor: 0 }))
      if (path === '/api/attendance/10') return new Response(JSON.stringify(history))
      if (path === '/api/attendance' || path === '/api/attendance/21') return new Response(JSON.stringify(next))
      if (path === '/api/schedule/current') return new Response(JSON.stringify({
        detected: true, serverTime: '2026-09-10T08:20:00+08:00',
        classId: 7, className: classes[0].name, course: '信息科技', sessionAt: next.sessionAt,
        period: 1, startTime: '08:00', endTime: '08:40', source: 'regular', message: '请核对当前课时。',
      }))
      throw new Error(`unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    const onDataChange = vi.fn(async () => undefined)
    render(<AttendancePage classes={[...classes, otherClass]} classId={selectedClassId}
      setClassId={vi.fn()} notify={vi.fn()} onDataChange={onDataChange} />)

    expect(await screen.findByText('已结束')).toBeInTheDocument()
    const start = screen.getByRole('button', { name: '智能发起点名' })
    expect(start).toBeEnabled()
    fireEvent.click(start)
    expect(await screen.findByText('服务器识别：第 1 节 · 08:00-08:40')).toBeInTheDocument()
    const submit = screen.getByRole('button', { name: '确认并开放签到' })
    expect(submit).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(submit)
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/attendance', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ classId: 7, course: '信息科技', sessionAt: next.sessionAt }),
    })))
    await waitFor(() => expect(onDataChange).toHaveBeenCalledOnce())
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByText('签到进行中')).toBeInTheDocument()
  })

  it('uses the server lesson suggestion and requires explicit teacher confirmation', async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path.startsWith('/api/attendance?')) {
        return new Response(JSON.stringify({ items: [], nextCursor: 0 }), { status: 200 })
      }
      if (path === '/api/schedule/current') {
        return new Response(JSON.stringify({
          detected: true,
          serverTime: '2026-09-07T08:20:00+08:00',
          classId: 7,
          className: '三 1 班',
          course: '信息科技',
          sessionAt: '2026-09-07T08:00',
          period: 1,
          startTime: '08:00',
          endTime: '08:40',
          source: 'regular',
          message: '已按服务器时间识别到当前课时，请核对。',
        }), { status: 200 })
      }
      throw new Error(`unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<AttendancePage
      classes={classes}
      classId={7}
      setClassId={vi.fn()}
      notify={vi.fn()}
      onDataChange={vi.fn(async () => undefined)}
    />)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/attendance?'), expect.anything()))
    fireEvent.click(screen.getAllByRole('button', { name: '智能发起点名' })[0])

    expect(await screen.findByText('服务器识别：第 1 节 · 08:00-08:40')).toBeInTheDocument()
    const submit = screen.getByRole('button', { name: '确认并开放签到' })
    expect(submit).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox'))
    expect(submit).toBeEnabled()
  })
})
