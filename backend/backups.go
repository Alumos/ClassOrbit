package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func validateBackupFile(path string) error {
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	db, err := sql.Open("sqlite", fileURL+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var integrity string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("database integrity check failed: %s", integrity)
	}
	required := []string{"classes", "students", "score_events", "attendance_sessions", "attendance_records", "settings", "teacher_accounts", "teacher_sessions", "navigation_links", "schedule_lessons", "schedule_changes", "schedule_settings", "schedule_periods"}
	version, err := (&store{DB: db}).schemaVersion()
	if err != nil {
		return err
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("backup schema %d is newer than this application", version)
	}
	if version >= 1 {
		required = append(required, "audit_logs")
	}
	if version >= 2 {
		required = append(required, "teaching_sites")
	}
	if version >= 3 {
		required = append(required, "teacher_qr_logins")
	}
	for _, table := range required {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("required table %s is missing", table)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("backup contains broken foreign-key references")
	}
	return rows.Err()
}

// Verify the durable source, not an extracted cache that may hide corruption.
func (s *store) validateTeachingSiteSources() error {
	var inconsistent bool
	if err := s.QueryRow(`SELECT EXISTS(
 SELECT 1 FROM navigation_links n LEFT JOIN teaching_sites t ON t.navigation_id=n.id
 WHERE (n.kind='site' AND t.navigation_id IS NULL) OR (n.kind<>'site' AND t.navigation_id IS NOT NULL)
 )`).Scan(&inconsistent); err != nil {
		return err
	}
	if inconsistent {
		return errors.New("teaching-site navigation is incomplete")
	}
	rows, err := s.Query(`SELECT public_id,revision,archive FROM teaching_sites`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var publicID, revision string
		var archive []byte
		if err := rows.Scan(&publicID, &revision, &archive); err != nil {
			return err
		}
		hash := sha256.Sum256(archive)
		if !validSiteToken(publicID) || revision != hex.EncodeToString(hash[:])[:24] {
			return errors.New("teaching-site source checksum is invalid")
		}
		if err := validateSiteArchive(archive); err != nil {
			return err
		}
	}
	return rows.Err()
}

func validateSiteArchive(archive []byte) error {
	workspace, err := os.MkdirTemp("", "classorbit-verify-site-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	zipPath := filepath.Join(workspace, "site.zip")
	if err := os.WriteFile(zipPath, archive, 0600); err != nil {
		return err
	}
	destination := filepath.Join(workspace, "site")
	prepared := &preparedTeachingSite{}
	if err := extractSiteArchive(zipPath, destination, prepared); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(destination, "index.html"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("teaching-site source metadata is invalid")
	}
	return nil
}

func (s *store) checkpoint() error {
	var busy, pages, checkpointed int
	if err := s.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &pages, &checkpointed); err != nil {
		return err
	}
	if busy != 0 {
		return errors.New("database checkpoint is busy")
	}
	return nil
}

func (s *store) createBackup() (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".classorbit-backup-*.db")
	if err != nil {
		return "", err
	}
	path := temp.Name()
	if err := temp.Close(); err != nil {
		return "", err
	}
	_ = os.Remove(path)
	if err := s.backupTo(path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (s *store) backupTo(path string) error {
	// An independent WAL reader snapshots committed data without occupying the
	// application's single writer connection throughout VACUUM INTO.
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(s.path)}).String()
	db, err := sql.Open("sqlite", fileURL+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`VACUUM INTO ?`, path); err != nil {
		_ = os.Remove(path)
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	return file.Sync()
}

func (s *store) createSafetyBackup() (string, error) {
	directory := filepath.Join(filepath.Dir(s.path), "backups")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "classorbit-before-restore-"+time.Now().Format("20060102-150405.000000000")+".db")
	if err := s.backupTo(path); err != nil {
		return "", err
	}
	return path, nil
}

// Called only after the failed candidate connection has been closed.
func restoreSafetyBackup(safetyPath, currentPath string) (*store, error) {
	source, err := os.Open(safetyPath)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	temp, err := os.CreateTemp(filepath.Dir(currentPath), ".classorbit-rollback-*.db")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temp.Name())
	_, copyErr := io.Copy(temp, source)
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return nil, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(currentPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := os.Rename(temp.Name(), currentPath); err != nil {
		return nil, err
	}
	return openStore(currentPath)
}
