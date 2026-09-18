import { Component, type ReactNode } from 'react'

export function freshPageURL() {
  const url = new URL(window.location.href)
  url.searchParams.set('refresh', String(Date.now()))
  return url.toString()
}

// Lazy import failures must leave a usable recovery action, not an empty root.
export class PageBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() {
    if (this.state.failed) return <main className="login-page"><section className="login-panel" role="alert"><h1>页面未能加载</h1><p>系统可能刚刚更新，或网络暂时中断。重新加载可获取最新页面，尚未保存的输入会丢失。</p><a className="button" href={freshPageURL()}>重新加载页面</a></section></main>
    return this.props.children
  }
}
