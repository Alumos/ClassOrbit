const retryKey = 'classorbit-page-retry'
const retryParameter = '_classorbit_reload'

export function freshPageURL() {
  const url = new URL(window.location.href)
  url.searchParams.delete('refresh')
  url.searchParams.set(retryParameter, String(Date.now()))
  return url.toString()
}

export function cleanPageURL() {
  const url = new URL(window.location.href)
  if (!url.searchParams.has(retryParameter) && !url.searchParams.has('refresh')) return
  url.searchParams.delete(retryParameter)
  url.searchParams.delete('refresh')
  window.history.replaceState(window.history.state, '', url)
}

export function recoverPage(error: Error) {
  if (!/Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Unable to preload CSS/i.test(error.message)) return
  try {
    const previous = Number(sessionStorage.getItem(retryKey))
    if (previous && Date.now() - previous < 60_000) return
    sessionStorage.setItem(retryKey, String(Date.now()))
  } catch { return } // Without a retry marker, show the button instead of risking a loop.
  window.location.replace(freshPageURL())
}
