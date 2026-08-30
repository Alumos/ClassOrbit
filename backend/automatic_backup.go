package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (s *server) startBackupScheduler() func() {
	ctx, cancel := context.WithCancel(context.Background())
	if env("AUTO_BACKUP_ENABLED", "true") != "true" {
		return cancel
	}
	run := func() {
		s.maintenanceMu.RLock()
		defer s.maintenanceMu.RUnlock()
		if path, err := s.db.createAutomaticBackup(); err != nil {
			log.Printf("automatic backup failed: %v", err)
		} else if path != "" {
			log.Printf("automatic backup ready: %s", filepath.Base(path))
		}
	}
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				run()
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		cancel()
		wait.Wait()
	}
}

func (s *store) createAutomaticBackup() (string, error) {
	directory := filepath.Join(filepath.Dir(s.path), "backups")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", err
	}
	target := filepath.Join(directory, "classorbit-auto-"+time.Now().Format("20060102")+".db")
	if _, err := os.Stat(target); err == nil {
		if err := cleanupAutomaticBackups(directory, backupRetentionDays()); err != nil {
			return "", err
		}
		return "", nil
	}
	temp, err := os.CreateTemp(directory, ".classorbit-auto-*.db")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return "", err
	}
	_ = os.Remove(tempPath)
	if err := s.backupTo(tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := os.Rename(tempPath, target); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := cleanupAutomaticBackups(directory, backupRetentionDays()); err != nil {
		return target, err
	}
	return target, nil
}

func backupRetentionDays() int {
	days, err := strconv.Atoi(env("BACKUP_RETENTION_DAYS", "14"))
	if err != nil || days < 1 || days > 90 {
		return 14
	}
	return days
}

func cleanupAutomaticBackups(directory string, retentionDays int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "classorbit-auto-") || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
