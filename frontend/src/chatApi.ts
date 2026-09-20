import { ApiError } from './api'

export class ChatError extends ApiError {
  code: string
  constructor(message: string, status: number, code = '') { super(message, status); this.code = code }
}

// Student authorization errors must never log the teacher workspace out.
export async function chatApi<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  if (init?.signal?.aborted) abort()
  init?.signal?.addEventListener('abort', abort, { once: true })
  const timer = setTimeout(abort, 15000)
  try {
    const response = await fetch(`/api${path}`, { ...init, signal: controller.signal, cache: 'no-store', credentials: 'same-origin' })
    const body = await response.json().catch(() => null)
    if (!response.ok) {
      if (response.status === 401 && path.startsWith('/chat/')) window.dispatchEvent(new CustomEvent('classorbit:unauthorized'))
      throw new ChatError(body?.error || '聊天室暂时不可用', response.status, body?.code)
    }
    return body as T
  } catch (reason) {
    if (controller.signal.aborted && !init?.signal?.aborted) throw new ChatError('连接超时，请重试', 503, 'request_timeout')
    throw reason
  } finally {
    clearTimeout(timer)
    init?.signal?.removeEventListener('abort', abort)
  }
}

export function chatJSON(method: string, body?: unknown): RequestInit {
  return { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) }
}

// crypto.randomUUID is unavailable on some school LAN HTTP origins.
export function messageID(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
}
