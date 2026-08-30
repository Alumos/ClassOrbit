package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	errNotFound = errors.New("not found")
	errConflict = errors.New("conflict")
)

type store struct {
	*sql.DB
	path string
}

type classInput struct {
	Name    string `json:"name"`
	Grade   string `json:"grade"`
	ClassNo string `json:"classNo"`
}
type studentInput struct {
	StudentNo string `json:"studentNo"`
	Name      string `json:"name"`
}
type scoreInput struct {
	Delta  int    `json:"delta"`
	Reason string `json:"reason"`
}
type attendanceInput struct {
	ClassID   int64  `json:"classId"`
	Title     string `json:"title"`
	Course    string `json:"course"`
	SessionAt string `json:"sessionAt"`
}

type scheduleInput struct {
	ClassID      int64  `json:"classId"`
	Course       string `json:"course"`
	Weekday      int    `json:"weekday"`
	Period       int    `json:"period"`
	LocationOdd  string `json:"locationOdd"`
	LocationEven string `json:"locationEven"`
}

type scheduleSettingsInput struct {
	SemesterStart string           `json:"semesterStart"`
	SemesterEnd   string           `json:"semesterEnd"`
	Periods       []schedulePeriod `json:"periods"`
}

type scheduleChangeInput struct {
	Date         string `json:"date"`
	Status       string `json:"status"`
	NewDate      string `json:"newDate"`
	NewStartTime string `json:"newStartTime"`
	NewEndTime   string `json:"newEndTime"`
	NewClassID   int64  `json:"newClassId"`
	Note         string `json:"note"`
}

type siteSettings struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
}

type teacherAccount struct {
	Username     string
	PasswordHash string
}

type navigationLinkInput struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	IconURL string `json:"iconUrl"`
}

type navigationLink struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	IconURL   string `json:"iconUrl"`
	SortOrder int    `json:"sortOrder"`
}

type classRow struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Grade           string `json:"grade"`
	ClassNo         string `json:"classNo"`
	StudentCount    int    `json:"studentCount"`
	TotalScore      int    `json:"totalScore"`
	ActiveSessionID *int64 `json:"activeSessionId"`
	CreatedAt       string `json:"createdAt"`
}
type studentRow struct {
	ID        int64  `json:"id"`
	ClassID   int64  `json:"classId"`
	StudentNo string `json:"studentNo"`
	Name      string `json:"name"`
	Score     int    `json:"score"`
	CreatedAt string `json:"createdAt"`
}
type scoreEvent struct {
	ID         int64   `json:"id"`
	Delta      int     `json:"delta"`
	Reason     string  `json:"reason"`
	ReversalOf *int64  `json:"reversalOf"`
	ReversedAt *string `json:"reversedAt"`
	Reversible bool    `json:"reversible"`
	CreatedAt  string  `json:"createdAt"`
}
type attendanceRecord struct {
	StudentID int64   `json:"studentId"`
	StudentNo string  `json:"studentNo"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CheckedAt *string `json:"checkedAt"`
	Method    string  `json:"method"`
}
type attendanceView struct {
	ID           int64              `json:"id"`
	ClassID      int64              `json:"classId"`
	ClassName    string             `json:"className"`
	Title        string             `json:"title"`
	Course       string             `json:"course"`
	Status       string             `json:"status"`
	StartedAt    string             `json:"startedAt"`
	SessionAt    string             `json:"sessionAt"`
	EndedAt      *string            `json:"endedAt"`
	DeletedAt    *string            `json:"deletedAt"`
	PresentCount int                `json:"presentCount"`
	AbsentCount  int                `json:"absentCount"`
	Records      []attendanceRecord `json:"records"`
}

type attendancePage struct {
	Items      []attendanceView `json:"items"`
	NextCursor int64            `json:"nextCursor"`
}

type scheduleLesson struct {
	ID           int64  `json:"id"`
	ClassID      int64  `json:"classId"`
	ClassName    string `json:"className"`
	Course       string `json:"course"`
	Weekday      int    `json:"weekday"`
	Period       int    `json:"period"`
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime"`
	LocationOdd  string `json:"locationOdd"`
	LocationEven string `json:"locationEven"`
}

type schedulePeriod struct {
	Period    int    `json:"period"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type scheduleSettings struct {
	SemesterStart string           `json:"semesterStart"`
	SemesterEnd   string           `json:"semesterEnd"`
	Periods       []schedulePeriod `json:"periods"`
}

type scheduleChange struct {
	ID           int64  `json:"id"`
	LessonID     int64  `json:"lessonId"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	NewDate      string `json:"newDate"`
	NewStartTime string `json:"newStartTime"`
	NewEndTime   string `json:"newEndTime"`
	NewClassID   int64  `json:"newClassId"`
	NewClassName string `json:"newClassName"`
	Note         string `json:"note"`
}

type scheduleData struct {
	Lessons  []scheduleLesson `json:"lessons"`
	Changes  []scheduleChange `json:"changes"`
	Settings scheduleSettings `json:"settings"`
}

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &store{DB: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) migrate() error {
	_, err := s.Exec(`
CREATE TABLE IF NOT EXISTS classes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  grade TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE TABLE IF NOT EXISTS students (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
  student_no TEXT NOT NULL,
  name TEXT NOT NULL,
  score INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  UNIQUE(class_id, student_no)
);
CREATE TABLE IF NOT EXISTS score_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  delta INTEGER NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_score_events_student ON score_events(student_id, id DESC);
CREATE TABLE IF NOT EXISTS attendance_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','closed')),
  started_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  ended_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_session ON attendance_sessions(class_id) WHERE status='active';
CREATE TABLE IF NOT EXISTS attendance_records (
  session_id INTEGER NOT NULL REFERENCES attendance_sessions(id) ON DELETE CASCADE,
  student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'absent' CHECK(status IN ('present','absent','late','leave')),
  checked_at TEXT,
  method TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(session_id, student_id)
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT OR IGNORE INTO settings(key,value) VALUES('site_title','智创课堂');
INSERT OR IGNORE INTO settings(key,value) VALUES('site_subtitle','小学信息科技课');
CREATE TABLE IF NOT EXISTS teacher_accounts (
  id INTEGER PRIMARY KEY CHECK(id=1),
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE TABLE IF NOT EXISTS teacher_sessions (
  token_hash TEXT PRIMARY KEY,
  teacher_id INTEGER NOT NULL DEFAULT 1 REFERENCES teacher_accounts(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_teacher_sessions_expires ON teacher_sessions(expires_at);
CREATE TABLE IF NOT EXISTS navigation_links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  url TEXT NOT NULL,
  icon_url TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE TABLE IF NOT EXISTS schedule_lessons (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
  course TEXT NOT NULL,
  weekday INTEGER NOT NULL CHECK(weekday BETWEEN 1 AND 7),
  period INTEGER NOT NULL DEFAULT 0,
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL,
  location_odd TEXT NOT NULL DEFAULT '机房 1',
  location_even TEXT NOT NULL DEFAULT '机房 1',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  UNIQUE(class_id,weekday,start_time)
);
CREATE INDEX IF NOT EXISTS idx_schedule_lessons_weekday_time ON schedule_lessons(weekday,start_time);
CREATE TABLE IF NOT EXISTS schedule_changes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  lesson_id INTEGER NOT NULL REFERENCES schedule_lessons(id) ON DELETE CASCADE,
  original_date TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('occupied','rescheduled')),
  new_date TEXT NOT NULL DEFAULT '',
  new_start_time TEXT NOT NULL DEFAULT '',
  new_end_time TEXT NOT NULL DEFAULT '',
  new_class_id INTEGER REFERENCES classes(id) ON DELETE SET NULL,
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  UNIQUE(lesson_id,original_date)
);
CREATE INDEX IF NOT EXISTS idx_schedule_changes_dates ON schedule_changes(original_date,new_date);
CREATE INDEX IF NOT EXISTS idx_schedule_changes_new_date ON schedule_changes(new_date);
CREATE TABLE IF NOT EXISTS schedule_settings (
  id INTEGER PRIMARY KEY CHECK(id=1),
  semester_start TEXT NOT NULL DEFAULT '',
  semester_end TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO schedule_settings(id,semester_start,semester_end) VALUES(1,'','');
CREATE TABLE IF NOT EXISTS schedule_periods (
  period INTEGER PRIMARY KEY CHECK(period BETWEEN 1 AND 7),
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL
);
INSERT OR IGNORE INTO schedule_periods(period,start_time,end_time) VALUES
(1,'08:00','08:40'),(2,'08:50','09:30'),(3,'09:40','10:20'),
(4,'13:30','14:10'),(5,'14:20','15:00'),(6,'15:10','15:50'),(7,'16:00','16:40');
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn("classes", "class_no", `ALTER TABLE classes ADD COLUMN class_no TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn("attendance_sessions", "course", `ALTER TABLE attendance_sessions ADD COLUMN course TEXT NOT NULL DEFAULT '信息课'`); err != nil {
		return err
	}
	if err := s.ensureColumn("attendance_sessions", "session_at", `ALTER TABLE attendance_sessions ADD COLUMN session_at TEXT`); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_lessons", "period", `ALTER TABLE schedule_lessons ADD COLUMN period INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_lessons", "location_odd", `ALTER TABLE schedule_lessons ADD COLUMN location_odd TEXT NOT NULL DEFAULT '机房 1'`); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_lessons", "location_even", `ALTER TABLE schedule_lessons ADD COLUMN location_even TEXT NOT NULL DEFAULT '机房 1'`); err != nil {
		return err
	}
	_, err = s.Exec(`
UPDATE attendance_sessions SET session_at=started_at WHERE session_at IS NULL OR session_at='';
CREATE INDEX IF NOT EXISTS idx_attendance_sessions_class_date ON attendance_sessions(class_id,session_at DESC);
CREATE INDEX IF NOT EXISTS idx_attendance_sessions_date ON attendance_sessions(session_at DESC);
DROP INDEX IF EXISTS idx_students_class;
WITH period_matches AS (
  SELECT l.id AS lesson_id,p.period,
    ROW_NUMBER() OVER(PARTITION BY l.id ORDER BY abs(
      (CAST(substr(p.start_time,1,2) AS INTEGER)*60+CAST(substr(p.start_time,4,2) AS INTEGER))-
      (CAST(substr(l.start_time,1,2) AS INTEGER)*60+CAST(substr(l.start_time,4,2) AS INTEGER))
    )) AS position
  FROM schedule_lessons l CROSS JOIN schedule_periods p WHERE l.period=0
)
UPDATE schedule_lessons SET period=COALESCE((SELECT period FROM period_matches WHERE lesson_id=schedule_lessons.id AND position=1),1) WHERE period=0;
`)
	if err != nil {
		return err
	}
	if err := s.resolveSchedulePeriodCollisions(); err != nil {
		return err
	}
	return s.applyVersionedMigrations()
}

func (s *store) resolveSchedulePeriodCollisions() error {
	periodRows, err := s.Query(`SELECT period,start_time,end_time FROM schedule_periods ORDER BY period`)
	if err != nil {
		return err
	}
	periods := []schedulePeriod{}
	for periodRows.Next() {
		var period schedulePeriod
		if err := periodRows.Scan(&period.Period, &period.StartTime, &period.EndTime); err != nil {
			periodRows.Close()
			return err
		}
		periods = append(periods, period)
	}
	if err := periodRows.Err(); err != nil {
		periodRows.Close()
		return err
	}
	periodRows.Close()
	rows, err := s.Query(`SELECT id,weekday,period,start_time FROM schedule_lessons ORDER BY weekday,start_time,id`)
	if err != nil {
		return err
	}
	type item struct {
		id        int64
		weekday   int
		period    int
		startTime string
	}
	items := []item{}
	for rows.Next() {
		var lesson item
		if err := rows.Scan(&lesson.id, &lesson.weekday, &lesson.period, &lesson.startTime); err != nil {
			rows.Close()
			return err
		}
		items = append(items, lesson)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	used := map[int]map[int]bool{}
	clockMinutes := func(value string) int {
		parsed, _ := time.Parse("15:04", value)
		return parsed.Hour()*60 + parsed.Minute()
	}
	for _, lesson := range items {
		if used[lesson.weekday] == nil {
			used[lesson.weekday] = map[int]bool{}
		}
		if !used[lesson.weekday][lesson.period] {
			used[lesson.weekday][lesson.period] = true
			continue
		}
		selected := schedulePeriod{}
		bestDistance := 24 * 60
		for _, period := range periods {
			if used[lesson.weekday][period.Period] {
				continue
			}
			distance := clockMinutes(period.StartTime) - clockMinutes(lesson.startTime)
			if distance < 0 {
				distance = -distance
			}
			if distance < bestDistance {
				selected, bestDistance = period, distance
			}
		}
		if selected.Period == 0 {
			continue
		}
		if _, err := s.Exec(`UPDATE schedule_lessons SET period=?,start_time=?,end_time=? WHERE id=?`, selected.Period, selected.StartTime, selected.EndTime, lesson.id); err != nil {
			return err
		}
		used[lesson.weekday][selected.Period] = true
	}
	return nil
}

func (s *store) ensureColumn(table, column, ddl string) error {
	rows, err := s.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = s.Exec(ddl)
	return err
}

func (s *store) dashboard() (map[string]any, error) {
	var classes, students, score, active int
	err := s.QueryRow(`SELECT
(SELECT COUNT(*) FROM classes WHERE deleted_at IS NULL),
(SELECT COUNT(*) FROM students WHERE deleted_at IS NULL),
COALESCE((SELECT SUM(score) FROM students WHERE deleted_at IS NULL),0),
(SELECT COUNT(*) FROM attendance_sessions WHERE status='active' AND deleted_at IS NULL)`).Scan(&classes, &students, &score, &active)
	return map[string]any{"classCount": classes, "studentCount": students, "totalScore": score, "activeSessions": active}, err
}

func (s *store) classes(publicOnly bool) ([]classRow, error) {
	where := " WHERE c.deleted_at IS NULL"
	if publicOnly {
		where += " AND EXISTS (SELECT 1 FROM attendance_sessions a2 WHERE a2.class_id=c.id AND a2.status='active' AND a2.deleted_at IS NULL)"
	}
	rows, err := s.Query(`SELECT c.id,c.name,c.grade,c.class_no,c.created_at,
(SELECT COUNT(*) FROM students st WHERE st.class_id=c.id AND st.deleted_at IS NULL),
(SELECT COALESCE(SUM(score),0) FROM students st WHERE st.class_id=c.id AND st.deleted_at IS NULL),
(SELECT id FROM attendance_sessions a WHERE a.class_id=c.id AND a.status='active' AND a.deleted_at IS NULL LIMIT 1)
FROM classes c` + where + ` ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []classRow{}
	for rows.Next() {
		var c classRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Grade, &c.ClassNo, &c.CreatedAt, &c.StudentCount, &c.TotalScore, &c.ActiveSessionID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *store) createClass(in classInput) (classRow, error) {
	name := className(in.Grade, in.ClassNo)
	tx, err := s.Begin()
	if err != nil {
		return classRow{}, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE classes SET grade=?,class_no=?,deleted_at=NULL
WHERE name=? AND deleted_at IS NOT NULL`, strings.TrimSpace(in.Grade), strings.TrimSpace(in.ClassNo), name)
	if err != nil {
		return classRow{}, err
	}
	changed, _ := res.RowsAffected()
	var id int64
	if changed == 1 {
		if err := tx.QueryRow(`SELECT id FROM classes WHERE name=?`, name).Scan(&id); err != nil {
			return classRow{}, err
		}
	} else {
		res, err = tx.Exec(`INSERT INTO classes(name,grade,class_no) VALUES(?,?,?)`, name, strings.TrimSpace(in.Grade), strings.TrimSpace(in.ClassNo))
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return classRow{}, fmt.Errorf("%w: 班级名称已存在", errConflict)
			}
			return classRow{}, err
		}
		id, _ = res.LastInsertId()
	}
	if err := addAudit(tx, "class.create", "class", id, "创建班级 "+name, ""); err != nil {
		return classRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return classRow{}, err
	}
	return s.classByID(id)
}
func (s *store) updateClass(id int64, in classInput) (classRow, error) {
	res, err := s.Exec(`UPDATE classes SET name=?,grade=?,class_no=? WHERE id=? AND deleted_at IS NULL`, className(in.Grade, in.ClassNo), strings.TrimSpace(in.Grade), strings.TrimSpace(in.ClassNo), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return classRow{}, fmt.Errorf("%w: 班级已存在", errConflict)
		}
		return classRow{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return classRow{}, errNotFound
	}
	return s.classByID(id)
}
func (s *store) classByID(id int64) (classRow, error) {
	var c classRow
	err := s.QueryRow(`SELECT c.id,c.name,c.grade,c.class_no,c.created_at,
(SELECT COUNT(*) FROM students st WHERE st.class_id=c.id AND st.deleted_at IS NULL),
(SELECT COALESCE(SUM(score),0) FROM students st WHERE st.class_id=c.id AND st.deleted_at IS NULL),
(SELECT id FROM attendance_sessions a WHERE a.class_id=c.id AND a.status='active' AND a.deleted_at IS NULL LIMIT 1)
FROM classes c WHERE c.id=? AND c.deleted_at IS NULL`, id).Scan(&c.ID, &c.Name, &c.Grade, &c.ClassNo, &c.CreatedAt, &c.StudentCount, &c.TotalScore, &c.ActiveSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return c, err
}

func className(grade, classNo string) string {
	return strings.TrimSpace(grade) + " " + strings.TrimSpace(classNo) + " 班"
}
func (s *store) deleteClass(id int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name string
	if err := tx.QueryRow(`SELECT name FROM classes WHERE id=? AND deleted_at IS NULL`, id).Scan(&name); errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	} else if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE classes SET deleted_at=datetime('now','localtime') WHERE id=? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errNotFound
	}
	if _, err := tx.Exec(`UPDATE students SET deleted_at=datetime('now','localtime') WHERE class_id=? AND deleted_at IS NULL`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE attendance_sessions SET status='closed',
ended_at=COALESCE(ended_at,datetime('now','localtime')) WHERE class_id=? AND status='active' AND deleted_at IS NULL`, id); err != nil {
		return err
	}
	if err := addAudit(tx, "class.delete", "class", id, "删除班级 "+name, "班级和当前名单已软删除，历史记录保留"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) students(classID int64, sortKey string) ([]studentRow, error) {
	order := `CAST(student_no AS INTEGER), student_no, id`
	if sortKey == "score_asc" {
		order = `score ASC, CAST(student_no AS INTEGER), student_no`
	} else if sortKey == "score_desc" {
		order = `score DESC, CAST(student_no AS INTEGER), student_no`
	}
	rows, err := s.Query(`SELECT id,class_id,student_no,name,score,created_at FROM students WHERE class_id=? AND deleted_at IS NULL ORDER BY `+order, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []studentRow{}
	for rows.Next() {
		var st studentRow
		if err := rows.Scan(&st.ID, &st.ClassID, &st.StudentNo, &st.Name, &st.Score, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *store) integrationClasses() ([]integrationClass, error) {
	rows, err := s.Query(`SELECT c.id,c.name,st.student_no,st.id,st.name
FROM classes c
LEFT JOIN students st ON st.class_id=c.id AND st.deleted_at IS NULL
WHERE c.deleted_at IS NULL
ORDER BY c.id,CAST(st.student_no AS INTEGER),st.student_no,st.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []integrationClass{}
	var currentClassID int64
	for rows.Next() {
		var classID int64
		var className string
		var studentNo, studentName sql.NullString
		var studentID sql.NullInt64
		if err := rows.Scan(&classID, &className, &studentNo, &studentID, &studentName); err != nil {
			return nil, err
		}
		if len(out) == 0 || currentClassID != classID {
			currentClassID = classID
			out = append(out, integrationClass{Name: className, Students: []integrationStudent{}})
		}
		if !studentID.Valid {
			continue
		}
		id := strings.TrimSpace(studentNo.String)
		if id == "" {
			id = strconv.FormatInt(studentID.Int64, 10)
		}
		out[len(out)-1].Students = append(out[len(out)-1].Students, integrationStudent{ID: id, Name: studentName.String})
	}
	return out, rows.Err()
}
func (s *store) createStudent(classID int64, in studentInput) (studentRow, error) {
	tx, err := s.Begin()
	if err != nil {
		return studentRow{}, err
	}
	defer tx.Rollback()
	var classExists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM classes WHERE id=? AND deleted_at IS NULL)`, classID).Scan(&classExists); err != nil {
		return studentRow{}, err
	}
	if !classExists {
		return studentRow{}, errNotFound
	}
	studentNo, name := strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name)
	res, err := tx.Exec(`UPDATE students SET name=?,deleted_at=NULL
WHERE class_id=? AND student_no=? AND deleted_at IS NOT NULL`, name, classID, studentNo)
	if err != nil {
		return studentRow{}, err
	}
	changed, _ := res.RowsAffected()
	var id int64
	if changed == 1 {
		if err := tx.QueryRow(`SELECT id FROM students WHERE class_id=? AND student_no=?`, classID, studentNo).Scan(&id); err != nil {
			return studentRow{}, err
		}
	} else {
		res, err = tx.Exec(`INSERT INTO students(class_id,student_no,name) VALUES(?,?,?)`, classID, studentNo, name)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return studentRow{}, fmt.Errorf("%w: 该班已有相同学号", errConflict)
			}
			return studentRow{}, err
		}
		id, _ = res.LastInsertId()
	}
	if err := addAudit(tx, "student.create", "student", id, "添加学生 "+name, "student_no="+studentNo); err != nil {
		return studentRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return studentRow{}, err
	}
	return s.studentByID(id)
}
func (s *store) studentByID(id int64) (studentRow, error) {
	var st studentRow
	err := s.QueryRow(`SELECT id,class_id,student_no,name,score,created_at
FROM students WHERE id=? AND deleted_at IS NULL`, id).Scan(&st.ID, &st.ClassID, &st.StudentNo, &st.Name, &st.Score, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return st, err
}
func (s *store) updateStudent(id int64, in studentInput) (studentRow, error) {
	res, err := s.Exec(`UPDATE students SET student_no=?,name=? WHERE id=? AND deleted_at IS NULL`, strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return studentRow{}, fmt.Errorf("%w: 该班已有相同学号", errConflict)
		}
		return studentRow{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return studentRow{}, errNotFound
	}
	return s.studentByID(id)
}
func (s *store) deleteStudent(id int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name, studentNo string
	if err := tx.QueryRow(`SELECT name,student_no FROM students WHERE id=? AND deleted_at IS NULL`, id).Scan(&name, &studentNo); errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	} else if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE students SET deleted_at=datetime('now','localtime') WHERE id=? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errNotFound
	}
	if err := addAudit(tx, "student.delete", "student", id, "删除学生 "+name, "student_no="+studentNo+"；历史考勤快照保留"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *store) importStudents(classID int64, students []studentInput) (map[string]int, error) {
	tx, err := s.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var classExists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM classes WHERE id=? AND deleted_at IS NULL)`, classID).Scan(&classExists); err != nil {
		return nil, err
	}
	if !classExists {
		return nil, errNotFound
	}
	added, restored, skipped := 0, 0, 0
	for _, st := range students {
		var id int64
		var deletedAt sql.NullString
		err := tx.QueryRow(`SELECT id,deleted_at FROM students WHERE class_id=? AND student_no=?`, classID, st.StudentNo).Scan(&id, &deletedAt)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.Exec(`INSERT INTO students(class_id,student_no,name) VALUES(?,?,?)`, classID, st.StudentNo, st.Name); err != nil {
				return nil, err
			}
			added++
			continue
		}
		if err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			if _, err := tx.Exec(`UPDATE students SET name=?,deleted_at=NULL WHERE id=?`, st.Name, id); err != nil {
				return nil, err
			}
			restored++
		} else {
			skipped++
		}
	}
	if err := addAudit(tx, "student.import", "class", classID, "导入学生名单", fmt.Sprintf("added=%d, restored=%d, skipped=%d", added, restored, skipped)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]int{"added": added, "restored": restored, "skipped": skipped, "total": len(students)}, nil
}

func (s *store) changeScore(studentID int64, in scoreInput) (studentRow, error) {
	tx, err := s.Begin()
	if err != nil {
		return studentRow{}, err
	}
	defer tx.Rollback()
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		if in.Delta > 0 {
			reason = "课堂加分"
		} else {
			reason = "课堂扣分"
		}
	}
	res, err := tx.Exec(`UPDATE students SET score=score+? WHERE id=? AND deleted_at IS NULL`, in.Delta, studentID)
	if err != nil {
		return studentRow{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return studentRow{}, errNotFound
	}
	if _, err = tx.Exec(`INSERT INTO score_events(student_id,delta,reason) VALUES(?,?,?)`, studentID, in.Delta, reason); err != nil {
		return studentRow{}, err
	}
	if err := addAudit(tx, "score.change", "student", studentID, "调整学生积分", fmt.Sprintf("delta=%d, reason=%s", in.Delta, reason)); err != nil {
		return studentRow{}, err
	}
	if err = tx.Commit(); err != nil {
		return studentRow{}, err
	}
	return s.studentByID(studentID)
}
func (s *store) scoreEvents(studentID int64) ([]scoreEvent, error) {
	rows, err := s.Query(`SELECT id,delta,reason,reversal_of,reversed_at,
CASE WHEN reversal_of IS NULL AND reversed_at IS NULL THEN 1 ELSE 0 END,created_at
FROM score_events WHERE student_id=? ORDER BY id DESC LIMIT 50`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []scoreEvent{}
	for rows.Next() {
		var e scoreEvent
		if err := rows.Scan(&e.ID, &e.Delta, &e.Reason, &e.ReversalOf, &e.ReversedAt, &e.Reversible, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *store) createAttendance(in attendanceInput) (attendanceView, error) {
	tx, err := s.Begin()
	if err != nil {
		return attendanceView{}, err
	}
	defer tx.Rollback()
	var active int64
	err = tx.QueryRow(`SELECT id FROM attendance_sessions
WHERE class_id=? AND status='active' AND deleted_at IS NULL`, in.ClassID).Scan(&active)
	if err == nil {
		return attendanceView{}, fmt.Errorf("%w: 该班已有进行中的点名", errConflict)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return attendanceView{}, err
	}
	var studentCount int
	var currentClassName string
	if err = tx.QueryRow(`SELECT c.name,
(SELECT COUNT(*) FROM students st WHERE st.class_id=c.id AND st.deleted_at IS NULL)
FROM classes c WHERE c.id=? AND c.deleted_at IS NULL`, in.ClassID).Scan(&currentClassName, &studentCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return attendanceView{}, errNotFound
		}
		return attendanceView{}, err
	}
	if studentCount == 0 {
		return attendanceView{}, fmt.Errorf("%w: 班级名单为空，无法发起点名", errConflict)
	}
	sessionTime := time.Now()
	if value := strings.TrimSpace(in.SessionAt); value != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02T15:04", value, time.Local)
		if parseErr != nil {
			return attendanceView{}, fmt.Errorf("%w: 上课日期时间格式不正确", errConflict)
		}
		sessionTime = parsed
	}
	course := strings.TrimSpace(in.Course)
	if course == "" {
		course = "信息课"
	}
	title := attendanceTitle(currentClassName, sessionTime)
	res, err := tx.Exec(`INSERT INTO attendance_sessions(class_id,title,course,session_at,class_name_snapshot)
VALUES(?,?,?,?,?)`, in.ClassID, title, course, sessionTime.Format("2006-01-02 15:04:05"), currentClassName)
	if err != nil {
		return attendanceView{}, err
	}
	id, _ := res.LastInsertId()
	if _, err = tx.Exec(`INSERT INTO attendance_records(
session_id,student_id,status,student_no_snapshot,student_name_snapshot)
SELECT ?,id,'absent',student_no,name FROM students
WHERE class_id=? AND deleted_at IS NULL`, id, in.ClassID); err != nil {
		return attendanceView{}, err
	}
	if err := addAudit(tx, "attendance.create", "attendance", id, "发起课堂点名 "+title, "course="+course); err != nil {
		return attendanceView{}, err
	}
	if err = tx.Commit(); err != nil {
		return attendanceView{}, err
	}
	return s.attendanceByID(id)
}
func (s *store) attendance(classID int64, date string, trash bool, cursor int64, limit int) (attendancePage, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	query := `SELECT a.id,a.class_id,
COALESCE(NULLIF(a.class_name_snapshot,''),c.name,'已删除班级'),
a.course,a.status,a.started_at,a.session_at,a.ended_at,a.deleted_at,
COALESCE(SUM(CASE WHEN r.status IN ('present','late') THEN 1 ELSE 0 END),0),
COALESCE(SUM(CASE WHEN r.status='absent' THEN 1 ELSE 0 END),0)
FROM attendance_sessions a
LEFT JOIN classes c ON c.id=a.class_id
LEFT JOIN attendance_records r ON r.session_id=a.id`
	args := []any{}
	conditions := []string{`a.deleted_at IS ` + map[bool]string{true: "NOT NULL", false: "NULL"}[trash]}
	if classID > 0 {
		conditions = append(conditions, `a.class_id=?`)
		args = append(args, classID)
	}
	if date != "" {
		conditions = append(conditions, `a.session_at>=? AND a.session_at<date(?,'+1 day')`)
		args = append(args, date, date)
	}
	if cursor > 0 {
		conditions = append(conditions, `a.id<?`)
		args = append(args, cursor)
	}
	query += ` WHERE ` + strings.Join(conditions, ` AND `)
	// Cursor pagination must use the same monotonic key as the ordering. An
	// older active session sorted ahead of newer closed sessions would otherwise
	// appear again after applying a.id < cursor on the next page.
	query += ` GROUP BY a.id ORDER BY a.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.Query(query, args...)
	if err != nil {
		return attendancePage{}, err
	}
	defer rows.Close()
	out := []attendanceView{}
	for rows.Next() {
		var v attendanceView
		if err := rows.Scan(&v.ID, &v.ClassID, &v.ClassName, &v.Course, &v.Status, &v.StartedAt, &v.SessionAt, &v.EndedAt, &v.DeletedAt, &v.PresentCount, &v.AbsentCount); err != nil {
			return attendancePage{}, err
		}
		v.Title = attendanceTitleFromString(v.ClassName, v.SessionAt)
		v.Records = []attendanceRecord{}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return attendancePage{}, err
	}
	page := attendancePage{Items: out}
	if len(out) > limit {
		page.Items = out[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}
func (s *store) attendanceByID(id int64) (attendanceView, error) {
	var v attendanceView
	err := s.QueryRow(`SELECT a.id,a.class_id,
COALESCE(NULLIF(a.class_name_snapshot,''),c.name,'已删除班级'),
a.title,a.course,a.status,a.started_at,a.session_at,a.ended_at,a.deleted_at,
COALESCE(SUM(CASE WHEN r.status IN ('present','late') THEN 1 ELSE 0 END),0),
COALESCE(SUM(CASE WHEN r.status='absent' THEN 1 ELSE 0 END),0)
FROM attendance_sessions a LEFT JOIN classes c ON c.id=a.class_id
LEFT JOIN attendance_records r ON r.session_id=a.id
WHERE a.id=? GROUP BY a.id`, id).Scan(
		&v.ID, &v.ClassID, &v.ClassName, &v.Title, &v.Course, &v.Status,
		&v.StartedAt, &v.SessionAt, &v.EndedAt, &v.DeletedAt, &v.PresentCount, &v.AbsentCount)
	if errors.Is(err, sql.ErrNoRows) {
		return v, errNotFound
	}
	if err != nil {
		return v, err
	}
	v.Title = attendanceTitleFromString(v.ClassName, v.SessionAt)
	rows, err := s.Query(`SELECT r.student_id,
COALESCE(NULLIF(r.student_no_snapshot,''),st.student_no,''),
COALESCE(NULLIF(r.student_name_snapshot,''),st.name,'已删除学生'),
r.status,r.checked_at,r.method
FROM attendance_records r LEFT JOIN students st ON st.id=r.student_id
WHERE r.session_id=?
ORDER BY CAST(COALESCE(NULLIF(r.student_no_snapshot,''),st.student_no,'') AS INTEGER),
COALESCE(NULLIF(r.student_no_snapshot,''),st.student_no,'')`, id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	v.Records = []attendanceRecord{}
	for rows.Next() {
		var r attendanceRecord
		if err := rows.Scan(&r.StudentID, &r.StudentNo, &r.Name, &r.Status, &r.CheckedAt, &r.Method); err != nil {
			return v, err
		}
		v.Records = append(v.Records, r)
	}
	return v, rows.Err()
}
func (s *store) closeAttendance(id int64) (attendanceView, error) {
	res, err := s.Exec(`UPDATE attendance_sessions SET status='closed',ended_at=datetime('now','localtime')
WHERE id=? AND status='active' AND deleted_at IS NULL`, id)
	if err != nil {
		return attendanceView{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return attendanceView{}, errNotFound
	}
	return s.attendanceByID(id)
}
func (s *store) deleteAttendance(id int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title string
	if err := tx.QueryRow(`SELECT title FROM attendance_sessions WHERE id=? AND deleted_at IS NULL`, id).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	} else if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE attendance_sessions SET deleted_at=datetime('now','localtime'),
status='closed',ended_at=COALESCE(ended_at,datetime('now','localtime'))
WHERE id=? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		return errNotFound
	}
	if err := addAudit(tx, "attendance.delete", "attendance", id, "删除考勤记录 "+title, "已移入回收站"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) restoreAttendance(id int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title string
	if err := tx.QueryRow(`SELECT title FROM attendance_sessions WHERE id=? AND deleted_at IS NOT NULL`, id).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE attendance_sessions SET deleted_at=NULL WHERE id=?`, id); err != nil {
		return err
	}
	if err := addAudit(tx, "attendance.restore", "attendance", id, "恢复考勤记录 "+title, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) purgeAttendance(id int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title string
	if err := tx.QueryRow(`SELECT title FROM attendance_sessions WHERE id=? AND deleted_at IS NOT NULL`, id).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	} else if err != nil {
		return err
	}
	if err := addAudit(tx, "attendance.purge", "attendance", id, "永久删除考勤记录 "+title, "明细已永久清除"); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM attendance_sessions WHERE id=? AND deleted_at IS NOT NULL`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *store) updateAttendanceRecord(sessionID, studentID int64, status, method string) (attendanceView, error) {
	checked := any(nil)
	if status != "absent" {
		checked = time.Now().Format("2006-01-02 15:04:05")
	}
	res, err := s.Exec(`UPDATE attendance_records SET status=?,checked_at=?,method=?
WHERE session_id=? AND student_id=? AND EXISTS(
SELECT 1 FROM attendance_sessions a
WHERE a.id=attendance_records.session_id AND a.deleted_at IS NULL)`, status, checked, method, sessionID, studentID)
	if err != nil {
		return attendanceView{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return attendanceView{}, errNotFound
	}
	return s.attendanceByID(sessionID)
}
func (s *store) publicStudents(classID int64) (map[string]any, error) {
	var sessionID int64
	var className, course, sessionAt string
	err := s.QueryRow(`SELECT a.id,COALESCE(NULLIF(a.class_name_snapshot,''),c.name),a.course,a.session_at
FROM attendance_sessions a JOIN classes c ON c.id=a.class_id
WHERE a.class_id=? AND a.status='active' AND a.deleted_at IS NULL AND c.deleted_at IS NULL`, classID).Scan(&sessionID, &className, &course, &sessionAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 该班当前没有开放签到", errConflict)
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.Query(`SELECT st.id,st.student_no,st.name,r.status
FROM students st JOIN attendance_records r ON r.student_id=st.id AND r.session_id=?
WHERE st.class_id=? AND st.deleted_at IS NULL
ORDER BY CAST(st.student_no AS INTEGER),st.student_no`, sessionID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	students := []map[string]any{}
	for rows.Next() {
		var id int64
		var no, name, status string
		if err := rows.Scan(&id, &no, &name, &status); err != nil {
			return nil, err
		}
		students = append(students, map[string]any{"id": id, "studentNo": no, "name": name, "checkedIn": status != "absent"})
	}
	return map[string]any{"sessionId": sessionID, "title": attendanceTitleFromString(className, sessionAt), "course": course, "sessionAt": sessionAt, "students": students}, rows.Err()
}

func attendanceTitle(className string, sessionTime time.Time) string {
	return fmt.Sprintf("%s · %s", className, sessionTime.Format("2006-01-02 15:04"))
}

func attendanceTitleFromString(className, sessionAt string) string {
	value, err := time.ParseInLocation("2006-01-02 15:04:05", sessionAt, time.Local)
	if err != nil {
		return strings.TrimSpace(className + " · " + sessionAt)
	}
	return attendanceTitle(className, value)
}
func (s *store) checkIn(classID, studentID int64) (map[string]any, error) {
	var sessionID int64
	var name, status string
	err := s.QueryRow(`SELECT a.id,st.name,r.status
FROM attendance_sessions a JOIN attendance_records r ON r.session_id=a.id
JOIN students st ON st.id=r.student_id
WHERE a.class_id=? AND a.status='active' AND a.deleted_at IS NULL
AND st.deleted_at IS NULL AND st.id=?`, classID, studentID).Scan(&sessionID, &name, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 未找到开放签到或学生信息不匹配", errConflict)
	}
	if err != nil {
		return nil, err
	}
	if status != "absent" {
		return nil, fmt.Errorf("%w: %s 已完成签到，请勿重复提交", errConflict, name)
	}
	res, err := s.Exec(`UPDATE attendance_records SET status='present',checked_at=datetime('now','localtime'),method='self' WHERE session_id=? AND student_id=? AND status='absent'`, sessionID, studentID)
	if err != nil {
		return nil, err
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		return nil, fmt.Errorf("%w: %s 已完成签到，请勿重复提交", errConflict, name)
	}
	return map[string]any{"ok": true, "name": name, "message": "签到成功"}, nil
}

func (s *store) settings() (siteSettings, error) {
	var result siteSettings
	rows, err := s.Query(`SELECT key,value FROM settings WHERE key IN ('site_title','site_subtitle')`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return result, err
		}
		if key == "site_title" {
			result.Title = value
		} else if key == "site_subtitle" {
			result.Subtitle = value
		}
	}
	return result, rows.Err()
}

func (s *store) updateSettings(in siteSettings) (siteSettings, error) {
	tx, err := s.Begin()
	if err != nil {
		return siteSettings{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('site_title',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strings.TrimSpace(in.Title)); err != nil {
		return siteSettings{}, err
	}
	if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('site_subtitle',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strings.TrimSpace(in.Subtitle)); err != nil {
		return siteSettings{}, err
	}
	if err := tx.Commit(); err != nil {
		return siteSettings{}, err
	}
	return s.settings()
}

func (s *store) teacherInitialized() (bool, error) {
	var initialized bool
	err := s.QueryRow(`SELECT EXISTS(SELECT 1 FROM teacher_accounts WHERE id=1)`).Scan(&initialized)
	return initialized, err
}

func (s *store) createTeacherAccount(username, passwordHash string) error {
	result, err := s.Exec(`INSERT INTO teacher_accounts(id,username,password_hash)
SELECT 1,?,? WHERE NOT EXISTS(SELECT 1 FROM teacher_accounts WHERE id=1)`, username, passwordHash)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("%w: 系统已完成初始化", errConflict)
	}
	return nil
}

func (s *store) teacherAccount() (teacherAccount, error) {
	var account teacherAccount
	err := s.QueryRow(`SELECT username,password_hash FROM teacher_accounts WHERE id=1`).Scan(&account.Username, &account.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return account, errNotFound
	}
	return account, err
}

func (s *store) createTeacherSession(tokenHash string, expiresAt int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM teacher_sessions WHERE expires_at<=unixepoch()`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO teacher_sessions(token_hash,teacher_id,expires_at) VALUES(?,1,?)`, tokenHash, expiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) teacherSession(tokenHash string, now int64) (string, error) {
	var username string
	err := s.QueryRow(`SELECT a.username FROM teacher_sessions s
JOIN teacher_accounts a ON a.id=s.teacher_id
WHERE s.token_hash=? AND s.expires_at>?`, tokenHash, now).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	return username, err
}

func (s *store) deleteTeacherSession(tokenHash string) error {
	_, err := s.Exec(`DELETE FROM teacher_sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *store) navigation() ([]navigationLink, error) {
	items := []navigationLink{}
	rows, err := s.Query(`SELECT id,title,url,icon_url,sort_order FROM navigation_links ORDER BY sort_order,id`)
	if err != nil {
		return items, err
	}
	defer rows.Close()
	for rows.Next() {
		var item navigationLink
		if err := rows.Scan(&item.ID, &item.Title, &item.URL, &item.IconURL, &item.SortOrder); err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *store) replaceNavigation(items []navigationLinkInput) ([]navigationLink, error) {
	tx, err := s.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM navigation_links`); err != nil {
		return nil, err
	}
	statement, err := tx.Prepare(`INSERT INTO navigation_links(title,url,icon_url,sort_order) VALUES(?,?,?,?)`)
	if err != nil {
		return nil, err
	}
	defer statement.Close()
	for index, item := range items {
		if _, err := statement.Exec(item.Title, item.URL, item.IconURL, index); err != nil {
			return nil, err
		}
	}
	if err := statement.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.navigation()
}

func (s *store) schedule() (scheduleData, error) {
	data := scheduleData{Lessons: []scheduleLesson{}, Changes: []scheduleChange{}}
	settings, err := s.scheduleSettings()
	if err != nil {
		return data, err
	}
	data.Settings = settings
	rows, err := s.Query(`SELECT l.id,l.class_id,c.name,l.course,l.weekday,l.period,p.start_time,p.end_time,l.location_odd,l.location_even
FROM schedule_lessons l JOIN classes c ON c.id=l.class_id
JOIN schedule_periods p ON p.period=l.period
WHERE c.deleted_at IS NULL
ORDER BY l.weekday,l.period,l.id`)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var lesson scheduleLesson
		if err := rows.Scan(&lesson.ID, &lesson.ClassID, &lesson.ClassName, &lesson.Course, &lesson.Weekday, &lesson.Period, &lesson.StartTime, &lesson.EndTime, &lesson.LocationOdd, &lesson.LocationEven); err != nil {
			rows.Close()
			return data, err
		}
		data.Lessons = append(data.Lessons, lesson)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return data, err
	}
	rows.Close()

	changes, err := s.Query(`SELECT ch.id,ch.lesson_id,ch.original_date,ch.status,ch.new_date,ch.new_start_time,ch.new_end_time,
COALESCE(ch.new_class_id,0),COALESCE(c.name,''),ch.note
FROM schedule_changes ch LEFT JOIN classes c ON c.id=ch.new_class_id
WHERE ch.original_date >= date('now','localtime','-35 days') OR ch.new_date >= date('now','localtime','-35 days')
ORDER BY ch.original_date,ch.id`)
	if err != nil {
		return data, err
	}
	defer changes.Close()
	for changes.Next() {
		var change scheduleChange
		if err := changes.Scan(&change.ID, &change.LessonID, &change.Date, &change.Status, &change.NewDate, &change.NewStartTime, &change.NewEndTime, &change.NewClassID, &change.NewClassName, &change.Note); err != nil {
			return data, err
		}
		data.Changes = append(data.Changes, change)
	}
	return data, changes.Err()
}

func (s *store) scheduleLessonByID(id int64) (scheduleLesson, error) {
	var lesson scheduleLesson
	err := s.QueryRow(`SELECT l.id,l.class_id,c.name,l.course,l.weekday,l.period,p.start_time,p.end_time,l.location_odd,l.location_even
FROM schedule_lessons l JOIN classes c ON c.id=l.class_id JOIN schedule_periods p ON p.period=l.period
WHERE l.id=? AND c.deleted_at IS NULL`, id).
		Scan(&lesson.ID, &lesson.ClassID, &lesson.ClassName, &lesson.Course, &lesson.Weekday, &lesson.Period, &lesson.StartTime, &lesson.EndTime, &lesson.LocationOdd, &lesson.LocationEven)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return lesson, err
}

func (s *store) createScheduleLesson(in scheduleInput) (scheduleLesson, error) {
	if _, err := s.classByID(in.ClassID); err != nil {
		return scheduleLesson{}, err
	}
	if err := s.ensureScheduleCellAvailable(0, in.Weekday, in.Period); err != nil {
		return scheduleLesson{}, err
	}
	start, end, err := s.schedulePeriodTimes(in.Period)
	if err != nil {
		return scheduleLesson{}, err
	}
	res, err := s.Exec(`INSERT INTO schedule_lessons(class_id,course,weekday,period,start_time,end_time,location_odd,location_even) VALUES(?,?,?,?,?,?,?,?)`, in.ClassID, in.Course, in.Weekday, in.Period, start, end, in.LocationOdd, in.LocationEven)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return scheduleLesson{}, fmt.Errorf("%w: 该班在这个时段已有课程", errConflict)
		}
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return scheduleLesson{}, errNotFound
		}
		return scheduleLesson{}, err
	}
	id, _ := res.LastInsertId()
	return s.scheduleLessonByID(id)
}

func (s *store) updateScheduleLesson(id int64, in scheduleInput) (scheduleLesson, error) {
	if _, err := s.classByID(in.ClassID); err != nil {
		return scheduleLesson{}, err
	}
	if err := s.ensureScheduleCellAvailable(id, in.Weekday, in.Period); err != nil {
		return scheduleLesson{}, err
	}
	start, end, err := s.schedulePeriodTimes(in.Period)
	if err != nil {
		return scheduleLesson{}, err
	}
	res, err := s.Exec(`UPDATE schedule_lessons SET class_id=?,course=?,weekday=?,period=?,start_time=?,end_time=?,location_odd=?,location_even=? WHERE id=?`, in.ClassID, in.Course, in.Weekday, in.Period, start, end, in.LocationOdd, in.LocationEven, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return scheduleLesson{}, fmt.Errorf("%w: 该班在这个时段已有课程", errConflict)
		}
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return scheduleLesson{}, errNotFound
		}
		return scheduleLesson{}, err
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		return scheduleLesson{}, errNotFound
	}
	return s.scheduleLessonByID(id)
}

func (s *store) deleteScheduleLesson(id int64) error {
	res, err := s.Exec(`DELETE FROM schedule_lessons WHERE id=?`, id)
	if err != nil {
		return err
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		return errNotFound
	}
	return nil
}

func (s *store) setScheduleChange(lessonID int64, in scheduleChangeInput) (scheduleChange, error) {
	if _, err := s.scheduleLessonByID(lessonID); err != nil {
		return scheduleChange{}, err
	}
	var newClass any
	if in.NewClassID > 0 {
		if _, err := s.classByID(in.NewClassID); err != nil {
			return scheduleChange{}, err
		}
		newClass = in.NewClassID
	}
	_, err := s.Exec(`INSERT INTO schedule_changes(lesson_id,original_date,status,new_date,new_start_time,new_end_time,new_class_id,note)
VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(lesson_id,original_date) DO UPDATE SET
status=excluded.status,new_date=excluded.new_date,new_start_time=excluded.new_start_time,new_end_time=excluded.new_end_time,new_class_id=excluded.new_class_id,note=excluded.note,created_at=datetime('now','localtime')`,
		lessonID, in.Date, in.Status, in.NewDate, in.NewStartTime, in.NewEndTime, newClass, in.Note)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return scheduleChange{}, errNotFound
		}
		return scheduleChange{}, err
	}
	return s.scheduleChangeByLessonDate(lessonID, in.Date)
}

func (s *store) scheduleChangeByLessonDate(lessonID int64, date string) (scheduleChange, error) {
	var change scheduleChange
	err := s.QueryRow(`SELECT ch.id,ch.lesson_id,ch.original_date,ch.status,ch.new_date,ch.new_start_time,ch.new_end_time,
COALESCE(ch.new_class_id,0),COALESCE(c.name,''),ch.note
FROM schedule_changes ch LEFT JOIN classes c ON c.id=ch.new_class_id
WHERE ch.lesson_id=? AND ch.original_date=?`, lessonID, date).
		Scan(&change.ID, &change.LessonID, &change.Date, &change.Status, &change.NewDate, &change.NewStartTime, &change.NewEndTime, &change.NewClassID, &change.NewClassName, &change.Note)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return change, err
}

func (s *store) deleteScheduleChange(lessonID int64, date string) error {
	res, err := s.Exec(`DELETE FROM schedule_changes WHERE lesson_id=? AND original_date=?`, lessonID, date)
	if err != nil {
		return err
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		return errNotFound
	}
	return nil
}

func (s *store) importSchedule(inputs []scheduleInput) (map[string]int, error) {
	tx, err := s.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	added, skipped := 0, 0
	for _, in := range inputs {
		var classExists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM classes WHERE id=? AND deleted_at IS NULL)`, in.ClassID).Scan(&classExists); err != nil {
			return nil, err
		}
		if !classExists {
			return nil, errNotFound
		}
		res, err := tx.Exec(`INSERT INTO schedule_lessons(class_id,course,weekday,period,start_time,end_time,location_odd,location_even)
SELECT ?,?,?,p.period,p.start_time,p.end_time,?,? FROM schedule_periods p
WHERE p.period=? AND NOT EXISTS (SELECT 1 FROM schedule_lessons l WHERE l.weekday=? AND l.period=?)`, in.ClassID, in.Course, in.Weekday, in.LocationOdd, in.LocationEven, in.Period, in.Weekday, in.Period)
		if err != nil {
			return nil, err
		}
		changed, _ := res.RowsAffected()
		if changed == 1 {
			added++
		} else {
			skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]int{"added": added, "skipped": skipped, "total": len(inputs)}, nil
}

func (s *store) ensureScheduleCellAvailable(id int64, weekday, period int) error {
	var existing int64
	err := s.QueryRow(`SELECT id FROM schedule_lessons WHERE weekday=? AND period=? AND id<>? LIMIT 1`, weekday, period, id).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: 该节次已有课程", errConflict)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func (s *store) schedulePeriodTimes(period int) (string, string, error) {
	var start, end string
	err := s.QueryRow(`SELECT start_time,end_time FROM schedule_periods WHERE period=?`, period).Scan(&start, &end)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return start, end, err
}

func (s *store) scheduleSettings() (scheduleSettings, error) {
	settings := scheduleSettings{Periods: []schedulePeriod{}}
	if err := s.QueryRow(`SELECT semester_start,semester_end FROM schedule_settings WHERE id=1`).Scan(&settings.SemesterStart, &settings.SemesterEnd); err != nil {
		return settings, err
	}
	rows, err := s.Query(`SELECT period,start_time,end_time FROM schedule_periods ORDER BY period`)
	if err != nil {
		return settings, err
	}
	defer rows.Close()
	for rows.Next() {
		var period schedulePeriod
		if err := rows.Scan(&period.Period, &period.StartTime, &period.EndTime); err != nil {
			return settings, err
		}
		settings.Periods = append(settings.Periods, period)
	}
	return settings, rows.Err()
}

func (s *store) updateScheduleSettings(in scheduleSettingsInput) (scheduleSettings, error) {
	tx, err := s.Begin()
	if err != nil {
		return scheduleSettings{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE schedule_lessons SET start_time='@'||id`); err != nil {
		return scheduleSettings{}, err
	}
	if _, err := tx.Exec(`UPDATE schedule_settings SET semester_start=?,semester_end=? WHERE id=1`, in.SemesterStart, in.SemesterEnd); err != nil {
		return scheduleSettings{}, err
	}
	for _, period := range in.Periods {
		if _, err := tx.Exec(`UPDATE schedule_periods SET start_time=?,end_time=? WHERE period=?`, period.StartTime, period.EndTime, period.Period); err != nil {
			return scheduleSettings{}, err
		}
	}
	if _, err := tx.Exec(`UPDATE schedule_lessons SET
start_time=(SELECT start_time FROM schedule_periods WHERE period=schedule_lessons.period),
end_time=(SELECT end_time FROM schedule_periods WHERE period=schedule_lessons.period)`); err != nil {
		return scheduleSettings{}, err
	}
	if err := tx.Commit(); err != nil {
		return scheduleSettings{}, err
	}
	return s.scheduleSettings()
}
