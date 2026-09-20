import { useCallback, useEffect, useRef, useState } from 'react'
import { MessageCircle, Pause, Play, Trash2 } from 'lucide-react'
import { ChatPanel } from '../ChatPanel'
import { chatApi, chatJSON } from '../chatApi'
import { Dialog } from '../dialog'
import type { ChatRoom, ChatSnapshot, Notify } from '../types'
import { Button, EmptyState } from '../ui'

export function ChatManagementPage({ classId, notify }: { classId: number; notify: Notify }) {
  const managing = useRef(false)
  const [room, setRoom] = useState<ChatRoom | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [clearOpen, setClearOpen] = useState(false)
  const refresh = useCallback(async (signal: AbortSignal) => {
    if (managing.current) return
    if (!classId) { setRoom(null); setLoading(false); return }
    try {
      const rooms = await chatApi<ChatRoom[]>(`/chat/rooms?classId=${classId}`, { signal })
      if (!signal.aborted && !managing.current) { setRoom(rooms[0] ?? null); setError('') }
    } catch (reason) { if (!signal.aborted) setError((reason as Error).message) }
    finally { if (!signal.aborted) setLoading(false) }
  }, [classId])
  useEffect(() => {
    const controller = new AbortController()
    setRoom(null); setLoading(true); setError(''); setClearOpen(false)
    void refresh(controller.signal)
    const timer = setInterval(() => void refresh(controller.signal), 15000)
    return () => { controller.abort(); clearInterval(timer) }
  }, [refresh])
  const manage = async (clear: boolean) => {
    if (!room) return
    const roomId = room.roomId
    managing.current = true
    setBusy(true)
    try {
      const result = await chatApi<ChatSnapshot>(`/chat/rooms/${roomId}${clear ? '/messages' : ''}`, clear ? chatJSON('DELETE') : chatJSON('PATCH', { paused: !room.paused }))
      setRoom(current => current?.roomId === roomId ? { ...current, paused: result.paused } : current)
      setClearOpen(false); notify(clear ? '本场聊天消息已清空' : result.paused ? '已暂停学生发言' : '已恢复学生发言')
    } catch (reason) { notify((reason as Error).message, 'error') }
    finally { managing.current = false; setBusy(false) }
  }
  return <div className="chat-management">
    <div className="chat-management-heading"><div><h1>班级聊天室</h1><p>{room?.title || '选择班级，管理本场课堂讨论。'}</p></div>{room && <div className="chat-header-actions"><Button variant="outline" disabled={busy} onClick={() => void manage(false)}>{room.paused ? <Play size={15} /> : <Pause size={15} />}{room.paused ? '恢复发言' : '暂停发言'}</Button><Button variant="danger" disabled={busy} onClick={() => setClearOpen(true)}><Trash2 size={15} />清空消息</Button></div>}</div>
    {loading ? <p role="status">正在读取课堂…</p> : error ? <EmptyState icon={<MessageCircle />} title="聊天室暂不可用" detail={error} /> : room ? <ChatPanel key={room.roomId} base={`/chat/rooms/${room.roomId}`} teacher /> : <EmptyState icon={<MessageCircle />} title="暂无进行中的课堂" detail="请先为当前班级发起签到；学生签到后即可从学习导航进入聊天室。" />}
    <Dialog open={clearOpen} onOpenChange={setClearOpen} title={`清空 ${room?.className || ''} 的聊天消息？`} description="本场已发送的消息将从服务器和在线页面清除，无法恢复。学生仍可继续发言，考勤和积分不受影响。" footer={<><Button variant="outline" disabled={busy} onClick={() => setClearOpen(false)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void manage(true)}>确认清空</Button></>}><p>消息不会进入数据库备份；已经保存的截图无法收回。</p></Dialog>
  </div>
}
