package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func testAPI(t *testing.T) (*store, http.Handler) {
	t.Helper()
	db := testStore(t)
	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)
	return db, s.requireTeacher(mux)
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
	if _, err := db.createClass(classInput{Grade: "四", ClassNo: "2"}); err != nil {
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
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache policy = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	var payload integrationClassesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Classes) != 2 || payload.Classes[0].Name != "三 1 班" || payload.Classes[1].Name != "四 2 班" {
		t.Fatalf("classes payload = %+v", payload)
	}
	if len(payload.Classes[0].Students) != 2 || payload.Classes[0].Students[0].ID != "1001" || payload.Classes[0].Students[0].Name != "张三" || payload.Classes[0].Students[1].ID != "1002" {
		t.Fatalf("students payload = %+v", payload.Classes[0].Students)
	}
	if payload.Classes[1].Students == nil {
		t.Fatal("empty class students should be an empty JSON array")
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
