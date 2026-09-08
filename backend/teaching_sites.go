package main

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// Keep this path versioned so previously immutable CDN responses cannot
	// preserve an outdated document sandbox policy after an application update.
	teachingSiteURLPrefix = "/published-v2"
	maxSiteArchiveSize    = 32 << 20
	maxSiteExtractedSize  = 128 << 20
	maxSiteRequestSize    = maxSiteExtractedSize + (4 << 20)
	maxSiteFileCount      = 2000
	maxSiteStoredSize     = 384 << 20
)

type preparedTeachingSite struct {
	workspace     string
	siteDir       string
	archivePath   string
	revision      string
	sourceName    string
	sourceSize    int64
	extractedSize int64
	fileCount     int
}

func (p *preparedTeachingSite) close() { _ = os.RemoveAll(p.workspace) }

func (s *server) createTeachingSite(w http.ResponseWriter, r *http.Request) {
	title, iconURL, prepared, ok := s.readTeachingSiteUpload(w, r)
	if !ok {
		return
	}
	defer prepared.close()

	publicID, err := randomSiteID()
	if err != nil {
		respond(w, nil, err)
		return
	}
	archive, err := os.ReadFile(prepared.archivePath)
	if err != nil {
		respond(w, nil, err)
		return
	}
	cachePath, err := s.publishPreparedSite(prepared, publicID)
	if err != nil {
		respond(w, nil, err)
		return
	}
	item, err := s.db.createTeachingSite(title, iconURL, publicID, prepared, archive)
	if err != nil {
		_ = os.RemoveAll(cachePath)
		respond(w, nil, err)
		return
	}
	s.setSiteAccess(publicID, prepared.revision)
	respondStatus(w, http.StatusCreated, item, nil)
}

func (s *server) getTeachingSite(w http.ResponseWriter, r *http.Request) {
	item, err := s.db.navigationByID(pathID(r, "id"))
	if err == nil && item.Kind != "site" {
		err = errNotFound
	}
	respond(w, item, err)
}

func (s *server) updateTeachingSite(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	current, err := s.db.navigationByID(id)
	if err != nil || current.Kind != "site" || current.Site == nil {
		if err == nil {
			err = errNotFound
		}
		respond(w, nil, err)
		return
	}
	_, _, prepared, ok := s.readTeachingSiteUpload(w, r)
	if !ok {
		return
	}
	defer prepared.close()
	archive, err := os.ReadFile(prepared.archivePath)
	if err != nil {
		respond(w, nil, err)
		return
	}
	cachePath, err := s.publishPreparedSite(prepared, current.Site.PublicID)
	if err != nil {
		respond(w, nil, err)
		return
	}
	item, err := s.db.updateTeachingSite(id, prepared, archive)
	if err != nil {
		if prepared.revision != current.Site.Revision {
			_ = os.RemoveAll(cachePath)
		}
		respond(w, nil, err)
		return
	}
	s.setSiteAccess(current.Site.PublicID, current.Site.Revision, prepared.revision)
	s.pruneSiteRevisions(current.Site.PublicID)
	respond(w, item, nil)
}

func (s *server) deleteTeachingSite(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	publicID, err := s.db.deleteTeachingSite(id)
	if err != nil {
		respond(w, nil, err)
		return
	}
	s.removeSiteAccess(publicID)
	_ = os.RemoveAll(filepath.Join(s.siteCacheRoot(), publicID))
	writeJSON(w, http.StatusOK, operationResponse{OK: true})
}

func (s *server) readTeachingSiteUpload(w http.ResponseWriter, r *http.Request) (string, string, *preparedTeachingSite, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSiteRequestSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		badRequest(w, "上传内容不能超过 128MB")
		return "", "", nil, false
	}
	defer r.MultipartForm.RemoveAll()
	title := strings.TrimSpace(r.FormValue("title"))
	iconURL := strings.TrimSpace(r.FormValue("iconUrl"))
	if title == "" {
		badRequest(w, "教学网页标题不能为空")
		return "", "", nil, false
	}
	if len([]rune(title)) > 50 {
		badRequest(w, "教学网页标题不能超过 50 个字符")
		return "", "", nil, false
	}
	if len(iconURL) > 2048 || (iconURL != "" && !validWebURL(iconURL)) {
		badRequest(w, "网站图标须为有效的 http 或 https 地址")
		return "", "", nil, false
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		badRequest(w, "请选择 HTML、项目文件夹或 ZIP 文件")
		return "", "", nil, false
	}
	mode := strings.TrimSpace(r.FormValue("mode"))
	paths := r.MultipartForm.Value["paths"]
	prepared, err := prepareTeachingSite(filepath.Dir(s.db.path), mode, strings.TrimSpace(r.FormValue("sourceName")), files, paths)
	if err != nil {
		badRequest(w, err.Error())
		return "", "", nil, false
	}
	return title, iconURL, prepared, true
}

func prepareTeachingSite(dataDir, mode, sourceName string, files []*multipart.FileHeader, paths []string) (*preparedTeachingSite, error) {
	workspace, err := os.MkdirTemp(dataDir, ".site-upload-")
	if err != nil {
		return nil, err
	}
	p := &preparedTeachingSite{workspace: workspace, sourceName: sourceName}
	fail := func(err error) (*preparedTeachingSite, error) {
		p.close()
		return nil, err
	}
	rawDir := filepath.Join(workspace, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return fail(err)
	}

	switch mode {
	case "html":
		if len(files) != 1 || !isHTMLName(files[0].Filename) {
			return fail(errors.New("单文件模式请选择一个 .html 或 .htm 文件"))
		}
		p.sourceName = files[0].Filename
		if err := copyMultipartFile(files[0], filepath.Join(rawDir, "index.html"), maxSiteExtractedSize, &p.sourceSize, &p.extractedSize, false); err != nil {
			return fail(err)
		}
		p.fileCount = 1
	case "zip":
		if len(files) != 1 || strings.ToLower(filepath.Ext(files[0].Filename)) != ".zip" {
			return fail(errors.New("ZIP 模式请选择一个 .zip 文件"))
		}
		if files[0].Size <= 0 || files[0].Size > maxSiteArchiveSize {
			return fail(errors.New("ZIP 文件不能超过 32MB"))
		}
		p.sourceName = files[0].Filename
		zipPath := filepath.Join(workspace, "upload.zip")
		if err := copyMultipartFile(files[0], zipPath, maxSiteArchiveSize, &p.sourceSize, nil, false); err != nil {
			return fail(err)
		}
		if err := extractUploadedZip(zipPath, rawDir, p); err != nil {
			return fail(err)
		}
	case "folder":
		if len(files) > maxSiteFileCount {
			return fail(fmt.Errorf("项目文件不能超过 %d 个", maxSiteFileCount))
		}
		if len(paths) != len(files) {
			return fail(errors.New("文件夹目录结构不完整，请重新选择文件夹"))
		}
		if p.sourceName == "" && len(paths) > 0 {
			p.sourceName = strings.Split(strings.ReplaceAll(paths[0], "\\", "/"), "/")[0]
		}
		seen := map[string]bool{}
		for index, header := range files {
			rel, ignored, err := safeSitePath(paths[index])
			if err != nil {
				return fail(err)
			}
			if ignored {
				continue
			}
			key := strings.ToLower(rel)
			if seen[key] {
				return fail(fmt.Errorf("项目中存在重复文件：%s", rel))
			}
			seen[key] = true
			if p.fileCount >= maxSiteFileCount {
				return fail(fmt.Errorf("项目文件不能超过 %d 个", maxSiteFileCount))
			}
			if err := copyMultipartFile(header, filepath.Join(rawDir, filepath.FromSlash(rel)), maxSiteExtractedSize-p.extractedSize, &p.sourceSize, &p.extractedSize, true); err != nil {
				return fail(err)
			}
			p.fileCount++
		}
	default:
		return fail(errors.New("请选择单 HTML、文件夹或 ZIP 上传方式"))
	}
	if p.fileCount == 0 || p.extractedSize == 0 {
		return fail(errors.New("项目中没有可发布的文件"))
	}

	siteDir, err := normalizeSiteRoot(workspace, rawDir)
	if err != nil {
		return fail(err)
	}
	p.siteDir = siteDir
	p.archivePath = filepath.Join(workspace, "site.zip")
	revision, err := writeCanonicalSiteArchive(siteDir, p.archivePath)
	if err != nil {
		return fail(err)
	}
	p.revision = revision
	if p.sourceName == "" {
		p.sourceName = "教学网页"
	}
	if len([]rune(p.sourceName)) > 255 {
		return fail(errors.New("项目文件名不能超过 255 个字符"))
	}
	return p, nil
}

func copyMultipartFile(header *multipart.FileHeader, destination string, limit int64, sourceTotal, extractedTotal *int64, allowEmpty bool) error {
	if limit <= 0 {
		return errors.New("项目解压后不能超过 128MB")
	}
	source, err := header.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(target, io.LimitReader(source, limit+1))
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written == 0 && !allowEmpty {
		return fmt.Errorf("文件为空：%s", header.Filename)
	}
	if written > limit {
		return errors.New("项目解压后不能超过 128MB")
	}
	if sourceTotal != nil {
		*sourceTotal += written
	}
	if extractedTotal != nil {
		*extractedTotal += written
	}
	return nil
}

func extractUploadedZip(zipPath, rawDir string, prepared *preparedTeachingSite) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return errors.New("无法读取 ZIP 文件，请确认压缩包没有损坏")
	}
	defer reader.Close()
	seen := map[string]bool{}
	for _, item := range reader.File {
		if item.FileInfo().IsDir() {
			continue
		}
		if item.Mode()&os.ModeSymlink != 0 || !item.Mode().IsRegular() {
			return fmt.Errorf("ZIP 包含不支持的文件：%s", item.Name)
		}
		rel, ignored, err := safeSitePath(item.Name)
		if err != nil {
			return err
		}
		if ignored {
			continue
		}
		key := strings.ToLower(rel)
		if seen[key] {
			return fmt.Errorf("ZIP 中存在重复文件：%s", rel)
		}
		seen[key] = true
		if prepared.fileCount >= maxSiteFileCount {
			return fmt.Errorf("项目文件不能超过 %d 个", maxSiteFileCount)
		}
		if item.UncompressedSize64 > uint64(maxSiteExtractedSize-prepared.extractedSize) {
			return errors.New("ZIP 解压后不能超过 128MB")
		}
		source, err := item.Open()
		if err != nil {
			return errors.New("ZIP 中有文件无法读取")
		}
		destination := filepath.Join(rawDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			source.Close()
			return err
		}
		target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			source.Close()
			return err
		}
		written, copyErr := io.Copy(target, io.LimitReader(source, maxSiteExtractedSize-prepared.extractedSize+1))
		closeErr := target.Close()
		source.Close()
		if copyErr != nil || closeErr != nil || written > maxSiteExtractedSize-prepared.extractedSize {
			return errors.New("ZIP 中有文件损坏或超出大小限制")
		}
		prepared.extractedSize += written
		prepared.fileCount++
	}
	return nil
}

func safeSitePath(value string) (string, bool, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") {
		return "", false, errors.New("项目包含无效文件路径")
	}
	cleaned := path.Clean(strings.TrimPrefix(value, "./"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", false, errors.New("项目文件不能访问所在目录之外的路径")
	}
	if strings.Contains(strings.Split(cleaned, "/")[0], ":") {
		return "", false, errors.New("项目包含无效文件路径")
	}
	if len(cleaned) > 500 {
		return "", false, errors.New("项目文件路径过长")
	}
	if strings.HasPrefix(cleaned, "__MACOSX/") || path.Base(cleaned) == ".DS_Store" {
		return cleaned, true, nil
	}
	return cleaned, false, nil
}

func normalizeSiteRoot(workspace, rawDir string) (string, error) {
	files := []string{}
	err := filepath.WalkDir(rawDir, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(rawDir, filePath)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	root := ""
	entry := ""
	for _, file := range files {
		if isIndexName(path.Base(file)) {
			candidate := path.Dir(file)
			if candidate == "." {
				candidate = ""
			}
			if entry == "" || sitePathDepth(candidate) < sitePathDepth(root) {
				root, entry = candidate, file
			}
		}
	}
	if entry == "" {
		return "", errors.New("项目根目录中没有 index.html")
	}
	if root != "" {
		prefix := root + "/"
		for _, file := range files {
			if !strings.HasPrefix(file, prefix) {
				return "", errors.New("无法确定项目根目录，请让 index.html 与其他资源位于同一项目文件夹")
			}
		}
	}
	siteDir := filepath.Join(workspace, "site")
	sourceRoot := rawDir
	if root != "" {
		sourceRoot = filepath.Join(rawDir, filepath.FromSlash(root))
	}
	if err := os.Rename(sourceRoot, siteDir); err != nil {
		return "", err
	}
	actualEntry := filepath.Join(siteDir, filepath.Base(entry))
	canonicalEntry := filepath.Join(siteDir, "index.html")
	if actualEntry != canonicalEntry {
		if err := os.Rename(actualEntry, canonicalEntry); err != nil {
			return "", err
		}
	}
	if info, err := os.Stat(canonicalEntry); err != nil || info.Size() == 0 {
		return "", errors.New("项目入口 index.html 不能为空")
	}
	return siteDir, nil
}

func sitePathDepth(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "/") + 1
}

func writeCanonicalSiteArchive(siteDir, destination string) (string, error) {
	files := []string{}
	if err := filepath.WalkDir(siteDir, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		files = append(files, filePath)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	archive := zip.NewWriter(io.MultiWriter(target, hash))
	for _, filePath := range files {
		rel, _ := filepath.Rel(siteDir, filePath)
		header := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate}
		header.SetMode(0o644)
		header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		part, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			target.Close()
			return "", err
		}
		source, err := os.Open(filePath)
		if err != nil {
			archive.Close()
			target.Close()
			return "", err
		}
		_, copyErr := io.Copy(part, source)
		source.Close()
		if copyErr != nil {
			archive.Close()
			target.Close()
			return "", copyErr
		}
	}
	if err := archive.Close(); err != nil {
		target.Close()
		return "", err
	}
	if err := target.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:24], nil
}

func isHTMLName(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	return extension == ".html" || extension == ".htm"
}

func isIndexName(name string) bool {
	return strings.EqualFold(name, "index.html") || strings.EqualFold(name, "index.htm")
}

func randomSiteID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (s *server) siteCacheRoot() string {
	return filepath.Join(filepath.Dir(s.db.path), "site-cache")
}

func (s *server) publishPreparedSite(prepared *preparedTeachingSite, publicID string) (string, error) {
	destination := filepath.Join(s.siteCacheRoot(), publicID, prepared.revision)
	if _, err := os.Stat(destination); err == nil {
		return destination, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(prepared.siteDir, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func (s *server) serveTeachingSite(w http.ResponseWriter, r *http.Request) {
	publicID, revision := r.PathValue("site"), r.PathValue("revision")
	requested, _, err := safeSitePath(r.PathValue("path"))
	if r.PathValue("path") == "" {
		requested, err = "index.html", nil
	}
	if err != nil || !validSiteToken(publicID) || !validSiteToken(revision) || !s.siteAccessAllowed(publicID, revision) {
		http.NotFound(w, r)
		return
	}
	root := filepath.Join(s.siteCacheRoot(), publicID, revision)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		if err := s.restoreSiteCache(publicID, revision, root); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	filePath := filepath.Join(root, filepath.FromSlash(requested))
	rel, err := filepath.Rel(root, filePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	// HTML carries the sandbox policy, so force CDNs to revalidate it instead
	// of pinning response headers for a year. Content-addressed assets remain
	// immutable and keep the long-lived cache benefit.
	if isHTMLName(requested) {
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Content-Security-Policy", teachingSiteCSP)
	http.ServeFile(w, r, filePath)
}

func (s *server) refreshTeachingSiteRegistry() error {
	items, err := s.db.navigation()
	if err != nil {
		return err
	}
	registry := map[string]map[string]bool{}
	for _, item := range items {
		if item.Site != nil {
			registry[item.Site.PublicID] = map[string]bool{item.Site.Revision: true}
		}
	}
	s.siteAccessMu.Lock()
	s.siteAccess = registry
	s.siteAccessMu.Unlock()
	return nil
}

func (s *server) setSiteAccess(publicID string, revisions ...string) {
	allowed := map[string]bool{}
	for _, revision := range revisions {
		if revision != "" {
			allowed[revision] = true
		}
	}
	s.siteAccessMu.Lock()
	if s.siteAccess == nil {
		s.siteAccess = map[string]map[string]bool{}
	}
	s.siteAccess[publicID] = allowed
	s.siteAccessMu.Unlock()
}

func (s *server) removeSiteAccess(publicID string) {
	s.siteAccessMu.Lock()
	delete(s.siteAccess, publicID)
	s.siteAccessMu.Unlock()
}

func (s *server) siteAccessAllowed(publicID, revision string) bool {
	s.siteAccessMu.RLock()
	allowed := s.siteAccess[publicID][revision]
	s.siteAccessMu.RUnlock()
	return allowed
}

func (s *server) pruneSiteRevisions(publicID string) {
	directory := filepath.Join(s.siteCacheRoot(), publicID)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	s.siteAccessMu.RLock()
	allowed := s.siteAccess[publicID]
	for _, entry := range entries {
		if entry.IsDir() && !allowed[entry.Name()] {
			_ = os.RemoveAll(filepath.Join(directory, entry.Name()))
		}
	}
	s.siteAccessMu.RUnlock()
}

func (s *server) pruneSiteCache() error {
	entries, err := os.ReadDir(s.siteCacheRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		s.siteAccessMu.RLock()
		_, active := s.siteAccess[entry.Name()]
		s.siteAccessMu.RUnlock()
		if !active {
			if err := os.RemoveAll(filepath.Join(s.siteCacheRoot(), entry.Name())); err != nil {
				return err
			}
			continue
		}
		s.pruneSiteRevisions(entry.Name())
	}
	return nil
}

const teachingSiteCSP = "sandbox allow-scripts allow-same-origin allow-forms allow-modals allow-downloads allow-popups; default-src * data: blob:; script-src * data: blob: 'unsafe-inline' 'unsafe-eval'; style-src * data: blob: 'unsafe-inline'; img-src * data: blob:; media-src * data: blob:; font-src * data: blob:; connect-src * data: blob:; frame-src * data: blob:; object-src 'none'; form-action 'none'"

func validSiteToken(value string) bool {
	if len(value) < 16 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (s *server) restoreSiteCache(publicID, revision, destination string) error {
	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	if _, err := os.Stat(destination); err == nil {
		return nil
	}
	archive, err := s.db.teachingSiteArchive(publicID, revision)
	if err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(filepath.Dir(s.db.path), ".site-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	zipPath := filepath.Join(workspace, "site.zip")
	if err := os.WriteFile(zipPath, archive, 0o600); err != nil {
		return err
	}
	prepared := &preparedTeachingSite{workspace: workspace, siteDir: filepath.Join(workspace, "site"), revision: revision}
	if err := os.MkdirAll(prepared.siteDir, 0o755); err != nil {
		return err
	}
	if err := extractCanonicalArchive(zipPath, prepared.siteDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.Rename(prepared.siteDir, destination)
}

func extractCanonicalArchive(zipPath, destination string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	var total int64
	for _, item := range reader.File {
		if item.FileInfo().IsDir() {
			continue
		}
		rel, ignored, err := safeSitePath(item.Name)
		if err != nil || ignored || item.Mode()&os.ModeSymlink != 0 {
			return errors.New("保存的教学网页数据无效")
		}
		if total+int64(item.UncompressedSize64) > maxSiteExtractedSize {
			return errors.New("保存的教学网页超出大小限制")
		}
		source, err := item.Open()
		if err != nil {
			return err
		}
		filePath := filepath.Join(destination, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			source.Close()
			return err
		}
		target, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			source.Close()
			return err
		}
		written, copyErr := io.Copy(target, io.LimitReader(source, maxSiteExtractedSize-total+1))
		closeErr := target.Close()
		source.Close()
		if copyErr != nil || closeErr != nil || written > maxSiteExtractedSize-total {
			return errors.New("保存的教学网页数据损坏")
		}
		total += written
	}
	return nil
}

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

func (s *store) navigationByID(id int64) (navigationLink, error) {
	items, err := s.navigation()
	if err != nil {
		return navigationLink{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return navigationLink{}, errNotFound
}

func (s *store) teachingSiteArchive(publicID, revision string) ([]byte, error) {
	var archive []byte
	err := s.QueryRow(`SELECT archive FROM teaching_sites WHERE public_id=? AND revision=?`, publicID, revision).Scan(&archive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return archive, err
}
