// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Login anti-automation budget (OWASP ASVS L2 V2.2.1 / V11.1.2: rate-limit
// authentication to resist credential brute-forcing). argon2id already makes
// each guess expensive, but ASVS L2 requires an explicit control on top —
// this is it. Only consecutive FAILED attempts count; a successful login
// clears the counter, so legitimate operators (and the test/E2E suites,
// which log in successfully) are never throttled.
const (
	loginFailLimit  = 10
	loginFailWindow = 15 * time.Minute
)

// loginRateLimiter throttles repeated failed login attempts per source key
// (client IP). Safe for concurrent use.
type loginRateLimiter struct {
	mu     sync.Mutex
	fails  map[string]*failWindow
	limit  int
	window time.Duration
	now    func() time.Time
}

type failWindow struct {
	count int
	start time.Time
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		fails:  make(map[string]*failWindow),
		limit:  limit,
		window: window,
		now:    time.Now,
	}
}

// blocked reports whether key has spent its failure budget within the current
// window. Windows are rolling per key and reset lazily on the first call
// after expiry, so idle keys do not accumulate memory indefinitely.
func (l *loginRateLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.fails[key]
	if !ok {
		return false
	}
	if l.now().Sub(w.start) >= l.window {
		delete(l.fails, key)
		return false
	}
	return w.count >= l.limit
}

// fail records one failed attempt for key, starting a fresh window if the
// previous one has expired.
func (l *loginRateLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w, ok := l.fails[key]
	if !ok || now.Sub(w.start) >= l.window {
		l.fails[key] = &failWindow{count: 1, start: now}
		return
	}
	w.count++
}

// reset clears any failure counter for key (called on a successful login).
func (l *loginRateLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.fails, key)
	l.mu.Unlock()
}

// clientIP extracts the peer IP from r.RemoteAddr. It deliberately ignores
// X-Forwarded-For and similar client-supplied headers: those are trivially
// spoofable, and the venue/hub deployment terminates connections directly
// (no trusted reverse proxy in the ADR-003 single-binary model), so the
// transport peer address is the only trustworthy key.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
