import { useEffect, useRef, useState } from 'react'
import { MessageCircle, Send } from 'lucide-react'
import { chatApi, chatJSON, messageID } from './chatApi'
import type { ChatMessage } from './types'
import { useChat } from './useChat'
import { Button } from './ui'

export function ChatPanel({ base, teacher = false }: { base: string; teacher?: boolean }) {
  const { snapshot, identity, connection, error } = useChat(base)
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState('')
  const [newMessages, setNewMessages] = useState(false)
  const scroll = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const mounted = useRef(true)
  const scope = useRef('')
  const pending = useRef<{ text: string; id: string; epoch: string; scope: string } | null>(null)
  const draft = useRef(text)
  draft.current = text
  scope.current = `${snapshot?.epoch || ''}:${identity?.studentId || 'teacher'}`
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => { pending.current = null; setSendError('') }, [snapshot?.epoch])
  useEffect(() => { setText(''); pending.current = null; setSendError('') }, [identity?.studentId])
  useEffect(() => {
    if (atBottom.current && scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight
    else if (snapshot?.messages.length) setNewMessages(true)
  }, [snapshot?.messages])
  const count = Array.from(text).length
  const paused = !!snapshot?.paused && !teacher
  const canSend = connection === 'connected' && !paused && !sending && text.trim().length > 0 && count <= 500
  const send = async () => {
    if (!canSend || !snapshot) return
    const content = text.trim()
    const request = pending.current?.text === content && pending.current.scope === scope.current
      ? pending.current : { text: content, id: messageID(), epoch: snapshot.epoch, scope: scope.current }
    pending.current = request
    setSending(true); setSendError('')
    try {
      await chatApi<ChatMessage>(`${base}/messages`, chatJSON('POST', { text: request.text, clientMessageId: request.id, epoch: request.epoch }))
      if (mounted.current && scope.current === request.scope) {
        if (draft.current.trim() === request.text) setText('')
        pending.current = null
      }
    } catch (reason) {
      if (mounted.current && scope.current === request.scope) setSendError((reason as Error).message)
    } finally { if (mounted.current) setSending(false) }
  }
  const status = connection === 'connected' ? paused ? '老师已暂停学生发言' : '已连接' : connection === 'connecting' ? '正在连接课堂…' : error
  return <section className="chat-panel" aria-label="课堂聊天">
    <div className="chat-status"><div><strong>{identity ? `${identity.className} · ${identity.name}` : teacher ? '教师视角' : '班级聊天室'}</strong><span role="status">{status}</span></div><span>{snapshot?.onlineStudents ?? 0} 位学生在线</span></div>
    <div className="chat-messages" ref={scroll} aria-label="消息列表" onScroll={() => {
      const element = scroll.current
      if (element) atBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight < 64
      if (atBottom.current) setNewMessages(false)
    }}>
      {snapshot && snapshot.messages.length === 0 && <div className="chat-empty"><MessageCircle size={30} /><strong>从一句问候开始</strong><p>文明交流，围绕课堂内容讨论。</p></div>}
      {snapshot?.messages.map(message => <article className={`chat-message ${message.sender.role === 'teacher' ? 'chat-message-teacher' : ''}`} key={message.id}>
        <div className="chat-message-meta"><strong>{message.sender.name}</strong>{message.sender.role === 'teacher' && <span className="chat-teacher-badge">老师</span>}<time dateTime={message.createdAt}>{new Date(message.createdAt).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}</time></div>
        <p>{message.text}</p>
      </article>)}
      {connection === 'closed' && <div className="chat-empty"><strong>{error}</strong>{!teacher && <a className="button button-outline" href="/checkin">前往签到</a>}</div>}
    </div>
    {newMessages && <button className="chat-new-messages" onClick={() => { if (scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight; atBottom.current = true; setNewMessages(false) }}>有新消息 · 回到底部</button>}
    <form className="chat-composer" onSubmit={event => { event.preventDefault(); void send() }}>
      <label className="sr-only" htmlFor="chat-message-input">聊天消息</label>
      <textarea id="chat-message-input" value={text} disabled={connection === 'closed' || paused} rows={3} maxLength={1000} placeholder={paused ? '老师暂停了发言，请稍候' : '分享你的想法…'} onChange={event => setText(event.target.value)} onKeyDown={event => {
        if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) { event.preventDefault(); void send() }
      }} />
      {sendError && <p className="form-error" role="alert">{sendError}，可再次点击发送重试。</p>}
      <div className="chat-composer-footer"><small className={count > 500 ? 'chat-over-limit' : ''}>{count}/500 · Shift+Enter 换行</small><Button disabled={!canSend} type="submit"><Send size={15} />{sending ? '发送中' : '发送'}</Button></div>
    </form>
    <p className="chat-retention">仅显示最近消息；消息暂存最多 2 小时，结束课堂、清空或重启后消失。</p>
  </section>
}
