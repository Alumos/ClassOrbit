import { useEffect, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode } from 'react'
import { api } from './api'
import type { BuildInfo } from './types'

let buildInfoRequest: Promise<BuildInfo> | null = null

function loadBuildInfo() {
  if (!buildInfoRequest) {
    buildInfoRequest = api<BuildInfo>('/health').catch(error => {
      buildInfoRequest = null
      throw error
    })
  }
  return buildInfoRequest
}

export function Button({ className = '', variant = 'default', size = 'default', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'default' | 'outline' | 'ghost' | 'danger'; size?: 'default' | 'icon' | 'sm' }) {
  return <button className={`button button-${variant} button-${size} ${className}`} {...props} />
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) { return <input {...props} className={`input ${props.className || ''}`} /> }

export function EmptyState({ icon, title, detail, action }: { icon: ReactNode; title: string; detail: string; action?: ReactNode }) {
  return <div className="empty-state"><div className="empty-icon">{icon}</div><strong>{title}</strong><p>{detail}</p>{action}</div>
}

export function DeploymentVersion({ className = '' }: { className?: string }) {
  const [build, setBuild] = useState<BuildInfo | null | false>(null)
  useEffect(() => {
    let active = true
    loadBuildInfo().then(info => { if (active) setBuild(info) }).catch(() => { if (active) setBuild(false) })
    return () => { active = false }
  }, [])

  const version = build === null ? '检测中' : build === false ? '版本未知' : build.version === 'dev' ? '开发版' : `v${build.version.replace(/^v/, '')}`
  const commit = build && build.commit && build.commit !== 'unknown' ? build.commit.slice(0, 7) : ''
  const detail = build ? `当前部署版本 ClassOrbit ${version}${commit ? `，构建 ${build.commit}` : ''}` : `ClassOrbit ${version}`
  return <span className={`deployment-version ${className}`.trim()} title={detail} aria-label={detail}><span>ClassOrbit</span><strong>{version}</strong>{commit && <code>{commit}</code>}</span>
}

export function SiteFooter({ note }: { note?: string }) {
  return <footer className="site-footer">{note && <span>{note}</span>}<DeploymentVersion /><a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">苏ICP备2021038338号-1</a></footer>
}

export function SkeletonGrid() { return <div className="student-grid">{Array.from({ length: 8 }).map((_, i) => <div className="student-card skeleton-card" key={i}><span /><span /><span /></div>)}</div> }

const dateTime = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
export function formatTime(value?: string | null) {
  if (!value) return '—'
  const date = new Date(value.includes('T') ? value : value.replace(' ', 'T'))
  return Number.isNaN(date.getTime()) ? value : dateTime.format(date)
}
