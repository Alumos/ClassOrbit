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

export type ScoreEvent = { id: number; delta: number; reason: string; createdAt: string }

export type AttendanceRecord = {
  studentId: number
  studentNo: string
  name: string
  status: 'present' | 'absent' | 'late' | 'leave'
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
  presentCount: number
  absentCount: number
  records: AttendanceRecord[]
}

export type Dashboard = { classCount: number; studentCount: number; totalScore: number; activeSessions: number }

export type SiteSettings = { title: string; subtitle: string }
export type NavigationItem = { id: number; title: string; url: string; iconUrl: string | null; sortOrder: number }
export type Notify = (message: string, kind?: 'success' | 'error') => void

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
  status: 'occupied' | 'rescheduled'
  newDate: string
  newStartTime: string
  newEndTime: string
  newClassId: number
  newClassName: string
  note: string
}

export type ScheduleData = { lessons: ScheduleLesson[]; changes: ScheduleChange[]; settings: ScheduleSettings }
