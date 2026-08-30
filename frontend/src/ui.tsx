import { type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode } from 'react'

export function Button({ className = '', variant = 'default', size = 'default', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'default' | 'outline' | 'ghost' | 'danger'; size?: 'default' | 'icon' | 'sm' }) {
  return <button className={`button button-${variant} button-${size} ${className}`} {...props} />
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) { return <input {...props} className={`input ${props.className || ''}`} /> }

export function EmptyState({ icon, title, detail, action }: { icon: ReactNode; title: string; detail: string; action?: ReactNode }) {
  return <div className="empty-state"><div className="empty-icon">{icon}</div><strong>{title}</strong><p>{detail}</p>{action}</div>
}

export function SiteFooter({ note }: { note?: string }) {
  return <footer className="site-footer">{note && <span>{note}</span>}<a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">苏ICP备2021038338号-1</a></footer>
}

export function SkeletonGrid() { return <div className="student-grid">{Array.from({ length: 8 }).map((_, i) => <div className="student-card skeleton-card" key={i}><span /><span /><span /></div>)}</div> }

const dateTime = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
export function formatTime(value?: string | null) {
  if (!value) return '—'
  const date = new Date(value.includes('T') ? value : value.replace(' ', 'T'))
  return Number.isNaN(date.getTime()) ? value : dateTime.format(date)
}
