package main

import (
	"database/sql"
	"errors"
	"fmt"
)

func (s *store) createTeachingSite(title, iconURL, publicID string, prepared *preparedTeachingSite, archive []byte) (navigationLink, error) {
	tx, err := s.Begin()
	if err != nil {
		return navigationLink{}, err
	}
	defer tx.Rollback()
	var navigationCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM navigation_links`).Scan(&navigationCount); err != nil {
		return navigationLink{}, err
	}
	if navigationCount >= 100 {
		return navigationLink{}, fmt.Errorf("%w: 导航项目不能超过 100 个", errConflict)
	}
	var storedSize int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(length(archive)),0) FROM teaching_sites`).Scan(&storedSize); err != nil {
		return navigationLink{}, err
	}
	if storedSize+int64(len(archive)) > maxSiteStoredSize {
		return navigationLink{}, fmt.Errorf("%w: 教学网页压缩源总量不能超过 384MB，请先删除不再使用的项目", errConflict)
	}
	var sortOrder int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order)+1,0) FROM navigation_links`).Scan(&sortOrder); err != nil {
		return navigationLink{}, err
	}
	result, err := tx.Exec(`INSERT INTO navigation_links(kind,title,url,icon_url,sort_order) VALUES('site',?,'',?,?)`, title, iconURL, sortOrder)
	if err != nil {
		return navigationLink{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return navigationLink{}, err
	}
	if _, err := tx.Exec(`INSERT INTO teaching_sites(navigation_id,public_id,revision,source_name,source_size,extracted_size,file_count,archive) VALUES(?,?,?,?,?,?,?,?)`,
		id, publicID, prepared.revision, prepared.sourceName, prepared.sourceSize, prepared.extractedSize, prepared.fileCount, archive); err != nil {
		return navigationLink{}, err
	}
	if err := addAudit(tx, "teaching_site.create", "navigation", id, "上传教学网页", fmt.Sprintf("%s，%d 个文件", title, prepared.fileCount)); err != nil {
		return navigationLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return navigationLink{}, err
	}
	return s.navigationByID(id)
}

func (s *store) updateTeachingSite(id int64, prepared *preparedTeachingSite, archive []byte) (navigationLink, error) {
	tx, err := s.Begin()
	if err != nil {
		return navigationLink{}, err
	}
	defer tx.Rollback()
	var storedSize int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(length(archive)),0) FROM teaching_sites WHERE navigation_id<>?`, id).Scan(&storedSize); err != nil {
		return navigationLink{}, err
	}
	if storedSize+int64(len(archive)) > maxSiteStoredSize {
		return navigationLink{}, fmt.Errorf("%w: 教学网页压缩源总量不能超过 384MB，请先删除不再使用的项目", errConflict)
	}
	result, err := tx.Exec(`UPDATE teaching_sites SET revision=?,source_name=?,source_size=?,extracted_size=?,file_count=?,archive=?,updated_at=datetime('now','localtime') WHERE navigation_id=?`,
		prepared.revision, prepared.sourceName, prepared.sourceSize, prepared.extractedSize, prepared.fileCount, archive, id)
	if err != nil {
		return navigationLink{}, err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return navigationLink{}, errNotFound
	}
	if err := addAudit(tx, "teaching_site.update", "navigation", id, "替换教学网页", fmt.Sprintf("%s，%d 个文件", prepared.sourceName, prepared.fileCount)); err != nil {
		return navigationLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return navigationLink{}, err
	}
	return s.navigationByID(id)
}

func (s *store) deleteTeachingSite(id int64) (string, error) {
	tx, err := s.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var publicID, title string
	err = tx.QueryRow(`SELECT s.public_id,n.title FROM teaching_sites s JOIN navigation_links n ON n.id=s.navigation_id WHERE s.navigation_id=?`, id).Scan(&publicID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`DELETE FROM navigation_links WHERE id=? AND kind='site'`, id); err != nil {
		return "", err
	}
	if err := addAudit(tx, "teaching_site.delete", "navigation", id, "删除教学网页", title); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return publicID, nil
}

func (s *store) teachingSiteArchive(publicID, revision string) ([]byte, error) {
	var archive []byte
	err := s.QueryRow(`SELECT archive FROM teaching_sites WHERE public_id=? AND revision=?`, publicID, revision).Scan(&archive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return archive, err
}
