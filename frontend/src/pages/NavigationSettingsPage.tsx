import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import * as Dropdown from '@radix-ui/react-dropdown-menu'
import { ArrowDown, ArrowRight, ArrowUp, ExternalLink, FileArchive, FileCode2, FolderOpen, Globe2, Plus, RefreshCw, Save, Trash2, Upload } from 'lucide-react'
import { api, json } from '../api'
import { Dialog } from '../dialog'
import { Button, EmptyState, Input, formatTime } from '../ui'
import type { NavigationItem, Notify } from '../types'

type NavigationDraft = {
  key: string
  id?: number
  kind: 'external' | 'site'
  title: string
  url: string
  iconUrl: string
  site?: NavigationItem['site']
}

type UploadMode = 'html' | 'folder' | 'zip'

let draftSequence = 0

function draftKey(item: NavigationItem | Omit<NavigationDraft, 'key'>) {
  return item.id === undefined ? `draft-${Date.now()}-${++draftSequence}` : `item-${item.id}`
}

function toDrafts(items: NavigationItem[]): NavigationDraft[] {
  return items.map(item => ({ id: item.id, kind: item.kind, title: item.title, url: item.url, iconUrl: item.iconUrl || '', site: item.site, key: draftKey(item) }))
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
  return items.map(({ id, kind, title, url, iconUrl }) => ({ id, kind, title: title.trim(), url: kind === 'site' ? '' : url.trim(), iconUrl: iconUrl.trim() }))
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
  const [uploadOpen, setUploadOpen] = useState(false)
  const [replacing, setReplacing] = useState<NavigationDraft | null>(null)
  const [deleting, setDeleting] = useState<NavigationDraft | null>(null)

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
  const teachingSiteStats = useMemo(() => items.reduce((total, item) => item.site ? { count: total.count + 1, size: total.size + item.site.extractedSize } : total, { count: 0, size: 0 }), [items])
  useEffect(() => {
    onDirtyChange?.(dirty)
    return () => onDirtyChange?.(false)
  }, [dirty, onDirtyChange])

  const update = (key: string, field: 'title' | 'url' | 'iconUrl', value: string) => {
    setItems(current => current.map(item => item.key === key ? { ...item, [field]: value } : item))
  }

  const addExternal = () => {
    const item = { kind: 'external' as const, title: '', url: '', iconUrl: '' }
    setItems(current => [...current, { ...item, key: draftKey(item) }])
  }

  const openUpload = () => {
    if (dirty) { notify('请先保存导航列表中的修改，再上传教学网页', 'error'); return }
    setReplacing(null)
    setUploadOpen(true)
  }

  const openReplace = (item: NavigationDraft) => {
    if (dirty) { notify('请先保存导航列表中的修改，再替换教学网页', 'error'); return }
    setReplacing(item)
    setUploadOpen(true)
  }

  const removeExternal = (key: string) => setItems(current => current.filter(item => item.key !== key))

  const removeTeachingSite = async () => {
    if (!deleting?.id) return
    setBusy(true)
    try {
      await api(`/navigation/sites/${deleting.id}`, { method: 'DELETE' })
      notify(`${deleting.title}及其网页文件已删除`)
      setDeleting(null)
      await load()
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }

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
      if (item.key !== key || item.kind !== 'external' || (!force && item.iconUrl.trim())) return item
      const iconUrl = faviconURL(item.url)
      return iconUrl ? { ...item, iconUrl } : item
    }))
  }

  const save = async () => {
    const next: Array<{ id?: number; kind: 'external' | 'site'; title: string; url: string; iconUrl: string }> = []
    const seen = new Set<string>()
    for (const item of items) {
      const title = item.title.trim()
      const url = item.kind === 'site' ? '' : normalizedURL(item.url)
      const iconUrl = item.iconUrl.trim() ? normalizedURL(item.iconUrl) : item.kind === 'external' ? faviconURL(url) : ''
      if (!title || (item.kind === 'external' && !url)) {
        notify('请填写每个网站的标题和有效链接', 'error')
        return
      }
      if (item.iconUrl.trim() && !iconUrl) {
        notify(`${title} 的图标 URL 无效`, 'error')
        return
      }
      if (item.kind === 'external' && seen.has(url)) {
        notify(`${title} 的网站链接与其他项目重复`, 'error')
        return
      }
      if (url) seen.add(url)
      next.push({ id: item.id, kind: item.kind, title, url, iconUrl })
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
    <div className="page-heading"><div><h1>学生导航</h1><p>维护外部学习网站，并直接发布 HTML 教学网页。</p></div><Button disabled={loading || busy || !dirty} onClick={() => void save()}><Save size={15} />{busy ? '保存中' : '保存导航'}</Button></div>
    <section className="panel table-panel navigation-settings-panel">
      <div className="panel-header"><div><h2>导航项目</h2><p>{items.length} 个项目 · 按当前顺序展示{teachingSiteStats.count ? ` · ${teachingSiteStats.count} 个教学网页共 ${formatBytes(teachingSiteStats.size)}` : ''}</p></div><div className="row-actions panel-header-actions"><a href="/navigation" target="_blank" rel="noreferrer" className="button button-outline button-sm" aria-label="预览学生导航" title="在新标签页打开学生导航"><ExternalLink size={15} />预览学生页</a><Dropdown.Root><Dropdown.Trigger asChild><Button variant="outline" size="sm" disabled={loading || busy || items.length >= 100}><Plus size={15} />添加项目</Button></Dropdown.Trigger><Dropdown.Portal><Dropdown.Content className="dropdown-content" align="end" sideOffset={5}><Dropdown.Item className="dropdown-item" onSelect={addExternal}><Globe2 size={14} />添加外部链接</Dropdown.Item><Dropdown.Item className="dropdown-item" onSelect={openUpload}><Upload size={14} />上传教学网页</Dropdown.Item></Dropdown.Content></Dropdown.Portal></Dropdown.Root></div></div>
      {loading ? <div className="page-module-loading" aria-label="正在加载导航项目" /> : loadError ? <EmptyState icon={<Globe2 size={22} />} title="导航项目加载失败" detail="请检查网络连接后重试。" action={<Button variant="outline" onClick={() => void load()}><RefreshCw size={15} />重新加载</Button>} /> : items.length === 0 ? <EmptyState icon={<Globe2 size={22} />} title="还没有导航项目" detail="添加外部网站，或直接上传自己的 HTML 教学网页。" action={<Button onClick={openUpload}><Upload size={15} />上传教学网页</Button>} /> : <div className="table-scroll"><table>
        <thead><tr><th>顺序</th><th>类型</th><th>图标</th><th>标题</th><th>地址或项目文件</th><th>图标 URL</th><th className="cell-action">操作</th></tr></thead>
        <tbody>{items.map((item, index) => {
          const previewURL = item.kind === 'site' ? item.url : normalizedURL(item.url)
          return <tr key={item.key}>
            <td><div className="row-actions"><Button variant="ghost" size="icon" disabled={busy || index === 0} onClick={() => move(index, -1)} aria-label={`上移${item.title || '项目'}`} title="上移"><ArrowUp size={14} /></Button><Button variant="ghost" size="icon" disabled={busy || index === items.length - 1} onClick={() => move(index, 1)} aria-label={`下移${item.title || '项目'}`} title="下移"><ArrowDown size={14} /></Button></div></td>
            <td><span className={`navigation-kind navigation-kind-${item.kind}`}>{item.kind === 'site' ? <FileCode2 size={12} /> : <Globe2 size={12} />}{item.kind === 'site' ? '教学网页' : '外部链接'}</span></td>
            <td><NavigationIcon src={item.iconUrl} title={item.title} kind={item.kind} /></td>
            <td><Input disabled={busy} aria-label={`第 ${index + 1} 个项目标题`} maxLength={50} value={item.title} onChange={event => update(item.key, 'title', event.target.value)} placeholder="如：Scratch 练习" style={{ minWidth: 170 }} /></td>
            <td>{item.kind === 'site' ? <div className="site-file-summary"><strong title={item.site?.sourceName}>{item.site?.sourceName || '教学网页'}</strong><span>{item.site ? `${item.site.fileCount} 个文件 · ${formatBytes(item.site.extractedSize)} · ${formatTime(item.site.updatedAt)}` : '已发布'}</span></div> : <Input disabled={busy} aria-label={`第 ${index + 1} 个网站链接`} maxLength={2048} value={item.url} onChange={event => update(item.key, 'url', event.target.value)} onBlur={() => fillFavicon(item.key)} placeholder="https://example.com" style={{ minWidth: 240 }} />}</td>
            <td><Input disabled={busy} aria-label={`第 ${index + 1} 个项目图标 URL`} maxLength={2048} value={item.iconUrl} onChange={event => update(item.key, 'iconUrl', event.target.value)} placeholder={item.kind === 'site' ? '可选在线图标' : '自动使用 /favicon.ico'} style={{ minWidth: 205 }} /></td>
            <td className="cell-action"><div className="row-actions">{item.kind === 'external' && <Button variant="ghost" size="icon" disabled={busy || !previewURL} onClick={() => fillFavicon(item.key, true)} aria-label={`重新获取${item.title || '网站'}图标`} title="使用网站 favicon.ico"><RefreshCw size={14} /></Button>}{item.kind === 'site' && <Button variant="ghost" size="icon" disabled={busy} onClick={() => openReplace(item)} aria-label={`替换${item.title || '教学网页'}文件`} title="替换项目文件"><Upload size={14} /></Button>}{previewURL ? <a href={previewURL} target="_blank" rel="noreferrer" className="button button-ghost button-icon" aria-label={`打开${item.title || '项目'}`} title="打开预览"><ExternalLink size={14} /></a> : <Button variant="ghost" size="icon" disabled aria-label="项目地址无效"><ExternalLink size={14} /></Button>}{item.kind === 'site' ? <Button variant="ghost" size="icon" disabled={busy} onClick={() => dirty ? notify('请先保存其他导航修改，再删除教学网页', 'error') : setDeleting(item)} aria-label={`删除${item.title || '教学网页'}`} title="删除教学网页及文件"><Trash2 size={14} /></Button> : <Button variant="ghost" size="icon" disabled={busy} onClick={() => removeExternal(item.key)} aria-label={`移除${item.title || '网站'}`} title="移除"><Trash2 size={14} /></Button>}</div></td>
          </tr>
        })}</tbody>
      </table></div>}
    </section>
    <TeachingSiteDialog open={uploadOpen} onOpenChange={setUploadOpen} item={replacing} notify={notify} onDone={async () => { setUploadOpen(false); await load() }} />
    <Dialog open={!!deleting} onOpenChange={open => !open && !busy && setDeleting(null)} title="删除教学网页" description="导航项和上传的全部网页文件都会删除，无法恢复。" footer={<><Button variant="outline" disabled={busy} onClick={() => setDeleting(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void removeTeachingSite()}><Trash2 size={14} />{busy ? '正在删除' : '确认删除'}</Button></>}><div className="danger-box">确定删除 <strong>{deleting?.title}</strong> 及其 {deleting?.site?.fileCount || 0} 个项目文件吗？</div></Dialog>
  </>
}

function TeachingSiteDialog({ open, onOpenChange, item, notify, onDone }: { open: boolean; onOpenChange: (open: boolean) => void; item?: NavigationDraft | null; notify: Notify; onDone: () => Promise<void> }) {
  const [mode, setMode] = useState<UploadMode>('html')
  const [title, setTitle] = useState('')
  const [iconUrl, setIconUrl] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [busy, setBusy] = useState(false)
  const folderInput = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!open) return
    setMode('html'); setTitle(item?.title || ''); setIconUrl(item?.iconUrl || ''); setFiles([])
  }, [open, item])

  useEffect(() => {
    const input = folderInput.current
    if (!input) return
    input.setAttribute('webkitdirectory', '')
    input.setAttribute('directory', '')
  }, [mode, open])

  const changeMode = (next: UploadMode) => { setMode(next); setFiles([]) }
  const choose = (event: ChangeEvent<HTMLInputElement>) => setFiles(Array.from(event.target.files || []))
  const upload = async () => {
    if (!files.length || (!item && !title.trim())) return
    const body = new FormData()
    body.set('mode', mode)
    body.set('title', item?.title || title.trim())
    body.set('iconUrl', item?.iconUrl || iconUrl.trim())
    if (mode === 'folder') {
      const firstPath = files[0]?.webkitRelativePath || files[0]?.name || ''
      body.set('sourceName', firstPath.split('/')[0] || '项目文件夹')
    }
    files.forEach(file => {
      body.append('files', file, file.name)
      body.append('paths', mode === 'folder' ? file.webkitRelativePath || file.name : file.name)
    })
    setBusy(true)
    try {
      await api<NavigationItem>(item?.id ? `/navigation/sites/${item.id}` : '/navigation/sites', { method: item?.id ? 'PUT' : 'POST', body })
      notify(item ? `${item.title}的项目文件已替换` : '教学网页已上传并添加到导航')
      await onDone()
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }

  const selectedLabel = files.length ? mode === 'folder' ? `${files.length} 个文件` : files[0].name : mode === 'folder' ? '选择项目文件夹' : mode === 'zip' ? '选择 ZIP 压缩包' : '选择 HTML 文件'
  const selectedDetail = files.length ? `${formatBytes(files.reduce((total, file) => total + file.size, 0))}${mode === 'folder' ? ' · 保留目录结构' : ''}` : uploadModeHelp[mode]
  const accept = mode === 'html' ? '.html,.htm,text/html' : '.zip,application/zip'

  return <Dialog open={open} onOpenChange={value => !busy && onOpenChange(value)} title={item ? `替换 ${item.title} 的项目文件` : '上传教学网页'} description={item ? '新版本发布后导航地址会自动更新，旧页面不会被半途覆盖。' : '上传后由 ClassOrbit 托管，并自动添加到学生导航。'} footer={<><Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>取消</Button><Button disabled={busy || !files.length || (!item && !title.trim())} onClick={() => void upload()}><Upload size={15} />{busy ? '正在检查并发布' : item ? '发布新版本' : '上传并添加'}</Button></>}>
    <div className="form-stack teaching-upload-form">
      {!item && <><div className="field"><label htmlFor="teaching-site-title">导航标题</label><Input id="teaching-site-title" autoFocus maxLength={50} value={title} onChange={event => setTitle(event.target.value)} placeholder="如：二进制互动练习" /></div><div className="field"><label htmlFor="teaching-site-icon">图标 URL（可选）</label><Input id="teaching-site-icon" maxLength={2048} value={iconUrl} onChange={event => setIconUrl(event.target.value)} placeholder="https://example.com/icon.png" /></div></>}
      <div className="field"><label>上传方式</label><div className="segment segment-full upload-mode-segment"><button type="button" className={mode === 'html' ? 'active' : ''} onClick={() => changeMode('html')}><FileCode2 size={14} />单 HTML</button><button type="button" className={mode === 'folder' ? 'active' : ''} onClick={() => changeMode('folder')}><FolderOpen size={14} />项目文件夹</button><button type="button" className={mode === 'zip' ? 'active' : ''} onClick={() => changeMode('zip')}><FileArchive size={14} />ZIP</button></div></div>
      <label className={`upload-zone teaching-upload-zone ${files.length ? 'has-file' : ''}`} key={mode}>{mode === 'folder' ? <input ref={folderInput} type="file" multiple onChange={choose} /> : <input type="file" accept={accept} onChange={choose} />}<span className="upload-icon">{mode === 'folder' ? <FolderOpen size={24} /> : mode === 'zip' ? <FileArchive size={24} /> : <FileCode2 size={24} />}</span><strong>{selectedLabel}</strong><p>{selectedDetail}</p><span className="button button-outline button-sm">浏览{mode === 'folder' ? '文件夹' : '文件'}<ArrowRight size={14} /></span></label>
      <div className="import-note"><strong>发布要求</strong><span>项目必须包含入口 index.html；ZIP 最大 32MB，解压后或文件夹最大 128MB、最多 2000 个文件。网页可使用本站本地存储，并与后台同源运行，请只上传完全信任的 HTML、CSS、JavaScript 和资源文件。</span></div>
    </div>
  </Dialog>
}

const uploadModeHelp: Record<UploadMode, string> = {
  html: '适合所有样式和脚本都写在一个文件中的网页',
  folder: '直接选择包含 index.html 的完整项目目录',
  zip: '支持包含 index.html 的 ZIP，允许外层套一层项目目录',
}

function NavigationIcon({ src, title, kind }: { src: string; title: string; kind: 'external' | 'site' }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [src])
  return <span className="class-icon">{src && !failed ? <img src={src} alt="" width="20" height="20" style={{ objectFit: 'contain' }} referrerPolicy="no-referrer" onError={() => setFailed(true)} /> : kind === 'site' ? <FileCode2 size={15} aria-label={title ? `${title} 教学网页图标` : '教学网页图标'} /> : <Globe2 size={15} aria-label={title ? `${title} 默认图标` : '默认网站图标'} />}</span>
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
