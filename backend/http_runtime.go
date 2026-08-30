package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func runHTTP(addr string, handler http.Handler) error {
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      5 * time.Minute, // Database backups and reports can be large.
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	errs := make(chan error, 1)
	go func() { errs <- httpServer.ListenAndServe() }()
	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(ctx)
	}
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	if err := s.db.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "database": false})
		return
	}
	var one int
	if err := s.db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "database": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "database": true})
}

func (s *server) maintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/admin/restore" {
			s.maintenanceMu.Lock()
			defer s.maintenanceMu.Unlock()
		} else {
			s.maintenanceMu.RLock()
			defer s.maintenanceMu.RUnlock()
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https: http:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if requestUsesHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type rateEntry struct {
	windowStart time.Time
	count       int
}

type requestLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
	window  time.Duration
	limit   int
}

func newRequestLimiter(limit int, window time.Duration) *requestLimiter {
	return &requestLimiter{entries: map[string]rateEntry{}, window: window, limit: limit}
}

func (l *requestLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window {
		entry = rateEntry{windowStart: now}
	}
	entry.count++
	l.entries[key] = entry
	if len(l.entries) > 5000 {
		for itemKey, item := range l.entries {
			if now.Sub(item.windowStart) > 2*l.window {
				delete(l.entries, itemKey)
			}
		}
	}
	return entry.count <= l.limit
}

func rateLimitPublic(next http.Handler) http.Handler {
	login := newRequestLimiter(10, time.Minute)
	// A whole classroom may share one NAT address, so leave enough burst room
	// for concentrated check-ins while still bounding abusive retries.
	checkin := newRequestLimiter(300, time.Minute)
	integration := newRequestLimiter(120, time.Minute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var limiter *requestLimiter
		switch {
		case r.Method == http.MethodPost && (r.URL.Path == "/api/auth" || r.URL.Path == "/api/setup"):
			limiter = login
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/check-in":
			limiter = checkin
		case r.URL.Path == "/api/integration/classes":
			limiter = integration
		}
		if limiter != nil && !limiter.allow(clientIP(r)+"|"+r.URL.Path, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, apiError{Error: "请求过于频繁，请稍后再试"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if env("TRUST_PROXY_HEADERS", "false") == "true" {
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); value != "" {
			return value
		}
		if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); value != "" {
			return value
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
