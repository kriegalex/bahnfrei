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

// ErrAccountDisabled means the credentials were correct but the account has
// been disabled (TASK-013, SYS-091). Distinguishing it from
// ErrInvalidCredentials is safe here because it only ever surfaces after a
// successful password check, so it discloses nothing to a guesser.
var ErrAccountDisabled = errors.New("account is disabled")

// ErrLastEnabledAdmin means the requested change would leave the instance
// with no enabled instance-admin account (a self-lockout guard on top of
// SYS-090's least-privilege model).
var ErrLastEnabledAdmin = errors.New("refused: this would leave no enabled instance-admin account")

// ErrDuplicateUsername aliases the store sentinel for web handlers
// (architecture.md §3: web never imports internal/store directly).
var ErrDuplicateUsername = store.ErrDuplicateUsername

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
	if !acct.Enabled {
		return Session{}, ErrAccountDisabled
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

	return a.sessions.Create(acct.ID, acct.Username, role, acct.MustChangePassword)
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

// ListAccounts returns every provisioned account (TASK-013 account
// administration view, SYS-090), instance-admin only.
func (a *AuthService) ListAccounts(ctx context.Context, actor Session) ([]store.Account, error) {
	if err := Authorize(actor.Role, CapManageAccounts); err != nil {
		return nil, err
	}
	return store.ListAccounts(ctx, a.db)
}

// otherEnabledAdminsExist reports whether an enabled instance-admin account
// other than excludeID exists, guarding SetAccountEnabled/ChangeAccountRole
// against locking the instance out of its own administration.
func otherEnabledAdminsExist(ctx context.Context, db *sql.DB, excludeID string) (bool, error) {
	accounts, err := store.ListAccounts(ctx, db)
	if err != nil {
		return false, err
	}
	for _, acc := range accounts {
		if acc.ID != excludeID && acc.Enabled && acc.Role == string(RoleInstanceAdmin) {
			return true, nil
		}
	}
	return false, nil
}

// SetAccountEnabled enables or disables an account (TASK-013, SYS-091):
// disabling revokes its live sessions immediately, so a session issued
// before the change stops working right away, not merely at next login.
// Disabling the last enabled instance-admin account is refused
// (ErrLastEnabledAdmin) so the instance never loses all administration.
// The action is written to the audit trail (SYS-046).
func (a *AuthService) SetAccountEnabled(ctx context.Context, actor Session, accountID string, enabled bool, reason string) (store.Account, error) {
	if err := Authorize(actor.Role, CapManageAccounts); err != nil {
		return store.Account{}, err
	}
	acct, err := store.GetAccountByID(ctx, a.db, accountID)
	if err != nil {
		return store.Account{}, err
	}
	if !enabled && acct.Role == string(RoleInstanceAdmin) {
		ok, err := otherEnabledAdminsExist(ctx, a.db, accountID)
		if err != nil {
			return store.Account{}, err
		}
		if !ok {
			return store.Account{}, ErrLastEnabledAdmin
		}
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Account{}, fmt.Errorf("set account enabled: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.SetAccountEnabled(ctx, tx, accountID, enabled, acct.Version); err != nil {
		return store.Account{}, err
	}
	action := "account.disable"
	if enabled {
		action = "account.enable"
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: action,
		EntityType: "account", EntityID: accountID, Reason: reason,
	}); err != nil {
		return store.Account{}, fmt.Errorf("audit %s: %w", action, err)
	}
	if err := tx.Commit(); err != nil {
		return store.Account{}, fmt.Errorf("set account enabled: %w", err)
	}
	if !enabled {
		a.sessions.RevokeAccount(accountID)
	}
	return store.GetAccountByID(ctx, a.db, accountID)
}

// ChangeAccountRole reassigns an account's instance-wide role (TASK-013,
// SYS-090). Demoting the last enabled instance-admin account away from
// RoleInstanceAdmin is refused (ErrLastEnabledAdmin). The action is written
// to the audit trail with the before/after role (SYS-046).
func (a *AuthService) ChangeAccountRole(ctx context.Context, actor Session, accountID string, newRole Role, reason string) (store.Account, error) {
	if err := Authorize(actor.Role, CapManageAccounts); err != nil {
		return store.Account{}, err
	}
	if !newRole.Valid() {
		return store.Account{}, ErrInvalidRole{Value: string(newRole)}
	}
	acct, err := store.GetAccountByID(ctx, a.db, accountID)
	if err != nil {
		return store.Account{}, err
	}
	if acct.Role == string(RoleInstanceAdmin) && newRole != RoleInstanceAdmin {
		ok, err := otherEnabledAdminsExist(ctx, a.db, accountID)
		if err != nil {
			return store.Account{}, err
		}
		if !ok {
			return store.Account{}, ErrLastEnabledAdmin
		}
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Account{}, fmt.Errorf("change account role: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.SetAccountRole(ctx, tx, accountID, string(newRole), acct.Version); err != nil {
		return store.Account{}, err
	}
	before, _ := json.Marshal(map[string]string{"role": acct.Role})
	after, _ := json.Marshal(map[string]string{"role": string(newRole)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "account.role_change",
		EntityType: "account", EntityID: accountID,
		Before: string(before), After: string(after), Reason: reason,
	}); err != nil {
		return store.Account{}, fmt.Errorf("audit account.role_change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.Account{}, fmt.Errorf("change account role: %w", err)
	}
	return store.GetAccountByID(ctx, a.db, accountID)
}

// ResetPassword issues an admin-set temporary password for any account
// (TASK-053, DEC-030, SYS-090/091): the offline-venue-friendly answer to a
// meet-morning lockout, needing no email infrastructure. It marks the
// account must-change-password so the temporary password only ever grants
// access to the forced change-password step (ChangePassword), and revokes
// every one of the account's live sessions immediately — a session issued
// on the old password must not keep working past the reset, the same
// posture SetAccountEnabled(false) takes. The action is written to the
// audit trail (SYS-046) like every other account mutation.
func (a *AuthService) ResetPassword(ctx context.Context, actor Session, accountID, newPassword, reason string) (store.Account, error) {
	if err := Authorize(actor.Role, CapManageAccounts); err != nil {
		return store.Account{}, err
	}
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return store.Account{}, err
	}
	acct, err := store.GetAccountByID(ctx, a.db, accountID)
	if err != nil {
		return store.Account{}, err
	}
	hash, err := HashPassword(newPassword, a.params)
	if err != nil {
		return store.Account{}, fmt.Errorf("reset password %q: %w", accountID, err)
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Account{}, fmt.Errorf("reset password %q: %w", accountID, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.ResetAccountPassword(ctx, tx, accountID, hash, acct.Version); err != nil {
		return store.Account{}, err
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "account.password_reset",
		EntityType: "account", EntityID: accountID, Reason: reason,
	}); err != nil {
		return store.Account{}, fmt.Errorf("audit account.password_reset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.Account{}, fmt.Errorf("reset password %q: %w", accountID, err)
	}
	a.sessions.RevokeAccount(accountID)
	return store.GetAccountByID(ctx, a.db, accountID)
}

// ChangePassword completes the forced change-password step (TASK-053,
// SYS-091): actor sets their own new password after an admin-issued reset,
// re-proving the current (temporary) password first — the same
// credential-verification surface as Login, so callers MUST subject it to
// the same brute-force throttle (internal/web/ratelimit.go's loginLimiter)
// rather than assuming the /login route alone covers it. On success it
// clears must-change-password (both on the stored account and, via
// SessionManager.ClearMustChangePassword, on actor's own live session, so
// the forced-change gate stops redirecting immediately, without requiring a
// fresh login) and audits the change like every other account mutation.
// Unlike ResetPassword this needs no CapManageAccounts — an operator may
// always change their own password.
func (a *AuthService) ChangePassword(ctx context.Context, actor Session, currentPassword, newPassword string) (store.Account, error) {
	acct, err := store.GetAccountByID(ctx, a.db, actor.AccountID)
	if err != nil {
		return store.Account{}, err
	}
	if err := VerifyPassword(currentPassword, acct.PasswordHash); err != nil {
		return store.Account{}, ErrInvalidCredentials
	}
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return store.Account{}, err
	}
	hash, err := HashPassword(newPassword, a.params)
	if err != nil {
		return store.Account{}, fmt.Errorf("change password %q: %w", actor.AccountID, err)
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Account{}, fmt.Errorf("change password %q: %w", actor.AccountID, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.CompletePasswordChange(ctx, tx, actor.AccountID, hash, acct.Version); err != nil {
		return store.Account{}, err
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "account.password_change",
		EntityType: "account", EntityID: actor.AccountID,
	}); err != nil {
		return store.Account{}, fmt.Errorf("audit account.password_change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.Account{}, fmt.Errorf("change password %q: %w", actor.AccountID, err)
	}
	a.sessions.ClearMustChangePassword(actor.Token)
	return store.GetAccountByID(ctx, a.db, actor.AccountID)
}
