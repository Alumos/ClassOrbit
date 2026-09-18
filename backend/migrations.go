package main

import (
	"database/sql"
	"fmt"
)

const currentSchemaVersion = 4

// schemaMigration is append-only. The original ClassOrbit schema predates
// versioned migrations, so migrate() first guarantees the baseline and then
// hands every subsequent schema change to this atomic runner.
type schemaMigration struct {
	version int
	apply   func(*sql.Tx) error
}

func (s *store) applyVersionedMigrations() error {
	if _, err := s.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
	)`); err != nil {
		return err
	}

	migrations := []schemaMigration{
		{version: 1, apply: migrateOperationalSafety},
		{version: 2, apply: migrateTeachingSites},
		{version: 3, apply: migrateQRLogin},
		{version: 4, apply: migrateStorageIndexes},
	}
	for _, migration := range migrations {
		var applied bool
		if err := s.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=?)`, migration.version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		tx, err := s.Begin()
		if err != nil {
			return err
		}
		if err := migration.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply database migration %d: %w", migration.version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, migration.version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Versioned databases already contain the baseline; never replay legacy data repairs.
func (s *store) schemaVersion() (int, error) {
	var exists bool
	if err := s.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='schema_migrations')`).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var version int
	err := s.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version)
	return version, err
}

func migrateStorageIndexes(tx *sql.Tx) error {
	_, err := tx.Exec(`
  DROP INDEX IF EXISTS idx_teaching_sites_public;
  DROP INDEX IF EXISTS idx_audit_logs_created;
  CREATE INDEX IF NOT EXISTS idx_attendance_class_page ON attendance_sessions(class_id,deleted_at,id DESC);
 `)
	return err
}

func migrateQRLogin(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS teacher_qr_logins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scan_token_hash TEXT NOT NULL UNIQUE,
			claim_token_hash TEXT NOT NULL UNIQUE,
			device_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','scanned','approved','denied','consumed','expired')),
			created_at INTEGER NOT NULL DEFAULT (unixepoch()),
			expires_at INTEGER NOT NULL,
			approved_at INTEGER,
			consumed_at INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_teacher_qr_logins_expires ON teacher_qr_logins(expires_at);
	`)
	return err
}

func migrateTeachingSites(tx *sql.Tx) error {
	exists, err := migrationColumnExists(tx, "navigation_links", "kind")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec(`ALTER TABLE navigation_links ADD COLUMN kind TEXT NOT NULL DEFAULT 'external' CHECK(kind IN ('external','site'))`); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS teaching_sites (
			navigation_id INTEGER PRIMARY KEY REFERENCES navigation_links(id) ON DELETE CASCADE,
			public_id TEXT NOT NULL UNIQUE,
			revision TEXT NOT NULL,
			source_name TEXT NOT NULL,
			source_size INTEGER NOT NULL,
			extracted_size INTEGER NOT NULL,
			file_count INTEGER NOT NULL,
			archive BLOB NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_teaching_sites_public ON teaching_sites(public_id);
	`)
	return err
}

func migrateOperationalSafety(tx *sql.Tx) error {
	columns := []struct {
		table  string
		column string
		ddl    string
	}{
		{"classes", "deleted_at", `ALTER TABLE classes ADD COLUMN deleted_at TEXT`},
		{"students", "deleted_at", `ALTER TABLE students ADD COLUMN deleted_at TEXT`},
		{"attendance_sessions", "class_name_snapshot", `ALTER TABLE attendance_sessions ADD COLUMN class_name_snapshot TEXT NOT NULL DEFAULT ''`},
		{"attendance_sessions", "deleted_at", `ALTER TABLE attendance_sessions ADD COLUMN deleted_at TEXT`},
		{"attendance_records", "student_no_snapshot", `ALTER TABLE attendance_records ADD COLUMN student_no_snapshot TEXT NOT NULL DEFAULT ''`},
		{"attendance_records", "student_name_snapshot", `ALTER TABLE attendance_records ADD COLUMN student_name_snapshot TEXT NOT NULL DEFAULT ''`},
		{"score_events", "reversal_of", `ALTER TABLE score_events ADD COLUMN reversal_of INTEGER REFERENCES score_events(id)`},
		{"score_events", "reversed_at", `ALTER TABLE score_events ADD COLUMN reversed_at TEXT`},
		{"score_events", "actor", `ALTER TABLE score_events ADD COLUMN actor TEXT NOT NULL DEFAULT 'teacher'`},
	}
	for _, column := range columns {
		exists, err := migrationColumnExists(tx, column.table, column.column)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(column.ddl); err != nil {
				return err
			}
		}
	}

	_, err := tx.Exec(`
		UPDATE attendance_sessions
		SET class_name_snapshot=COALESCE((SELECT name FROM classes WHERE id=attendance_sessions.class_id),'')
		WHERE class_name_snapshot='';
		UPDATE attendance_records
		SET student_no_snapshot=COALESCE((SELECT student_no FROM students WHERE id=attendance_records.student_id),''),
			student_name_snapshot=COALESCE((SELECT name FROM students WHERE id=attendance_records.student_id),'')
		WHERE student_no_snapshot='' OR student_name_snapshot='';

		DROP INDEX IF EXISTS idx_one_active_session;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_session
		ON attendance_sessions(class_id) WHERE status='active' AND deleted_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_classes_active ON classes(deleted_at,id);
		CREATE INDEX IF NOT EXISTS idx_students_active_class ON students(class_id,deleted_at,student_no);
		CREATE INDEX IF NOT EXISTS idx_attendance_deleted ON attendance_sessions(deleted_at,id DESC);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_score_events_one_reversal
		ON score_events(reversal_of) WHERE reversal_of IS NOT NULL;

		CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER,
			summary TEXT NOT NULL,
			details TEXT NOT NULL DEFAULT '',
			actor TEXT NOT NULL DEFAULT 'teacher',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(id DESC);
	`)
	return err
}

func migrationColumnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
