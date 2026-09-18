import { lazy, Suspense } from 'react'
import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { PageBoundary } from './PageBoundary'

vi.mock('./pageRecovery', () => ({ recoverPage: vi.fn(), freshPageURL: () => window.location.origin + '/?_classorbit_reload=test' }))

it('shows a fresh navigation link when a lazy chunk fails instead of a blank page', async () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {})
  try {
    const MissingPage = lazy(() => Promise.reject(new TypeError('Failed to fetch dynamically imported module')))
    render(<PageBoundary><Suspense fallback="loading"><MissingPage /></Suspense></PageBoundary>)
    expect(await screen.findByRole('alert')).toHaveTextContent('页面未能加载')
    const link = screen.getByRole('link', { name: '重新加载页面' })
    const url = new URL(link.getAttribute('href')!)
    expect(url.origin).toBe(window.location.origin)
    expect(url.searchParams.has('_classorbit_reload')).toBe(true)
  } finally { log.mockRestore() }
})
