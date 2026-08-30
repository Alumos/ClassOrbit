package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type auditLog struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	EntityType string `json:"entityType"`
	EntityID   *int64 `json:"entityId"`
	Summary    string `json:"summary"`
	Details    string `json:"details"`
	Actor      string `json:"actor"`
	CreatedAt  string `json:"createdAt"`
}

type statementExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func addAudit(executor statementExecutor, action, entityType string, entityID int64, summary, details string) error {
	var nullableID any
	if entityID > 0 {
		nullableID = entityID
	}
	_, err := executor.Exec(`INSERT INTO audit_logs(action,entity_type,entity_id,summary,details)
VALUES(?,?,?,?,?)`, action, entityType, nullableID, strings.TrimSpace(summary), strings.TrimSpace(details))
	return err
}

func (s *store) auditLogs(limit int, beforeID int64) ([]auditLog, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	query := `SELECT id,action,entity_type,entity_id,summary,details,actor,created_at
FROM audit_logs`
	args := []any{}
	if beforeID > 0 {
		query += ` WHERE id<?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []auditLog{}
	for rows.Next() {
		var item auditLog
		var entityID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Action, &item.EntityType, &entityID, &item.Summary, &item.Details, &item.Actor, &item.CreatedAt); err != nil {
			return nil, err
		}
		if entityID.Valid {
			item.EntityID = &entityID.Int64
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *store) updateTeacherPassword(passwordHash string) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE teacher_accounts SET password_hash=? WHERE id=1`, passwordHash)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return errNotFound
	}
	if _, err := tx.Exec(`DELETE FROM teacher_sessions`); err != nil {
		return err
	}
	if err := addAudit(tx, "password.change", "teacher", 1, "修改教师登录密码", "已注销其他登录会话"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) undoScoreEvent(eventID int64) (studentRow, error) {
	tx, err := s.Begin()
	if err != nil {
		return studentRow{}, err
	}
	defer tx.Rollback()

	var studentID int64
	var delta int
	var reason string
	var reversalOf sql.NullInt64
	var reversedAt sql.NullString
	err = tx.QueryRow(`SELECT student_id,delta,reason,reversal_of,reversed_at
FROM score_events WHERE id=?`, eventID).Scan(&studentID, &delta, &reason, &reversalOf, &reversedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return studentRow{}, errNotFound
	}
	if err != nil {
		return studentRow{}, err
	}
	if reversalOf.Valid || reversedAt.Valid {
		return studentRow{}, fmt.Errorf("%w: 该积分流水已经撤销或属于撤销记录", errConflict)
	}

	result, err := tx.Exec(`UPDATE students SET score=score-? WHERE id=? AND deleted_at IS NULL`, delta, studentID)
	if err != nil {
		return studentRow{}, err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return studentRow{}, errNotFound
	}
	undoReason := "撤销：" + strings.TrimSpace(reason)
	if strings.TrimSpace(reason) == "" {
		undoReason = "撤销积分变更"
	}
	if _, err := tx.Exec(`INSERT INTO score_events(student_id,delta,reason,reversal_of)
VALUES(?,?,?,?)`, studentID, -delta, undoReason, eventID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return studentRow{}, fmt.Errorf("%w: 该积分流水已经撤销", errConflict)
		}
		return studentRow{}, err
	}
	if _, err := tx.Exec(`UPDATE score_events SET reversed_at=datetime('now','localtime') WHERE id=?`, eventID); err != nil {
		return studentRow{}, err
	}
	if err := addAudit(tx, "score.undo", "score_event", eventID, "撤销积分流水", fmt.Sprintf("student_id=%d, delta=%d", studentID, delta)); err != nil {
		return studentRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return studentRow{}, err
	}
	return s.studentByID(studentID)
}
