package main

import (
	"database/sql"
	"fmt"
)

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

	migrations := []schemaMigration{{version: 1, apply: migrateOperationalSafety}}
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
