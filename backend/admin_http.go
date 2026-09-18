package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

// A complete SQLite backup can also contain compressed teaching-site sources.
const maxRestoreSize = 512 << 20

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(w, r, &input) {
		return
	}
	if message := validateTeacherCredentials(&teacherCredentials{Username: "teacher", Password: input.NewPassword}); message != "" {
		badRequest(w, message)
		return
	}
	account, err := s.db.teacherAccount()
	if err != nil {
		respond(w, nil, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(input.CurrentPassword)) != nil {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: "当前密码不正确"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), teacherPasswordCost)
	if err != nil {
		respond(w, nil, err)
		return
	}
	if err := s.db.updateTeacherPassword(string(hash)); err != nil {
		respond(w, nil, err)
		return
	}
	clearTeacherSessionCookie(w, r)
	writeJSON(w, http.StatusOK, operationResponse{OK: true})
}

func (s *server) getAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before_id"), 10, 64)
	items, err := s.db.auditLogs(limit, beforeID)
	respond(w, items, err)
}

func (s *server) undoScore(w http.ResponseWriter, r *http.Request) {
	student, err := s.db.undoScoreEvent(pathID(r, "id"))
	respond(w, student, err)
}

func (s *server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	path, err := s.db.createBackup()
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer os.Remove(path)
	name := "classorbit-backup-" + time.Now().Format("20060102-150405") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	file, err := os.Open(path)
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		respond(w, nil, err)
		return
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (s *server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreSize+(4<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		badRequest(w, "备份文件不能超过 512MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil {
		badRequest(w, "请选择 ClassOrbit 数据库备份")
		return
	}
	defer file.Close()

	dataDir := filepath.Dir(s.db.path)
	temp, err := os.CreateTemp(dataDir, ".classorbit-restore-*.db")
	if err != nil {
		respond(w, nil, err)
		return
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	written, copyErr := io.Copy(temp, io.LimitReader(file, maxRestoreSize+1))
	closeErr := temp.Close()
	if copyErr != nil || closeErr != nil || written == 0 || written > maxRestoreSize {
		badRequest(w, "备份文件无效或超过 512MB")
		return
	}
	defer os.Remove(tempPath + "-wal")
	defer os.Remove(tempPath + "-shm")

	if err := validateBackupFile(tempPath); err != nil {
		badRequest(w, "无法识别该备份，请确认文件来自 ClassOrbit")
		return
	}
	candidate, err := openStore(tempPath)
	if err != nil {
		badRequest(w, "备份数据库升级失败，请确认文件版本正确")
		return
	}
	defer candidate.Close()
	if err := candidate.validateTeachingSiteSources(); err != nil {
		badRequest(w, "备份中的教学网页源文件不完整或损坏")
		return
	}
	prepared := &server{db: candidate}
	if err := prepared.refreshTeachingSiteRegistry(); err != nil {
		badRequest(w, "备份中的导航数据无效")
		return
	}
	safetyPath, err := s.db.createSafetyBackup()
	if err != nil {
		respond(w, nil, err)
		return
	}
	// Complete all fallible data changes before replacing the live database.
	tx, err := candidate.Begin()
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM teacher_sessions; DELETE FROM teacher_qr_logins;`); err == nil {
		err = addAudit(tx, "backup.restore", "system", 0, "恢复数据库备份", "恢复前安全备份："+filepath.Base(safetyPath))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	if err := candidate.checkpoint(); err != nil {
		respond(w, nil, err)
		return
	}
	if err := candidate.Close(); err != nil {
		respond(w, nil, err)
		return
	}
	if err := s.db.checkpoint(); err != nil {
		respond(w, nil, err)
		return
	}
	currentPath := s.db.path
	if err := s.db.Close(); err != nil {
		respond(w, nil, err)
		return
	}
	if err := os.Rename(tempPath, currentPath); err != nil {
		reopened, reopenErr := openStore(currentPath)
		if reopenErr == nil {
			s.db = reopened
		} else {
			err = fmt.Errorf("替换数据库失败：%v；重新打开原数据库失败：%w；安全备份：%s", err, reopenErr, safetyPath)
		}
		respond(w, nil, err)
		return
	}
	reopened, err := openStore(currentPath)
	if err != nil {
		// Keep the safety backup intact even if rollback itself encounters an error.
		rollbackStore, rollbackErr := restoreSafetyBackup(safetyPath, currentPath)
		if rollbackErr != nil {
			respond(w, nil, fmt.Errorf("恢复数据库失败：%v；回滚失败：%w；安全备份：%s", err, rollbackErr, safetyPath))
			return
		}
		s.db = rollbackStore
		respond(w, nil, err)
		return
	}
	s.db = reopened
	s.siteAccessMu.Lock()
	s.siteAccess = prepared.siteAccess
	s.siteAccessMu.Unlock()
	// The database is committed. A disposable cache cleanup failure must not
	// report a failed restore or prevent revocation of old credentials.
	if err := s.pruneSiteCache(); err != nil {
		log.Printf("warning: restored teaching-site cache cleanup failed: %v", err)
	}
	clearTeacherSessionCookie(w, r)
	writeJSON(w, http.StatusOK, backupRestoreResponse{
		OK:           true,
		SafetyBackup: filepath.Base(safetyPath),
		Message:      "恢复成功，请重新登录",
	})
}

func (s *server) exportReport(w http.ResponseWriter, r *http.Request) {
	reportType := strings.TrimSpace(r.URL.Query().Get("type"))
	classID, _ := strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	from, to := strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to"))
	if reportType != "roster" && reportType != "attendance" && reportType != "scores" {
		badRequest(w, "报表类型不正确")
		return
	}
	if classID <= 0 {
		badRequest(w, "请选择班级")
		return
	}
	if reportType == "attendance" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			badRequest(w, "请选择正确的开始日期")
			return
		}
		if _, err := time.Parse("2006-01-02", to); err != nil || to < from {
			badRequest(w, "请选择正确的结束日期")
			return
		}
	}
	book, className, err := s.db.buildReport(reportType, classID, from, to)
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer book.Close()
	fileName := fmt.Sprintf("ClassOrbit_%s_%s_%s.xlsx", strings.ReplaceAll(className, " ", ""), reportType, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+urlQueryEscape(fileName))
	w.Header().Set("Cache-Control", "no-store")
	if err := book.Write(w); err != nil {
		return
	}
}

func urlQueryEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func (s *store) buildReport(kind string, classID int64, from, to string) (*excelize.File, string, error) {
	var className string
	if err := s.QueryRow(`SELECT name FROM classes WHERE id=?`, classID).Scan(&className); errors.Is(err, sql.ErrNoRows) {
		return nil, "", errNotFound
	} else if err != nil {
		return nil, "", err
	}
	book := excelize.NewFile()
	sheet := "报表"
	_ = book.SetSheetName("Sheet1", sheet)
	writeRow := func(row int, values ...any) {
		for column, value := range values {
			cell, _ := excelize.CoordinatesToCellName(column+1, row)
			_ = book.SetCellValue(sheet, cell, value)
		}
	}
	switch kind {
	case "roster":
		writeRow(1, "学号", "姓名", "当前积分")
		rows, err := s.Query(`SELECT student_no,name,score FROM students
WHERE class_id=? AND deleted_at IS NULL ORDER BY CAST(student_no AS INTEGER),student_no`, classID)
		if err != nil {
			book.Close()
			return nil, "", err
		}
		row := 2
		for rows.Next() {
			var no, name string
			var score int
			if err := rows.Scan(&no, &name, &score); err != nil {
				rows.Close()
				book.Close()
				return nil, "", err
			}
			writeRow(row, no, name, score)
			row++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			book.Close()
			return nil, "", err
		}
		rows.Close()
	case "scores":
		writeRow(1, "学号", "姓名", "积分变更", "原因", "时间")
		rows, err := s.Query(`SELECT st.student_no,st.name,e.delta,e.reason,e.created_at
FROM score_events e JOIN students st ON st.id=e.student_id
WHERE st.class_id=? ORDER BY e.id DESC`, classID)
		if err != nil {
			book.Close()
			return nil, "", err
		}
		row := 2
		for rows.Next() {
			var no, name, reason, created string
			var delta int
			if err := rows.Scan(&no, &name, &delta, &reason, &created); err != nil {
				rows.Close()
				book.Close()
				return nil, "", err
			}
			writeRow(row, no, name, delta, reason, created)
			row++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			book.Close()
			return nil, "", err
		}
		rows.Close()
	case "attendance":
		writeRow(1, "上课日期", "课程", "班级", "学号", "姓名", "状态", "签到时间", "签到方式")
		rows, err := s.Query(`SELECT a.session_at,a.course,
COALESCE(NULLIF(a.class_name_snapshot,''),c.name,'已删除班级'),
COALESCE(NULLIF(r.student_no_snapshot,''),st.student_no,''),
COALESCE(NULLIF(r.student_name_snapshot,''),st.name,'已删除学生'),
r.status,COALESCE(r.checked_at,''),r.method
FROM attendance_sessions a LEFT JOIN classes c ON c.id=a.class_id
JOIN attendance_records r ON r.session_id=a.id LEFT JOIN students st ON st.id=r.student_id
WHERE a.class_id=? AND a.deleted_at IS NULL AND a.session_at>=? AND a.session_at<date(?,'+1 day')
ORDER BY a.session_at,CAST(COALESCE(NULLIF(r.student_no_snapshot,''),st.student_no,'') AS INTEGER)`, classID, from, to)
		if err != nil {
			book.Close()
			return nil, "", err
		}
		labels := map[string]string{"present": "已到", "absent": "缺席", "late": "迟到", "leave": "请假", "self": "学生自助", "teacher": "教师修正"}
		row := 2
		for rows.Next() {
			var date, course, classSnapshot, no, name, status, checked, method string
			if err := rows.Scan(&date, &course, &classSnapshot, &no, &name, &status, &checked, &method); err != nil {
				rows.Close()
				book.Close()
				return nil, "", err
			}
			writeRow(row, date, course, classSnapshot, no, name, labels[status], checked, labels[method])
			row++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			book.Close()
			return nil, "", err
		}
		rows.Close()
	}
	_ = book.SetColWidth(sheet, "A", "H", 18)
	_ = addAudit(s, "report.export", "class", classID, "导出 "+className+" 报表", "type="+kind)
	return book, className, nil
}
