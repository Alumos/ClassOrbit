import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { cleanPageURL, freshPageURL, recoverPage } from './pageRecovery'

beforeEach(() => { sessionStorage.clear(); history.replaceState(null, '', '/') })
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); history.replaceState(null, '', '/') })

it('removes only recovery parameters and preserves the route, query and fragment', () => {
  history.replaceState({ saved: true }, '', '/checkin?class=2&refresh=185&_classorbit_reload=1#here')
  cleanPageURL()
  expect(location.pathname + location.search + location.hash).toBe('/checkin?class=2#here')
  expect(history.state).toEqual({ saved: true })
  expect(new URL(freshPageURL()).searchParams.has('_classorbit_reload')).toBe(true)
})

it('automatically reloads a failed module only once within a minute', () => {
  const replace = vi.fn()
  vi.stubGlobal('window', { location: { href: 'https://class.alumos.cn/', replace } })
  const error = new TypeError('Failed to fetch dynamically imported module')
  recoverPage(error)
  recoverPage(error)
  expect(replace).toHaveBeenCalledTimes(1)
  expect(new URL(replace.mock.calls[0][0]).pathname).toBe('/')
})

it('does not reload business errors or loop when session storage is unavailable', () => {
  const replace = vi.fn()
  vi.stubGlobal('window', { location: { href: 'https://class.alumos.cn/', replace } })
  recoverPage(new Error('render failed'))
  vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: () => { throw new Error('blocked') } })
  recoverPage(new TypeError('Failed to fetch dynamically imported module'))
  expect(replace).not.toHaveBeenCalled()
})
