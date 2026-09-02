import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SettingsPage } from './pages/SettingsPage'
import type { AuditLog, ClassItem, SiteSettings } from './types'

const settings: SiteSettings = { title: '智创课堂', subtitle: '小学信息科技课' }
const classes: ClassItem[] = [{
  id: 7,
  name: '三 1 班',
  grade: '三',
  classNo: '1',
  studentCount: 1,
  totalScore: 0,
  activeSessionId: null,
  createdAt: '2026-09-01 08:00:00',
}]

const audit = (id: number): AuditLog => ({
  id,
  action: 'student.update',
  entityType: 'student',
  entityId: id,
  summary: `日志 ${id}`,
  details: `detail-${id}`,
  actor: 'teacher',
  createdAt: '2026-09-02 08:00:00',
})

afterEach(() => vi.unstubAllGlobals())

describe('SettingsPage audit logs', () => {
  it('loads on expansion, paginates ten rows, and can collapse again', async () => {
    const firstPage = Array.from({ length: 11 }, (_, index) => audit(30 - index))
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/admin/audit-logs?limit=11') {
        return new Response(JSON.stringify(firstPage), { status: 200 })
      }
      if (path === '/api/admin/audit-logs?limit=11&before_id=21') {
        return new Response(JSON.stringify([audit(20), audit(19)]), { status: 200 })
      }
      throw new Error(`unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<SettingsPage settings={settings} classes={classes} onChange={vi.fn()} notify={vi.fn()} />)

    expect(fetchMock).not.toHaveBeenCalled()
    expect(screen.queryByText('日志 30')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '展开' }))

    expect(await screen.findByText('日志 30')).toBeInTheDocument()
    expect(screen.queryByText('日志 20')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '下一页' }))

    expect(await screen.findByText('日志 20')).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/admin/audit-logs?limit=11&before_id=21', expect.anything()))
    expect(screen.getByText('第 2 页')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '收起' }))
    expect(screen.queryByText('日志 20')).not.toBeInTheDocument()
  })
})
