import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NavigationSettingsPage } from './pages/NavigationSettingsPage'
import type { NavigationItem } from './types'

afterEach(() => vi.unstubAllGlobals())

describe('NavigationSettingsPage teaching sites', () => {
  it('uploads a folder and supports edit, replace, preview metadata, and delete', async () => {
    let items: NavigationItem[] = []
    const created: NavigationItem = {
      id: 21,
      kind: 'site',
      title: '二进制互动练习',
      url: '/published/1234567890abcdef/abcdef1234567890/',
      iconUrl: null,
      sortOrder: 0,
      site: { publicId: '1234567890abcdef', revision: 'abcdef1234567890', sourceName: 'binary-demo', sourceSize: 120, extractedSize: 120, fileCount: 2, updatedAt: '2026-09-06 10:00:00' },
    }
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const requestPath = String(input)
      if (requestPath === '/api/navigation' && !init?.method) {
        return new Response(JSON.stringify(items), { status: 200 })
      }
      if (requestPath === '/api/navigation/sites' && init?.method === 'POST') {
        const form = init.body as FormData
        expect(form.get('mode')).toBe('folder')
        expect(form.get('title')).toBe('二进制互动练习')
        expect(form.getAll('files')).toHaveLength(2)
        expect(form.getAll('paths')).toEqual(['binary-demo/index.html', 'binary-demo/assets/app.js'])
        items = [created]
        return new Response(JSON.stringify(created), { status: 201 })
      }
      if (requestPath === '/api/navigation' && init?.method === 'PUT') {
        const body = JSON.parse(String(init.body))
        expect(body.items[0]).toMatchObject({ id: 21, kind: 'site', title: '更新后的练习', url: '' })
        items = [{ ...created, title: '更新后的练习' }]
        return new Response(JSON.stringify(items), { status: 200 })
      }
      if (requestPath === '/api/navigation/sites/21' && init?.method === 'PUT') {
        const form = init.body as FormData
        expect(form.get('mode')).toBe('html')
        expect((form.get('files') as File).name).toBe('new-version.html')
        items = [{ ...items[0], url: '/published/1234567890abcdef/fedcba0987654321/', site: { ...created.site!, revision: 'fedcba0987654321', sourceName: 'new-version.html' } }]
        return new Response(JSON.stringify(items[0]), { status: 200 })
      }
      if (requestPath === '/api/navigation/sites/21' && init?.method === 'DELETE') {
        items = []
        return new Response(JSON.stringify({ ok: true }), { status: 200 })
      }
      throw new Error(`unexpected request: ${requestPath} ${init?.method || 'GET'}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    const notify = vi.fn()

    render(<NavigationSettingsPage notify={notify} />)
    fireEvent.click(await screen.findByRole('button', { name: '上传教学网页' }))
    const createDialog = await screen.findByRole('dialog', { name: '上传教学网页' })
    fireEvent.change(within(createDialog).getByLabelText('导航标题'), { target: { value: '二进制互动练习' } })
    fireEvent.click(within(createDialog).getByRole('button', { name: '项目文件夹' }))
    const index = new File(['<h1>练习</h1>'], 'index.html', { type: 'text/html' }) as File & { webkitRelativePath: string }
    const script = new File(['console.log(1)'], 'app.js', { type: 'text/javascript' }) as File & { webkitRelativePath: string }
    Object.defineProperty(index, 'webkitRelativePath', { value: 'binary-demo/index.html' })
    Object.defineProperty(script, 'webkitRelativePath', { value: 'binary-demo/assets/app.js' })
    const folderInput = createDialog.querySelector('input[type="file"]') as HTMLInputElement
    expect(folderInput).toHaveAttribute('webkitdirectory')
    fireEvent.change(folderInput, { target: { files: [index, script] } })
    fireEvent.click(within(createDialog).getByRole('button', { name: '上传并添加' }))

    expect(await screen.findByText('binary-demo')).toBeInTheDocument()
    expect(screen.getByText(/2 个文件/)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('第 1 个项目标题'), { target: { value: '更新后的练习' } })
    fireEvent.click(screen.getByRole('button', { name: '保存导航' }))
    await waitFor(() => expect(screen.getByDisplayValue('更新后的练习')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '替换更新后的练习文件' }))
    const replaceDialog = await screen.findByRole('dialog', { name: '替换 更新后的练习 的项目文件' })
    const replacement = new File(['<h1>新版</h1>'], 'new-version.html', { type: 'text/html' })
    fireEvent.change(replaceDialog.querySelector('input[type="file"]') as HTMLInputElement, { target: { files: [replacement] } })
    fireEvent.click(within(replaceDialog).getByRole('button', { name: '发布新版本' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '替换 更新后的练习 的项目文件' })).not.toBeInTheDocument())
    expect(screen.getByText('new-version.html')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '删除更新后的练习' }))
    const deleteDialog = await screen.findByRole('dialog', { name: '删除教学网页' })
    fireEvent.click(within(deleteDialog).getByRole('button', { name: '确认删除' }))
    expect(await screen.findByText('还没有导航项目')).toBeInTheDocument()
    expect(notify).toHaveBeenCalledWith('教学网页已上传并添加到导航')
    expect(notify).toHaveBeenCalledWith('更新后的练习及其网页文件已删除')
  })
})
