import { useState } from 'react'
import { fireEvent, render, screen, within, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { RandomPicker } from './pages/PointsPage'
import type { Student } from './types'

afterEach(() => vi.unstubAllGlobals())
it('shows confirmed score changes without stale scores or duplicate requests', async () => {
  vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  const save = vi.fn()
  function Harness() {
    const [students, setStudents] = useState<Student[]>([{ id: 1, classId: 1, name: '张同学', studentNo: '2026001', score: 5, createdAt: '' }])
    return <RandomPicker open onOpenChange={() => {}} students={students} className="五 2 班" onAdjust={async (student, delta) => {
      if (!await save()) return undefined
      const updated = { ...student, score: student.score + delta }
      setStudents([updated])
      return updated
    }} />
  }
  render(<Harness />)
  const dialog = within(screen.getByRole('dialog'))
  fireEvent.click(dialog.getByRole('button', { name: '开始抽取' }))
  expect(dialog.getByLabelText('张同学当前积分 5')).toBeInTheDocument()
  let finish!: (value: boolean) => void
  save.mockImplementationOnce(() => new Promise<boolean>(resolve => { finish = resolve }))
  fireEvent.click(dialog.getByRole('button', { name: '张同学加一分' }))
  expect(dialog.getByRole('button', { name: '张同学加一分' })).toBeDisabled()
  expect(dialog.getByText('正在保存…')).toBeInTheDocument()
  finish(true)
  expect(await dialog.findByLabelText('张同学当前积分 6')).toBeInTheDocument()
  expect(dialog.getByRole('status')).toHaveTextContent('已加 1 分 · 5 → 6')
  save.mockResolvedValueOnce(true)
  fireEvent.click(dialog.getByRole('button', { name: '张同学扣一分' }))
  expect(await dialog.findByLabelText('张同学当前积分 5')).toBeInTheDocument()
  expect(dialog.getByRole('status')).toHaveTextContent('已扣 1 分 · 6 → 5')
  save.mockResolvedValueOnce(false)
  fireEvent.click(dialog.getByRole('button', { name: '张同学扣一分' }))
  await waitFor(() => expect(dialog.getByRole('button', { name: '张同学扣一分' })).not.toBeDisabled())
  expect(dialog.getByLabelText('张同学当前积分 5')).toBeInTheDocument()
  expect(dialog.getByRole('status')).not.toHaveTextContent('已扣')
  fireEvent.click(dialog.getByRole('button', { name: '重新抽取' }))
  expect(dialog.getByRole('status')).toHaveTextContent('每次调整 1 分')
})
