package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
)

const qrLoginLifetime = 2 * time.Minute

func randomURLToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validQRLoginToken(token string) bool {
	value, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(value) == 32
}

func (s *server) startQRLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceName string `json:"deviceName"`
	}
	if !decode(w, r, &in) {
		return
	}
	deviceName := strings.TrimSpace(in.DeviceName)
	if deviceName == "" {
		deviceName = "电脑浏览器"
	}
	if len([]rune(deviceName)) > 80 {
		badRequest(w, "设备名称过长")
		return
	}
	scanToken, err := randomURLToken()
	if err != nil {
		respond(w, nil, err)
		return
	}
	claimToken, err := randomURLToken()
	if err != nil {
		respond(w, nil, err)
		return
	}
	expiresAt := time.Now().Add(qrLoginLifetime).Unix()
	if err := s.db.createQRLogin(hashSessionToken(scanToken), hashSessionToken(claimToken), deviceName, expiresAt); err != nil {
		respond(w, nil, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, qrLoginStartResponse{Token: scanToken, ClaimToken: claimToken, ExpiresAt: expiresAt})
}

func (s *server) getQRLoginChallenge(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if !validQRLoginToken(token) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "二维码无效或已失效"})
		return
	}
	record, err := s.db.scanQRLogin(hashSessionToken(token), time.Now().Unix())
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "二维码无效或已失效"})
		return
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, record)
}

func (s *server) decideQRLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token   string `json:"token"`
		Approve bool   `json:"approve"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validQRLoginToken(in.Token) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "二维码无效或已失效"})
		return
	}
	record, err := s.db.decideQRLogin(hashSessionToken(in.Token), in.Approve, time.Now().Unix())
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "二维码无效或已失效"})
		return
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	if record.Status == "expired" {
		writeJSON(w, http.StatusGone, apiError{Error: "二维码已过期，请在电脑上刷新"})
		return
	}
	if record.Status == "consumed" {
		writeJSON(w, http.StatusConflict, apiError{Error: "二维码已经使用"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, record)
}

func (s *server) getQRLoginStatus(w http.ResponseWriter, r *http.Request) {
	var in qrLoginTokenInput
	if !decode(w, r, &in) {
		return
	}
	if !validQRLoginToken(in.Token) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "扫码登录请求不存在"})
		return
	}
	sessionToken, err := randomURLToken()
	if err != nil {
		respond(w, nil, err)
		return
	}
	now := time.Now()
	record, claimed, err := s.db.claimQRLogin(hashSessionToken(in.Token), hashSessionToken(sessionToken), now.Add(teacherSessionLifetime).Unix(), now.Unix())
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "扫码登录请求不存在"})
		return
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if claimed {
		setTeacherSessionCookie(w, r, sessionToken, now.Add(teacherSessionLifetime))
		account, accountErr := s.db.teacherAccount()
		if accountErr != nil {
			respond(w, nil, accountErr)
			return
		}
		writeJSON(w, http.StatusOK, qrAuthenticatedResponse{Status: "authenticated", Authenticated: true, Username: account.Username})
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *store) createQRLogin(scanHash, claimHash, deviceName string, expiresAt int64) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM teacher_qr_logins WHERE expires_at<?`, time.Now().Add(-24*time.Hour).Unix()); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO teacher_qr_logins(scan_token_hash,claim_token_hash,device_name,expires_at) VALUES(?,?,?,?)`, scanHash, claimHash, deviceName, expiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) scanQRLogin(scanHash string, now int64) (qrLoginRecord, error) {
	if _, err := s.Exec(`UPDATE teacher_qr_logins SET status='expired' WHERE scan_token_hash=? AND expires_at<=? AND status IN ('pending','scanned','approved')`, scanHash, now); err != nil {
		return qrLoginRecord{}, err
	}
	if _, err := s.Exec(`UPDATE teacher_qr_logins SET status='scanned' WHERE scan_token_hash=? AND expires_at>? AND status='pending'`, scanHash, now); err != nil {
		return qrLoginRecord{}, err
	}
	return s.qrLoginByScanHash(scanHash)
}

func (s *store) qrLoginByScanHash(scanHash string) (qrLoginRecord, error) {
	var record qrLoginRecord
	err := s.QueryRow(`SELECT device_name,status,expires_at FROM teacher_qr_logins WHERE scan_token_hash=?`, scanHash).Scan(&record.DeviceName, &record.Status, &record.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return record, errNotFound
	}
	return record, err
}

func (s *store) decideQRLogin(scanHash string, approve bool, now int64) (qrLoginRecord, error) {
	record, err := s.scanQRLogin(scanHash, now)
	if err != nil || record.Status == "expired" || record.Status == "consumed" {
		return record, err
	}
	status := "denied"
	if approve {
		status = "approved"
	}
	if _, err := s.Exec(`UPDATE teacher_qr_logins SET status=?,approved_at=CASE WHEN ?='approved' THEN ? ELSE NULL END WHERE scan_token_hash=? AND status IN ('pending','scanned') AND expires_at>?`, status, status, now, scanHash, now); err != nil {
		return qrLoginRecord{}, err
	}
	return s.qrLoginByScanHash(scanHash)
}

func (s *store) claimQRLogin(claimHash, sessionHash string, sessionExpiresAt, now int64) (qrLoginRecord, bool, error) {
	tx, err := s.Begin()
	if err != nil {
		return qrLoginRecord{}, false, err
	}
	defer tx.Rollback()
	var record qrLoginRecord
	if err := tx.QueryRow(`SELECT device_name,status,expires_at FROM teacher_qr_logins WHERE claim_token_hash=?`, claimHash).Scan(&record.DeviceName, &record.Status, &record.ExpiresAt); errors.Is(err, sql.ErrNoRows) {
		return record, false, errNotFound
	} else if err != nil {
		return record, false, err
	}
	if record.ExpiresAt <= now && record.Status != "consumed" && record.Status != "denied" {
		if _, err := tx.Exec(`UPDATE teacher_qr_logins SET status='expired' WHERE claim_token_hash=?`, claimHash); err != nil {
			return record, false, err
		}
		record.Status = "expired"
	}
	if record.Status != "approved" {
		return record, false, tx.Commit()
	}
	result, err := tx.Exec(`UPDATE teacher_qr_logins SET status='consumed',consumed_at=? WHERE claim_token_hash=? AND status='approved' AND expires_at>?`, now, claimHash, now)
	if err != nil {
		return record, false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return record, false, err
	}
	if _, err := tx.Exec(`DELETE FROM teacher_sessions WHERE expires_at<=?`, now); err != nil {
		return record, false, err
	}
	if _, err := tx.Exec(`INSERT INTO teacher_sessions(token_hash,teacher_id,expires_at) VALUES(?,1,?)`, sessionHash, sessionExpiresAt); err != nil {
		return record, false, err
	}
	record.Status = "consumed"
	return record, true, tx.Commit()
}
