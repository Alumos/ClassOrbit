import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AttendancePage } from './pages/AttendancePage'
import type { ClassItem } from './types'

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

afterEach(() => vi.unstubAllGlobals())

describe('AttendancePage', () => {
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
