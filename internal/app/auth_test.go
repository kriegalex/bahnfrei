// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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

// TestListAccountsRequiresCapability covers TASK-013's account-admin listing
// view (SYS-090): a non-admin session is refused, and an instance-admin sees
// every provisioned account, including itself.
func TestListAccountsRequiresCapability(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	}); err != nil {
		t.Fatal(err)
	}

	nonAdmin := Session{AccountID: "someone-else", Role: RoleFieldOfficial}
	var forbidden ErrForbidden
	if _, err := auth.ListAccounts(ctx, nonAdmin); !errors.As(err, &forbidden) {
		t.Errorf("ListAccounts by a non-admin = %v, want ErrForbidden", err)
	}

	accounts, err := auth.ListAccounts(ctx, adminSession)
	if err != nil {
		t.Fatalf("ListAccounts by admin: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("ListAccounts = %d accounts, want 2", len(accounts))
	}
}

// TestSetAccountEnabledRequiresCapability covers SYS-090 least privilege on
// the enable/disable action.
func TestSetAccountEnabledRequiresCapability(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	target, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	})
	if err != nil {
		t.Fatal(err)
	}

	nonAdmin := Session{AccountID: "someone-else", Role: RoleFieldOfficial}
	var forbidden ErrForbidden
	if _, err := auth.SetAccountEnabled(ctx, nonAdmin, target.ID, false, "test"); !errors.As(err, &forbidden) {
		t.Errorf("SetAccountEnabled by a non-admin = %v, want ErrForbidden", err)
	}
}

// TestSetAccountEnabledUnknownAccount covers the not-found lookup path: an
// accountID that does not exist is refused, not silently ignored.
func TestSetAccountEnabledUnknownAccount(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	if _, err := auth.SetAccountEnabled(ctx, adminSession, "does-not-exist", false, "test"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetAccountEnabled on unknown account = %v, want store.ErrNotFound", err)
	}
}

// TestSetAccountEnabledDisableRevokesSessionAndAudits covers the allow path:
// disabling a non-admin account flips Enabled, revokes its live session
// immediately (not merely at next login), and lands an account.disable audit
// row; re-enabling lands account.enable.
func TestSetAccountEnabledDisableRevokesSessionAndAudits(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	}); err != nil {
		t.Fatal(err)
	}

	sess, err := auth.Login(ctx, "office1", "p4ssword-here")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := auth.CurrentSession(sess.Token); err != nil {
		t.Fatalf("live session before disable: %v", err)
	}

	disabled, err := auth.SetAccountEnabled(ctx, adminSession, sess.AccountID, false, "policy violation")
	if err != nil {
		t.Fatalf("SetAccountEnabled(disable): %v", err)
	}
	if disabled.Enabled {
		t.Error("Enabled = true, want false")
	}
	if _, err := auth.CurrentSession(sess.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("CurrentSession after disable = %v, want ErrSessionNotFound (immediate revocation)", err)
	}
	if _, err := auth.Login(ctx, "office1", "p4ssword-here"); !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("Login on a disabled account = %v, want ErrAccountDisabled", err)
	}

	trail, err := storeAuditTrail(ctx, auth, sess.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	foundDisable := false
	for _, e := range trail {
		if e.Action == "account.disable" {
			foundDisable = true
			if e.Reason != "policy violation" {
				t.Errorf("disable audit Reason = %q, want %q", e.Reason, "policy violation")
			}
		}
	}
	if !foundDisable {
		t.Errorf("audit trail = %+v, want an account.disable entry", trail)
	}

	enabled, err := auth.SetAccountEnabled(ctx, adminSession, sess.AccountID, true, "restored")
	if err != nil {
		t.Fatalf("SetAccountEnabled(enable): %v", err)
	}
	if !enabled.Enabled {
		t.Error("Enabled = false, want true after re-enabling")
	}
	if _, err := auth.Login(ctx, "office1", "p4ssword-here"); err != nil {
		t.Errorf("Login after re-enabling: %v", err)
	}
}

// TestSetAccountEnabledRefusesLastEnabledAdminLockout covers the SYS-090
// self-lockout guard: disabling the sole enabled instance-admin is refused,
// but the same call succeeds once a second enabled admin exists.
func TestSetAccountEnabledRefusesLastEnabledAdminLockout(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}

	if _, err := auth.SetAccountEnabled(ctx, adminSession, admin.ID, false, "oops"); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Errorf("disabling the sole enabled admin = %v, want ErrLastEnabledAdmin", err)
	}

	admin2, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "admin2", DisplayName: "Second Admin", Password: "p4ssword-here", Role: RoleInstanceAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.SetAccountEnabled(ctx, adminSession, admin.ID, false, "handing off"); err != nil {
		t.Fatalf("disabling an admin while another enabled admin exists: %v", err)
	}

	// Now admin2 is the sole enabled admin: disabling it is refused too.
	admin2Session := Session{AccountID: admin2.ID, Role: RoleInstanceAdmin}
	if _, err := auth.SetAccountEnabled(ctx, admin2Session, admin2.ID, false, "oops again"); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Errorf("disabling the new sole enabled admin = %v, want ErrLastEnabledAdmin", err)
	}
}

// TestChangeAccountRoleRequiresCapability covers SYS-090 least privilege on
// the role-reassignment action.
func TestChangeAccountRoleRequiresCapability(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	target, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	})
	if err != nil {
		t.Fatal(err)
	}

	nonAdmin := Session{AccountID: "someone-else", Role: RoleFieldOfficial}
	var forbidden ErrForbidden
	if _, err := auth.ChangeAccountRole(ctx, nonAdmin, target.ID, RoleMeetOrganizer, "test"); !errors.As(err, &forbidden) {
		t.Errorf("ChangeAccountRole by a non-admin = %v, want ErrForbidden", err)
	}
}

// TestChangeAccountRoleRejectsInvalidRole covers the invalid-enum-value
// guard: an unrecognized role string is refused before touching storage.
func TestChangeAccountRoleRejectsInvalidRole(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	target, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	})
	if err != nil {
		t.Fatal(err)
	}

	var invalidRole ErrInvalidRole
	if _, err := auth.ChangeAccountRole(ctx, adminSession, target.ID, Role("not-a-role"), "test"); !errors.As(err, &invalidRole) {
		t.Errorf("ChangeAccountRole with an invalid role = %v, want ErrInvalidRole", err)
	}
}

// TestChangeAccountRoleUnknownAccount covers the not-found lookup path.
func TestChangeAccountRoleUnknownAccount(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	if _, err := auth.ChangeAccountRole(ctx, adminSession, "does-not-exist", RoleMeetOrganizer, "test"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ChangeAccountRole on unknown account = %v, want store.ErrNotFound", err)
	}
}

// TestChangeAccountRoleRefusesLastEnabledAdminLockout covers the SYS-090
// self-lockout guard on demotion: demoting the sole enabled instance-admin
// away from RoleInstanceAdmin is refused, but succeeds once a second enabled
// admin exists to keep the instance administrable.
func TestChangeAccountRoleRefusesLastEnabledAdminLockout(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}

	if _, err := auth.ChangeAccountRole(ctx, adminSession, admin.ID, RoleMeetOrganizer, "oops"); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Errorf("demoting the sole enabled admin = %v, want ErrLastEnabledAdmin", err)
	}

	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "admin2", DisplayName: "Second Admin", Password: "p4ssword-here", Role: RoleInstanceAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := auth.ChangeAccountRole(ctx, adminSession, admin.ID, RoleMeetOrganizer, "handing off")
	if err != nil {
		t.Fatalf("demoting an admin while another enabled admin exists: %v", err)
	}
	if changed.Role != string(RoleMeetOrganizer) {
		t.Errorf("Role = %q, want %q", changed.Role, RoleMeetOrganizer)
	}

	// Re-promoting is not itself a lockout scenario, but changing a
	// non-admin account's role is unaffected by the guard.
	if _, err := auth.ChangeAccountRole(ctx, adminSession, admin.ID, RoleCompetitionOffice, "reassigned again"); err != nil {
		t.Errorf("changing a non-admin's role: %v", err)
	}
}

// TestChangeAccountRoleAudits covers the audit trail's before/after role
// documentation (SYS-046/091).
func TestChangeAccountRoleAudits(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}
	target, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := auth.ChangeAccountRole(ctx, adminSession, target.ID, RoleFieldOfficial, "reassignment"); err != nil {
		t.Fatalf("ChangeAccountRole: %v", err)
	}
	trail, err := storeAuditTrail(ctx, auth, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range trail {
		if e.Action != "account.role_change" {
			continue
		}
		found = true
		if !strings.Contains(e.Before, string(RoleCompetitionOffice)) {
			t.Errorf("Before = %q, want it to mention the prior role %q", e.Before, RoleCompetitionOffice)
		}
		if !strings.Contains(e.After, string(RoleFieldOfficial)) {
			t.Errorf("After = %q, want it to mention the new role %q", e.After, RoleFieldOfficial)
		}
		if e.Reason != "reassignment" {
			t.Errorf("Reason = %q, want %q", e.Reason, "reassignment")
		}
	}
	if !found {
		t.Fatal("expected an account.role_change audit row")
	}
}
