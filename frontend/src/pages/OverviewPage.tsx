import { ArrowRight, BarChart3, BookOpenCheck, School, Users } from 'lucide-react'
import { Button } from '../ui'
import type { ClassItem, Dashboard } from '../types'

export function OverviewPage({ classes, dashboard, onNavigate }: { classes: ClassItem[]; dashboard: Dashboard; onNavigate: (page: 'points' | 'classes' | 'attendance') => void }) {
  const stats = [
    { label: '班级总数', value: dashboard.classCount, hint: '当前执教', icon: School },
    { label: '学生总数', value: dashboard.studentCount, hint: '已录入名单', icon: Users },
    { label: '累计积分', value: dashboard.totalScore, hint: '全部班级', icon: BarChart3 },
    { label: '签到进行中', value: dashboard.activeSessions, hint: '开放场次', icon: BookOpenCheck },
  ]
  return <>
    <div className="page-heading"><div><h1>工作台总览</h1><p>查看班级运行情况和当前课堂状态。</p></div><Button onClick={() => onNavigate('points')}><BarChart3 size={16} />进入积分台</Button></div>
    <section className="stats-panel">{stats.map((item, index) => <div className="stat-item" key={item.label}><span className="stat-icon"><item.icon size={17} /></span><div><span>{item.label}</span><strong>{item.value}</strong><small>{item.hint}</small></div>{index < stats.length - 1 && <i />}</div>)}</section>
    <section className="panel table-panel">
      <div className="panel-header"><div><h2>班级概况</h2><p>各班名单和积分汇总</p></div><Button variant="outline" size="sm" onClick={() => onNavigate('classes')}>管理班级<ArrowRight size={14} /></Button></div>
      <div className="table-scroll"><table><thead><tr><th>班级</th><th>年级</th><th>学生人数</th><th>积分合计</th><th>签到状态</th><th></th></tr></thead><tbody>
        {classes.map(item => <tr key={item.id}><td><strong>{item.name}</strong></td><td>{item.grade ? `${item.grade}年级` : '—'}</td><td>{item.studentCount} 人</td><td><span className={item.totalScore < 0 ? 'score-negative' : ''}>{item.totalScore}</span></td><td>{item.activeSessionId ? <span className="status status-active">签到中</span> : <span className="status">未开始</span>}</td><td className="cell-action"><Button variant="ghost" size="sm" onClick={() => onNavigate('points')}>查看<ArrowRight size={14} /></Button></td></tr>)}
        {classes.length === 0 && <tr><td colSpan={6} className="table-empty">还没有班级，请先创建班级并导入名单。</td></tr>}
      </tbody></table></div>
    </section>
  </>
}
