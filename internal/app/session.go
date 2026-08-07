// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Session is one authenticated browser session (SYS-091: "sessions SHALL
// expire configurably"). Sessions are held in memory only: the venue/hub
// process is the single point of truth for who is logged in right now, and
// a restart requiring re-login is an accepted trade-off for this shell
// (see TASK-005 assumptions) — no durability claim is made for sessions the
// way SYS-081 makes one for committed domain writes.
type Session struct {
	Token     string
	AccountID string
	Username  string
	Role      Role
	CreatedAt time.Time
	ExpiresAt time.Time
	// MustChangePassword mirrors the account's flag at the moment this
	// session was issued (TASK-053, SYS-091): while true, the web layer's
	// forced-change gate redirects every other authenticated route to the
	// change-password step. ChangePassword clears it in place via
	// ClearMustChangePassword so completing the step takes effect
	// immediately, without requiring a fresh login.
	MustChangePassword bool
}

// ErrSessionNotFound means the token is unknown or has expired.
var ErrSessionNotFound = errors.New("session not found or expired")

// Clock abstracts time.Now for deterministic expiry tests.
type Clock func() time.Time

// SessionManager issues, looks up and revokes sessions with a configurable
// TTL. Safe for concurrent use.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]Session
	ttl      time.Duration
	now      Clock
}

// NewSessionManager builds a manager with the given expiry TTL. ttl <= 0 is
// rejected by the caller's configuration validation, not here, so a
// misconfiguration surfaces at startup rather than as sessions that never
// expire.
func NewSessionManager(ttl time.Duration) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]Session),
		ttl:      ttl,
		now:      time.Now,
	}
}

// WithClock overrides the time source (tests only).
func (m *SessionManager) WithClock(now Clock) *SessionManager {
	m.now = now
	return m
}

// Create issues a new session for the given account/role and returns its
// opaque bearer token (32 random bytes, base64url-encoded — 256 bits of
// entropy, unguessable per OWASP session-ID guidance). mustChangePassword
// carries the account's current flag (TASK-053) so a session issued right
// after an admin reset starts out forced into the change-password step.
func (m *SessionManager) Create(accountID, username string, role Role, mustChangePassword bool) (Session, error) {
	tok, err := newSessionToken()
	if err != nil {
		return Session{}, err
	}
	now := m.now()
	s := Session{
		Token:              tok,
		AccountID:          accountID,
		Username:           username,
		Role:               role,
		CreatedAt:          now,
		ExpiresAt:          now.Add(m.ttl),
		MustChangePassword: mustChangePassword,
	}
	m.mu.Lock()
	m.sessions[tok] = s
	m.mu.Unlock()
	return s, nil
}

// Lookup returns the session for token if it exists and has not expired. An
// expired session is evicted as a side effect.
func (m *SessionManager) Lookup(token string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if !m.now().Before(s.ExpiresAt) {
		delete(m.sessions, token)
		return Session{}, ErrSessionNotFound
	}
	return s, nil
}

// Revoke deletes a session (logout). Revoking an unknown token is a no-op.
func (m *SessionManager) Revoke(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// RevokeAccount deletes every live session for accountID: a disabled
// account (TASK-013, SYS-091) must not keep working through a session
// issued before it was disabled.
func (m *SessionManager) RevokeAccount(accountID string) {
	m.mu.Lock()
	for tok, s := range m.sessions {
		if s.AccountID == accountID {
			delete(m.sessions, tok)
		}
	}
	m.mu.Unlock()
}

// ClearMustChangePassword unsets the must-change-password flag on the live
// session identified by token, if any (TASK-053): called once
// ChangePassword has completed, so the forced-change gate stops redirecting
// this session without requiring a fresh login. A no-op for an unknown or
// already-expired token.
func (m *SessionManager) ClearMustChangePassword(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[token]; ok {
		s.MustChangePassword = false
		m.sessions[token] = s
	}
}

// Sweep evicts every expired session and returns how many were removed.
// Intended to be called periodically (e.g. from a ticker in the server
// lifecycle) so long-idle memory does not grow unbounded.
func (m *SessionManager) Sweep() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	n := 0
	for tok, s := range m.sessions {
		if !now.Before(s.ExpiresAt) {
			delete(m.sessions, tok)
			n++
		}
	}
	return n
}

// Count returns the number of live (not necessarily unexpired) sessions
// held in memory; used by tests and health diagnostics.
func (m *SessionManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
