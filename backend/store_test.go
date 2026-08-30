package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func testStore(t *testing.T) *store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestScoreAndAttendanceFlow(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "三", ClassNo: "2"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.createStudent(class.ID, studentInput{StudentNo: "01", Name: "张同学"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.createStudent(class.ID, studentInput{StudentNo: "02", Name: "李同学"}); err != nil {
		t.Fatal(err)
	}
	updated, err := s.changeScore(first.ID, scoreInput{Delta: 3, Reason: "积极回答"})
	if err != nil || updated.Score != 3 {
		t.Fatalf("score update = %+v, %v", updated, err)
	}

	session, err := s.createAttendance(attendanceInput{ClassID: class.ID, Title: "第一节", Course: "信息课", SessionAt: "2026-07-15T10:20"})
	if err != nil || session.AbsentCount != 2 {
		t.Fatalf("session = %+v, %v", session, err)
	}
	if session.Title != "三 2 班 · 2026-07-15 10:20" {
		t.Fatalf("generated attendance title = %q", session.Title)
	}
	if _, err = s.checkIn(class.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.checkIn(class.ID, first.ID); !errors.Is(err, errConflict) {
		t.Fatalf("duplicate check-in should conflict, got %v", err)
	}
	view, err := s.attendanceByID(session.ID)
	if err != nil || view.PresentCount != 1 || view.AbsentCount != 1 {
		t.Fatalf("attendance counts = %+v, %v", view, err)
	}
	if _, err = s.closeAttendance(session.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.createAttendance(attendanceInput{ClassID: class.ID, Title: "第二节", SessionAt: "2026-07-16T09:00"})
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := s.attendance(class.ID, "2026-07-15", false, 0, 30)
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != session.ID {
		t.Fatalf("date filter = %+v, %v", filtered, err)
	}
	allClasses, err := s.attendance(0, "2026-07-15", false, 0, 30)
	if err != nil || len(allClasses.Items) != 1 || allClasses.Items[0].ID != session.ID {
		t.Fatalf("all-class date filter = %+v, %v", allClasses, err)
	}
	if _, err = s.updateClass(class.ID, classInput{Grade: "四", ClassNo: "2"}); err != nil {
		t.Fatal(err)
	}
	historical, err := s.attendanceByID(session.ID)
	if err != nil || historical.ClassName != "三 2 班" || historical.Title != "三 2 班 · 2026-07-15 10:20" {
		t.Fatalf("historical class snapshot changed unexpectedly: %+v, %v", historical, err)
	}
	if err = s.deleteAttendance(second.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.attendanceByID(second.ID)
	if err != nil || deleted.DeletedAt == nil {
		t.Fatalf("deleted attendance should be in trash: %+v, %v", deleted, err)
	}
	var recordCount int
	if err = s.QueryRow(`SELECT COUNT(*) FROM attendance_records WHERE session_id=?`, second.ID).Scan(&recordCount); err != nil || recordCount != 2 {
		t.Fatalf("deleted attendance records = %d, %v", recordCount, err)
	}
	if err = s.restoreAttendance(second.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.deleteAttendance(second.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.purgeAttendance(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.attendanceByID(second.ID); !errors.Is(err, errNotFound) {
		t.Fatalf("purged attendance should not exist, got %v", err)
	}
	if _, err = s.createAttendance(attendanceInput{ClassID: class.ID, SessionAt: "2026-07-16T10:00"}); err != nil {
		t.Fatalf("class should allow a new session after deleting the active one: %v", err)
	}

	classes, err := s.classes(false)
	if err != nil || len(classes) != 1 || classes[0].TotalScore != 3 {
		t.Fatalf("class score must not multiply across sessions: %+v, %v", classes, err)
	}
	imported, err := s.importStudents(class.ID, []studentInput{{StudentNo: "01", Name: "重复学生"}, {StudentNo: "03", Name: "新学生"}})
	if err != nil || imported["added"] != 1 || imported["skipped"] != 1 {
		t.Fatalf("student import result = %+v, %v", imported, err)
	}
}

func TestAttendanceSnapshotsAndScoreUndo(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "二", ClassNo: "3"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := s.createStudent(class.ID, studentInput{StudentNo: "07", Name: "原姓名"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.createAttendance(attendanceInput{ClassID: class.ID, SessionAt: "2026-09-07T08:00"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.updateStudent(student.ID, studentInput{StudentNo: "99", Name: "新姓名"}); err != nil {
		t.Fatal(err)
	}
	if err := s.deleteStudent(student.ID); err != nil {
		t.Fatal(err)
	}
	history, err := s.attendanceByID(session.ID)
	if err != nil || len(history.Records) != 1 || history.Records[0].StudentNo != "07" || history.Records[0].Name != "原姓名" {
		t.Fatalf("attendance snapshot = %+v, %v", history.Records, err)
	}

	restored, err := s.createStudent(class.ID, studentInput{StudentNo: "99", Name: "新姓名"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.changeScore(restored.ID, scoreInput{Delta: 5, Reason: "课堂表现"})
	if err != nil || updated.Score != 5 {
		t.Fatalf("score = %+v, %v", updated, err)
	}
	events, err := s.scoreEvents(restored.ID)
	if err != nil || len(events) != 1 || !events[0].Reversible {
		t.Fatalf("events = %+v, %v", events, err)
	}
	updated, err = s.undoScoreEvent(events[0].ID)
	if err != nil || updated.Score != 0 {
		t.Fatalf("undo score = %+v, %v", updated, err)
	}
	if _, err := s.undoScoreEvent(events[0].ID); !errors.Is(err, errConflict) {
		t.Fatalf("second undo should conflict, got %v", err)
	}
	logs, err := s.auditLogs(100, 0)
	if err != nil || len(logs) < 5 {
		t.Fatalf("audit logs = %+v, %v", logs, err)
	}
}

func TestAttendanceCursorDoesNotRepeatOlderActiveSession(t *testing.T) {
	s := testStore(t)
	first, err := s.createClass(classInput{Grade: "二", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.createClass(classInput{Grade: "二", ClassNo: "2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range []classRow{first, second} {
		if _, err := s.createStudent(class.ID, studentInput{StudentNo: "01", Name: "测试学生"}); err != nil {
			t.Fatal(err)
		}
	}
	olderActive, err := s.createAttendance(attendanceInput{ClassID: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		session, err := s.createAttendance(attendanceInput{ClassID: second.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.closeAttendance(session.ID); err != nil {
			t.Fatal(err)
		}
	}
	firstPage, err := s.attendance(0, "", false, 0, 2)
	if err != nil || len(firstPage.Items) != 2 || firstPage.NextCursor == 0 {
		t.Fatalf("first page = %+v, %v", firstPage, err)
	}
	secondPage, err := s.attendance(0, "", false, firstPage.NextCursor, 2)
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != olderActive.ID {
		t.Fatalf("second page = %+v, %v", secondPage, err)
	}
	seen := map[int64]bool{}
	for _, item := range append(firstPage.Items, secondPage.Items...) {
		if seen[item.ID] {
			t.Fatalf("attendance %d appeared on more than one page", item.ID)
		}
		seen[item.ID] = true
	}
}

func TestBackupReportAndCurrentLesson(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "五", ClassNo: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.createStudent(class.ID, studentInput{StudentNo: "01", Name: "张同学"}); err != nil {
		t.Fatal(err)
	}
	periods := []schedulePeriod{{1, "08:00", "08:40"}, {2, "08:50", "09:30"}, {3, "09:40", "10:20"}, {4, "13:30", "14:10"}, {5, "14:20", "15:00"}, {6, "15:10", "15:50"}, {7, "16:00", "16:40"}}
	if _, err := s.updateScheduleSettings(scheduleSettingsInput{SemesterStart: "2026-09-01", SemesterEnd: "2027-01-20", Periods: periods}); err != nil {
		t.Fatal(err)
	}
	lesson, err := s.createScheduleLesson(scheduleInput{ClassID: class.ID, Course: "信息科技", Weekday: 1, Period: 1, LocationOdd: "机房 1", LocationEven: "机房 1"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 8, 20, 0, 0, time.Local)
	current, err := s.currentLesson(now)
	if err != nil || !current.Detected || current.ClassID != class.ID || current.Source != "regular" || current.SessionAt != "2026-09-07T08:00" {
		t.Fatalf("current lesson = %+v, %v", current, err)
	}
	if _, err := s.setScheduleChange(lesson.ID, scheduleChangeInput{Date: "2026-09-07", Status: "occupied"}); err != nil {
		t.Fatal(err)
	}
	current, err = s.currentLesson(now)
	if err != nil || current.Detected {
		t.Fatalf("occupied lesson detected = %+v, %v", current, err)
	}
	if _, err := s.setScheduleChange(lesson.ID, scheduleChangeInput{Date: "2026-09-07", Status: "rescheduled", NewDate: "2026-09-07", NewStartTime: "08:10", NewEndTime: "08:50", NewClassID: class.ID}); err != nil {
		t.Fatal(err)
	}
	current, err = s.currentLesson(now)
	if err != nil || !current.Detected || current.Source != "rescheduled" || current.SessionAt != "2026-09-07T08:10" {
		t.Fatalf("rescheduled lesson = %+v, %v", current, err)
	}

	backup, err := s.createBackup()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(backup)
	if err := validateBackupFile(backup); err != nil {
		t.Fatalf("valid backup rejected: %v", err)
	}
	copyStore, err := openStore(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer copyStore.Close()
	classes, err := copyStore.classes(false)
	if err != nil || len(classes) != 1 {
		t.Fatalf("backup classes = %+v, %v", classes, err)
	}
	book, className, err := s.buildReport("roster", class.ID, "", "")
	if err != nil || className != class.Name {
		t.Fatalf("report = %q, %v", className, err)
	}
	defer book.Close()
	if rows, err := book.GetRows("报表"); err != nil || len(rows) != 2 {
		t.Fatalf("report rows = %+v, %v", rows, err)
	}
}

func TestBackupValidationAndAutomaticRetention(t *testing.T) {
	s := testStore(t)
	invalid := filepath.Join(t.TempDir(), "not-a-classorbit-backup.db")
	if err := os.WriteFile(invalid, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupFile(invalid); err == nil {
		t.Fatal("invalid backup should be rejected")
	}
	if _, err := s.createClass(classInput{Grade: "六", ClassNo: "1"}); err != nil {
		t.Fatal(err)
	}
	created, err := s.createAutomaticBackup()
	if err != nil || created == "" {
		t.Fatalf("automatic backup = %q, %v", created, err)
	}
	if err := validateBackupFile(created); err != nil {
		t.Fatalf("automatic backup rejected: %v", err)
	}
	if duplicate, err := s.createAutomaticBackup(); err != nil || duplicate != "" {
		t.Fatalf("second daily backup = %q, %v", duplicate, err)
	}
	directory := filepath.Dir(created)
	oldAutomatic := filepath.Join(directory, "classorbit-auto-20000101.db")
	safety := filepath.Join(directory, "classorbit-before-restore-20000101.db")
	for _, path := range []string{oldAutomatic, safety} {
		if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().AddDate(0, 0, -30)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupAutomaticBackups(directory, 14); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldAutomatic); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired automatic backup still exists: %v", err)
	}
	if _, err := os.Stat(safety); err != nil {
		t.Fatalf("pre-restore safety backup should be retained: %v", err)
	}
}

func TestParseExcelHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "students.xlsx")
	x := excelize.NewFile()
	defer x.Close()
	rows := [][]any{{"学号", "姓名", "备注"}, {"2026001", "陈同学", ""}, {"2026002", "王同学", ""}}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err := x.SetCellValue("Sheet1", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := x.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	students, err := parseExcel(file)
	if err != nil || len(students) != 2 || students[1].StudentNo != "2026002" || students[1].Name != "王同学" {
		t.Fatalf("parsed students = %+v, %v", students, err)
	}
}

func TestEmptyClassCannotStartAttendance(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "一", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.createAttendance(attendanceInput{ClassID: class.ID})
	if !errors.Is(err, errConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestSiteSettings(t *testing.T) {
	s := testStore(t)
	defaults, err := s.settings()
	if err != nil || defaults.Title != "智创课堂" || defaults.Subtitle != "小学信息科技课" {
		t.Fatalf("default settings = %+v, %v", defaults, err)
	}
	updated, err := s.updateSettings(siteSettings{Title: "创想信息课", Subtitle: "一起发现数字世界"})
	if err != nil || updated.Title != "创想信息课" || updated.Subtitle != "一起发现数字世界" {
		t.Fatalf("updated settings = %+v, %v", updated, err)
	}
}

func TestTeacherSessionExpiryAndNavigationReplacement(t *testing.T) {
	s := testStore(t)
	initialized, err := s.teacherInitialized()
	if err != nil || initialized {
		t.Fatalf("initial teacher state = %v, %v", initialized, err)
	}
	if err := s.createTeacherAccount("teacher", "bcrypt-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.createTeacherAccount("other", "other-hash"); !errors.Is(err, errConflict) {
		t.Fatalf("second teacher setup should conflict, got %v", err)
	}
	if err := s.createTeacherSession("expired", time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.teacherSession("expired", time.Now().Unix()); !errors.Is(err, errNotFound) {
		t.Fatalf("expired session should not authenticate, got %v", err)
	}
	if err := s.createTeacherSession("active", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if username, err := s.teacherSession("active", time.Now().Unix()); err != nil || username != "teacher" {
		t.Fatalf("active session = %q, %v", username, err)
	}

	items, err := s.replaceNavigation([]navigationLinkInput{
		{Title: "第二项", URL: "https://second.example", IconURL: ""},
		{Title: "第一项", URL: "https://first.example", IconURL: "https://first.example/favicon.ico"},
	})
	if err != nil || len(items) != 2 || items[0].SortOrder != 0 || items[1].SortOrder != 1 {
		t.Fatalf("navigation replacement = %+v, %v", items, err)
	}
	items, err = s.replaceNavigation([]navigationLinkInput{{Title: "唯一项", URL: "https://only.example", IconURL: ""}})
	if err != nil || len(items) != 1 || items[0].Title != "唯一项" || items[0].SortOrder != 0 {
		t.Fatalf("second navigation replacement = %+v, %v", items, err)
	}
}

func TestScheduleAndOneOffChanges(t *testing.T) {
	s := testStore(t)
	first, err := s.createClass(classInput{Grade: "三", ClassNo: "2"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.createClass(classInput{Grade: "四", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.updateScheduleSettings(scheduleSettingsInput{SemesterStart: "2026-09-01", SemesterEnd: "2027-01-20", Periods: []schedulePeriod{{1, "08:00", "08:40"}, {2, "08:50", "09:30"}, {3, "09:40", "10:20"}, {4, "13:30", "14:10"}, {5, "14:20", "15:00"}, {6, "15:10", "15:50"}, {7, "16:00", "16:40"}}})
	if err != nil || settings.SemesterStart != "2026-09-01" || len(settings.Periods) != 7 {
		t.Fatalf("schedule settings = %+v, %v", settings, err)
	}
	lesson, err := s.createScheduleLesson(scheduleInput{ClassID: first.ID, Course: "信息课", Weekday: 1, Period: 1, LocationOdd: "机房 1", LocationEven: "机房 2"})
	if err != nil || lesson.ClassName != "三 2 班" {
		t.Fatalf("created lesson = %+v, %v", lesson, err)
	}
	lesson, err = s.updateScheduleLesson(lesson.ID, scheduleInput{ClassID: first.ID, Course: "人工智能", Weekday: 1, Period: 2, LocationOdd: "教室", LocationEven: "机房 2"})
	if err != nil || lesson.Course != "人工智能" || lesson.StartTime != "08:50" || lesson.LocationOdd != "教室" {
		t.Fatalf("updated lesson = %+v, %v", lesson, err)
	}
	originalDate := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	newDate := time.Now().AddDate(0, 0, 4).Format("2006-01-02")
	change, err := s.setScheduleChange(lesson.ID, scheduleChangeInput{Date: originalDate, Status: "occupied", Note: "被占课"})
	if err != nil || change.Status != "occupied" {
		t.Fatalf("occupied change = %+v, %v", change, err)
	}
	change, err = s.setScheduleChange(lesson.ID, scheduleChangeInput{Date: originalDate, Status: "rescheduled", NewDate: newDate, NewStartTime: "10:20", NewEndTime: "11:00", NewClassID: second.ID, Note: "与王老师调课"})
	if err != nil || change.Status != "rescheduled" || change.NewClassName != "四 1 班" {
		t.Fatalf("rescheduled change = %+v, %v", change, err)
	}
	data, err := s.schedule()
	if err != nil || len(data.Lessons) != 1 || len(data.Changes) != 1 {
		t.Fatalf("schedule data = %+v, %v", data, err)
	}
	if err := s.deleteScheduleChange(lesson.ID, originalDate); err != nil {
		t.Fatal(err)
	}
	if err := s.deleteScheduleLesson(lesson.ID); err != nil {
		t.Fatal(err)
	}
	imported, err := s.importSchedule([]scheduleInput{{ClassID: first.ID, Course: "信息课", Weekday: 3, Period: 3, LocationOdd: "机房 1", LocationEven: "机房 2"}, {ClassID: second.ID, Course: "人工智能", Weekday: 3, Period: 3, LocationOdd: "教室", LocationEven: "教室"}})
	if err != nil || imported["added"] != 1 || imported["skipped"] != 1 {
		t.Fatalf("schedule import result = %+v, %v", imported, err)
	}
}

func TestParseScheduleExcel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.xlsx")
	x := excelize.NewFile()
	defer x.Close()
	rows := [][]any{{"星期", "开始时间", "结束时间", "班级", "课程"}, {"周一", "08:00", "08:40", "三 2 班", "信息课"}, {"星期五", "14:10", "14:50", "四 1 班", "人工智能"}}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err := x.SetCellValue("Sheet1", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := x.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	parsed, err := parseScheduleExcel(file)
	if err != nil || len(parsed) != 2 || parsed[0].Weekday != 1 || parsed[1].Weekday != 5 || parsed[1].Course != "人工智能" {
		t.Fatalf("parsed schedule = %+v, %v", parsed, err)
	}
}

func TestScheduleMigrationResolvesPeriodCollisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule-collision.db")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := s.createClass(classInput{Grade: "三", ClassNo: "1"})
	second, _ := s.createClass(classInput{Grade: "三", ClassNo: "2"})
	if _, err := s.Exec(`INSERT INTO schedule_lessons(class_id,course,weekday,period,start_time,end_time,location_odd,location_even) VALUES(?,?,?,?,?,?,?,?),(?,?,?,?,?,?,?,?)`, first.ID, "信息课", 5, 7, "21:00", "21:40", "机房 1", "机房 1", second.ID, "人工智能", 5, 7, "22:00", "22:40", "机房 2", "机房 2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var distinct int
	if err := s.QueryRow(`SELECT COUNT(DISTINCT period) FROM schedule_lessons WHERE weekday=5`).Scan(&distinct); err != nil || distinct != 2 {
		t.Fatalf("resolved periods = %d, %v", distinct, err)
	}
}

func TestSchedulePeriodUpdateAvoidsLegacyUniqueConflict(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "五", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for period := 1; period <= 2; period++ {
		if _, err := s.createScheduleLesson(scheduleInput{ClassID: class.ID, Course: "信息课", Weekday: 1, Period: period, LocationOdd: "机房 1", LocationEven: "机房 1"}); err != nil {
			t.Fatal(err)
		}
	}
	periods := []schedulePeriod{{1, "08:50", "09:20"}, {2, "09:30", "10:00"}, {3, "10:10", "10:40"}, {4, "13:40", "14:10"}, {5, "14:20", "14:50"}, {6, "15:00", "15:30"}, {7, "15:40", "16:10"}}
	if _, err := s.updateScheduleSettings(scheduleSettingsInput{SemesterStart: "2026-09-01", SemesterEnd: "2027-01-20", Periods: periods}); err != nil {
		t.Fatal(err)
	}
	data, err := s.schedule()
	if err != nil || data.Lessons[0].StartTime != "08:50" || data.Lessons[1].StartTime != "09:30" {
		t.Fatalf("updated lesson times = %+v, %v", data.Lessons, err)
	}
}

func TestMigrationFromLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE classes (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,grade TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL);
CREATE TABLE attendance_sessions (id INTEGER PRIMARY KEY AUTOINCREMENT,class_id INTEGER NOT NULL REFERENCES classes(id),title TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',started_at TEXT NOT NULL,ended_at TEXT);
INSERT INTO classes(id,name,grade,created_at) VALUES(1,'五 2 班','五','2026-07-14 08:00:00');
INSERT INTO attendance_sessions(id,class_id,title,status,started_at) VALUES(1,1,'旧场次','closed','2026-07-14 09:00:00');
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var classNo, course, sessionAt string
	if err := s.QueryRow(`SELECT c.class_no,a.course,a.session_at FROM classes c JOIN attendance_sessions a ON a.class_id=c.id WHERE a.id=1`).Scan(&classNo, &course, &sessionAt); err != nil {
		t.Fatal(err)
	}
	if classNo != "" || course != "信息课" || sessionAt != "2026-07-14 09:00:00" {
		t.Fatalf("migrated values = classNo %q, course %q, sessionAt %q", classNo, course, sessionAt)
	}
}
