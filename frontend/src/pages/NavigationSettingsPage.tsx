import { useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ExternalLink, Globe2, Plus, RefreshCw, Save, Trash2 } from 'lucide-react'
import { api, json } from '../api'
import { Button, EmptyState, Input } from '../ui'
import type { NavigationItem, Notify } from '../types'

type NavigationDraft = {
  key: string
  id?: number
  title: string
  url: string
  iconUrl: string
}

let draftSequence = 0

function draftKey(item: NavigationItem | Omit<NavigationDraft, 'key'>) {
  return item.id === undefined ? `draft-${Date.now()}-${++draftSequence}` : `item-${item.id}`
}

function toDrafts(items: NavigationItem[]): NavigationDraft[] {
  return items.map(item => ({ id: item.id, title: item.title, url: item.url, iconUrl: item.iconUrl || '', key: draftKey(item) }))
}

function normalizedURL(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ''
  try {
    const parsed = new URL(/^[a-z][a-z\d+.-]*:/i.test(trimmed) ? trimmed : `https://${trimmed}`)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : ''
  } catch {
    return ''
  }
}

function faviconURL(value: string) {
  const url = normalizedURL(value)
  return url ? new URL('/favicon.ico', url).toString() : ''
}

function serializable(items: NavigationDraft[]) {
  return items.map(({ title, url, iconUrl }) => ({ title: title.trim(), url: url.trim(), iconUrl: iconUrl.trim() }))
}

function snapshot(items: NavigationDraft[]) {
  return JSON.stringify(serializable(items))
}

export function NavigationSettingsPage({ notify, onDirtyChange }: { notify: Notify; onDirtyChange?: (dirty: boolean) => void }) {
  const [items, setItems] = useState<NavigationDraft[]>([])
  const [savedSnapshot, setSavedSnapshot] = useState('[]')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setLoadError(false)
    try {
      const next = toDrafts(await api<NavigationItem[]>('/navigation'))
      setItems(next)
      setSavedSnapshot(snapshot(next))
    } catch (error) {
      setLoadError(true)
      notify((error as Error).message, 'error')
    } finally {
      setLoading(false)
    }
  }, [notify])

  useEffect(() => { void load() }, [load])

  const dirty = useMemo(() => snapshot(items) !== savedSnapshot, [items, savedSnapshot])
  useEffect(() => {
    onDirtyChange?.(dirty)
    return () => onDirtyChange?.(false)
  }, [dirty, onDirtyChange])

  const update = (key: string, field: 'title' | 'url' | 'iconUrl', value: string) => {
    setItems(current => current.map(item => item.key === key ? { ...item, [field]: value } : item))
  }

  const add = () => {
    const item = { title: '', url: '', iconUrl: '' }
    setItems(current => [...current, { ...item, key: draftKey(item) }])
  }

  const remove = (key: string) => setItems(current => current.filter(item => item.key !== key))

  const move = (index: number, direction: -1 | 1) => {
    setItems(current => {
      const target = index + direction
      if (target < 0 || target >= current.length) return current
      const next = [...current]
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })
  }

  const fillFavicon = (key: string, force = false) => {
    setItems(current => current.map(item => {
      if (item.key !== key || (!force && item.iconUrl.trim())) return item
      const iconUrl = faviconURL(item.url)
      return iconUrl ? { ...item, iconUrl } : item
    }))
  }

  const save = async () => {
    const next: Array<{ title: string; url: string; iconUrl: string }> = []
    const seen = new Set<string>()
    for (const item of items) {
      const title = item.title.trim()
      const url = normalizedURL(item.url)
      const iconUrl = item.iconUrl.trim() ? normalizedURL(item.iconUrl) : faviconURL(url)
      if (!title || !url) {
        notify('请填写每个网站的标题和有效链接', 'error')
        return
      }
      if (item.iconUrl.trim() && !iconUrl) {
        notify(`${title} 的图标 URL 无效`, 'error')
        return
      }
      if (seen.has(url)) {
        notify(`${title} 的网站链接与其他项目重复`, 'error')
        return
      }
      seen.add(url)
      next.push({ title, url, iconUrl })
    }

    setBusy(true)
    try {
      const saved = toDrafts(await api<NavigationItem[]>('/navigation', json('PUT', { items: next })))
      setItems(saved)
      setSavedSnapshot(snapshot(saved))
      notify('学生导航已保存')
    } catch (error) {
      notify((error as Error).message, 'error')
    } finally {
      setBusy(false)
    }
  }

  return <>
    <div className="page-heading"><div><h1>学生导航</h1><p>维护签到完成后向学生展示的课堂常用网站。</p></div><Button disabled={loading || busy || !dirty} onClick={() => void save()}><Save size={15} />{busy ? '保存中' : '保存导航'}</Button></div>
    <section className="panel table-panel">
      <div className="panel-header"><div><h2>导航网站</h2><p>{items.length} 个网站 · 按当前顺序向学生展示</p></div><div className="row-actions"><a href="/navigation" target="_blank" rel="noreferrer" className="button button-outline button-icon" aria-label="预览学生导航" title="预览学生导航"><ExternalLink size={15} /></a><Button variant="outline" size="sm" disabled={loading || busy || items.length >= 100} onClick={add}><Plus size={15} />添加网站</Button></div></div>
      {loading ? <div className="page-module-loading" aria-label="正在加载导航网站" /> : loadError ? <EmptyState icon={<Globe2 size={22} />} title="导航网站加载失败" detail="请检查网络连接后重试。" action={<Button variant="outline" onClick={() => void load()}><RefreshCw size={15} />重新加载</Button>} /> : items.length === 0 ? <EmptyState icon={<Globe2 size={22} />} title="还没有导航网站" detail="添加学生课堂中经常使用的网站。" action={<Button onClick={add}><Plus size={15} />添加第一个网站</Button>} /> : <div className="table-scroll"><table>
        <thead><tr><th>顺序</th><th>图标</th><th>网站标题</th><th>网站链接</th><th>图标 URL</th><th className="cell-action">操作</th></tr></thead>
        <tbody>{items.map((item, index) => {
          const previewURL = normalizedURL(item.url)
          return <tr key={item.key}>
            <td><div className="row-actions"><Button variant="ghost" size="icon" disabled={busy || index === 0} onClick={() => move(index, -1)} aria-label={`上移${item.title || '网站'}`} title="上移"><ArrowUp size={14} /></Button><Button variant="ghost" size="icon" disabled={busy || index === items.length - 1} onClick={() => move(index, 1)} aria-label={`下移${item.title || '网站'}`} title="下移"><ArrowDown size={14} /></Button></div></td>
            <td><NavigationIcon src={item.iconUrl} title={item.title} /></td>
            <td><Input disabled={busy} aria-label={`第 ${index + 1} 个网站标题`} maxLength={50} value={item.title} onChange={event => update(item.key, 'title', event.target.value)} placeholder="如：国家中小学智慧教育平台" style={{ minWidth: 180 }} /></td>
            <td><Input disabled={busy} aria-label={`第 ${index + 1} 个网站链接`} maxLength={2048} value={item.url} onChange={event => update(item.key, 'url', event.target.value)} onBlur={() => fillFavicon(item.key)} placeholder="https://example.com" style={{ minWidth: 240 }} /></td>
            <td><Input disabled={busy} aria-label={`第 ${index + 1} 个网站图标 URL`} maxLength={2048} value={item.iconUrl} onChange={event => update(item.key, 'iconUrl', event.target.value)} placeholder="自动使用网站 /favicon.ico" style={{ minWidth: 220 }} /></td>
            <td className="cell-action"><div className="row-actions"><Button variant="ghost" size="icon" disabled={busy || !previewURL} onClick={() => fillFavicon(item.key, true)} aria-label={`重新获取${item.title || '网站'}图标`} title="使用网站 favicon.ico"><RefreshCw size={14} /></Button>{previewURL ? <a href={previewURL} target="_blank" rel="noreferrer" className="button button-ghost button-icon" aria-label={`打开${item.title || '网站'}`} title="打开网站"><ExternalLink size={14} /></a> : <Button variant="ghost" size="icon" disabled aria-label="网站链接无效"><ExternalLink size={14} /></Button>}<Button variant="ghost" size="icon" disabled={busy} onClick={() => remove(item.key)} aria-label={`移除${item.title || '网站'}`} title="移除"><Trash2 size={14} /></Button></div></td>
          </tr>
        })}</tbody>
      </table></div>}
    </section>
  </>
}

function NavigationIcon({ src, title }: { src: string; title: string }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [src])
  return <span className="class-icon">{src && !failed ? <img src={src} alt="" width="20" height="20" style={{ objectFit: 'contain' }} referrerPolicy="no-referrer" onError={() => setFailed(true)} /> : <Globe2 size={15} aria-label={title ? `${title} 默认图标` : '默认网站图标'} />}</span>
}
