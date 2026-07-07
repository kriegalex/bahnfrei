// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// testAuthParams is a cheap argon2id cost so the suite runs fast; only
// DefaultPasswordParams ships in production (auth.go, main.go wiring).
var testAuthParams = PasswordParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func newTestAuth(t *testing.T) *AuthService {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sessions := NewSessionManager(time.Hour)
	return NewAuthService(s.DB(), sessions, testAuthParams)
}

func TestBootstrapAndLogin(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()

	acct, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if acct.Role != string(RoleInstanceAdmin) {
		t.Errorf("bootstrap role = %q, want %q", acct.Role, RoleInstanceAdmin)
	}

	sess, err := auth.Login(ctx, "admin", "s3cret-passphrase")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sess.Role != RoleInstanceAdmin || sess.AccountID != acct.ID {
		t.Errorf("session = %+v, want AccountID=%q Role=%q", sess, acct.ID, RoleInstanceAdmin)
	}
}

func TestBootstrapRefusedWhenAccountsExist(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()

	if _, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Bootstrap(ctx, "admin2", "Administrator 2", "s3cret-passphrase-2"); err == nil {
		t.Error("second Bootstrap should be refused once an account exists")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()

	if _, err := auth.Bootstrap(ctx, "admin", "Administrator", "correct-password"); err != nil {
		t.Fatal(err)
	}

	if _, err := auth.Login(ctx, "admin", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login(wrong password) error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := auth.Login(ctx, "nobody", "anything"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login(unknown user) error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginUpgradesWeakHash(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Seed an account with a deliberately weaker hash than the service's
	// configured params, simulating a pre-upgrade stored credential.
	weak := PasswordParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	hash, err := HashPassword("my-password", weak)
	if err != nil {
		t.Fatal(err)
	}
	acct, err := store.CreateAccount(ctx, s.DB(), store.Account{
		Username: "eve", DisplayName: "Eve", PasswordHash: hash, Role: string(RoleFieldOfficial),
	})
	if err != nil {
		t.Fatal(err)
	}

	strong := PasswordParams{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	auth := NewAuthService(s.DB(), NewSessionManager(time.Hour), strong)

	if _, err := auth.Login(ctx, "eve", "my-password"); err != nil {
		t.Fatalf("Login: %v", err)
	}

	got, err := store.GetAccountByID(ctx, s.DB(), acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash == hash {
		t.Error("stored hash was not upgraded on successful login with stronger params configured")
	}
	if NeedsRehash(got.PasswordHash, strong) {
		t.Error("upgraded hash should satisfy the strong params")
	}
	// The upgraded hash must still verify the original password.
	if err := VerifyPassword("my-password", got.PasswordHash); err != nil {
		t.Errorf("VerifyPassword after upgrade: %v", err)
	}
}

func TestCreateAccountRequiresCapability(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()

	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	nonAdminSession := Session{AccountID: "someone-else", Role: RoleFieldOfficial}

	if _, err := auth.CreateAccount(ctx, nonAdminSession, CreateAccountRequest{
		Username: "newuser", DisplayName: "New User", Password: "p4ssword-here", Role: RoleEntrySubmitter,
	}); err == nil {
		t.Error("CreateAccount by a non-admin session should be forbidden")
	}

	created, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "newuser", DisplayName: "New User", Password: "p4ssword-here", Role: RoleEntrySubmitter,
	})
	if err != nil {
		t.Fatalf("CreateAccount by admin: %v", err)
	}
	if created.Role != string(RoleEntrySubmitter) {
		t.Errorf("created.Role = %q, want %q", created.Role, RoleEntrySubmitter)
	}

	// The privileged action must land in the audit trail (SYS-046/091).
	trail, err := storeAuditTrail(ctx, auth, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 1 || trail[0].Action != "account.create" {
		t.Errorf("audit trail for created account = %+v, want one account.create entry", trail)
	}
}

func TestCreateAccountRejectsDuplicateUsername(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}

	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "dupe", DisplayName: "First", Password: "p4ssword-here", Role: RoleEntrySubmitter,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "dupe", DisplayName: "Second", Password: "p4ssword-here-2", Role: RoleEntrySubmitter,
	}); !errors.Is(err, store.ErrDuplicateUsername) {
		t.Errorf("duplicate username error = %v, want store.ErrDuplicateUsername", err)
	}
}

func TestCreateAccountRejectsInvalidRole(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "x", DisplayName: "X", Password: "p4ssword-here", Role: Role("not-a-role"),
	}); err == nil {
		t.Error("CreateAccount with an invalid role should be rejected")
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	if _, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.Login(ctx, "admin", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	auth.Logout(sess.Token)
	auth.Logout(sess.Token) // second call must not panic or error

	if _, err := auth.CurrentSession(sess.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("CurrentSession after logout = %v, want ErrSessionNotFound", err)
	}
}

// storeAuditTrail is a small test-only helper reaching into the store to
// confirm CreateAccount recorded the privileged action (SYS-046).
func storeAuditTrail(ctx context.Context, auth *AuthService, accountID string) ([]store.AuditEntry, error) {
	return store.AuditTrail(ctx, auth.db, "account", accountID)
}

// TestNeedsBootstrap covers the first-run detection behind the /setup flow
// (UC-001 #1): true on an empty instance, false once any account exists.
func TestNeedsBootstrap(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()

	needs, err := auth.NeedsBootstrap(ctx)
	if err != nil {
		t.Fatalf("NeedsBootstrap: %v", err)
	}
	if !needs {
		t.Error("fresh instance should need bootstrap")
	}

	if _, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	needs, err = auth.NeedsBootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Error("bootstrapped instance must not offer setup again")
	}
}
