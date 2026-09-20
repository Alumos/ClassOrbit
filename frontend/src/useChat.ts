import { useEffect, useState } from 'react'
import { ChatError, chatApi } from './chatApi'
import type { ChatIdentity, ChatMessage, ChatRoom, ChatSnapshot } from './types'

export type ChatConnection = 'connecting' | 'connected' | 'reconnecting' | 'closed'
export function useChat(base: string) {
  const [snapshot, setSnapshot] = useState<ChatSnapshot | null>(null)
  const [identity, setIdentity] = useState<ChatIdentity | null>(null)
  const [connection, setConnection] = useState<ChatConnection>('connecting')
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    let source: EventSource | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let studentKey = ''
    const controller = new AbortController()
    setSnapshot(null); setIdentity(null); setConnection('connecting'); setError('')
    const retry = () => {
      source?.close(); source = null
      if (!active) return
      setConnection('reconnecting')
      setError('连接中断，正在自动重连')
      clearTimeout(timer)
      timer = setTimeout(() => void connect(), Math.min(30000, 1000 * 2 ** Math.min(attempt++, 5)) * (0.8 + Math.random() * 0.4))
    }
    const connect = async () => {
      if (!active) return
      try {
        if (base === '/public/chat') {
          const next = await chatApi<ChatIdentity>(`${base}/session`, { signal: controller.signal })
          if (!active) return
          const nextKey = `${next.roomId}:${next.studentId}`
          if (studentKey && studentKey !== nextKey) setSnapshot(null)
          studentKey = nextKey
          setIdentity(next)
        } else {
          const rooms = await chatApi<ChatRoom[]>('/chat/rooms', { signal: controller.signal })
          if (!active) return
          if (!rooms.some(room => base === `/chat/rooms/${room.roomId}`)) throw new ChatError('课堂已结束', 404)
        }
        const stream = new EventSource(`/api${base}/events`)
        source = stream
        const event = <T,>(name: string, handle: (data: T) => void) => {
          stream.addEventListener(name, incoming => {
            if (!active || source !== stream) return
            try { handle(JSON.parse((incoming as MessageEvent).data) as T) }
            catch { retry() }
          })
        }
        const replace = (data: ChatSnapshot) => {
          setSnapshot(data); setConnection('connected'); setError(''); attempt = 0
        }
        event<ChatSnapshot>('snapshot', replace)
        event<ChatSnapshot>('room_cleared', replace)
        event<{ epoch: string; message: ChatMessage }>('message', data => {
          setSnapshot(current => {
            if (!current || current.epoch !== data.epoch || current.messages.some(item => item.id === data.message.id)) return current
            return { ...current, latestSeq: Math.max(current.latestSeq, data.message.seq), messages: [...current.messages, data.message].slice(-200) }
          })
        })
        event<{ epoch: string; firstSeq: number }>('history_trimmed', data => {
          setSnapshot(current => current?.epoch === data.epoch ? { ...current, messages: current.messages.filter(message => message.seq >= data.firstSeq) } : current)
        })
        event<{ epoch: string; onlineStudents: number }>('presence', data => {
          setSnapshot(current => current?.epoch === data.epoch ? { ...current, onlineStudents: data.onlineStudents } : current)
        })
        event<{ epoch: string; paused: boolean }>('room_state', data => {
          setSnapshot(current => current?.epoch === data.epoch ? { ...current, paused: data.paused } : current)
        })
        stream.onerror = () => { if (active && source === stream) retry() }
      } catch (reason) {
        if (!active) return
        if (reason instanceof ChatError && ([401, 403, 404].includes(reason.status) || reason.code === 'chat_disabled')) {
          source?.close(); setSnapshot(null); setIdentity(null); setConnection('closed'); setError(reason.message)
        } else retry()
      }
    }
    void connect()
    return () => { active = false; source?.close(); clearTimeout(timer); controller.abort() }
  }, [base])

  return { snapshot, identity, connection, error }
}
