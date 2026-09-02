import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClassesPage } from './pages/ClassesPage'
import type { ClassItem, Student } from './types'

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

const existing: Student = {
  id: 10,
  classId: 7,
  studentNo: '01',
  name: '原有学生',
  score: 0,
  createdAt: '2026-09-01 08:00:00',
}

afterEach(() => vi.unstubAllGlobals())

describe('ClassesPage', () => {
  it('keeps manual student creation inside the visible roster manager', async () => {
    const created: Student = { ...existing, id: 11, studentNo: '02', name: '插班生' }
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/classes/7/students?sort=student_no') {
        return new Response(JSON.stringify([existing]), { status: 200 })
      }
      if (path === '/api/classes/7/students' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toEqual({ studentNo: '02', name: '插班生' })
        return new Response(JSON.stringify(created), { status: 201 })
      }
      throw new Error(`unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    const notify = vi.fn()
    const onDataChange = vi.fn(async () => undefined)

    render(<ClassesPage classes={classes} notify={notify} onDataChange={onDataChange} onSelectClass={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '维护名单' }))
    expect(await screen.findByText('原有学生')).toBeInTheDocument()
    const editButton = screen.getByRole('button', { name: '编辑原有学生' })
    const deleteButton = screen.getByRole('button', { name: '删除原有学生' })
    expect(editButton).toHaveAttribute('title', '编辑学生')
    expect(editButton).toHaveTextContent('编辑')
    expect(deleteButton).toHaveClass('roster-delete-action')
    expect(deleteButton).toHaveTextContent('删除')
    fireEvent.click(screen.getByRole('button', { name: '添加学生' }))

    const addDialog = await screen.findByRole('dialog', { name: '添加学生' })
    fireEvent.change(within(addDialog).getByLabelText('学号'), { target: { value: '02' } })
    fireEvent.change(within(addDialog).getByLabelText('姓名'), { target: { value: '插班生' } })
    fireEvent.click(within(addDialog).getByRole('button', { name: '添加到名单' }))

    expect(await screen.findByText('插班生')).toBeInTheDocument()
    await waitFor(() => expect(onDataChange).toHaveBeenCalledTimes(1))
    expect(notify).toHaveBeenCalledWith('插班生 已加入 三 1 班')
  })
})
