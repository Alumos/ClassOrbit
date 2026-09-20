export type AttendanceStatus = 'present' | 'absent' | 'late' | 'leave'
export type NavigationKind = 'external' | 'site'
export type ScheduleChangeStatus = 'occupied' | 'rescheduled'

export type ClassItem = {
  id: number
  name: string
  grade: string
  classNo: string
  studentCount: number
  totalScore: number
  activeSessionId: number | null
  createdAt: string
}

export type Student = {
  id: number
  classId: number
  studentNo: string
  name: string
  score: number
  createdAt: string
}

export type ScoreEvent = {
  id: number
  delta: number
  reason: string
  reversalOf: number | null
  reversedAt: string | null
  reversible: boolean
  createdAt: string
}

export type AttendanceRecord = {
  studentId: number
  studentNo: string
  name: string
  status: AttendanceStatus
  checkedAt: string | null
  method: string
}

export type Attendance = {
  id: number
  classId: number
  className: string
  title: string
  course: string
  status: 'active' | 'closed'
  startedAt: string
  sessionAt: string
  endedAt: string | null
  deletedAt: string | null
  presentCount: number
  absentCount: number
  records: AttendanceRecord[]
}

export type AttendancePageData = { items: Attendance[]; nextCursor: number }

export type AttendanceSuggestion = {
  detected: boolean
  serverTime: string
  classId: number
  className: string
  course: string
  sessionAt: string
  period: number
  startTime: string
  endTime: string
  source: 'regular' | 'rescheduled' | ''
  message: string
}

export type Dashboard = { classCount: number; studentCount: number; totalScore: number; activeSessions: number }

export type SiteSettings = { title: string; subtitle: string; chatEnabled?: boolean }
export type AuthStatus = { initialized: boolean; authenticated: boolean; username: string }
export type BuildInfo = { version: string; commit: string }
export type PublicStudent = { id: number; studentNo: string; name: string; checkedIn: boolean }
export type PublicRoster = { sessionId: number; title: string; course: string; sessionAt: string; students: PublicStudent[] }
export type TeachingSiteSummary = { publicId: string; revision: string; sourceName: string; sourceSize: number; extractedSize: number; fileCount: number; updatedAt: string }
export type NavigationItem = { id: number; kind: NavigationKind; title: string; url: string; iconUrl: string | null; sortOrder: number; site?: TeachingSiteSummary }
export type Notify = (message: string, kind?: 'success' | 'error') => void
export type AuditLog = {
  id: number
  action: string
  entityType: string
  entityId: number | null
  summary: string
  details: string
  actor: string
  createdAt: string
}

export type ScheduleLesson = {
  id: number
  classId: number
  className: string
  course: string
  weekday: number
  period: number
  startTime: string
  endTime: string
  locationOdd: string
  locationEven: string
}

export type SchedulePeriod = { period: number; startTime: string; endTime: string }
export type ScheduleSettings = { semesterStart: string; semesterEnd: string; periods: SchedulePeriod[] }

export type ScheduleChange = {
  id: number
  lessonId: number
  date: string
  status: ScheduleChangeStatus
  newDate: string
  newStartTime: string
  newEndTime: string
  newClassId: number
  newClassName: string
  note: string
}

export type ScheduleData = { lessons: ScheduleLesson[]; changes: ScheduleChange[]; settings: ScheduleSettings }

export type ChatSender = { role: 'student' | 'teacher'; id: string; name: string }
export type ChatMessage = { id: string; seq: number; clientMessageId: string; sender: ChatSender; text: string; createdAt: string }
export type ChatSnapshot = { roomId: number; epoch: string; latestSeq: number; messages: ChatMessage[]; onlineStudents: number; paused: boolean }
export type ChatIdentity = { roomId: number; classId: number; className: string; title: string; studentId: number; name: string; expiresAt: number }
export type ChatRoom = { roomId: number; classId: number; className: string; title: string; messageCount: number; onlineStudents: number; paused: boolean }
