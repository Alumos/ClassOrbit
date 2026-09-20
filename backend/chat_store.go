package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Keep attendance and credential issuance atomic. A repeated check-in never
// grants a new credential merely because the roster says the student is present.
func (s *store) checkInWithChat(classID, studentID int64, tokenHash, previousHash string, now time.Time) (checkInResponse, error) {
	tx, err := s.Begin()
	if err != nil {
		return checkInResponse{}, err
	}
	defer tx.Rollback()
	var sessionID int64
	var name, status string
	err = tx.QueryRow(`SELECT a.id,st.name,r.status FROM attendance_sessions a
 JOIN classes c ON c.id=a.class_id JOIN attendance_records r ON r.session_id=a.id
 JOIN students st ON st.id=r.student_id AND st.class_id=a.class_id
 WHERE a.class_id=? AND a.status='active' AND a.deleted_at IS NULL
 AND c.deleted_at IS NULL AND st.deleted_at IS NULL AND st.id=?`, classID, studentID).Scan(&sessionID, &name, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return checkInResponse{}, fmt.Errorf("%w: 未找到开放签到或学生信息不匹配", errConflict)
	}
	if err != nil {
		return checkInResponse{}, err
	}
	if status != "absent" {
		return checkInResponse{}, fmt.Errorf("%w: %s 已完成签到，请勿重复提交", errConflict, name)
	}
	result, err := tx.Exec(`UPDATE attendance_records SET status='present',checked_at=datetime('now','localtime'),method='self' WHERE session_id=? AND student_id=? AND status='absent'`, sessionID, studentID)
	if err != nil {
		return checkInResponse{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return checkInResponse{}, err
	}
	if changed != 1 {
		return checkInResponse{}, errConflict
	}
	if tokenHash != "" {
		if _, err = tx.Exec(`DELETE FROM student_chat_sessions WHERE token_hash=? OR (student_id=? AND attendance_session_id=?)`, previousHash, studentID, sessionID); err != nil {
			return checkInResponse{}, err
		}
		if _, err = tx.Exec(`INSERT INTO student_chat_sessions(token_hash,student_id,attendance_session_id,created_at,expires_at) VALUES(?,?,?,?,?)`, tokenHash, studentID, sessionID, now.Unix(), now.Add(studentChatLifetime).Unix()); err != nil {
			return checkInResponse{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return checkInResponse{}, err
	}
	return checkInResponse{OK: true, Name: name, Message: "签到成功"}, nil
}

const chatIdentityQuery = `SELECT t.token_hash,t.attendance_session_id,c.id,c.name,a.title,st.id,st.name,t.expires_at
 FROM student_chat_sessions t JOIN students st ON st.id=t.student_id
 JOIN attendance_sessions a ON a.id=t.attendance_session_id
 JOIN classes c ON c.id=a.class_id
 JOIN attendance_records r ON r.session_id=a.id AND r.student_id=st.id
 WHERE t.expires_at>? AND a.status='active' AND a.deleted_at IS NULL
 AND c.deleted_at IS NULL AND st.deleted_at IS NULL AND st.class_id=c.id
 AND r.status IN ('present','late')`

func (s *store) studentChatIdentity(hash string, now time.Time) (chatIdentity, error) {
	var v chatIdentity
	err := s.QueryRow(chatIdentityQuery+` AND t.token_hash=?`, now.Unix(), hash).Scan(&v.TokenHash, &v.RoomID, &v.ClassID, &v.ClassName, &v.Title, &v.StudentID, &v.Name, &v.ExpiresAt)
	return v, err
}

func (s *store) chatRooms(classID int64) ([]chatRoomInfo, error) {
	result := []chatRoomInfo{}
	rows, err := s.Query(`SELECT a.id,c.id,c.name,a.title FROM attendance_sessions a JOIN classes c ON c.id=a.class_id
 WHERE a.status='active' AND a.deleted_at IS NULL AND c.deleted_at IS NULL AND (?=0 OR c.id=?) ORDER BY a.id DESC`, classID, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v chatRoomInfo
		if err := rows.Scan(&v.RoomID, &v.ClassID, &v.ClassName, &v.Title); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// Only connected credentials are checked; the query is bounded by 500 clients.
func (s *store) validChatTokens(hashes []string, teacher bool, now time.Time) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(hashes) == 0 {
		return result, nil
	}
	args := []any{now.Unix()}
	for _, hash := range hashes {
		args = append(args, hash)
	}
	query := chatIdentityQuery + ` AND t.token_hash IN (` + strings.TrimSuffix(strings.Repeat("?,", len(hashes)), ",") + `)`
	if teacher {
		query = `SELECT token_hash FROM teacher_sessions WHERE expires_at>? AND token_hash IN (` + strings.TrimSuffix(strings.Repeat("?,", len(hashes)), ",") + `)`
	}
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var hash string
		if teacher {
			err = rows.Scan(&hash)
		} else {
			var v chatIdentity
			err = rows.Scan(&hash, &v.RoomID, &v.ClassID, &v.ClassName, &v.Title, &v.StudentID, &v.Name, &v.ExpiresAt)
		}
		if err != nil {
			return nil, err
		}
		result[hash] = true
	}
	return result, rows.Err()
}

func (s *store) deleteChatToken(hash string) error {
	_, err := s.Exec(`DELETE FROM student_chat_sessions WHERE token_hash=?`, hash)
	return err
}
func (s *store) pruneChatTokens(now time.Time) error {
	_, err := s.Exec(`DELETE FROM student_chat_sessions WHERE token_hash IN (SELECT token_hash FROM student_chat_sessions WHERE expires_at<=? ORDER BY expires_at LIMIT 1000)`, now.Unix())
	return err
}
func (s *store) clearChatTokens() error {
	_, err := s.Exec(`DELETE FROM student_chat_sessions`)
	return err
}
func (s *store) auditChat(roomID int64, action string, count int) error {
	return addAudit(s, "chat."+action, "attendance", roomID, "聊天室管理", fmt.Sprintf("%s; messages=%d", action, count))
}
