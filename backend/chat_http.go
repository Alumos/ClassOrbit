package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const studentChatLifetime = 4 * time.Hour
const studentChatCookie = "classorbit_student"

type requestLeaseKey struct{}

func releaseRequestLease(r *http.Request) {
	if release, ok := r.Context().Value(requestLeaseKey{}).(func()); ok {
		release()
	}
}

// Only these APIs share the authorization/mutation gate. Lock order is always
// maintenance -> chatAccess -> hub. SSE releases the request lease before I/O.
func chatRequest(path string, method string) bool {
	return strings.HasPrefix(path, "/api/public/chat/") || strings.HasPrefix(path, "/api/chat/") || chatChangesIdentity(path, method)
}
func chatChangesIdentity(path, method string) bool {
	if path == "/api/public/check-in" {
		return method == http.MethodPost
	}
	if path == "/api/auth" {
		return method == http.MethodDelete
	}
	if path == "/api/auth/password" {
		return method == http.MethodPatch
	}
	if method == http.MethodDelete {
		return strings.HasPrefix(path, "/api/attendance/") || strings.HasPrefix(path, "/api/classes/") || strings.HasPrefix(path, "/api/students/")
	}
	return strings.HasPrefix(path, "/api/attendance/") &&
		((method == http.MethodPost && strings.HasSuffix(path, "/close")) ||
			(method == http.MethodPatch && strings.Contains(path, "/records/")))
}

// Read the small chat/control body before taking the authorization gate. A
// slow upload must not monopolize all classrooms or hold the maintenance lock.
func bufferChatBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody || r.Method == http.MethodGet {
		return true
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	_ = r.Body.Close()
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			chatError(w, chatFail(413, "body_too_large", "请求内容过大"))
		} else {
			chatError(w, chatFail(400, "invalid_body", "无法读取请求，请重新发送"))
		}
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return true
}

func (s *server) chatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/public/chat/session", s.getChatSession)
	mux.HandleFunc("DELETE /api/public/chat/session", s.logoutChat)
	mux.HandleFunc("GET /api/public/chat/events", s.chatEvents)
	mux.HandleFunc("POST /api/public/chat/messages", s.sendChatMessage)
	mux.HandleFunc("GET /api/chat/rooms", s.getChatRooms)
	mux.HandleFunc("GET /api/chat/rooms/{sessionId}/events", s.chatEvents)
	mux.HandleFunc("POST /api/chat/rooms/{sessionId}/messages", s.sendChatMessage)
	mux.HandleFunc("PATCH /api/chat/rooms/{sessionId}", s.manageChat)
	mux.HandleFunc("DELETE /api/chat/rooms/{sessionId}/messages", s.manageChat)
}
func chatError(w http.ResponseWriter, err error) {
	var failure *chatFailure
	if errors.As(err, &failure) {
		if failure.status == 429 {
			w.Header().Set("Retry-After", "2")
		}
		writeJSON(w, failure.status, struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}{failure.message, failure.code})
		return
	}
	log.Printf("chat operation failed: %v", err)
	writeJSON(w, 500, apiError{Error: "聊天室暂时不可用，请稍后再试"})
}
func (s *server) chatEnabled(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if s.chat == nil {
		chatError(w, chatFail(503, "chat_disabled", "聊天室尚未开启"))
		return false
	}
	return true
}
func chatOriginAllowed(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" {
		return false
	}
	expected := strings.TrimSpace(env("CHAT_ORIGIN", ""))
	if expected != "" {
		return r.Header.Get("Origin") == expected
	}
	scheme := "http"
	if requestUsesHTTPS(r) {
		scheme = "https"
	}
	return origin.Scheme == scheme && origin.Host == r.Host
}
func requireChatOrigin(w http.ResponseWriter, r *http.Request) bool {
	if !chatOriginAllowed(r) {
		chatError(w, chatFail(403, "invalid_origin", "请求来源不受信任，请从本站页面操作"))
		return false
	}
	return true
}
func studentCookieHash(r *http.Request) string {
	cookie, err := r.Cookie(studentChatCookie)
	if err != nil || !validQRLoginToken(cookie.Value) {
		return ""
	}
	return hashSessionToken(cookie.Value)
}
func setStudentChatCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	maxAge := int(studentChatLifetime.Seconds())
	if token == "" {
		maxAge = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: studentChatCookie, Value: token, Path: "/api/public", HttpOnly: true, Secure: requestUsesHTTPS(r), SameSite: http.SameSiteStrictMode, MaxAge: maxAge, Expires: expires})
}
func (s *server) chatStudent(r *http.Request) (chatIdentity, error) {
	hash := studentCookieHash(r)
	if hash == "" {
		return chatIdentity{}, chatFail(401, "student_session_required", "请先在此浏览器完成本场签到；已签到但无法进入时，请联系老师重新开放本人签到")
	}
	identity, err := s.db.studentChatIdentity(hash, time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		return identity, chatFail(401, "student_session_expired", "学生身份已失效或课堂已结束，请联系老师确认签到")
	}
	return identity, err
}
func (s *server) chatActor(r *http.Request) (int64, chatSender, string, error) {
	if strings.HasPrefix(r.URL.Path, "/api/public/") {
		identity, err := s.chatStudent(r)
		return identity.RoomID, chatSender{"student", strconv.FormatInt(identity.StudentID, 10), identity.Name}, identity.TokenHash, err
	}
	username, ok, err := s.authenticateTeacher(r)
	if err != nil {
		return 0, chatSender{}, "", err
	}
	if !ok {
		return 0, chatSender{}, "", chatFail(401, "teacher_session_required", "请重新登录教师后台")
	}
	id := pathID(r, "sessionId")
	rooms, err := s.db.chatRooms(0)
	if err != nil {
		return 0, chatSender{}, "", err
	}
	found := false
	for _, room := range rooms {
		if room.RoomID == id {
			found = true
			break
		}
	}
	if !found {
		return 0, chatSender{}, "", chatFail(404, "room_closed", "当前没有进行中的课堂")
	}
	cookie, _ := r.Cookie(teacherSessionCookie)
	return id, chatSender{"teacher", "1", username}, hashSessionToken(cookie.Value), nil
}
func (s *server) getChatSession(w http.ResponseWriter, r *http.Request) {
	if !s.chatEnabled(w) {
		return
	}
	identity, err := s.chatStudent(r)
	if err != nil {
		chatError(w, err)
		return
	}
	writeJSON(w, 200, identity)
}
func (s *server) logoutChat(w http.ResponseWriter, r *http.Request) {
	if !s.chatEnabled(w) || !requireChatOrigin(w, r) {
		return
	}
	if err := s.db.deleteChatToken(studentCookieHash(r)); err != nil {
		chatError(w, err)
		return
	}
	s.reconcileChat()
	setStudentChatCookie(w, r, "", time.Time{})
	writeJSON(w, 200, operationResponse{OK: true})
}
func (s *server) getChatRooms(w http.ResponseWriter, r *http.Request) {
	if !s.chatEnabled(w) {
		return
	}
	classID, _ := strconv.ParseInt(r.URL.Query().Get("classId"), 10, 64)
	rooms, err := s.db.chatRooms(classID)
	if err != nil {
		chatError(w, err)
		return
	}
	for i := range rooms {
		rooms[i].MessageCount, rooms[i].OnlineStudents, rooms[i].Paused = s.chat.stats(rooms[i].RoomID)
	}
	writeJSON(w, 200, rooms)
}
func decodeChat(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = errors.New("extra JSON")
		}
	}
	if err != nil {
		var maxBody *http.MaxBytesError
		if errors.As(err, &maxBody) {
			chatError(w, chatFail(413, "body_too_large", "消息请求过大"))
		} else {
			chatError(w, chatFail(422, "invalid_message", "消息格式无效"))
		}
		return false
	}
	return true
}
func (s *server) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	if !s.chatEnabled(w) || !requireChatOrigin(w, r) {
		return
	}
	id, sender, _, err := s.chatActor(r)
	if err != nil {
		chatError(w, err)
		return
	}
	var input chatMessageInput
	if !decodeChat(w, r, &input) {
		return
	}
	input.Text = strings.TrimSpace(input.Text)
	if !utf8.ValidString(input.Text) || input.Text == "" || utf8.RuneCountInString(input.Text) > 500 || !validClientMessageID(input.ClientMessageID) {
		chatError(w, chatFail(422, "invalid_message", "请输入 1–500 个字符的消息"))
		return
	}
	message, duplicate, err := s.chat.send(id, sender, input, time.Now())
	if err != nil {
		chatError(w, err)
		return
	}
	status := 201
	if duplicate {
		status = 200
	}
	writeJSON(w, status, message)
}
func validClientMessageID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func (s *server) manageChat(w http.ResponseWriter, r *http.Request) {
	if !s.chatEnabled(w) || !requireChatOrigin(w, r) {
		return
	}
	id, _, _, err := s.chatActor(r)
	if err != nil {
		chatError(w, err)
		return
	}
	if !s.chat.allowControl(time.Now()) {
		chatError(w, chatFail(429, "rate_limit", "管理操作过于频繁，请稍后再试"))
		return
	}
	var paused *bool
	action := "clear"
	if r.Method == http.MethodPatch {
		var input struct {
			Paused *bool `json:"paused"`
		}
		if !decodeChat(w, r, &input) {
			return
		}
		if input.Paused == nil {
			chatError(w, chatFail(422, "invalid_state", "缺少暂停状态"))
			return
		}
		paused = input.Paused
		action = "resume"
		if *paused {
			action = "pause"
		}
	}
	epoch, err := randomURLToken()
	if err != nil {
		chatError(w, err)
		return
	}
	if err = s.chat.ensure(id, time.Now()); err != nil {
		chatError(w, err)
		return
	}
	count, _, _ := s.chat.stats(id)
	if err = s.db.auditChat(id, action, count); err != nil {
		chatError(w, err)
		return
	}
	snapshot, err := s.chat.manage(id, paused, epoch, time.Now())
	if err != nil {
		chatError(w, err)
		return
	}
	writeJSON(w, 200, snapshot)
}
func (s *server) chatEvents(w http.ResponseWriter, r *http.Request) {
	if !s.chatEnabled(w) {
		return
	}
	id, sender, hash, err := s.chatActor(r)
	if err != nil {
		chatError(w, err)
		return
	}
	controller := http.NewResponseController(w)
	if err = controller.SetWriteDeadline(time.Time{}); err != nil {
		chatError(w, err)
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		chatError(w, errors.New("SSE flushing unsupported"))
		return
	}
	client, err := s.chat.subscribe(id, sender, hash, time.Now())
	if err != nil {
		chatError(w, err)
		return
	}
	defer s.chat.unsubscribe(client)
	releaseRequestLease(r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	// Canceling a subscription must interrupt even a slow network writer.
	writerDone := make(chan struct{})
	writerExited := make(chan struct{})
	defer func() { close(writerDone); <-writerExited }()
	go func() {
		defer close(writerExited)
		select {
		case <-client.done:
			_ = controller.SetWriteDeadline(time.Now())
		case <-r.Context().Done():
			_ = controller.SetWriteDeadline(time.Now())
		case <-writerDone:
		}
	}()
	write := func(body []byte) bool {
		select {
		case <-client.done:
			return false
		default:
		}
		if controller.SetWriteDeadline(time.Now().Add(10*time.Second)) != nil {
			return false
		}
		if _, err := w.Write(body); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-client.done:
			return
		case event := <-client.events:
			s.chat.consumed(client, event)
			if !write(event.wire) {
				return
			}
		case <-heartbeat.C:
			if !write([]byte(": heartbeat\n\n")) {
				return
			}
		}
	}
}

// Caller holds maintenance and chatAccess leases. Fail closed if validation
// fails; do not keep broadcasting with stale authorization after a DB failure.
func (s *server) reconcileChat() {
	if s.chat == nil {
		return
	}
	rooms, err := s.db.chatRooms(0)
	students, teachers := s.chat.credentials()
	var validStudents, validTeachers map[string]bool
	if err == nil {
		validStudents, err = s.db.validChatTokens(students, false, time.Now())
	}
	if err == nil {
		validTeachers, err = s.db.validChatTokens(teachers, true, time.Now())
	}
	if err != nil {
		log.Printf("chat authorization refresh failed: %v", err)
		s.chat.reset(false)
		return
	}
	active := make(map[int64]bool)
	for _, room := range rooms {
		active[room.RoomID] = true
	}
	s.chat.reconcile(active, validStudents, validTeachers, time.Now())
}
func (s *server) startChatScheduler() func() {
	ctx, cancel := context.WithCancel(context.Background())
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		run := func() {
			s.maintenanceMu.RLock()
			defer s.maintenanceMu.RUnlock()
			s.chatAccessMu.Lock()
			defer s.chatAccessMu.Unlock()
			if err := s.db.pruneChatTokens(time.Now()); err != nil {
				log.Printf("chat credential cleanup failed: %v", err)
			}
			s.reconcileChat()
		}
		run()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return func() {
		cancel()
		wait.Wait()
		if s.chat != nil {
			s.chat.reset(true)
		}
	}
}
