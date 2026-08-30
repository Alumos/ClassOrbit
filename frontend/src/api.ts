const base = '/api'

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) { super(message); this.status = status }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  const response = await fetch(base + path, { ...init, headers })
  const body = await response.json().catch(() => null)
  if (!response.ok) {
    if (response.status === 401 && typeof window !== 'undefined' && path !== '/auth' && path !== '/auth/password') window.dispatchEvent(new CustomEvent('classorbit:unauthorized'))
    throw new ApiError(body?.error || '请求失败，请稍后重试', response.status)
  }
  return body as T
}

export const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
})
