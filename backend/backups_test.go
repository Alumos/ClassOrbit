package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func restoreRequest(t *testing.T, handler http.Handler, payload []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "backup.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/restore", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func backupTestAPI(t *testing.T) (*server, http.Handler, *http.Cookie) {
	t.Helper()
	s := &server{db: testStore(t)}
	t.Cleanup(func() { s.db.Close() })
	mux := http.NewServeMux()
	s.routes(mux)
	handler := s.maintenance(s.requireTeacher(mux))
	response := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	return s, handler, responseCookie(t, response)
}

func backupBytes(t *testing.T, s *store) []byte {
	t.Helper()
	path, err := s.createBackup()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func tableSnapshot(t *testing.T, s *store, table string) string {
	t.Helper()
	rows, err := s.Query(`SELECT * FROM ` + table + ` ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var data [][]any
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		data = append(data, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestBackupRestoreHTTPRoundTrip(t *testing.T) {
	s, handler, cookie := backupTestAPI(t)
	class, err := s.db.createClass(classInput{Grade: "三", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := s.db.createStudent(class.ID, studentInput{StudentNo: "01", Name: "备份学生"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.changeScore(student.ID, scoreInput{Delta: 5, Reason: "备份验证"}); err != nil {
		t.Fatal(err)
	}
	attendance, err := s.db.createAttendance(attendanceInput{ClassID: class.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.checkIn(class.ID, student.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.db.deleteAttendance(attendance.ID); err != nil {
		t.Fatal(err)
	}
	lesson, err := s.db.createScheduleLesson(scheduleInput{ClassID: class.ID, Course: "信息课", Weekday: 1, Period: 1, LocationOdd: "机房 1", LocationEven: "机房 2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO schedule_changes(lesson_id,original_date,status,note) VALUES(?,'2026-09-21','occupied','保留换课')`, lesson.ID); err != nil {
		t.Fatal(err)
	}
	html := []byte("<h1>备份网页</h1>")
	response := teachingSiteAPIRequest(t, handler, http.MethodPost, "/api/navigation/sites", "html", "备份网页", []teachingSiteTestFile{{Name: "index.html", Data: html}}, cookie)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	var site navigationLink
	if err = json.Unmarshal(response.Body.Bytes(), &site); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO teacher_qr_logins(scan_token_hash,claim_token_hash,device_name,expires_at,status) VALUES('scan','claim','old',unixepoch()+300,'approved')`); err != nil {
		t.Fatal(err)
	}
	tables := []string{"classes", "students", "score_events", "attendance_sessions", "attendance_records", "settings", "teacher_accounts", "navigation_links", "teaching_sites", "schedule_lessons", "schedule_changes", "schedule_settings", "schedule_periods", "schema_migrations"}
	expected := map[string]string{}
	for _, table := range tables {
		expected[table] = tableSnapshot(t, s.db, table)
	}
	response = apiRequest(t, handler, http.MethodGet, "/api/admin/backup", "", cookie)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/vnd.sqlite3" {
		t.Fatalf("download: %d %s", response.Code, response.Body.String())
	}
	backup := append([]byte(nil), response.Body.Bytes()...)
	if _, err = s.db.changeScore(student.ID, scoreInput{Delta: 7}); err != nil {
		t.Fatal(err)
	}
	if err = s.db.deleteClass(class.ID); err != nil {
		t.Fatal(err)
	}
	safetyStudents := tableSnapshot(t, s.db, "students")
	if err = os.RemoveAll(s.siteCacheRoot()); err != nil {
		t.Fatal(err)
	}
	response = restoreRequest(t, handler, backup, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", response.Code, response.Body.String())
	}
	var result backupRestoreResponse
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if actual := tableSnapshot(t, s.db, table); actual != expected[table] {
			t.Errorf("restored %s differs", table)
		}
	}
	for _, table := range []string{"teacher_sessions", "teacher_qr_logins"} {
		var count int
		if err = s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("credentials survived: %s %d %v", table, count, err)
		}
	}
	if response = apiRequest(t, handler, http.MethodGet, "/api/navigation", "", cookie); response.Code != http.StatusUnauthorized {
		t.Fatal("old session remains valid")
	}
	response = apiRequest(t, handler, http.MethodGet, site.URL, "", nil)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), html) {
		t.Fatalf("site cache rebuild: %d %s", response.Code, response.Body.String())
	}
	safetyPath := filepath.Join(filepath.Dir(s.db.path), "backups", result.SafetyBackup)
	safety, err := openStore(safetyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := tableSnapshot(t, safety, "students"); got != safetyStudents {
		t.Fatal("safety backup lost pre-restore changes")
	}
	safety.Close()
	var count int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='backup.restore'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("restore audit: %d %v", count, err)
	}
	if err = s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s.db, err = openStore(s.db.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if actual := tableSnapshot(t, s.db, table); actual != expected[table] {
			t.Errorf("restart changed %s", table)
		}
	}
}

func TestRestoreRejectsInvalidBackupWithoutChangingLiveData(t *testing.T) {
	for name, mutation := range map[string]string{
		"missing score history":     `DROP TABLE score_events`,
		"broken foreign key":        `PRAGMA foreign_keys=OFF; INSERT INTO students(class_id,student_no,name) VALUES(999,'01','orphan')`,
		"future schema":             `INSERT INTO schema_migrations(version) VALUES(999)`,
		"missing website source":    `INSERT INTO navigation_links(kind,title,url,sort_order) VALUES('site','missing','',0)`,
		"corrupt website archive":   `INSERT INTO navigation_links(id,kind,title,url,sort_order) VALUES(1,'site','broken','',0); INSERT INTO teaching_sites(navigation_id,public_id,revision,source_name,source_size,extracted_size,file_count,archive) VALUES(1,'1234567890abcdef','1234567890abcdef','broken',3,3,1,x'010203')`,
		"failed session revocation": `CREATE TRIGGER deny_session_delete BEFORE DELETE ON teacher_sessions BEGIN SELECT RAISE(ABORT,'failure injection'); END`,
	} {
		t.Run(name, func(t *testing.T) {
			s, handler, cookie := backupTestAPI(t)
			live := tableSnapshot(t, s.db, "teacher_sessions")
			path := filepath.Join(t.TempDir(), "invalid.db")
			if err := os.WriteFile(path, backupBytes(t, s.db), 0600); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			db.Close()
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			response := restoreRequest(t, handler, payload, cookie)
			if response.Code < 400 {
				t.Fatalf("invalid backup accepted: %s", response.Body.String())
			}
			if got := tableSnapshot(t, s.db, "teacher_sessions"); got != live {
				t.Fatal("live sessions changed after failed restore")
			}
			if response = apiRequest(t, handler, http.MethodGet, "/api/navigation", "", cookie); response.Code != http.StatusOK {
				t.Fatal("live database unavailable after rejection")
			}
		})
	}
}

func TestBackupUsesIndependentConnectionAndCommittedSnapshot(t *testing.T) {
	s := testStore(t)
	tx, err := s.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO classes(name) VALUES('uncommitted')`); err != nil {
		t.Fatal(err)
	}
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	go func() { p, e := s.createBackup(); done <- result{p, e} }()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		defer os.Remove(got.path)
		copy, err := openStore(got.path)
		if err != nil {
			t.Fatal(err)
		}
		defer copy.Close()
		classes, err := copy.classes(false)
		if err != nil || len(classes) != 0 {
			t.Fatalf("snapshot contains uncommitted data: %+v %v", classes, err)
		}
	case <-time.After(3 * time.Second):
		tx.Rollback()
		got := <-done
		os.Remove(got.path)
		t.Fatal("backup blocked on application's occupied writer connection")
	}
}

func TestCheckpointBusyDoesNotDiscardWAL(t *testing.T) {
	s := testStore(t)
	reader, err := sql.Open("sqlite", s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tx, err := reader.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM classes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Exec(`INSERT INTO classes(name) VALUES('WAL retained'); PRAGMA busy_timeout=10;`); err != nil {
		t.Fatal(err)
	}
	if err = s.checkpoint(); err == nil {
		t.Fatal("busy checkpoint accepted")
	}
	if info, err := os.Stat(s.path + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("WAL lost: %v", err)
	}
}

func TestSafetyRollbackPreservesBackup(t *testing.T) {
	s := testStore(t)
	if _, err := s.createClass(classInput{Grade: "六", ClassNo: "1"}); err != nil {
		t.Fatal(err)
	}
	safety, err := s.createSafetyBackup()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(safety)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = os.WriteFile(s.path, []byte("failed replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := restoreSafetyBackup(safety, s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	after, err := os.ReadFile(safety)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rollback consumed safety backup")
	}
	classes, err := restored.classes(false)
	if err != nil || len(classes) != 1 {
		t.Fatalf("rollback: %+v %v", classes, err)
	}
}

func TestStorageMigrationIsAppendOnlyAndIdempotent(t *testing.T) {
	s := testStore(t)
	// Simulate indexes and migration metadata from v1.8.3.
	if _, err := s.Exec(`DELETE FROM schema_migrations WHERE version=4; DROP INDEX idx_attendance_class_page;
 CREATE UNIQUE INDEX idx_teaching_sites_public ON teaching_sites(public_id);
 CREATE INDEX idx_audit_logs_created ON audit_logs(id DESC);`); err != nil {
		t.Fatal(err)
	}
	before := tableSnapshot(t, s, "schema_migrations")
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := s.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("version %d: %v", version, err)
	}
	after := tableSnapshot(t, s, "schema_migrations")
	if before == after {
		t.Fatal("migration not recorded")
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	if got := tableSnapshot(t, s, "schema_migrations"); got != after {
		t.Fatal("migration repeated")
	}
	// Query plans should use the page index, and avoid wrapping the date column.
	rows, err := s.Query(`EXPLAIN QUERY PLAN SELECT id FROM attendance_sessions WHERE class_id=1 AND deleted_at IS NULL ORDER BY id DESC LIMIT 30`)
	if err != nil {
		t.Fatal(err)
	}
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plans = append(plans, detail)
	}
	rows.Close()
	expected := []string{"SEARCH attendance_sessions USING COVERING INDEX idx_attendance_class_page (class_id=? AND deleted_at=?)"}
	if !reflect.DeepEqual(plans, expected) {
		t.Fatalf("query plan: %v", plans)
	}
	if _, err = s.Exec(fmt.Sprintf(`INSERT INTO schema_migrations(version) VALUES(%d)`, currentSchemaVersion+1)); err != nil {
		t.Fatal(err)
	}
	if err = s.migrate(); err == nil {
		t.Fatal("future schema was modified")
	}
}

func TestRestoreFailureBoundaries(t *testing.T) {
	for _, failSafety := range []bool{true, false} {
		t.Run(fmt.Sprintf("safety-failure=%v", failSafety), func(t *testing.T) {
			s, handler, cookie := backupTestAPI(t)
			payload := backupBytes(t, s.db)
			target := s.siteCacheRoot()
			if failSafety {
				target = filepath.Join(filepath.Dir(s.db.path), "backups")
			}
			if err := os.WriteFile(target, []byte("blocked directory"), 0600); err != nil {
				t.Fatal(err)
			}
			response := restoreRequest(t, handler, payload, cookie)
			auth := apiRequest(t, handler, http.MethodGet, "/api/navigation", "", cookie)
			if failSafety {
				if response.Code < 400 || auth.Code != http.StatusOK {
					t.Fatalf("safety failure changed live state: restore=%d auth=%d", response.Code, auth.Code)
				}
			} else if response.Code != http.StatusOK || auth.Code != http.StatusUnauthorized {
				t.Fatalf("cache failure left ambiguous restore: restore=%d auth=%d", response.Code, auth.Code)
			}
		})
	}
}

func TestAutomaticBackupReplacesCorruptDailyFile(t *testing.T) {
	s := testStore(t)
	path, err := s.createAutomaticBackup()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := s.createAutomaticBackup()
	if err != nil || rebuilt != path {
		t.Fatalf("daily repair: %q %v", rebuilt, err)
	}
	if err = validateBackupFile(rebuilt); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(rebuilt)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode: %v %v", info, err)
	}
}

func TestAttendanceReportIncludesWholeDateRange(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "三", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.createStudent(class.ID, studentInput{StudentNo: "01", Name: "日期测试"}); err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-09-17T23:59", "2026-09-18T00:00", "2026-09-18T23:59", "2026-09-19T00:00"} {
		session, err := s.createAttendance(attendanceInput{ClassID: class.ID, SessionAt: date})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.closeAttendance(session.ID); err != nil {
			t.Fatal(err)
		}
	}
	report, _, err := s.buildReport("attendance", class.ID, "2026-09-18", "2026-09-18")
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	rows, err := report.GetRows("报表")
	if err != nil || len(rows) != 3 || rows[1][0] != "2026-09-18 00:00:00" || rows[2][0] != "2026-09-18 23:59:00" {
		t.Fatalf("date boundaries: %v %v", rows, err)
	}
}
