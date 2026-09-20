import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { ChatPanel } from './ChatPanel'
import { chatApi } from './chatApi'
import type { ChatSnapshot } from './types'

class FakeEvents extends EventTarget {
  static instances: FakeEvents[] = []
  onerror: (() => void) | null = null
  close = vi.fn()
  constructor(readonly url: string) { super(); FakeEvents.instances.push(this) }
  emit(name: string, value: unknown) { this.dispatchEvent(new MessageEvent(name, { data: JSON.stringify(value) })) }
}
const snapshot: ChatSnapshot = { roomId: 1, epoch: 'first', latestSeq: 0, messages: [], onlineStudents: 1, paused: false }
function setup() {
  FakeEvents.instances = []
  vi.stubGlobal('EventSource', FakeEvents)
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/session')) return { ok: true, json: async () => ({ roomId: 1, className: '三 1 班', name: '张同学' }) }
    if (url.endsWith('/messages')) return { ok: true, json: async () => JSON.parse(init?.body as string) }
    throw new Error(url)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers() })

it('renders text safely, respects IME and paused state, and sends a bounded plain-text message', async () => {
  const fetchMock = setup()
  const view = render(<ChatPanel base="/public/chat" />)
  await waitFor(() => expect(FakeEvents.instances).toHaveLength(1))
  const stream = FakeEvents.instances[0]
  act(() => stream.emit('snapshot', snapshot))
  const input = screen.getByLabelText('聊天消息')
  fireEvent.change(input, { target: { value: '你好' } })
  fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
  expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/messages'))).toHaveLength(0)
  fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
  expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/messages'))).toHaveLength(0)
  fireEvent.keyDown(input, { key: 'Enter' })
  await waitFor(() => expect(input).toHaveValue(''))
  const body = JSON.parse(fetchMock.mock.calls.find(([url]) => url.endsWith('/messages'))![1]!.body as string)
  expect(body).toMatchObject({ text: '你好', epoch: 'first' })
  expect(body.clientMessageId).toHaveLength(32)
  act(() => stream.emit('message', { epoch: 'first', message: { id: 'first:1', seq: 1, text: '<img src=x onerror=alert(1)>', createdAt: '2026-09-20T10:00:00Z', sender: { id: '2', role: 'student', name: '李同学' } } }))
  expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeInTheDocument()
  expect(view.container.querySelector('img')).toBeNull()
  act(() => stream.emit('room_state', { epoch: 'first', paused: true }))
  expect(input).toBeDisabled()
  expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
  view.unmount()
  expect(stream.close).toHaveBeenCalled()
})

it('retries an uncertain send with the same id and ignores old messages after clearing', async () => {
  const fetchMock = setup()
  render(<ChatPanel base="/public/chat" />)
  await waitFor(() => expect(FakeEvents.instances).toHaveLength(1))
  const stream = FakeEvents.instances[0]
  act(() => stream.emit('snapshot', snapshot))
  fetchMock.mockRejectedValueOnce(new Error('连接中断'))
  fireEvent.change(screen.getByLabelText('聊天消息'), { target: { value: '保留草稿' } })
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('连接中断')
  fireEvent.click(screen.getByRole('button', { name: '发送' }))
  await waitFor(() => expect(screen.getByLabelText('聊天消息')).toHaveValue(''))
  const sends = fetchMock.mock.calls.filter(([url]) => url.endsWith('/messages'))
  expect(sends[0][1]?.body).toBe(sends[1][1]?.body)
  act(() => {
    stream.emit('room_cleared', { ...snapshot, epoch: 'second' })
    stream.emit('message', { epoch: 'first', message: { id: 'first:99', text: '旧消息不得复活', sender: { id: '2', role: 'student', name: '同学' } } })
  })
  expect(screen.queryByText('旧消息不得复活')).not.toBeInTheDocument()
})

it('does not open a stream or log out the teacher on an unauthenticated student page', async () => {
  setup()
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 401, json: async () => ({ error: '请先签到', code: 'student_session_required' }) })))
  const unauthorized = vi.fn()
  window.addEventListener('classorbit:unauthorized', unauthorized)
  render(<ChatPanel base="/public/chat" />)
  expect(await screen.findByRole('link', { name: '前往签到' })).toHaveAttribute('href', '/checkin')
  expect(FakeEvents.instances).toHaveLength(0)
  await expect(chatApi('/public/chat/session')).rejects.toMatchObject({ status: 401 })
  expect(unauthorized).not.toHaveBeenCalled()
  window.removeEventListener('classorbit:unauthorized', unauthorized)
})

it('counts Unicode characters and never sends while over the limit', async () => {
  const fetchMock = setup()
  render(<ChatPanel base="/public/chat" />)
  await waitFor(() => expect(FakeEvents.instances).toHaveLength(1))
  act(() => FakeEvents.instances[0].emit('snapshot', snapshot))
  fireEvent.change(screen.getByLabelText('聊天消息'), { target: { value: '😀'.repeat(501) } })
  expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
  fireEvent.keyDown(screen.getByLabelText('聊天消息'), { key: 'Enter' })
  expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/messages'))).toHaveLength(0)
})
