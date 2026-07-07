// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// ErrInvalidCredentials is returned for both "no such user" and "wrong
// password" so login failures never disclose which half was wrong
// (standard authentication-enumeration mitigation).
var ErrInvalidCredentials = errors.New("invalid username or password")

// AuthService is the use-case service the web layer calls through for
// authentication and account management (architecture.md §3: web never
// touches store directly). It owns password verification/upgrade
// (SYS-091) and authorization checks (SYS-090), and records privileged
// actions in the audit trail (SYS-046, referenced by SYS-091).
type AuthService struct {
	db        *sql.DB
	sessions  *SessionManager
	params    PasswordParams
	dummyHash string // timing-guard hash at the same cost as params, see Login
}

// NewAuthService wires an AuthService to the store's database handle and a
// session manager. params controls the argon2id cost for newly hashed or
// upgraded passwords; pass DefaultPasswordParams outside tests (tests may
// use cheaper params to keep the suite fast).
func NewAuthService(db *sql.DB, sessions *SessionManager, params PasswordParams) *AuthService {
	dummy, err := HashPassword("no-such-account-timing-guard", params)
	if err != nil {
		// params are caller-controlled and validated by HashPassword only
		// for an empty password; any other failure is a caller bug.
		panic(fmt.Sprintf("app: invalid password params: %v", err))
	}
	return &AuthService{db: db, sessions: sessions, params: params, dummyHash: dummy}
}

// Login verifies the username/password against the stored hash and, on
// success, issues a session. A stored hash created with weaker parameters
// than a.params is transparently upgraded in place (upgrade-on-verify).
func (a *AuthService) Login(ctx context.Context, username, password string) (Session, error) {
	acct, err := store.GetAccountByUsername(ctx, a.db, username)
	if errors.Is(err, store.ErrNotFound) {
		// Run a verification against a fixed dummy hash so the two failure
		// paths (unknown user vs wrong password) take comparable time.
		_ = VerifyPassword(password, a.dummyHash)
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, fmt.Errorf("login %q: %w", username, err)
	}
	if err := VerifyPassword(password, acct.PasswordHash); err != nil {
		return Session{}, ErrInvalidCredentials
	}

	role, err := ParseRole(acct.Role)
	if err != nil {
		return Session{}, fmt.Errorf("login %q: %w", username, err)
	}

	if NeedsRehash(acct.PasswordHash, a.params) {
		if newHash, herr := HashPassword(password, a.params); herr == nil {
			// Best-effort upgrade: a lost race (version conflict) just means
			// another request upgraded it first, which is fine — never fail
			// the login over it.
			_, _ = store.UpdateAccountPasswordHash(ctx, a.db, acct.ID, acct.Version, newHash)
		}
	}

	return a.sessions.Create(acct.ID, acct.Username, role)
}

// Logout revokes the session identified by token. Revoking an unknown or
// already-expired token is a no-op (idempotent logout).
func (a *AuthService) Logout(token string) {
	a.sessions.Revoke(token)
}

// CurrentSession resolves a bearer token to its live session.
func (a *AuthService) CurrentSession(token string) (Session, error) {
	return a.sessions.Lookup(token)
}

// CreateAccountRequest is the input to CreateAccount.
type CreateAccountRequest struct {
	Username    string
	DisplayName string
	Password    string
	Role        Role
}

// CreateAccount provisions a new account. actor is the session performing
// the action; the caller must already know actor.Role carries
// CapManageAccounts — CreateAccount enforces it again defensively so the
// use-case is safe to call from anywhere, not just behind one guarded
// route. The action is written to the audit trail (SYS-046) because
// account provisioning is a privileged action (SYS-091).
func (a *AuthService) CreateAccount(ctx context.Context, actor Session, req CreateAccountRequest) (store.Account, error) {
	if err := Authorize(actor.Role, CapManageAccounts); err != nil {
		return store.Account{}, err
	}
	if !req.Role.Valid() {
		return store.Account{}, ErrInvalidRole{Value: string(req.Role)}
	}
	hash, err := HashPassword(req.Password, a.params)
	if err != nil {
		return store.Account{}, fmt.Errorf("create account %q: %w", req.Username, err)
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Account{}, fmt.Errorf("create account %q: %w", req.Username, err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful Commit

	acct, err := store.CreateAccount(ctx, tx, store.Account{
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		Role:         string(req.Role),
	})
	if err != nil {
		return store.Account{}, err
	}

	after, _ := json.Marshal(struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}{acct.Username, acct.DisplayName, acct.Role})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor:      actor.AccountID,
		Action:     "account.create",
		EntityType: "account",
		EntityID:   acct.ID,
		After:      string(after),
	}); err != nil {
		return store.Account{}, fmt.Errorf("audit account create: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return store.Account{}, fmt.Errorf("create account %q: %w", req.Username, err)
	}
	return acct, nil
}

// NeedsBootstrap reports whether no account exists yet, i.e. the instance
// is on its first run and must offer the setup flow (UC-001 #1: the
// quickstart ends with "a running system with an admin account" and no
// hand-edited configuration file).
func (a *AuthService) NeedsBootstrap(ctx context.Context) (bool, error) {
	n, err := store.CountAccounts(ctx, a.db)
	if err != nil {
		return false, fmt.Errorf("needs bootstrap: %w", err)
	}
	return n == 0, nil
}

// Bootstrap provisions the very first account (an instance admin) when no
// accounts exist yet. It is the only way to obtain the first
// RoleInstanceAdmin account: every later CreateAccount call requires an
// existing admin's session. Calling Bootstrap when accounts already exist
// is refused.
func (a *AuthService) Bootstrap(ctx context.Context, username, displayName, password string) (store.Account, error) {
	n, err := store.CountAccounts(ctx, a.db)
	if err != nil {
		return store.Account{}, fmt.Errorf("bootstrap: %w", err)
	}
	if n > 0 {
		return store.Account{}, errors.New("bootstrap refused: accounts already exist")
	}
	hash, err := HashPassword(password, a.params)
	if err != nil {
		return store.Account{}, fmt.Errorf("bootstrap: %w", err)
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Account{}, fmt.Errorf("bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	acct, err := store.CreateAccount(ctx, tx, store.Account{
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		Role:         string(RoleInstanceAdmin),
	})
	if err != nil {
		return store.Account{}, err
	}
	after, _ := json.Marshal(struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}{acct.Username, acct.Role})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor:      acct.ID,
		Action:     "account.bootstrap",
		EntityType: "account",
		EntityID:   acct.ID,
		After:      string(after),
		Reason:     "first-run instance admin bootstrap",
	}); err != nil {
		return store.Account{}, fmt.Errorf("audit bootstrap: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.Account{}, fmt.Errorf("bootstrap: %w", err)
	}
	return acct, nil
}
