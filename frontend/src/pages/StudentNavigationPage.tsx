import { useCallback, useEffect, useRef, useState } from 'react'
import { ExternalLink, Globe2, GraduationCap, LayoutGrid, RefreshCw } from 'lucide-react'
import { api } from '../api'
import { Button, EmptyState, SiteFooter } from '../ui'
import type { NavigationItem, SiteSettings } from '../types'

export function StudentNavigationPage() {
  const [settings, setSettings] = useState<SiteSettings>({ title: '智创课堂', subtitle: '小学信息科技课' })
  const [items, setItems] = useState<NavigationItem[] | null>(null)
  const [error, setError] = useState('')
  const requestVersion = useRef(0)

  const load = useCallback(() => {
    const version = ++requestVersion.current
    setItems(null)
    setError('')
    api<NavigationItem[]>('/public/navigation').then(navigation => { if (version === requestVersion.current) setItems(navigation) }).catch(reason => { if (version === requestVersion.current) { setItems([]); setError((reason as Error).message) } })
    api<SiteSettings>('/public/settings').then(site => { if (version === requestVersion.current) { setSettings(site); document.title = `${site.title} · 学习导航` } }).catch(() => undefined)
  }, [])

  useEffect(() => { load(); return () => { requestVersion.current++ } }, [load])

  return <main className="checkin-page navigation-page">
    <header className="checkin-header">
      <a href="/navigation" className="checkin-brand"><span className="brand-mark"><GraduationCap size={19} /></span><div><strong>{settings.title}</strong><span>{settings.subtitle}</span></div></a>
      <span className="secure-note"><LayoutGrid size={15} />学习导航</span>
    </header>
    <div className="checkin-shell navigation-shell">
      <div className="checkin-heading"><span className="checkin-symbol"><LayoutGrid size={22} /></span><div><h1>学习导航</h1><p>课堂常用网站</p></div></div>
      <section className="checkin-panel navigation-panel">
        {items === null && !error && <div className="navigation-loading" role="status"><span className="page-module-loading" /><span>正在加载学习导航</span></div>}
        {error && <EmptyState icon={<Globe2 size={22} />} title="导航加载失败" detail={error} action={<Button variant="outline" onClick={() => void load()}><RefreshCw size={15} />重新加载</Button>} />}
        {items?.length === 0 && <EmptyState icon={<LayoutGrid size={22} />} title="暂无学习网站" detail="老师还没有配置课堂导航。" />}
        {items && items.length > 0 && <div className="navigation-grid">{items.map(item => <a key={item.id} className="navigation-card" href={item.url} target="_blank" rel="noopener noreferrer" title={item.title}><NavigationIcon item={item} /><div className="navigation-card-copy"><strong>{item.title}</strong><span>{getHostname(item.url)}</span></div><ExternalLink size={16} aria-hidden="true" /></a>)}</div>}
      </section>
    </div>
    <SiteFooter note="课堂学习资源导航" />
  </main>
}

function NavigationIcon({ item }: { item: NavigationItem }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [item.iconUrl])
  return <span className="navigation-icon">{item.iconUrl && !failed ? <img src={item.iconUrl} alt="" loading="lazy" decoding="async" referrerPolicy="no-referrer" onError={() => setFailed(true)} /> : <Globe2 size={22} aria-hidden="true" />}</span>
}

function getHostname(url: string) {
  try { return new URL(url).hostname.replace(/^www\./, '') }
  catch { return url }
}
