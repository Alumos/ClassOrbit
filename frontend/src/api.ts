const base = '/api'

/** Structured error returned by the Go API. */
export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) { super(message); this.status = status }
}

/**
 * Shared JSON/file request helper.
 *
 * `path` is always relative to `/api`; callers should pass a `RequestInit`
 * created by `json()` for JSON bodies or a native `FormData` body for uploads.
 * A 401 response dispatches the global unauthorized event consumed by App.tsx.
 */
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

/** Build a request init object without repeating JSON serialization. */
export const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
})
