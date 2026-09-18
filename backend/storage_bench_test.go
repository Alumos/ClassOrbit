package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Synthetic classroom-scale history: 50 classes, 2,500 students,
// 100,000 score events and 3,000 sessions / 150,000 attendance records.
func benchmarkHistory(b *testing.B) *store {
	b.Helper()
	s, err := openStore(filepath.Join(b.TempDir(), "benchmark.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	_, err = s.Exec(`
 WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<50)
 INSERT INTO classes(id,name,grade,class_no) SELECT x,'班级'||x,'三',x FROM n;
 WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2500)
 INSERT INTO students(id,class_id,student_no,name,score) SELECT x,(x-1)/50+1,printf('%04d',x),'学生'||x,10 FROM n;
 WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<100000)
 INSERT INTO score_events(student_id,delta,reason) SELECT (x-1)%2500+1,1,'课堂' FROM n;
 WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<3000)
 INSERT INTO attendance_sessions(id,class_id,title,status,session_at,class_name_snapshot)
 SELECT x,(x-1)%50+1,'历史课堂','closed',datetime('2026-01-01','+'||(x/50)||' days'),'班级'||((x-1)%50+1) FROM n;
 INSERT INTO attendance_records(session_id,student_id,status,student_no_snapshot,student_name_snapshot)
 SELECT a.id,st.id,'present',st.student_no,st.name FROM attendance_sessions a JOIN students st ON st.class_id=a.class_id;
 ANALYZE;
 `)
	if err != nil {
		b.Fatal(err)
	}
	return s
}

func BenchmarkStorage(b *testing.B) {
	s := benchmarkHistory(b)
	for name, run := range map[string]func() error{
		"reopen": func() error {
			reopened, err := openStore(s.path)
			if err != nil {
				return err
			}
			return reopened.Close()
		},
		"classes":          func() error { _, e := s.classes(false); return e },
		"roster":           func() error { _, e := s.students(1, ""); return e },
		"integration":      func() error { _, e := s.integrationClasses(); return e },
		"attendance_all":   func() error { _, e := s.attendance(0, "", false, 0, 30); return e },
		"attendance_class": func() error { _, e := s.attendance(1, "", false, 0, 30); return e },
		"attendance_date":  func() error { _, e := s.attendance(0, "2026-01-20", false, 0, 30); return e },
		"score_history":    func() error { _, e := s.scoreEvents(1); return e },
		"backup": func() error {
			p, e := s.createBackup()
			if p != "" {
				os.Remove(p)
			}
			return e
		},
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := run(); err != nil {
					b.Fatal(fmt.Errorf("%s: %w", name, err))
				}
			}
		})
	}
}
