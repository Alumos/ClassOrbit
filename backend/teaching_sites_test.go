package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedCompatibilityRedirect(t *testing.T) {
	_, handler := testAPI(t)
	for _, suffix := range []string{
		"/1234567890abcdef/abcdef1234567890/",
		"/1234567890abcdef/abcdef1234567890/assets/app.js?v=2&lang=zh",
		"/1234567890abcdef/abcdef1234567890/assets/%E8%AF%BE%20%23%3F.html?q=%2F",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			request := httptest.NewRequest(method, "/published-v2"+suffix, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != "/published"+suffix {
				t.Fatalf("%s %s: status=%d location=%q", method, suffix, response.Code, response.Header().Get("Location"))
			}
		}
	}
}

func TestTeachingSiteBackupReopen(t *testing.T) {
	db, handler := testAPI(t)
	setup := apiRequest(t, handler, http.MethodPost, "/api/setup", `{"username":"teacher","password":"strong-pass-123"}`, nil)
	cookie := responseCookie(t, setup)
	response := teachingSiteAPIRequest(t, handler, http.MethodPost, "/api/navigation/sites", "folder", "升级练习", []teachingSiteTestFile{
		{Name: "index.html", Path: "lesson/index.html", Data: []byte("<h1>upgrade</h1>")},
		{Name: "课 #?.js", Path: "lesson/assets/课 #?.js", Data: []byte("console.log('upgrade')")},
	}, cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload: %s", response.Body.String())
	}
	before, err := db.navigation()
	if err != nil || len(before) != 1 {
		t.Fatalf("navigation: %+v, %v", before, err)
	}
	backupPath, err := db.createBackup()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(backupPath)
	// A fresh data directory has only SQLite, as when restoring a volume.
	restoredPath := filepath.Join(t.TempDir(), "classorbit.db")
	archive, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(restoredPath, archive, 0600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		restored, err := openStore(restoredPath)
		if err != nil {
			t.Fatal(err)
		}
		s := &server{db: restored}
		if err := s.refreshTeachingSiteRegistry(); err != nil {
			t.Fatal(err)
		}
		if err := s.pruneSiteCache(); err != nil {
			t.Fatal(err)
		}
		mux := http.NewServeMux()
		s.routes(mux)
		handler := s.requireTeacher(mux)
		after, err := restored.navigation()
		if err != nil || len(after) != 1 || after[0].URL != before[0].URL || *after[0].Site != *before[0].Site {
			t.Fatalf("reopen navigation: %+v, %v", after, err)
		}
		for suffix, body := range map[string]string{"": "<h1>upgrade</h1>", "assets/%E8%AF%BE%20%23%3F.js": "console.log('upgrade')"} {
			response := apiRequest(t, handler, http.MethodGet, after[0].URL+suffix, "", nil)
			if response.Code != http.StatusOK || response.Body.String() != body {
				t.Fatalf("restored page: %d %s", response.Code, response.Body.String())
			}
		}
		response := apiRequest(t, handler, http.MethodGet, "/api/navigation", "", cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("session lost after reopen: %d", response.Code)
		}
		if err := restored.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
