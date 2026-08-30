import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { EmptyState } from './ui'

describe('EmptyState', () => {
  it('renders guidance and an optional action', () => {
    render(<EmptyState icon={<span>图标</span>} title="暂无考勤" detail="请先发起点名" action={<button>立即发起</button>} />)
    expect(screen.getByText('暂无考勤')).toBeInTheDocument()
    expect(screen.getByText('请先发起点名')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '立即发起' })).toBeEnabled()
  })
})
