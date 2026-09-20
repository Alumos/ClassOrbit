import { render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { StudentNavigationPage } from './pages/StudentNavigationPage'

afterEach(() => vi.unstubAllGlobals())
it('shows the built-in classroom chat even without external navigation links', async () => {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => ({ ok: true, json: async () => url.endsWith('/navigation') ? [] : url.endsWith('/settings') ? { title: '课堂', subtitle: '学习', chatEnabled: true } : { version: 'test', commit: 'test' } })))
  render(<StudentNavigationPage />)
  expect(await screen.findByRole('link', { name: /班级聊天室/ })).toHaveAttribute('href', '/chat')
  expect(screen.queryByText('暂无学习网站')).not.toBeInTheDocument()
})
