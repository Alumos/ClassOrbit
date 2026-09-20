import { useEffect, useState } from 'react'
import { ArrowLeft, LogOut, MessageCircle } from 'lucide-react'
import { ChatPanel } from '../ChatPanel'
import { chatApi, chatJSON } from '../chatApi'
import { Button, SiteFooter } from '../ui'

export function StudentChatPage() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { document.title = '班级聊天室 · ClassOrbit' }, [])
  const logout = async () => {
    setBusy(true); setError('')
    try { await chatApi('/public/chat/session', chatJSON('DELETE')); window.location.replace('/checkin') }
    catch (reason) { setError((reason as Error).message); setBusy(false) }
  }
  return <main className="checkin-page chat-page">
    <header className="checkin-header"><a className="checkin-brand" href="/navigation"><span className="brand-mark"><MessageCircle size={20} /></span><strong>班级聊天室</strong></a><div className="chat-header-actions"><a href="/navigation" className="button button-ghost button-sm"><ArrowLeft size={15} />学习导航</a><Button variant="outline" size="sm" disabled={busy} onClick={() => void logout()}><LogOut size={15} />退出身份</Button></div></header>
    <div className="chat-shell">{error && <p className="form-error" role="alert">{error}</p>}<ChatPanel base="/public/chat" /></div>
    <SiteFooter note="友善交流，认真学习" />
  </main>
}
