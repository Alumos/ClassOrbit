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

describe('SettingsPage backup restore', () => {
  it('uploads the selected backup and signs out after successful restore', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const dispatch = vi.spyOn(window, 'dispatchEvent')
    const fetchMock = vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      expect((init?.body as FormData).get('file')).toBeInstanceOf(File)
      return new Response(JSON.stringify({ message: '恢复成功，请重新登录', safetyBackup: 'before.db' }), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)
    const notify = vi.fn()
    const { container } = render(<SettingsPage settings={settings} classes={classes} onChange={vi.fn()} notify={notify} />)
    fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [new File(['backup'], 'classorbit.db')] } })
    await waitFor(() => expect(notify).toHaveBeenCalledWith('恢复成功，请重新登录；恢复前备份：before.db'))
    await waitFor(() => expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({ type: 'classorbit:unauthorized' })))
    confirm.mockRestore()
    dispatch.mockRestore()
  })

  it('rejects oversized backups before uploading', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const notify = vi.fn()
    const { container } = render(<SettingsPage settings={settings} classes={classes} onChange={vi.fn()} notify={notify} />)
    const file = new File([], 'oversize.db')
    Object.defineProperty(file, 'size', { value: 512 * 1024 * 1024 + 1 })
    fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [file] } })
    expect(notify).toHaveBeenCalledWith('备份文件不能超过 512MB', 'error')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
