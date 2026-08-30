package main

import (
	"database/sql"
	"errors"
	"net/http"
	"time"
)

type attendanceSuggestion struct {
	Detected   bool   `json:"detected"`
	ServerTime string `json:"serverTime"`
	ClassID    int64  `json:"classId"`
	ClassName  string `json:"className"`
	Course     string `json:"course"`
	SessionAt  string `json:"sessionAt"`
	Period     int    `json:"period"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
	Source     string `json:"source"`
	Message    string `json:"message"`
}

func (s *server) getCurrentLesson(w http.ResponseWriter, _ *http.Request) {
	result, err := s.db.currentLesson(time.Now())
	respond(w, result, err)
}

func (s *store) currentLesson(now time.Time) (attendanceSuggestion, error) {
	result := attendanceSuggestion{
		ServerTime: now.Format(time.RFC3339),
		SessionAt:  now.Format("2006-01-02T15:04"),
		Message:    "当前时间不在已配置的有效课时内，请手动选择并确认。",
	}
	date, clock := now.Format("2006-01-02"), now.Format("15:04")

	// One-off rescheduled lessons take precedence over the regular timetable.
	err := s.QueryRow(`SELECT
COALESCE(ch.new_class_id,l.class_id),COALESCE(nc.name,c.name),l.course,l.period,
ch.new_start_time,ch.new_end_time
FROM schedule_changes ch
JOIN schedule_lessons l ON l.id=ch.lesson_id
JOIN classes c ON c.id=l.class_id
LEFT JOIN classes nc ON nc.id=ch.new_class_id
WHERE ch.status='rescheduled' AND ch.new_date=?
AND ch.new_start_time<=? AND ch.new_end_time>?
AND COALESCE(nc.deleted_at,c.deleted_at) IS NULL
ORDER BY ch.new_start_time LIMIT 1`, date, clock, clock).Scan(
		&result.ClassID, &result.ClassName, &result.Course, &result.Period,
		&result.StartTime, &result.EndTime)
	if err == nil {
		result.Detected = true
		result.Source = "rescheduled"
		result.SessionAt = date + "T" + result.StartTime
		result.Message = "已按服务器时间识别到换课安排，请核对班级、课程和时间后开放签到。"
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}

	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	err = s.QueryRow(`SELECT l.class_id,c.name,l.course,l.period,p.start_time,p.end_time
FROM schedule_lessons l JOIN classes c ON c.id=l.class_id
JOIN schedule_periods p ON p.period=l.period
JOIN schedule_settings ss ON ss.id=1
WHERE l.weekday=? AND p.start_time<=? AND p.end_time>?
AND c.deleted_at IS NULL
AND (ss.semester_start='' OR ? BETWEEN ss.semester_start AND ss.semester_end)
AND NOT EXISTS (
  SELECT 1 FROM schedule_changes ch
  WHERE ch.lesson_id=l.id AND ch.original_date=?
  AND ch.status IN ('occupied','rescheduled')
)
ORDER BY l.period LIMIT 1`, weekday, clock, clock, date, date).Scan(
		&result.ClassID, &result.ClassName, &result.Course, &result.Period,
		&result.StartTime, &result.EndTime)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Detected = true
	result.Source = "regular"
	result.SessionAt = date + "T" + result.StartTime
	result.Message = "已按服务器时间识别到当前课时，请核对班级、课程和时间后开放签到。"
	return result, nil
}
