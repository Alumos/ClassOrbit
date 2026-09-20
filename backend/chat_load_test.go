package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in end-to-end soak; never touches real rosters or production databases.
// CHAT_LOAD_TEST=1 CHAT_LOAD_DURATION=30m go test ./backend -run TestChatLoad -v -timeout 40m
func TestChatLoad(t *testing.T) {
	if os.Getenv("CHAT_LOAD_TEST") != "1" {
		t.Skip("opt-in real HTTP/SSE load test")
	}
	duration := 30 * time.Second
	if value := os.Getenv("CHAT_LOAD_DURATION"); value != "" {
		var err error
		duration, err = time.ParseDuration(value)
		if err != nil || duration <= 0 {
			t.Fatal("invalid CHAT_LOAD_DURATION")
		}
	}
	scenarios := []struct {
		name                  string
		rooms, students, rate int
		duration              time.Duration
	}{
		{"one-class", 1, 60, 10, duration},
		{"five-classes", 5, 60, 5, 30 * time.Second},
		{"connection-limit", 5, 100, 5, 30 * time.Second},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			runChatLoad(t, scenario.rooms, scenario.students, scenario.rate, scenario.duration)
		})
	}
}
func runChatLoad(t *testing.T, roomCount, studentsPerRoom, rate int, duration time.Duration) {
	s := &server{db: testStore(t), chat: newChatHub()}
	stopScheduler := s.startChatScheduler()
	defer stopScheduler()
	mux := http.NewServeMux()
	s.routes(mux)
	host := httptest.NewServer(s.maintenance(s.requireTeacher(mux)))
	defer host.Close()
	transport := &http.Transport{MaxIdleConns: 1024, MaxIdleConnsPerHost: 1024, MaxConnsPerHost: 1200}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type actor struct {
		cookie *http.Cookie
		epoch  string
	}
	actors := make([][]actor, roomCount)
	var readers sync.WaitGroup
	var received atomic.Int64
	var latencyMu sync.Mutex
	// Fixed histogram keeps test instrumentation itself from growing during soak.
	var latencyBuckets [10001]int64
	ready := make(chan bool, roomCount*studentsPerRoom)
	for ri := 0; ri < roomCount; ri++ {
		class, err := s.db.createClass(classInput{Grade: "六", ClassNo: fmt.Sprint(ri + 1)})
		if err != nil {
			t.Fatal(err)
		}
		students := make([]studentRow, studentsPerRoom)
		for i := range students {
			students[i], err = s.db.createStudent(class.ID, studentInput{StudentNo: fmt.Sprint(i + 1), Name: fmt.Sprintf("测试学生%d", i+1)})
			if err != nil {
				t.Fatal(err)
			}
		}
		attendance, err := s.db.createAttendance(attendanceInput{ClassID: class.ID})
		if err != nil {
			t.Fatal(err)
		}
		epoch := roomEpoch(t, s.chat, attendance.ID)
		for _, student := range students {
			token, _ := randomURLToken()
			if _, err := s.db.checkInWithChat(class.ID, student.ID, hashSessionToken(token), "", time.Now()); err != nil {
				t.Fatal(err)
			}
			cookie := &http.Cookie{Name: studentChatCookie, Value: token}
			actors[ri] = append(actors[ri], actor{cookie, epoch})
			readers.Add(1)
			go func(cookie *http.Cookie) {
				defer readers.Done()
				request, _ := http.NewRequestWithContext(ctx, "GET", host.URL+"/api/public/chat/events", nil)
				request.AddCookie(cookie)
				response, err := client.Do(request)
				if err != nil {
					ready <- false
					return
				}
				defer response.Body.Close()
				if response.StatusCode != 200 {
					ready <- false
					return
				}
				scanner := bufio.NewScanner(response.Body)
				scanner.Buffer(make([]byte, 4096), chatQueueBytes)
				first := true
				kind := ""
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "event: ") {
						kind = strings.TrimPrefix(line, "event: ")
					}
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					if first {
						ready <- true
						first = false
					}
					if kind == "message" {
						var event struct {
							Message chatMessage `json:"message"`
						}
						if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
							continue
						}
						var sent int64
						if _, err := fmt.Sscanf(event.Message.Text, "load:%d", &sent); err == nil {
							delay := time.Since(time.Unix(0, sent))
							received.Add(1)
							latencyMu.Lock()
							latencyBuckets[min(10000, max(0, int(delay/(100*time.Microsecond))))]++
							latencyMu.Unlock()
						}
					}
				}
			}(cookie)
		}
	}
	for i := 0; i < roomCount*studentsPerRoom; i++ {
		select {
		case ok := <-ready:
			if !ok {
				t.Fatal("SSE connection failed")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("SSE startup timed out")
		}
	}
	runtime.GC()
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)
	startGoroutines := runtime.NumGoroutine()
	start := time.Now()
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()
	sent := 0
	nextReport := time.Now().Add(time.Minute)
	for time.Since(start) < duration {
		<-ticker.C
		for ri := range actors {
			actor := actors[ri][sent%studentsPerRoom]
			now := time.Now()
			body, _ := json.Marshal(chatMessageInput{Text: fmt.Sprintf("load:%d", now.UnixNano()), ClientMessageID: fmt.Sprintf("load-%d-%016d", ri, sent), Epoch: actor.epoch})
			request, _ := http.NewRequestWithContext(ctx, "POST", host.URL+"/api/public/chat/messages", strings.NewReader(string(body)))
			request.AddCookie(actor.cookie)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", host.URL)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 201 {
				t.Fatalf("send status %d", response.StatusCode)
			}
		}
		sent++
		if time.Now().After(nextReport) {
			runtime.GC()
			var sample runtime.MemStats
			runtime.ReadMemStats(&sample)
			t.Logf("elapsed=%s heap=%d goroutines=%d received=%d", time.Since(start).Round(time.Second), sample.HeapAlloc, runtime.NumGoroutine(), received.Load())
			nextReport = time.Now().Add(time.Minute)
		}
	}
	expected := int64(sent * roomCount * studentsPerRoom)
	deadline := time.Now().Add(5 * time.Second)
	for received.Load() < expected && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	runtime.GC()
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)
	latencyMu.Lock()
	var p95 time.Duration
	var cumulative int64
	target := received.Load() * 95 / 100
	for index, count := range latencyBuckets {
		cumulative += count
		if cumulative >= target {
			p95 = time.Duration(index+1) * 100 * time.Microsecond
			break
		}
	}
	latencyMu.Unlock()
	t.Logf("rooms=%d clients=%d elapsed=%s sent=%d received=%d/%d p95=%s heap_start=%d heap_end=%d goroutines_start=%d goroutines_end=%d", roomCount, roomCount*studentsPerRoom, time.Since(start).Round(time.Millisecond), sent*roomCount, received.Load(), expected, p95, startMem.HeapAlloc, endMem.HeapAlloc, startGoroutines, runtime.NumGoroutine())
	if received.Load() != expected {
		t.Errorf("message delivery count mismatch")
	}
	if p95 > 500*time.Millisecond {
		t.Errorf("delivery p95 exceeds 500ms")
	}
	if roomCount*studentsPerRoom == chatMaxConnections {
		req, _ := http.NewRequestWithContext(ctx, "GET", host.URL+"/api/public/chat/events", nil)
		req.AddCookie(actors[0][0].cookie)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 429 {
			t.Errorf("capacity overflow status=%d", resp.StatusCode)
		}
	}
	cancel()
	s.chat.reset(true)
	readers.Wait()
}
