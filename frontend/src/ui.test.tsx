import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EmptyState, SiteFooter } from './ui'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('EmptyState', () => {
  it('renders guidance and an optional action', () => {
    render(<EmptyState icon={<span>图标</span>} title="暂无考勤" detail="请先发起点名" action={<button>立即发起</button>} />)
    expect(screen.getByText('暂无考勤')).toBeInTheDocument()
    expect(screen.getByText('请先发起点名')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '立即发起' })).toBeEnabled()
  })
})

describe('SiteFooter', () => {
  it('shows the version and commit reported by the running backend', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            version: '1.6.0',
            commit: '8f4a5aaf4cdde669',
          }),
          { status: 200 },
        ),
      ),
    )

    render(<SiteFooter />)

    expect(await screen.findByText('v1.6.0')).toBeInTheDocument()
    expect(screen.getByText('8f4a5aa')).toBeInTheDocument()
    expect(
      screen.getByLabelText(/当前部署版本 ClassOrbit v1\.6\.0/),
    ).toHaveAttribute(
      'title',
      expect.stringContaining('8f4a5aaf4cdde669'),
    )
  })
})
