import { useEffect, useState } from 'react'
import { GraduationCap, Save, Settings2 } from 'lucide-react'
import { api, json } from '../api'
import { Button, Input } from '../ui'
import type { Notify } from '../types'
import type { SiteSettings } from '../types'

export function SettingsPage({ settings, onChange, notify }: { settings: SiteSettings; onChange: (settings: SiteSettings) => void; notify: Notify }) {
  const [title, setTitle] = useState(settings.title)
  const [subtitle, setSubtitle] = useState(settings.subtitle)
  const [busy, setBusy] = useState(false)
  useEffect(() => { setTitle(settings.title); setSubtitle(settings.subtitle) }, [settings])
  const save = async () => {
    setBusy(true)
    try { const next = await api<SiteSettings>('/settings', json('PATCH', { title: title.trim(), subtitle: subtitle.trim() })); onChange(next); notify('站点名称已更新') }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  return <>
    <div className="page-heading"><div><h1>系统设置</h1><p>调整教师后台与学生签到页显示的站点名称。</p></div><Button disabled={busy || !title.trim() || !subtitle.trim()} onClick={() => void save()}><Save size={15} />保存设置</Button></div>
    <section className="panel settings-panel"><div className="panel-header"><div><h2>品牌名称</h2><p>保存后所有页面立即同步</p></div><Settings2 size={17} /></div><div className="settings-form"><div className="form-stack"><div className="field"><label htmlFor="site-title">主标题</label><Input id="site-title" maxLength={20} value={title} onChange={event => setTitle(event.target.value)} placeholder="智创课堂" /></div><div className="field"><label htmlFor="site-subtitle">副标题</label><Input id="site-subtitle" maxLength={30} value={subtitle} onChange={event => setSubtitle(event.target.value)} placeholder="小学信息科技课" /></div></div><div className="brand-preview"><span>预览</span><div><span className="brand-mark"><GraduationCap size={19} /></span><div><strong>{title || '主标题'}</strong><small>{subtitle || '副标题'}</small></div></div></div></div></section>
  </>
}
