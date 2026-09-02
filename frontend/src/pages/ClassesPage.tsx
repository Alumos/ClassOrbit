import { useEffect, useState } from 'react'
import * as Dropdown from '@radix-ui/react-dropdown-menu'
import { ArrowRight, FileSpreadsheet, MoreHorizontal, Pencil, Plus, School, Trash2, UserPlus, Users, Upload } from 'lucide-react'
import { api, json } from '../api'
import { Dialog } from '../dialog'
import { Select, SelectItem } from '../select'
import { Button, EmptyState, Input } from '../ui'
import type { ClassItem, Student } from '../types'
import type { Notify } from '../types'

export function ClassesPage({ classes, notify, onDataChange, onSelectClass }: { classes: ClassItem[]; notify: Notify; onDataChange: () => Promise<void>; onSelectClass: (id: number) => void }) {
  const [classDialog, setClassDialog] = useState(false)
  const [editingClass, setEditingClass] = useState<ClassItem | null>(null)
  const [importClass, setImportClass] = useState<ClassItem | null>(null)
  const [rosterClass, setRosterClass] = useState<ClassItem | null>(null)
  const [deleting, setDeleting] = useState<ClassItem | null>(null)
  const [busy, setBusy] = useState(false)

  const createClass = async (grade: string, classNo: string) => {
    setBusy(true)
    try { await api('/classes', json('POST', { grade, classNo })); notify('班级已创建'); setClassDialog(false); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const updateClass = async (grade: string, classNo: string) => {
    if (!editingClass) return
    setBusy(true)
    try { const next = await api<ClassItem>(`/classes/${editingClass.id}`, json('PATCH', { grade, classNo })); notify(`班级已更新为 ${next.name}，历史考勤已同步`); setEditingClass(null); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const removeClass = async () => {
    if (!deleting) return
    setBusy(true)
    try { await api(`/classes/${deleting.id}`, json('DELETE')); notify(`${deleting.name} 已删除`); setDeleting(null); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }

  return <>
    <div className="page-heading"><div><h1>班级与名单</h1><p>创建班级，导入或维护学生基础信息。</p></div><Button onClick={() => setClassDialog(true)}><Plus size={16} />创建班级</Button></div>
    <section className="panel table-panel">
      <div className="panel-header"><div><h2>全部班级</h2><p>{classes.length} 个班级 · Excel 支持“学号、姓名”表头</p></div></div>
      {classes.length ? <div className="table-scroll"><table><thead><tr><th>班级名称</th><th>年级</th><th>学生</th><th>积分合计</th><th>创建时间</th><th className="cell-action">操作</th></tr></thead><tbody>{classes.map(item => <tr key={item.id}>
        <td><button className="table-link" onClick={() => onSelectClass(item.id)}><span className="class-icon"><School size={15} /></span><strong>{item.name}</strong></button></td><td>{item.grade ? `${item.grade}年级` : '—'}</td><td>{item.studentCount} 人</td><td className={item.totalScore < 0 ? 'score-negative' : ''}>{item.totalScore}</td><td>{item.createdAt.slice(0, 10)}</td><td className="cell-action"><div className="row-actions"><Button variant="outline" size="sm" onClick={() => setRosterClass(item)}><Users size={14} />维护名单</Button><Button variant="outline" size="sm" onClick={() => setImportClass(item)}><Upload size={14} />导入</Button><Dropdown.Root><Dropdown.Trigger asChild><Button variant="ghost" size="icon" aria-label="更多操作"><MoreHorizontal size={17} /></Button></Dropdown.Trigger><Dropdown.Portal><Dropdown.Content className="dropdown-content" align="end" sideOffset={5}><Dropdown.Item className="dropdown-item" onSelect={() => setEditingClass(item)}><Pencil size={14} />修改年级与班号</Dropdown.Item><Dropdown.Item className="dropdown-item dropdown-danger" onSelect={() => setDeleting(item)}><Trash2 size={14} />删除班级</Dropdown.Item></Dropdown.Content></Dropdown.Portal></Dropdown.Root></div></td>
      </tr>)}</tbody></table></div> : <EmptyState icon={<School size={22} />} title="还没有班级" detail="先创建一个班级，再通过 Excel 导入学生名单。" action={<Button onClick={() => setClassDialog(true)}><Plus size={16} />创建第一个班级</Button>} />}
    </section>
    <ClassDialog open={classDialog} busy={busy} onOpenChange={setClassDialog} onSubmit={createClass} />
    <ClassDialog open={!!editingClass} target={editingClass} busy={busy} onOpenChange={open => !open && setEditingClass(null)} onSubmit={updateClass} />
    <ImportDialog target={importClass} onClose={() => setImportClass(null)} notify={notify} onDone={async () => { setImportClass(null); await onDataChange() }} />
    <RosterDialog target={rosterClass} onClose={() => setRosterClass(null)} notify={notify} onDataChange={onDataChange} />
    <Dialog open={!!deleting} onOpenChange={open => !open && setDeleting(null)} title="删除班级" description="此操作会一并删除学生、积分流水和考勤记录，且无法恢复。" footer={<><Button variant="outline" onClick={() => setDeleting(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void removeClass()}><Trash2 size={15} />确认删除</Button></>}><div className="danger-box">确定删除 <strong>{deleting?.name}</strong> 及其 {deleting?.studentCount} 名学生吗？</div></Dialog>
  </>
}

function ClassDialog({ open, target, busy, onOpenChange, onSubmit }: { open: boolean; target?: ClassItem | null; busy: boolean; onOpenChange: (v: boolean) => void; onSubmit: (grade: string, classNo: string) => Promise<void> }) {
  const [grade, setGrade] = useState('')
  const [classNo, setClassNo] = useState('')
  useEffect(() => { if (open) { setGrade(target?.grade || ''); setClassNo(target?.classNo || '') } }, [open, target])
  const submit = () => { if (grade && classNo.trim()) void onSubmit(grade, classNo.trim()) }
  const preview = grade && classNo.trim() ? `${grade} ${classNo.trim()} 班` : '选择年级并填写班号'
  return <Dialog open={open} onOpenChange={onOpenChange} title={target ? '修改班级' : '创建班级'} description={target ? '历史考勤记录会同步显示新的班级名称。' : '班级名称将根据年级和班号自动生成。'} footer={<><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={busy || !grade || !classNo.trim()} onClick={submit}>{target ? '保存修改' : '创建班级'}</Button></>}><div className="form-stack"><div className="class-fields"><div className="field"><label>年级</label><Select value={grade || undefined} onValueChange={setGrade} placeholder="选择年级">{['一','二','三','四','五','六'].map(item => <SelectItem key={item} value={item}>{item}年级</SelectItem>)}</Select></div><div className="field"><label htmlFor="class-no">班号</label><Input id="class-no" type="number" min="1" max="99" autoFocus value={classNo} onChange={e => setClassNo(e.target.value)} placeholder="如：2" /></div></div><div className="class-name-preview"><span>班级名称</span><strong>{preview}</strong></div></div></Dialog>
}

function RosterDialog({ target, onClose, notify, onDataChange }: { target: ClassItem | null; onClose: () => void; notify: Notify; onDataChange: () => Promise<void> }) {
  const [students, setStudents] = useState<Student[]>([])
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<Student | null>(null)
  const [deleting, setDeleting] = useState<Student | null>(null)
  const [studentNo, setStudentNo] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    let active = true
    setAdding(false); setEditing(null); setDeleting(null); setStudentNo(''); setName('')
    if (!target) { setStudents([]); setLoading(false); return () => { active = false } }
    setLoading(true)
    api<Student[]>(`/classes/${target.id}/students?sort=student_no`)
      .then(next => { if (active) setStudents(next) })
      .catch(error => { if (active) notify((error as Error).message, 'error') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [target, notify])
  const sortStudents = (items: Student[]) => [...items].sort((left, right) => left.studentNo.localeCompare(right.studentNo, undefined, { numeric: true }))
  const openAdd = () => { setStudentNo(''); setName(''); setAdding(true) }
  const openEdit = (student: Student) => { setStudentNo(student.studentNo); setName(student.name); setEditing(student) }
  const add = async () => {
    if (!target) return
    setBusy(true)
    try {
      const created = await api<Student>(`/classes/${target.id}/students`, json('POST', { studentNo: studentNo.trim(), name: name.trim() }))
      setStudents(current => sortStudents([...current, created])); setAdding(false); notify(`${created.name} 已加入 ${target.name}`); await onDataChange()
    } catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const update = async () => {
    if (!editing) return
    setBusy(true)
    try { const next = await api<Student>(`/students/${editing.id}`, json('PATCH', { studentNo: studentNo.trim(), name: name.trim() })); setStudents(current => sortStudents(current.map(item => item.id === next.id ? next : item))); setEditing(null); notify('学生信息已更新'); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  const remove = async () => {
    if (!deleting) return
    setBusy(true)
    try { await api(`/students/${deleting.id}`, json('DELETE')); setStudents(current => current.filter(item => item.id !== deleting.id)); notify(`${deleting.name} 已从名单中删除`); setDeleting(null); await onDataChange() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  return <>
    <Dialog open={!!target} onOpenChange={open => !open && onClose()} title={`${target?.name || ''}学生名单`} description={loading ? '正在加载名单…' : `共 ${students.length} 名学生，可手动新增、编辑或删除。`} width="wide" footer={<Button variant="outline" onClick={onClose}>完成</Button>}><div className="roster-manager"><div className="roster-toolbar"><div><strong>名单维护</strong><span>导入后可在这里补录插班生或修正学生信息</span></div><Button size="sm" onClick={openAdd}><UserPlus size={14} />添加学生</Button></div>{loading ? <div className="roster-loading" role="status">正在加载学生名单…</div> : students.length ? <div className="roster-table"><table><thead><tr><th>学号</th><th>姓名</th><th>当前积分</th><th className="cell-action">操作</th></tr></thead><tbody>{students.map(student => <tr key={student.id}><td className="mono">{student.studentNo}</td><td><strong>{student.name}</strong></td><td className={student.score < 0 ? 'score-negative' : ''}>{student.score}</td><td className="cell-action"><div className="row-actions roster-actions"><Button className="roster-edit-action" variant="outline" size="sm" aria-label={`编辑${student.name}`} title="编辑学生" onClick={() => openEdit(student)}><Pencil size={17} />编辑</Button><Button className="roster-delete-action" variant="outline" size="sm" aria-label={`删除${student.name}`} title="删除学生" onClick={() => setDeleting(student)}><Trash2 size={17} />删除</Button></div></td></tr>)}</tbody></table></div> : <div className="roster-empty"><EmptyState icon={<Users size={21} />} title="名单为空" detail="点击“添加学生”手动录入第一名学生。" /></div>}</div></Dialog>
    <Dialog open={adding} onOpenChange={setAdding} title="添加学生" description={`手动添加到 ${target?.name || ''}`} footer={<><Button variant="outline" onClick={() => setAdding(false)}>取消</Button><Button disabled={busy || !studentNo.trim() || !name.trim()} onClick={() => void add()}><UserPlus size={14} />添加到名单</Button></>}><div className="form-stack"><div className="field"><label htmlFor="add-student-no">学号</label><Input id="add-student-no" autoFocus value={studentNo} onChange={e => setStudentNo(e.target.value)} placeholder="请输入学号" /></div><div className="field"><label htmlFor="add-student-name">姓名</label><Input id="add-student-name" value={name} onChange={e => setName(e.target.value)} placeholder="请输入姓名" /></div></div></Dialog>
    <Dialog open={!!editing} onOpenChange={open => !open && setEditing(null)} title="编辑学生信息" description={editing ? `原学号：${editing.studentNo}` : ''} footer={<><Button variant="outline" onClick={() => setEditing(null)}>取消</Button><Button disabled={busy || !studentNo.trim() || !name.trim()} onClick={() => void update()}>保存修改</Button></>}><div className="form-stack"><div className="field"><label htmlFor="edit-no">学号</label><Input id="edit-no" value={studentNo} onChange={e => setStudentNo(e.target.value)} /></div><div className="field"><label htmlFor="edit-name">姓名</label><Input id="edit-name" value={name} onChange={e => setName(e.target.value)} /></div></div></Dialog>
    <Dialog open={!!deleting} onOpenChange={open => !open && setDeleting(null)} title="删除学生" description="积分流水和考勤记录也会一并删除。" footer={<><Button variant="outline" onClick={() => setDeleting(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}><Trash2 size={14} />确认删除</Button></>}><div className="danger-box">确定从班级中删除 <strong>{deleting?.name}</strong> 吗？</div></Dialog>
  </>
}

function ImportDialog({ target, onClose, notify, onDone }: { target: ClassItem | null; onClose: () => void; notify: Notify; onDone: () => Promise<void> }) {
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const upload = async () => {
    if (!target || !file) return
    setBusy(true)
    const form = new FormData(); form.set('file', file)
    try { const result = await api<{ added: number; skipped: number }>(`/classes/${target.id}/import`, { method: 'POST', body: form }); notify(`已导入 ${result.added} 名学生${result.skipped ? `，跳过 ${result.skipped} 条重复学号` : ''}`); setFile(null); await onDone() }
    catch (error) { notify((error as Error).message, 'error') }
    finally { setBusy(false) }
  }
  return <Dialog open={!!target} onOpenChange={open => !open && onClose()} title="导入学生名单" description={`目标班级：${target?.name || ''}`} footer={<><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={!file || busy} onClick={() => void upload()}><Upload size={15} />{busy ? '正在导入' : '开始导入'}</Button></>}><label className={`upload-zone ${file ? 'has-file' : ''}`}><input type="file" accept=".xlsx,.xlsm,.xls" onChange={e => setFile(e.target.files?.[0] || null)} /><span className="upload-icon"><FileSpreadsheet size={24} /></span><strong>{file ? file.name : '选择 Excel 文件'}</strong><p>{file ? `${(file.size / 1024).toFixed(1)} KB` : '支持 .xlsx / .xlsm / .xls，首个工作表，最多 200 人'}</p><span className="button button-outline button-sm">浏览文件<ArrowRight size={14} /></span></label><div className="import-note"><strong>表格要求</strong><span>推荐第一行为“学号、姓名”。没有表头时默认读取前两列；重复学号会自动跳过。</span></div></Dialog>
}
