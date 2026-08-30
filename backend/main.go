package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type server struct {
	db               *store
	public           fs.FS
	integrationToken string
}

const (
	teacherSessionLifetime = 30 * 24 * time.Hour
	teacherPasswordCost    = 12
	teacherSessionCookie   = "classorbit_session"
)

type teacherCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type navigationBatchInput struct {
	Items []navigationLinkInput `json:"items"`
}

type apiError struct {
	Error string `json:"error"`
}

func main() {
	dataDir := env("DATA_DIR", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := openStore(databasePath(dataDir))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := initializeTeacherFromEnvironment(db); err != nil {
		log.Fatal(err)
	}

	publicDir := env("PUBLIC_DIR", "frontend/dist")
	public := os.DirFS(publicDir)
	if _, err := fs.Stat(public, "index.html"); err != nil {
		log.Fatalf("frontend not found in %s: %v", publicDir, err)
	}
	s := &server{
		db:               db,
		public:           public,
		integrationToken: strings.TrimSpace(os.Getenv("CLASS_SYSTEM_TOKEN")),
	}
	mux := http.NewServeMux()
	s.routes(mux)
	addr := env("ADDR", "127.0.0.1:8080")
	log.Printf("ClassOrbit server running at http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, logRequests(s.requireTeacher(mux))))
}

// Optional bootstrap for unattended deployments. Existing database credentials always win.
func initializeTeacherFromEnvironment(db *store) error {
	username := strings.TrimSpace(os.Getenv("TEACHER_USERNAME"))
	password := os.Getenv("TEACHER_PASSWORD")
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		log.Printf("warning: TEACHER_USERNAME and TEACHER_PASSWORD must be provided together; use the setup page instead")
		return nil
	}
	initialized, err := db.teacherInitialized()
	if err != nil || initialized {
		return err
	}
	credentials := teacherCredentials{Username: username, Password: password}
	if message := validateTeacherCredentials(&credentials); message != "" {
		return fmt.Errorf("initial teacher credentials are invalid: %s", message)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(credentials.Password), teacherPasswordCost)
	if err != nil {
		return err
	}
	if err := db.createTeacherAccount(credentials.Username, string(hash)); err != nil && !errors.Is(err, errConflict) {
		return err
	}
	log.Printf("teacher account initialized from deployment environment")
	return nil
}

// Existing ClassPoint installations keep using their original database file.
// Fresh installations use the ClassOrbit name without requiring a destructive
// data migration during an application rename.
func databasePath(dataDir string) string {
	current := filepath.Join(dataDir, "classorbit.db")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	legacy := filepath.Join(dataDir, "classpoint.db")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/auth", s.authStatus)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/auth", s.login)
	mux.HandleFunc("DELETE /api/auth", s.logout)
	mux.HandleFunc("GET /api/dashboard", s.getDashboard)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PATCH /api/settings", s.updateSettings)
	mux.HandleFunc("GET /api/navigation", s.getNavigation)
	mux.HandleFunc("PUT /api/navigation", s.updateNavigation)
	mux.HandleFunc("GET /api/classes", s.getClasses)
	mux.HandleFunc("POST /api/classes", s.createClass)
	mux.HandleFunc("PATCH /api/classes/{id}", s.updateClass)
	mux.HandleFunc("DELETE /api/classes/{id}", s.deleteClass)
	mux.HandleFunc("GET /api/classes/{id}/students", s.getStudents)
	mux.HandleFunc("POST /api/classes/{id}/students", s.createStudent)
	mux.HandleFunc("POST /api/classes/{id}/import", s.importStudents)
	mux.HandleFunc("GET /api/students/{id}/events", s.getScoreEvents)
	mux.HandleFunc("PATCH /api/students/{id}", s.updateStudent)
	mux.HandleFunc("DELETE /api/students/{id}", s.deleteStudent)
	mux.HandleFunc("POST /api/students/{id}/score", s.changeScore)
	mux.HandleFunc("GET /api/attendance", s.getAttendance)
	mux.HandleFunc("POST /api/attendance", s.createAttendance)
	mux.HandleFunc("GET /api/attendance/{id}", s.getAttendanceByID)
	mux.HandleFunc("DELETE /api/attendance/{id}", s.deleteAttendance)
	mux.HandleFunc("POST /api/attendance/{id}/close", s.closeAttendance)
	mux.HandleFunc("PATCH /api/attendance/{id}/records/{studentID}", s.updateAttendanceRecord)
	mux.HandleFunc("GET /api/schedule", s.getSchedule)
	mux.HandleFunc("POST /api/schedule", s.createScheduleLesson)
	mux.HandleFunc("POST /api/schedule/import", s.importSchedule)
	mux.HandleFunc("PUT /api/schedule/settings", s.updateScheduleSettings)
	mux.HandleFunc("PATCH /api/schedule/{id}", s.updateScheduleLesson)
	mux.HandleFunc("DELETE /api/schedule/{id}", s.deleteScheduleLesson)
	mux.HandleFunc("PUT /api/schedule/{id}/changes", s.setScheduleChange)
	mux.HandleFunc("DELETE /api/schedule/{id}/changes", s.deleteScheduleChange)
	mux.HandleFunc("GET /api/public/classes", s.getPublicClasses)
	mux.HandleFunc("GET /api/public/settings", s.getPublicSettings)
	mux.HandleFunc("GET /api/public/navigation", s.getPublicNavigation)
	mux.HandleFunc("GET /api/public/classes/{id}/students", s.getPublicStudents)
	mux.HandleFunc("POST /api/public/check-in", s.checkIn)
	mux.HandleFunc("GET /api/integration/classes", s.getIntegrationClasses)

	assets := http.FileServer(http.FS(s.public))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, apiError{Error: "接口不存在"})
			return
		}
		if r.URL.Path != "/" {
			if _, err := fs.Stat(s.public, strings.TrimPrefix(r.URL.Path, "/")); err == nil {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				assets.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		assets.ServeHTTP(w, r)
	})
}

func (s *server) getDashboard(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.dashboard()
	respond(w, data, err)
}

func (s *server) getClasses(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.classes(false)
	respond(w, data, err)
}

func (s *server) createClass(w http.ResponseWriter, r *http.Request) {
	var in classInput
	if !decode(w, r, &in) {
		return
	}
	if message := validateClassInput(&in); message != "" {
		badRequest(w, message)
		return
	}
	data, err := s.db.createClass(in)
	respondStatus(w, http.StatusCreated, data, err)
}

func (s *server) updateClass(w http.ResponseWriter, r *http.Request) {
	var in classInput
	if !decode(w, r, &in) {
		return
	}
	if message := validateClassInput(&in); message != "" {
		badRequest(w, message)
		return
	}
	data, err := s.db.updateClass(pathID(r, "id"), in)
	respond(w, data, err)
}

func (s *server) deleteClass(w http.ResponseWriter, r *http.Request) {
	err := s.db.deleteClass(pathID(r, "id"))
	respond(w, map[string]bool{"ok": err == nil}, err)
}

func (s *server) getStudents(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	if sort != "score_asc" && sort != "score_desc" {
		sort = "student_no"
	}
	data, err := s.db.students(pathID(r, "id"), sort)
	respond(w, data, err)
}

func (s *server) createStudent(w http.ResponseWriter, r *http.Request) {
	var in studentInput
	if !decode(w, r, &in) {
		return
	}
	in.StudentNo, in.Name = strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name)
	if in.StudentNo == "" || in.Name == "" {
		badRequest(w, "学号和姓名不能为空")
		return
	}
	data, err := s.db.createStudent(pathID(r, "id"), in)
	respondStatus(w, http.StatusCreated, data, err)
}

func (s *server) importStudents(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		badRequest(w, "文件不能超过 12MB")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		badRequest(w, "请选择 Excel 文件")
		return
	}
	defer file.Close()
	rows, err := parseExcel(file)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	result, err := s.db.importStudents(pathID(r, "id"), rows)
	respond(w, result, err)
}

func (s *server) getScoreEvents(w http.ResponseWriter, r *http.Request) {
	data, err := s.db.scoreEvents(pathID(r, "id"))
	respond(w, data, err)
}

func (s *server) updateStudent(w http.ResponseWriter, r *http.Request) {
	var in studentInput
	if !decode(w, r, &in) {
		return
	}
	in.StudentNo, in.Name = strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name)
	if in.StudentNo == "" || in.Name == "" {
		badRequest(w, "学号和姓名不能为空")
		return
	}
	data, err := s.db.updateStudent(pathID(r, "id"), in)
	respond(w, data, err)
}

func (s *server) deleteStudent(w http.ResponseWriter, r *http.Request) {
	err := s.db.deleteStudent(pathID(r, "id"))
	respond(w, map[string]bool{"ok": err == nil}, err)
}

func (s *server) changeScore(w http.ResponseWriter, r *http.Request) {
	var in scoreInput
	if !decode(w, r, &in) {
		return
	}
	if in.Delta == 0 || in.Delta < -100 || in.Delta > 100 {
		badRequest(w, "单次积分变更须在 -100 到 100 之间且不能为 0")
		return
	}
	data, err := s.db.changeScore(pathID(r, "id"), in)
	respond(w, data, err)
}

func (s *server) getAttendance(w http.ResponseWriter, r *http.Request) {
	classID, _ := strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			badRequest(w, "考勤日期格式不正确")
			return
		}
	}
	data, err := s.db.attendance(classID, date)
	respond(w, data, err)
}

func (s *server) createAttendance(w http.ResponseWriter, r *http.Request) {
	var in attendanceInput
	if !decode(w, r, &in) {
		return
	}
	if in.ClassID == 0 {
		badRequest(w, "请选择班级")
		return
	}
	data, err := s.db.createAttendance(in)
	respondStatus(w, http.StatusCreated, data, err)
}

func (s *server) getAttendanceByID(w http.ResponseWriter, r *http.Request) {
	data, err := s.db.attendanceByID(pathID(r, "id"))
	respond(w, data, err)
}

func (s *server) closeAttendance(w http.ResponseWriter, r *http.Request) {
	data, err := s.db.closeAttendance(pathID(r, "id"))
	respond(w, data, err)
}

func (s *server) deleteAttendance(w http.ResponseWriter, r *http.Request) {
	err := s.db.deleteAttendance(pathID(r, "id"))
	respond(w, map[string]bool{"ok": err == nil}, err)
}

func (s *server) updateAttendanceRecord(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Status != "present" && in.Status != "absent" && in.Status != "late" && in.Status != "leave" {
		badRequest(w, "无效的考勤状态")
		return
	}
	data, err := s.db.updateAttendanceRecord(pathID(r, "id"), pathID(r, "studentID"), in.Status, "teacher")
	respond(w, data, err)
}

func (s *server) getSchedule(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.schedule()
	respond(w, data, err)
}

func (s *server) updateScheduleSettings(w http.ResponseWriter, r *http.Request) {
	var in scheduleSettingsInput
	if !decode(w, r, &in) {
		return
	}
	if message := validateScheduleSettings(&in); message != "" {
		badRequest(w, message)
		return
	}
	data, err := s.db.updateScheduleSettings(in)
	respond(w, data, err)
}

func (s *server) createScheduleLesson(w http.ResponseWriter, r *http.Request) {
	var in scheduleInput
	if !decode(w, r, &in) {
		return
	}
	if message := validateScheduleInput(&in); message != "" {
		badRequest(w, message)
		return
	}
	data, err := s.db.createScheduleLesson(in)
	respondStatus(w, http.StatusCreated, data, err)
}

func (s *server) updateScheduleLesson(w http.ResponseWriter, r *http.Request) {
	var in scheduleInput
	if !decode(w, r, &in) {
		return
	}
	if message := validateScheduleInput(&in); message != "" {
		badRequest(w, message)
		return
	}
	data, err := s.db.updateScheduleLesson(pathID(r, "id"), in)
	respond(w, data, err)
}

func (s *server) deleteScheduleLesson(w http.ResponseWriter, r *http.Request) {
	err := s.db.deleteScheduleLesson(pathID(r, "id"))
	respond(w, map[string]bool{"ok": err == nil}, err)
}

func (s *server) setScheduleChange(w http.ResponseWriter, r *http.Request) {
	var in scheduleChangeInput
	if !decode(w, r, &in) {
		return
	}
	if message := validateScheduleChange(&in); message != "" {
		badRequest(w, message)
		return
	}
	data, err := s.db.setScheduleChange(pathID(r, "id"), in)
	respond(w, data, err)
}

func (s *server) deleteScheduleChange(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if _, err := time.Parse("2006-01-02", date); err != nil {
		badRequest(w, "日期格式不正确")
		return
	}
	err := s.db.deleteScheduleChange(pathID(r, "id"), date)
	respond(w, map[string]bool{"ok": err == nil}, err)
}

func (s *server) importSchedule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		badRequest(w, "文件不能超过 12MB")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		badRequest(w, "请选择 Excel 文件")
		return
	}
	defer file.Close()
	rows, err := parseScheduleExcel(file)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	classes, err := s.db.classes(false)
	if err != nil {
		respond(w, nil, err)
		return
	}
	classIDs := map[string]int64{}
	for _, item := range classes {
		classIDs[normalizeClassName(item.Name)] = item.ID
	}
	settings, err := s.db.scheduleSettings()
	if err != nil {
		respond(w, nil, err)
		return
	}
	inputs := make([]scheduleInput, 0, len(rows))
	for index, row := range rows {
		classID := classIDs[normalizeClassName(row.ClassName)]
		if classID == 0 {
			badRequest(w, fmt.Sprintf("第 %d 行的班级“%s”在后台不存在", index+2, row.ClassName))
			return
		}
		period := 0
		for _, item := range settings.Periods {
			if item.StartTime == row.StartTime && item.EndTime == row.EndTime {
				period = item.Period
				break
			}
		}
		if period == 0 {
			badRequest(w, fmt.Sprintf("第 %d 行的时间段不匹配当前 7 节课时间", index+2))
			return
		}
		input := scheduleInput{ClassID: classID, Course: row.Course, Weekday: row.Weekday, Period: period, LocationOdd: row.LocationOdd, LocationEven: row.LocationEven}
		if message := validateScheduleInput(&input); message != "" {
			badRequest(w, fmt.Sprintf("第 %d 行：%s", index+2, message))
			return
		}
		inputs = append(inputs, input)
	}
	result, err := s.db.importSchedule(inputs)
	respond(w, result, err)
}

func (s *server) getPublicClasses(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.classes(true)
	respond(w, data, err)
}

func (s *server) getSettings(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.settings()
	respond(w, data, err)
}

func (s *server) getPublicSettings(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.settings()
	respond(w, data, err)
}

func (s *server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in siteSettings
	if !decode(w, r, &in) {
		return
	}
	in.Title, in.Subtitle = strings.TrimSpace(in.Title), strings.TrimSpace(in.Subtitle)
	if in.Title == "" || in.Subtitle == "" {
		badRequest(w, "主标题和副标题不能为空")
		return
	}
	if len([]rune(in.Title)) > 20 || len([]rune(in.Subtitle)) > 30 {
		badRequest(w, "标题长度超出限制")
		return
	}
	data, err := s.db.updateSettings(in)
	respond(w, data, err)
}

func (s *server) getNavigation(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.navigation()
	respond(w, data, err)
}

func (s *server) getPublicNavigation(w http.ResponseWriter, _ *http.Request) {
	data, err := s.db.navigation()
	respond(w, data, err)
}

func (s *server) updateNavigation(w http.ResponseWriter, r *http.Request) {
	var in navigationBatchInput
	if !decode(w, r, &in) {
		return
	}
	if in.Items == nil {
		badRequest(w, "缺少导航网站列表")
		return
	}
	if len(in.Items) > 100 {
		badRequest(w, "导航网站不能超过 100 个")
		return
	}
	for index := range in.Items {
		if message := validateNavigationLink(&in.Items[index]); message != "" {
			badRequest(w, fmt.Sprintf("第 %d 个网站：%s", index+1, message))
			return
		}
	}
	data, err := s.db.replaceNavigation(in.Items)
	respond(w, data, err)
}

func (s *server) getPublicStudents(w http.ResponseWriter, r *http.Request) {
	data, err := s.db.publicStudents(pathID(r, "id"))
	respond(w, data, err)
}

type integrationClassesResponse struct {
	Classes []integrationClass `json:"classes"`
}

type integrationClass struct {
	Name     string               `json:"name"`
	Students []integrationStudent `json:"students"`
}

type integrationStudent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getIntegrationClasses exposes the minimal read-only shape consumed by
// TypeMatch. It deliberately uses a separate bearer token instead of the
// browser session cookie used by the teacher UI.
func (s *server) getIntegrationClasses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: "缺少 Authorization 凭证"})
		return
	}
	token, ok := parseBearerToken(authorization)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: "Authorization 格式错误"})
		return
	}
	configuredToken := strings.TrimSpace(s.integrationToken)
	if configuredToken == "" {
		// Reading the environment here keeps lightweight in-process test servers
		// compatible while main still snapshots the deployment configuration.
		configuredToken = strings.TrimSpace(os.Getenv("CLASS_SYSTEM_TOKEN"))
	}
	if configuredToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(configuredToken)) != 1 {
		writeJSON(w, http.StatusForbidden, apiError{Error: "共享密钥错误"})
		return
	}

	username := strings.TrimSpace(r.URL.Query().Get("teacher_username"))
	headerUsername := strings.TrimSpace(r.Header.Get("X-Teacher-Username"))
	if username == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "缺少教师用户名"})
		return
	}
	if headerUsername != "" && !strings.EqualFold(username, headerUsername) {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "教师用户名不一致"})
		return
	}

	account, err := s.db.teacherAccount()
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "教师账号不存在"})
		return
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	if !strings.EqualFold(username, account.Username) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "教师账号不存在"})
		return
	}

	classes, err := s.db.integrationClasses()
	if err != nil {
		respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, integrationClassesResponse{Classes: classes})
}

func parseBearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func (s *server) checkIn(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClassID   int64 `json:"classId"`
		StudentID int64 `json:"studentId"`
	}
	if !decode(w, r, &in) {
		return
	}
	data, err := s.db.checkIn(in.ClassID, in.StudentID)
	respond(w, data, err)
}

func (s *server) authStatus(w http.ResponseWriter, r *http.Request) {
	initialized, err := s.db.teacherInitialized()
	if err != nil {
		respond(w, nil, err)
		return
	}
	username, authenticated, err := s.authenticateTeacher(r)
	if err != nil {
		respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized":   initialized,
		"authenticated": authenticated,
		"username":      username,
	})
}

func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	var in teacherCredentials
	if !decode(w, r, &in) {
		return
	}
	initialized, err := s.db.teacherInitialized()
	if err != nil {
		respond(w, nil, err)
		return
	}
	if initialized {
		writeJSON(w, http.StatusConflict, apiError{Error: "系统已完成初始化"})
		return
	}
	if message := validateTeacherCredentials(&in); message != "" {
		badRequest(w, message)
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), teacherPasswordCost)
	if err != nil {
		respond(w, nil, err)
		return
	}
	if err := s.db.createTeacherAccount(in.Username, string(passwordHash)); err != nil {
		if errors.Is(err, errConflict) {
			writeJSON(w, http.StatusConflict, apiError{Error: "系统已完成初始化"})
			return
		}
		respond(w, nil, err)
		return
	}
	if err := s.startTeacherSession(w, r); err != nil {
		respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"initialized": true, "authenticated": true, "username": in.Username})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in teacherCredentials
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	account, err := s.db.teacherAccount()
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusPreconditionRequired, apiError{Error: "请先完成系统初始化"})
		return
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(in.Password))
	if !strings.EqualFold(in.Username, account.Username) || passwordErr != nil {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: "账号或密码不正确"})
		return
	}
	if err := s.startTeacherSession(w, r); err != nil {
		respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"initialized": true, "authenticated": true, "username": account.Username})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(teacherSessionCookie); err == nil {
		if err := s.db.deleteTeacherSession(hashSessionToken(cookie.Value)); err != nil {
			respond(w, nil, err)
			return
		}
	}
	clearTeacherSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"initialized": true, "authenticated": false, "username": ""})
}

func (s *server) requireTeacher(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		isPublic := path == "/api/health" || path == "/api/auth" || path == "/api/setup" || strings.HasPrefix(path, "/api/public/") || path == "/api/integration/classes"
		if strings.HasPrefix(path, "/api/") && !isPublic {
			_, authenticated, err := s.authenticateTeacher(r)
			if err != nil {
				respond(w, nil, err)
				return
			}
			if !authenticated {
				writeJSON(w, http.StatusUnauthorized, apiError{Error: "请先登录教师后台"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) authenticateTeacher(r *http.Request) (string, bool, error) {
	cookie, err := r.Cookie(teacherSessionCookie)
	if err != nil {
		return "", false, nil
	}
	username, err := s.db.teacherSession(hashSessionToken(cookie.Value), time.Now().Unix())
	if errors.Is(err, errNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return username, true, nil
}

func (s *server) startTeacherSession(w http.ResponseWriter, r *http.Request) error {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	expiresAt := time.Now().Add(teacherSessionLifetime)
	if err := s.db.createTeacherSession(hashSessionToken(token), expiresAt.Unix()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     teacherSessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestUsesHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(teacherSessionLifetime.Seconds()),
		Expires:  expiresAt,
	})
	return nil
}

func clearTeacherSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     teacherSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestUsesHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func requestUsesHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func parseExcel(file multipart.File) ([]studentInput, error) {
	x, err := excelize.OpenReader(file)
	if err != nil {
		return nil, errors.New("无法读取 Excel，请使用 .xlsx 或 .xlsm 文件")
	}
	defer x.Close()
	sheets := x.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("Excel 中没有工作表")
	}
	rows, err := x.GetRows(sheets[0])
	if err != nil || len(rows) == 0 {
		return nil, errors.New("Excel 中没有可导入的数据")
	}
	noCol, nameCol, start := -1, -1, 0
	for i, cell := range rows[0] {
		v := strings.ToLower(strings.TrimSpace(cell))
		if v == "学号" || v == "student no" || v == "student_no" || v == "id" {
			noCol = i
		}
		if v == "姓名" || v == "名字" || v == "name" || v == "student name" {
			nameCol = i
		}
	}
	if noCol >= 0 && nameCol >= 0 {
		start = 1
	} else {
		noCol, nameCol = 0, 1
	}
	var out []studentInput
	seen := map[string]bool{}
	for _, row := range rows[start:] {
		if noCol >= len(row) || nameCol >= len(row) {
			continue
		}
		no, name := strings.TrimSpace(row[noCol]), strings.TrimSpace(row[nameCol])
		if no == "" || name == "" || seen[no] {
			continue
		}
		seen[no] = true
		out = append(out, studentInput{StudentNo: no, Name: name})
	}
	if len(out) == 0 {
		return nil, errors.New("未找到有效学生；请确认前两列或“学号/姓名”列有数据")
	}
	if len(out) > 200 {
		return nil, errors.New("单次最多导入 200 名学生")
	}
	return out, nil
}

type scheduleImportRow struct {
	ClassName    string
	Course       string
	Weekday      int
	StartTime    string
	EndTime      string
	LocationOdd  string
	LocationEven string
}

func parseScheduleExcel(file multipart.File) ([]scheduleImportRow, error) {
	x, err := excelize.OpenReader(file)
	if err != nil {
		return nil, errors.New("无法读取 Excel，请使用 .xlsx 或 .xlsm 文件")
	}
	defer x.Close()
	sheets := x.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("Excel 中没有工作表")
	}
	rows, err := x.GetRows(sheets[0])
	if err != nil || len(rows) < 2 {
		return nil, errors.New("Excel 中没有可导入的课表数据")
	}
	columns := map[string]int{}
	for index, cell := range rows[0] {
		name := strings.ToLower(strings.TrimSpace(cell))
		switch name {
		case "星期", "周几", "weekday":
			columns["weekday"] = index
		case "开始时间", "上课时间", "start", "start time":
			columns["start"] = index
		case "结束时间", "下课时间", "end", "end time":
			columns["end"] = index
		case "班级", "class":
			columns["class"] = index
		case "课程", "科目", "course":
			columns["course"] = index
		case "地点", "上课地点", "location":
			columns["location"] = index
		case "单周地点", "odd location":
			columns["location_odd"] = index
		case "双周地点", "even location":
			columns["location_even"] = index
		}
	}
	for _, key := range []string{"weekday", "start", "end", "class", "course"} {
		if _, ok := columns[key]; !ok {
			return nil, errors.New("表头需要包含：星期、开始时间、结束时间、班级、课程")
		}
	}
	value := func(row []string, key string) string {
		index, exists := columns[key]
		if !exists {
			return ""
		}
		if index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	out := []scheduleImportRow{}
	for rowIndex, row := range rows[1:] {
		weekdayText := value(row, "weekday")
		className := value(row, "class")
		course := value(row, "course")
		start := normalizeClock(value(row, "start"))
		end := normalizeClock(value(row, "end"))
		location := value(row, "location")
		locationOdd, locationEven := value(row, "location_odd"), value(row, "location_even")
		if locationOdd == "" {
			locationOdd = location
		}
		if locationEven == "" {
			locationEven = location
		}
		if locationOdd == "" {
			locationOdd = "机房 1"
		}
		if locationEven == "" {
			locationEven = locationOdd
		}
		if weekdayText == "" && className == "" && course == "" && start == "" && end == "" {
			continue
		}
		weekday := parseWeekday(weekdayText)
		if weekday == 0 {
			return nil, fmt.Errorf("第 %d 行的星期无法识别", rowIndex+2)
		}
		out = append(out, scheduleImportRow{ClassName: className, Course: course, Weekday: weekday, StartTime: start, EndTime: end, LocationOdd: locationOdd, LocationEven: locationEven})
	}
	if len(out) == 0 {
		return nil, errors.New("未找到有效课程")
	}
	if len(out) > 100 {
		return nil, errors.New("单次最多导入 100 节课程")
	}
	return out, nil
}

func parseWeekday(value string) int {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "星期"), "周"))
	weekdays := map[string]int{"一": 1, "1": 1, "二": 2, "2": 2, "三": 3, "3": 3, "四": 4, "4": 4, "五": 5, "5": 5, "六": 6, "6": 6, "日": 7, "天": 7, "七": 7, "7": 7}
	return weekdays[value]
}

func normalizeClock(value string) string {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"15:04", "15:04:05", "3:04 PM", "3:04PM"} {
		if parsed, err := time.Parse(layout, strings.ToUpper(value)); err == nil {
			return parsed.Format("15:04")
		}
	}
	return value
}

func normalizeClassName(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), "")
}

func validateScheduleInput(in *scheduleInput) string {
	in.Course = strings.TrimSpace(in.Course)
	in.LocationOdd, in.LocationEven = strings.TrimSpace(in.LocationOdd), strings.TrimSpace(in.LocationEven)
	if in.ClassID <= 0 {
		return "请选择班级"
	}
	if in.Course == "" || len([]rune(in.Course)) > 20 {
		return "课程名称须为 1 至 20 个字"
	}
	if in.Weekday < 1 || in.Weekday > 5 {
		return "课程只能安排在周一至周五"
	}
	if in.Period < 1 || in.Period > 7 {
		return "请选择节次"
	}
	locations := map[string]bool{"机房 1": true, "机房 2": true, "教室": true}
	if !locations[in.LocationOdd] || !locations[in.LocationEven] {
		return "请选择有效的上课地点"
	}
	return ""
}

func validateScheduleSettings(in *scheduleSettingsInput) string {
	in.SemesterStart, in.SemesterEnd = strings.TrimSpace(in.SemesterStart), strings.TrimSpace(in.SemesterEnd)
	start, startErr := time.Parse("2006-01-02", in.SemesterStart)
	end, endErr := time.Parse("2006-01-02", in.SemesterEnd)
	if startErr != nil || endErr != nil || end.Before(start) {
		return "请设置有效的学期起止日期"
	}
	if end.Sub(start) > 370*24*time.Hour {
		return "单个学期不能超过一年"
	}
	if len(in.Periods) != 7 {
		return "需要设置完整的 7 节课时间"
	}
	previousEnd := ""
	for index := range in.Periods {
		period := &in.Periods[index]
		period.StartTime, period.EndTime = normalizeClock(period.StartTime), normalizeClock(period.EndTime)
		if period.Period != index+1 {
			return "节次顺序不正确"
		}
		periodStart, startErr := time.Parse("15:04", period.StartTime)
		periodEnd, endErr := time.Parse("15:04", period.EndTime)
		if startErr != nil || endErr != nil || !periodEnd.After(periodStart) {
			return fmt.Sprintf("第 %d 节课时间不正确", period.Period)
		}
		if previousEnd != "" && period.StartTime < previousEnd {
			return "相邻节次时间不能重叠"
		}
		previousEnd = period.EndTime
	}
	return ""
}

func validateScheduleChange(in *scheduleChangeInput) string {
	in.Date, in.NewDate = strings.TrimSpace(in.Date), strings.TrimSpace(in.NewDate)
	in.NewStartTime, in.NewEndTime = normalizeClock(in.NewStartTime), normalizeClock(in.NewEndTime)
	in.Note = strings.TrimSpace(in.Note)
	if _, err := time.Parse("2006-01-02", in.Date); err != nil {
		return "原上课日期格式不正确"
	}
	if in.Status != "occupied" && in.Status != "rescheduled" {
		return "变动类型不正确"
	}
	if len([]rune(in.Note)) > 50 {
		return "备注不能超过 50 个字"
	}
	if in.Status == "occupied" {
		in.NewDate, in.NewStartTime, in.NewEndTime, in.NewClassID = "", "", "", 0
		return ""
	}
	if _, err := time.Parse("2006-01-02", in.NewDate); err != nil {
		return "换课日期格式不正确"
	}
	start, startErr := time.Parse("15:04", in.NewStartTime)
	end, endErr := time.Parse("15:04", in.NewEndTime)
	if startErr != nil || endErr != nil || !end.After(start) {
		return "换课时间格式不正确"
	}
	return ""
}

func validateTeacherCredentials(in *teacherCredentials) string {
	in.Username = strings.TrimSpace(in.Username)
	usernameLength := len([]rune(in.Username))
	if usernameLength < 2 || usernameLength > 32 {
		return "账号长度须为 2 至 32 个字符"
	}
	for _, character := range in.Username {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return "账号不能包含空格或控制字符"
		}
	}
	if len([]rune(in.Password)) < 8 {
		return "密码至少需要 8 个字符"
	}
	if len([]byte(in.Password)) > 72 {
		return "密码不能超过 72 个字节"
	}
	return ""
}

func validateNavigationLink(in *navigationLinkInput) string {
	in.Title = strings.TrimSpace(in.Title)
	in.URL = strings.TrimSpace(in.URL)
	in.IconURL = strings.TrimSpace(in.IconURL)
	if in.Title == "" {
		return "网站标题不能为空"
	}
	if len([]rune(in.Title)) > 50 {
		return "网站标题不能超过 50 个字符"
	}
	if len(in.URL) > 2048 || len(in.IconURL) > 2048 {
		return "网站链接不能超过 2048 个字符"
	}
	if !validWebURL(in.URL) {
		return "网站链接须为有效的 http 或 https 地址"
	}
	if in.IconURL != "" && !validWebURL(in.IconURL) {
		return "网站图标须为有效的 http 或 https 地址"
	}
	return ""
}

func validWebURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func validateClassInput(in *classInput) string {
	grades := map[string]bool{"一": true, "二": true, "三": true, "四": true, "五": true, "六": true}
	in.Grade, in.ClassNo = strings.TrimSpace(in.Grade), strings.TrimSpace(in.ClassNo)
	if !grades[in.Grade] {
		return "请选择一至六年级"
	}
	if in.ClassNo == "" {
		return "请输入班号"
	}
	number, err := strconv.Atoi(in.ClassNo)
	if err != nil || number < 1 || number > 99 {
		return "班号请输入 1 至 99 的数字"
	}
	in.ClassNo = strconv.Itoa(number)
	in.Name = className(in.Grade, in.ClassNo)
	return ""
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		badRequest(w, "请求内容格式错误")
		return false
	}
	return true
}

func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}
func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, apiError{Error: msg})
}
func respond(w http.ResponseWriter, data any, err error) { respondStatus(w, http.StatusOK, data, err) }
func respondStatus(w http.ResponseWriter, status int, data any, err error) {
	if err != nil {
		code := http.StatusInternalServerError
		msg := "操作失败，请稍后重试"
		if errors.Is(err, errNotFound) {
			code, msg = http.StatusNotFound, "记录不存在"
		}
		if errors.Is(err, errConflict) {
			code, msg = http.StatusConflict, strings.TrimPrefix(err.Error(), errConflict.Error()+": ")
		}
		writeJSON(w, code, apiError{Error: msg})
		return
	}
	writeJSON(w, status, data)
}
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
