package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func testAPI(t *testing.T) (*store, http.Handler) {
	t.Helper()
	db := testStore(t)
	s := &server{db: db}
	if err := s.refreshTeachingSiteRegistry(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.routes(mux)
	return db, s.requireTeacher(mux)
}

func TestHealthReportsDatabaseAndBuild(t *testing.T) {
	_, handler := testAPI(t)
	response := apiRequest(t, handler, http.MethodGet, "/api/health", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		OK       bool   `json:"ok"`
		Database bool   `json:"database"`
		Version  string `json:"version"`
		Commit   string `json:"commit"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || !payload.Database || payload.Version == "" || payload.Commit == "" {
		t.Fatalf("health payload = %+v", payload)
	}
}

func apiRequest(t *testing.T, handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func responseCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("response cookies = %d, want 1", len(cookies))
	}
	return cookies[0]
}

func integrationRequest(t *testing.T, handler http.Handler, path, authorization, username string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if username != "" {
		request.Header.Set("X-Teacher-Username", username)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestIntegrationClassesEndpoint(t *testing.T) {
	t.Setenv("CLASS_SYSTEM_TOKEN", "classorbit-shared-secret")
	db, handler := testAPI(t)

	// The integration endpoint has its own authentication and must not require
	// the browser session cookie used by the teacher UI.
	if err := db.createTeacherAccount("teacher", "unused-password-hash"); err != nil {
		t.Fatal(err)
	}
	first, err := db.createClass(classInput{Grade: "三", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.createClass(classInput{Grade: "四", ClassNo: "2"})
	if err != nil {
		t.Fatal(err)
	}
	deletedStudent, err := db.createStudent(second.ID, studentInput{StudentNo: "2001", Name: "已删除学生"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.deleteStudent(deletedStudent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.createStudent(first.ID, studentInput{StudentNo: "1002", Name: "李四"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.createStudent(first.ID, studentInput{StudentNo: "1001", Name: "张三"}); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, path, authorization, username string
		status                              int
	}{
		{name: "missing credentials", path: "/api/integration/classes?teacher_username=teacher", status: http.StatusUnauthorized},
		{name: "malformed credentials", path: "/api/integration/classes?teacher_username=teacher", authorization: "Basic classorbit-shared-secret", status: http.StatusUnauthorized},
		{name: "wrong shared secret", path: "/api/integration/classes?teacher_username=teacher", authorization: "Bearer wrong-secret", username: "teacher", status: http.StatusForbidden},
		{name: "unknown teacher", path: "/api/integration/classes?teacher_username=unknown", authorization: "Bearer classorbit-shared-secret", username: "unknown", status: http.StatusNotFound},
		{name: "header mismatch", path: "/api/integration/classes?teacher_username=teacher", authorization: "Bearer classorbit-shared-secret", username: "other", status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := integrationRequest(t, handler, test.path, test.authorization, test.username)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}

	response := integrationRequest(t, handler, "/api/integration/classes?teacher_username=TEACHER", "Bearer classorbit-shared-secret", "teacher")
	if response.Code != http.StatusOK {
		t.Fatalf("success status = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("cache policy = %q", response.Header().Get("Cache-Control"))
	}
	etag := response.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	var payload integrationClassesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Classes) != 2 || payload.Classes[0].ID != fmt.Sprint(first.ID) || payload.Classes[0].Name != "三 1 班" || payload.Classes[1].ID != fmt.Sprint(second.ID) || payload.Classes[1].Name != "四 2 班" {
		t.Fatalf("classes payload = %+v", payload)
	}
	conditional := httptest.NewRequest(http.MethodGet, "/api/integration/classes?teacher_username=teacher", nil)
	conditional.Header.Set("Authorization", "Bearer classorbit-shared-secret")
	conditional.Header.Set("X-Teacher-Username", "teacher")
	conditional.Header.Set("If-None-Match", etag)
	conditionalResponse := httptest.NewRecorder()
	handler.ServeHTTP(conditionalResponse, conditional)
	if conditionalResponse.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", conditionalResponse.Code)
	}
	if len(payload.Classes[0].Students) != 2 || payload.Classes[0].Students[0].ID != "1001" || payload.Classes[0].Students[0].Name != "张三" || payload.Classes[0].Students[1].ID != "1002" {
		t.Fatalf("students payload = %+v", payload.Classes[0].Students)
	}
	if payload.Classes[1].Students == nil {
		t.Fatal("a class containing only deleted students should remain present with an empty JSON array")
	}
}

func TestDatabasePathUsesLegacyFileOnlyWhenNeeded(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "classorbit.db")
	legacy := filepath.Join(dir, "classpoint.db")
	if got := databasePath(dir); got != current {
		t.Fatalf("fresh database path = %q, want %q", got, current)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := databasePath(dir); got != legacy {
		t.Fatalf("legacy database path = %q, want %q", got, legacy)
	}
	if err := os.WriteFile(current, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := databasePath(dir); got != current {
		t.Fatalf("current database path = %q, want %q", got, current)
	}
}

func TestTeacherSetupLoginAndSession(t *testing.T) {
	db, handler := testAPI(t)

	response := apiRequest(t, handler, http.MethodGet, "/api/auth", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("initial auth status = %d: %s", response.Code, response.Body.String())
	}
	var status struct {
		Initialized   bool   `json:"initialized"`
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Initialized || status.Authenticated || status.Username != "" {
		t.Fatalf("initial auth payload = %+v", status)
	}

	response = apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", response.Code, response.Body.String())
	}
	sessionCookie := responseCookie(t, response)
	if sessionCookie.Value == "" || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %+v", sessionCookie)
	}
	account, err := db.teacherAccount()
	if err != nil {
		t.Fatal(err)
	}
	if account.PasswordHash == "strong-pass-123" || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte("strong-pass-123")) != nil {
		t.Fatal("teacher password was not stored as a valid bcrypt hash")
	}

	response = apiRequest(t, handler, http.MethodGet, "/api/auth", "", sessionCookie)
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Initialized || !status.Authenticated || status.Username != "teacher" {
		t.Fatalf("authenticated payload = %+v", status)
	}

	response = apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"other","password":"another-pass-123"}`, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, "/api/navigation", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated navigation status = %d", response.Code)
	}

	response = apiRequest(t, handler, http.MethodDelete, "/api/auth", "", sessionCookie)
	if response.Code != http.StatusOK || responseCookie(t, response).MaxAge != -1 {
		t.Fatalf("logout response = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, "/api/auth", "", sessionCookie)
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Authenticated || status.Username != "" {
		t.Fatalf("logged-out payload = %+v", status)
	}

	response = apiRequest(t, handler, http.MethodPost, "/api/auth", `{"username":"teacher","password":"wrong-pass"}`, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login status = %d", response.Code)
	}
	response = apiRequest(t, handler, http.MethodPost, "/api/auth", `{"username":"TEACHER","password":"strong-pass-123"}`, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", response.Code, response.Body.String())
	}
	if responseCookie(t, response).Value == sessionCookie.Value {
		t.Fatal("login reused a revoked session token")
	}
}

func TestQRLoginRequiresPhoneConfirmationAndCreatesIndependentSession(t *testing.T) {
	_, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	phoneCookie := responseCookie(t, setup)

	start := apiRequest(t, handler, http.MethodPost, "/api/auth/qr/start", `{"deviceName":"Chrome · macOS"}`, nil)
	if start.Code != http.StatusCreated {
		t.Fatalf("start QR login = %d: %s", start.Code, start.Body.String())
	}
	var created struct {
		Token      string `json:"token"`
		ClaimToken string `json:"claimToken"`
		ExpiresAt  int64  `json:"expiresAt"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !validQRLoginToken(created.Token) || !validQRLoginToken(created.ClaimToken) || created.Token == created.ClaimToken || created.ExpiresAt == 0 {
		t.Fatalf("created QR login = %+v", created)
	}

	challenge := apiRequest(t, handler, http.MethodGet, "/api/auth/qr/challenge?token="+created.Token, "", nil)
	if challenge.Code != http.StatusOK || !strings.Contains(challenge.Body.String(), `"status":"scanned"`) || !strings.Contains(challenge.Body.String(), "Chrome · macOS") {
		t.Fatalf("scan QR login = %d: %s", challenge.Code, challenge.Body.String())
	}
	statusBody := fmt.Sprintf(`{"token":%q}`, created.ClaimToken)
	status := apiRequest(t, handler, http.MethodPost, "/api/auth/qr/status", statusBody, nil)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"scanned"`) || len(status.Result().Cookies()) != 0 {
		t.Fatalf("unapproved QR status = %d: %s", status.Code, status.Body.String())
	}

	decisionBody := fmt.Sprintf(`{"token":%q,"approve":true}`, created.Token)
	unauthorized := apiRequest(t, handler, http.MethodPost, "/api/auth/qr/decision", decisionBody, nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated QR approval = %d", unauthorized.Code)
	}
	approved := apiRequest(t, handler, http.MethodPost, "/api/auth/qr/decision", decisionBody, phoneCookie)
	if approved.Code != http.StatusOK || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("approve QR login = %d: %s", approved.Code, approved.Body.String())
	}

	claimed := apiRequest(t, handler, http.MethodPost, "/api/auth/qr/status", statusBody, nil)
	if claimed.Code != http.StatusOK || !strings.Contains(claimed.Body.String(), `"authenticated":true`) {
		t.Fatalf("claim QR login = %d: %s", claimed.Code, claimed.Body.String())
	}
	computerCookie := responseCookie(t, claimed)
	if computerCookie.Value == phoneCookie.Value || !computerCookie.HttpOnly || computerCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("computer session is not independent and secure: %+v", computerCookie)
	}
	auth := apiRequest(t, handler, http.MethodGet, "/api/auth", "", computerCookie)
	if auth.Code != http.StatusOK || !strings.Contains(auth.Body.String(), `"authenticated":true`) || !strings.Contains(auth.Body.String(), `"username":"teacher"`) {
		t.Fatalf("QR-authenticated session = %d: %s", auth.Code, auth.Body.String())
	}

	reused := apiRequest(t, handler, http.MethodPost, "/api/auth/qr/status", statusBody, nil)
	if reused.Code != http.StatusOK || !strings.Contains(reused.Body.String(), `"status":"consumed"`) || len(reused.Result().Cookies()) != 0 {
		t.Fatalf("reused QR claim = %d: %s", reused.Code, reused.Body.String())
	}
}

func TestTeacherEnvironmentBootstrapOnlyInitializesEmptyStore(t *testing.T) {
	db := testStore(t)
	t.Setenv("TEACHER_USERNAME", "deployed-teacher")
	t.Setenv("TEACHER_PASSWORD", "deployed-pass-123")
	if err := initializeTeacherFromEnvironment(db); err != nil {
		t.Fatal(err)
	}
	account, err := db.teacherAccount()
	if err != nil || account.Username != "deployed-teacher" || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte("deployed-pass-123")) != nil {
		t.Fatalf("environment account = %+v, %v", account, err)
	}
	t.Setenv("TEACHER_USERNAME", "replacement")
	t.Setenv("TEACHER_PASSWORD", "replacement-pass-123")
	if err := initializeTeacherFromEnvironment(db); err != nil {
		t.Fatal(err)
	}
	account, err = db.teacherAccount()
	if err != nil || account.Username != "deployed-teacher" {
		t.Fatalf("initialized account should not be replaced = %+v, %v", account, err)
	}
}

func TestDeleteAttendanceEndpoint(t *testing.T) {
	db, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup)
	class, err := db.createClass(classInput{Grade: "三", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.createStudent(class.ID, studentInput{StudentNo: "01", Name: "张同学"}); err != nil {
		t.Fatal(err)
	}
	session, err := db.createAttendance(attendanceInput{ClassID: class.ID, SessionAt: "2026-08-30T09:00"})
	if err != nil {
		t.Fatal(err)
	}

	response := apiRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/attendance/%d", session.ID), "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("attendance detail status = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodDelete, fmt.Sprintf("/api/attendance/%d", session.ID), "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete status = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodDelete, fmt.Sprintf("/api/attendance/%d", session.ID), "", cookie)
	if response.Code != http.StatusOK || response.Body.String() != "{\"ok\":true}\n" {
		t.Fatalf("delete response = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodDelete, fmt.Sprintf("/api/attendance/%d", session.ID), "", cookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d: %s", response.Code, response.Body.String())
	}
}

func TestClassroomEndToEndAPIFlow(t *testing.T) {
	_, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup)

	response := apiRequest(t, handler, http.MethodPost, "/api/classes", `{"grade":"三","classNo":"4"}`, cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("create class = %d: %s", response.Code, response.Body.String())
	}
	var class classRow
	if err := json.Unmarshal(response.Body.Bytes(), &class); err != nil {
		t.Fatal(err)
	}
	response = apiRequest(t, handler, http.MethodPost, fmt.Sprintf("/api/classes/%d/students", class.ID), `{"studentNo":"01","name":"张同学"}`, cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("create student = %d: %s", response.Code, response.Body.String())
	}
	var student studentRow
	if err := json.Unmarshal(response.Body.Bytes(), &student); err != nil {
		t.Fatal(err)
	}

	attendanceBody := fmt.Sprintf(`{"classId":%d,"course":"信息科技","sessionAt":"2026-09-07T08:00"}`, class.ID)
	response = apiRequest(t, handler, http.MethodPost, "/api/attendance", attendanceBody, cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("create attendance = %d: %s", response.Code, response.Body.String())
	}
	var attendance attendanceView
	if err := json.Unmarshal(response.Body.Bytes(), &attendance); err != nil {
		t.Fatal(err)
	}

	response = apiRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/public/classes/%d/students", class.ID), "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("public roster = %d: %s", response.Code, response.Body.String())
	}
	checkinBody := fmt.Sprintf(`{"classId":%d,"studentId":%d}`, class.ID, student.ID)
	response = apiRequest(t, handler, http.MethodPost, "/api/public/check-in", checkinBody, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("check in = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodPost, fmt.Sprintf("/api/attendance/%d/close", attendance.ID), "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("close attendance = %d: %s", response.Code, response.Body.String())
	}

	response = apiRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/attendance?class_id=%d&limit=30", class.ID), "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("attendance page = %d: %s", response.Code, response.Body.String())
	}
	var page attendancePage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].PresentCount != 1 || len(page.Items[0].Records) != 0 {
		t.Fatalf("attendance summary page = %+v", page)
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	_, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	cookie := responseCookie(t, setup)
	response := apiRequest(t, handler, http.MethodPatch, "/api/auth/password", `{"currentPassword":"wrong","newPassword":"new-strong-pass-456"}`, cookie)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password change = %d", response.Code)
	}
	response = apiRequest(t, handler, http.MethodPatch, "/api/auth/password", `{"currentPassword":"strong-pass-123","newPassword":"new-strong-pass-456"}`, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("password change = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, "/api/dashboard", "", cookie)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("old session after password change = %d", response.Code)
	}
	response = apiRequest(t, handler, http.MethodPost, "/api/auth", `{"username":"teacher","password":"new-strong-pass-456"}`, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("login with new password = %d: %s", response.Code, response.Body.String())
	}
}

func TestNavigationBatchValidationAndPublicRead(t *testing.T) {
	_, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup)

	body := `{"items":[{"title":" Scratch ","url":" https://scratch.mit.edu/ ","iconUrl":"https://scratch.mit.edu/favicon.ico"},{"title":"国家中小学智慧教育平台","url":"https://basic.smartedu.cn/","iconUrl":""}]}`
	response := apiRequest(t, handler, http.MethodPut, "/api/navigation", body, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("navigation update status = %d: %s", response.Code, response.Body.String())
	}
	var items []navigationLink
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "Scratch" || items[0].SortOrder != 0 || items[1].SortOrder != 1 {
		t.Fatalf("saved navigation = %+v", items)
	}

	response = apiRequest(t, handler, http.MethodGet, "/api/public/navigation", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("public navigation status = %d: %s", response.Code, response.Body.String())
	}
	var publicItems []navigationLink
	if err := json.Unmarshal(response.Body.Bytes(), &publicItems); err != nil {
		t.Fatal(err)
	}
	if len(publicItems) != 2 || publicItems[0].ID == 0 || publicItems[0].URL != "https://scratch.mit.edu/" {
		t.Fatalf("public navigation = %+v", publicItems)
	}

	for _, invalidBody := range []string{
		`{}`,
		`{"items":[{"title":"危险链接","url":"javascript:alert(1)","iconUrl":""}]}`,
		`{"items":[{"title":"含账号链接","url":"https://user:pass@example.com/","iconUrl":""}]}`,
		`{"items":[{"title":"图标错误","url":"https://example.com/","iconUrl":"data:image/png;base64,AA=="}]}`,
	} {
		response = apiRequest(t, handler, http.MethodPut, "/api/navigation", invalidBody, cookie)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid navigation %s status = %d: %s", invalidBody, response.Code, response.Body.String())
		}
	}

	response = apiRequest(t, handler, http.MethodPut, "/api/navigation", `{"items":[]}`, cookie)
	if response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatalf("empty navigation response = %d: %q", response.Code, response.Body.String())
	}
}

type teachingSiteTestFile struct {
	Name string
	Path string
	Data []byte
}

func teachingSiteAPIRequest(t *testing.T, handler http.Handler, method, requestPath, mode, title string, files []teachingSiteTestFile, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("mode", mode); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("title", title); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("sourceName", "示例项目"); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		part, err := writer.CreateFormFile("files", file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file.Data); err != nil {
			t.Fatal(err)
		}
		if err := writer.WriteField("paths", file.Path); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, requestPath, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func makeTeachingSiteZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for _, name := range sortedKeys(files) {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(files[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestTeachingSiteCRUDDirectoryZIPAndConcurrentServing(t *testing.T) {
	db, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	cookie := responseCookie(t, setup)

	files := []teachingSiteTestFile{
		{Name: "index.html", Path: "binary-demo/index.html", Data: []byte(`<!doctype html><link rel="stylesheet" href="assets/style.css"><script>document.body.dataset.ready='yes'</script><h1>二进制练习</h1>`)},
		{Name: "style.css", Path: "binary-demo/assets/style.css", Data: []byte("h1{color:green}")},
		{Name: "empty.txt", Path: "binary-demo/empty.txt", Data: []byte{}},
	}
	response := teachingSiteAPIRequest(t, handler, http.MethodPost, "/api/navigation/sites", "folder", "二进制互动练习", files, cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("create teaching site = %d: %s", response.Code, response.Body.String())
	}
	var created navigationLink
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Kind != "site" || created.Site == nil || created.Site.FileCount != 3 || created.Site.ExtractedSize == 0 || created.URL == "" {
		t.Fatalf("created teaching site = %+v", created)
	}
	if !strings.HasPrefix(created.URL, "/published/") {
		t.Fatalf("teaching site URL is not canonical: %q", created.URL)
	}

	response = apiRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/navigation/sites/%d", created.ID), "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("read teaching site = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/navigation/sites/%d", created.ID), "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated teaching-site metadata = %d", response.Code)
	}

	// Remove the extracted cache and hit it concurrently. One request rebuilds
	// from the database archive while all readers receive the same static page.
	if err := os.RemoveAll(filepath.Join(filepath.Dir(db.path), "site-cache")); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsFound := make(chan string, 50)
	for index := 0; index < 50; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result := apiRequest(t, handler, http.MethodGet, created.URL, "", nil)
			if result.Code != http.StatusOK || !bytes.Contains(result.Body.Bytes(), []byte("二进制练习")) {
				errorsFound <- fmt.Sprintf("status=%d body=%q", result.Code, result.Body.String())
			}
			if csp := result.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "allow-same-origin") {
				errorsFound <- "teaching page does not allow same-origin storage"
			}
			if cacheControl := result.Header().Get("Cache-Control"); cacheControl != "public, max-age=0, must-revalidate" {
				errorsFound <- "teaching HTML can retain stale sandbox headers in a CDN"
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for message := range errorsFound {
		t.Error(message)
	}
	response = apiRequest(t, handler, http.MethodGet, created.URL+"assets/style.css", "", nil)
	if response.Code != http.StatusOK || response.Body.String() != "h1{color:green}" {
		t.Fatalf("nested teaching-site asset = %d: %q", response.Code, response.Body.String())
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "public, max-age=31536000, immutable" {
		t.Fatalf("teaching-site asset cache policy = %q", cacheControl)
	}

	updateBody := fmt.Sprintf(`{"items":[{"id":%d,"kind":"site","title":"更新后的练习","url":"javascript:ignored","iconUrl":""},{"kind":"external","title":"示例","url":"https://example.com/","iconUrl":""}]}`, created.ID)
	response = apiRequest(t, handler, http.MethodPut, "/api/navigation", updateBody, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("update mixed navigation = %d: %s", response.Code, response.Body.String())
	}
	var navigation []navigationLink
	if err := json.Unmarshal(response.Body.Bytes(), &navigation); err != nil {
		t.Fatal(err)
	}
	if len(navigation) != 2 || navigation[0].ID != created.ID || navigation[0].Title != "更新后的练习" || navigation[0].Kind != "site" {
		t.Fatalf("updated mixed navigation = %+v", navigation)
	}

	zipData := makeTeachingSiteZIP(t, map[string]string{
		"lesson/index.html":     "<!doctype html><h1>新版本</h1>",
		"lesson/scripts/app.js": "document.body.dataset.version='2'",
	})
	response = teachingSiteAPIRequest(t, handler, http.MethodPut, fmt.Sprintf("/api/navigation/sites/%d", created.ID), "zip", "更新后的练习", []teachingSiteTestFile{{Name: "lesson.zip", Path: "lesson.zip", Data: zipData}}, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("replace teaching site = %d: %s", response.Code, response.Body.String())
	}
	var updated navigationLink
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Site == nil || updated.Site.Revision == created.Site.Revision || updated.Site.FileCount != 2 {
		t.Fatalf("replaced teaching site = %+v", updated)
	}
	response = apiRequest(t, handler, http.MethodGet, updated.URL, "", nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("新版本")) {
		t.Fatalf("updated teaching page = %d: %s", response.Code, response.Body.String())
	}
	singleHTML := []byte(`<!doctype html><meta charset="utf-8"><h1>单文件终稿</h1>`)
	response = teachingSiteAPIRequest(t, handler, http.MethodPut, fmt.Sprintf("/api/navigation/sites/%d", created.ID), "html", "更新后的练习", []teachingSiteTestFile{{Name: "final.html", Path: "final.html", Data: singleHTML}}, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("replace with single HTML = %d: %s", response.Code, response.Body.String())
	}
	var singleFileVersion navigationLink
	if err := json.Unmarshal(response.Body.Bytes(), &singleFileVersion); err != nil {
		t.Fatal(err)
	}
	if singleFileVersion.Site == nil || singleFileVersion.Site.FileCount != 1 || singleFileVersion.Site.SourceName != "final.html" {
		t.Fatalf("single-file teaching site = %+v", singleFileVersion)
	}
	updated = singleFileVersion
	response = apiRequest(t, handler, http.MethodGet, updated.URL, "", nil)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), singleHTML) {
		t.Fatalf("single-file teaching page = %d: %s", response.Code, response.Body.String())
	}

	backupPath, err := db.createBackup()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(backupPath)
	backup, err := openStore(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if payload, err := backup.teachingSiteArchive(updated.Site.PublicID, updated.Site.Revision); err != nil || len(payload) == 0 {
		t.Fatalf("teaching-site archive in backup = %d bytes, %v", len(payload), err)
	}

	response = apiRequest(t, handler, http.MethodDelete, fmt.Sprintf("/api/navigation/sites/%d", created.ID), "", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("delete teaching site = %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, updated.URL, "", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("deleted teaching page remains public: %d", response.Code)
	}
}

func TestTeachingSiteRejectsUnsafeOrIncompleteZIP(t *testing.T) {
	_, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	cookie := responseCookie(t, setup)
	for name, archive := range map[string][]byte{
		"missing index": makeTeachingSiteZIP(t, map[string]string{"lesson.html": "<h1>无入口</h1>"}),
		"zip slip":      makeTeachingSiteZIP(t, map[string]string{"../index.html": "<h1>越界</h1>"}),
	} {
		t.Run(name, func(t *testing.T) {
			response := teachingSiteAPIRequest(t, handler, http.MethodPost, "/api/navigation/sites", "zip", "无效项目", []teachingSiteTestFile{{Name: "invalid.zip", Path: path.Base("invalid.zip"), Data: archive}}, cookie)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid ZIP status = %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
