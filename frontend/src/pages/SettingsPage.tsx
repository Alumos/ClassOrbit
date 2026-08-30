import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { DatabaseBackup, Download, FileSpreadsheet, GraduationCap, KeyRound, RotateCcw, Save, ScrollText, Settings2, Upload } from 'lucide-react'
import { api, json } from '../api'
import { Select, SelectItem } from '../select'
import { Button, Input, formatTime } from '../ui'
import type { AuditLog, ClassItem, Notify, SiteSettings } from '../types'

type Props = {
  settings: SiteSettings
  classes: ClassItem[]
  onChange: (settings: SiteSettings) => void
  notify: Notify
}

const localDate = (date: Date) => {
  const local = new Date(date)
  local.setMinutes(local.getMinutes() - local.getTimezoneOffset())
  return local.toISOString().slice(0, 10)
}
const today = localDate(new Date())
const monthAgo = localDate(new Date(Date.now() - 30 * 86400000))

export function SettingsPage({ settings, classes, onChange, notify }: Props) {
  const [title, setTitle] = useState(settings.title)
  const [subtitle, setSubtitle] = useState(settings.subtitle)
  const [passwords, setPasswords] = useState({ current: '', next: '', confirm: '' })
  const [report, setReport] = useState({ type: 'attendance', classId: String(classes[0]?.id || ''), from: monthAgo, to: today })
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [busy, setBusy] = useState(false)
  const restoreRef = useRef<HTMLInputElement>(null)

  useEffect(() => { setTitle(settings.title); setSubtitle(settings.subtitle) }, [settings])
  useEffect(() => { if (!report.classId && classes[0]) setReport(current => ({ ...current, classId: String(classes[0].id) })) }, [classes, report.classId])
  const loadLogs = async () => {
    try { setLogs(await api<AuditLog[]>('/admin/audit-logs?limit=30')) }
    catch (error) { notify((error as Error).message, 'error') }
  }
  useEffect(() => { void loadLogs() }, [])

  const save = async () => {
    setBusy(true)
    try {
      const next = await api<SiteSettings>('/settings', json('PATCH', { title: title.trim(), subtitle: subtitle.trim() }))
      onChange(next); notify('站点名称已更新')
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const changePassword = async () => {
    if (passwords.next !== passwords.confirm) { notify('两次输入的新密码不一致', 'error'); return }
    setBusy(true)
    try {
      await api('/auth/password', json('PATCH', { currentPassword: passwords.current, newPassword: passwords.next }))
      notify('密码已修改，请重新登录')
      window.setTimeout(() => window.dispatchEvent(new CustomEvent('classorbit:unauthorized')), 500)
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const restoreBackup = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || !window.confirm(`确认用“${file.name}”恢复系统？当前数据会先自动生成一份安全备份。`)) return
    const body = new FormData()
    body.append('file', file)
    setBusy(true)
    try {
      const result = await api<{ message: string; safetyBackup: string }>('/admin/restore', { method: 'POST', body })
      notify(`${result.message}；恢复前备份：${result.safetyBackup}`)
      window.setTimeout(() => window.dispatchEvent(new CustomEvent('classorbit:unauthorized')), 700)
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const exportReport = () => {
    if (!report.classId) { notify('请选择班级', 'error'); return }
    const params = new URLSearchParams({ type: report.type, class_id: report.classId })
    if (report.type === 'attendance') { params.set('from', report.from); params.set('to', report.to) }
    window.location.assign(`/api/admin/reports?${params}`)
    window.setTimeout(() => void loadLogs(), 800)
  }

  return <>
    <div className="page-heading"><div><h1>系统设置</h1><p>管理品牌、账号、备份、报表与操作审计。</p></div><Button disabled={busy || !title.trim() || !subtitle.trim()} onClick={() => void save()}><Save size={15} />保存品牌设置</Button></div>

    <section className="panel settings-panel"><div className="panel-header"><div><h2>品牌名称</h2><p>保存后所有页面立即同步</p></div><Settings2 size={17} /></div><div className="settings-form"><div className="form-stack"><div className="field"><label htmlFor="site-title">主标题</label><Input id="site-title" maxLength={20} value={title} onChange={event => setTitle(event.target.value)} /></div><div className="field"><label htmlFor="site-subtitle">副标题</label><Input id="site-subtitle" maxLength={30} value={subtitle} onChange={event => setSubtitle(event.target.value)} /></div></div><div className="brand-preview"><span>预览</span><div><span className="brand-mark"><GraduationCap size={19} /></span><div><strong>{title || '主标题'}</strong><small>{subtitle || '副标题'}</small></div></div></div></div></section>

    <div className="settings-grid">
      <section className="panel admin-card"><div className="panel-header"><div><h2>登录密码</h2><p>修改后会注销所有登录会话</p></div><KeyRound size={17} /></div><div className="form-stack admin-card-body"><div className="field"><label>当前密码</label><Input type="password" autoComplete="current-password" value={passwords.current} onChange={event => setPasswords(current => ({ ...current, current: event.target.value }))} /></div><div className="class-fields"><div className="field"><label>新密码</label><Input type="password" minLength={8} maxLength={72} autoComplete="new-password" value={passwords.next} onChange={event => setPasswords(current => ({ ...current, next: event.target.value }))} /></div><div className="field"><label>确认新密码</label><Input type="password" minLength={8} maxLength={72} autoComplete="new-password" value={passwords.confirm} onChange={event => setPasswords(current => ({ ...current, confirm: event.target.value }))} /></div></div><Button disabled={busy || !passwords.current || passwords.next.length < 8 || !passwords.confirm} onClick={() => void changePassword()}><KeyRound size={14} />修改密码</Button></div></section>

      <section className="panel admin-card"><div className="panel-header"><div><h2>数据备份与恢复</h2><p>在线生成一致性 SQLite 备份</p></div><DatabaseBackup size={17} /></div><div className="admin-card-body"><div className="admin-action-row"><div><strong>下载完整备份</strong><span>包含账号、名单、积分、考勤、课表和设置</span></div><a className="button button-outline" href="/api/admin/backup"><Download size={14} />下载</a></div><div className="admin-action-row"><div><strong>从备份恢复</strong><span>恢复前自动保留当前数据库，完成后重新登录</span></div><Button variant="outline" disabled={busy} onClick={() => restoreRef.current?.click()}><Upload size={14} />选择文件</Button><input ref={restoreRef} className="hidden-file" type="file" accept=".db,.sqlite,.sqlite3" onChange={event => void restoreBackup(event)} /></div></div></section>
    </div>

    <section className="panel admin-card"><div className="panel-header"><div><h2>Excel 报表</h2><p>导出名单、积分流水或指定日期范围考勤</p></div><FileSpreadsheet size={17} /></div><div className="report-controls"><Select value={report.classId || undefined} onValueChange={value => setReport(current => ({ ...current, classId: value }))} placeholder="选择班级">{classes.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</Select><Select value={report.type} onValueChange={value => setReport(current => ({ ...current, type: value }))}><SelectItem value="attendance">考勤明细</SelectItem><SelectItem value="scores">积分流水</SelectItem><SelectItem value="roster">当前名单</SelectItem></Select>{report.type === 'attendance' && <><Input type="date" value={report.from} onChange={event => setReport(current => ({ ...current, from: event.target.value }))} /><Input type="date" value={report.to} onChange={event => setReport(current => ({ ...current, to: event.target.value }))} /></>}<Button onClick={exportReport}><Download size={14} />导出 Excel</Button></div></section>

    <section className="panel admin-card"><div className="panel-header"><div><h2>操作审计</h2><p>保留删除、改分、恢复、报表等重要操作</p></div><Button variant="ghost" size="icon" title="刷新" onClick={() => void loadLogs()}><RotateCcw size={15} /></Button></div><div className="audit-list">{logs.length ? logs.map(item => <div className="audit-item" key={item.id}><span><ScrollText size={14} /></span><div><strong>{item.summary}</strong><small>{item.details || item.action}</small></div><time>{formatTime(item.createdAt)}</time></div>) : <p className="muted">暂无重要操作记录</p>}</div></section>
  </>
}
