// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Account is one local operator record (SYS-090/091). PasswordHash holds the
// adaptive-hash encoding produced by internal/app — store never interprets
// or validates its contents, only persists it.
type Account struct {
	ID           string
	Username     string
	DisplayName  string
	PasswordHash string
	Role         string
	// Enabled gates login (SYS-091: an office/admin action can disable an
	// account without deleting its audit history, TASK-013). Always true
	// for a freshly created account; CreateAccount ignores any caller-set
	// value on this field for that reason.
	Enabled bool
	// MustChangePassword marks a temporary password an instance admin just
	// set via ResetAccountPassword (TASK-053, SYS-091): the next login is
	// forced through a change-password step (web layer) before reaching
	// anything else. Always false for a freshly created account.
	MustChangePassword bool
	Version            int64
}

// ErrDuplicateUsername means the username is already taken.
var ErrDuplicateUsername = errors.New("username already exists")

// CreateAccount inserts a new account with a fresh ID and version 1. New
// accounts always start enabled.
func CreateAccount(ctx context.Context, db DBTX, a Account) (Account, error) {
	a.ID = NewID()
	a.Version = 1
	a.Enabled = true
	a.MustChangePassword = false
	_, err := db.ExecContext(ctx, `INSERT INTO accounts
		(id, username, display_name, password_hash, role, enabled, must_change_password, version)
		VALUES (?, ?, ?, ?, ?, 1, 0, ?)`,
		a.ID, a.Username, a.DisplayName, a.PasswordHash, a.Role, a.Version)
	if err != nil {
		if isUniqueConstraint(err) {
			return Account{}, fmt.Errorf("create account %q: %w", a.Username, ErrDuplicateUsername)
		}
		return Account{}, fmt.Errorf("create account %q: %w", a.Username, err)
	}
	return a, nil
}

// GetAccountByUsername looks up an account by its unique username.
func GetAccountByUsername(ctx context.Context, db DBTX, username string) (Account, error) {
	return scanAccount(db.QueryRowContext(ctx, `SELECT id, username, display_name,
		password_hash, role, enabled, must_change_password, version FROM accounts WHERE username = ?`, username))
}

// GetAccountByID looks up an account by its primary key.
func GetAccountByID(ctx context.Context, db DBTX, id string) (Account, error) {
	return scanAccount(db.QueryRowContext(ctx, `SELECT id, username, display_name,
		password_hash, role, enabled, must_change_password, version FROM accounts WHERE id = ?`, id))
}

// ListAccounts returns every account ordered by username (TASK-013 account
// administration view, SYS-090).
func ListAccounts(ctx context.Context, db DBTX) ([]Account, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, username, display_name,
		password_hash, role, enabled, must_change_password, version FROM accounts ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		var enabled, mustChange int
		if err := rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.PasswordHash, &a.Role, &enabled, &mustChange, &a.Version); err != nil {
			return nil, err
		}
		a.Enabled = enabled != 0
		a.MustChangePassword = mustChange != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanAccount(row *sql.Row) (Account, error) {
	var a Account
	var enabled, mustChange int
	err := row.Scan(&a.ID, &a.Username, &a.DisplayName, &a.PasswordHash, &a.Role, &enabled, &mustChange, &a.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Account{}, ErrNotFound
	case err != nil:
		return Account{}, err
	}
	a.Enabled = enabled != 0
	a.MustChangePassword = mustChange != 0
	return a, nil
}

// UpdateAccountPasswordHash rewrites the stored hash (upgrade-on-verify,
// SYS-091) using optimistic concurrency so a concurrent password change
// cannot be silently clobbered.
func UpdateAccountPasswordHash(ctx context.Context, db DBTX, id string, expectedVersion int64, newHash string) (int64, error) {
	return OptimisticUpdate(ctx, db, "accounts", id, expectedVersion,
		Set{Column: "password_hash", Value: newHash})
}

// ResetAccountPassword rewrites an account's stored hash and marks it
// must-change-password (TASK-053, DEC-030, SYS-090/091): an instance
// admin's one-time temporary password for a locked-out account. The
// temporary password only ever grants access to the forced change-password
// step — see CompletePasswordChange.
func ResetAccountPassword(ctx context.Context, db DBTX, id string, newHash string, expectedVersion int64) (int64, error) {
	return OptimisticUpdate(ctx, db, "accounts", id, expectedVersion,
		Set{Column: "password_hash", Value: newHash},
		Set{Column: "must_change_password", Value: 1})
}

// CompletePasswordChange rewrites an account's stored hash and clears
// must_change_password (TASK-053, SYS-091): called when an operator
// completes the forced change-password step after an admin-issued reset.
func CompletePasswordChange(ctx context.Context, db DBTX, id string, newHash string, expectedVersion int64) (int64, error) {
	return OptimisticUpdate(ctx, db, "accounts", id, expectedVersion,
		Set{Column: "password_hash", Value: newHash},
		Set{Column: "must_change_password", Value: 0})
}

// SetAccountEnabled flips an account's enabled flag (TASK-013 disable/enable
// flow, SYS-091) using optimistic concurrency.
func SetAccountEnabled(ctx context.Context, db DBTX, id string, enabled bool, expectedVersion int64) (int64, error) {
	v := 0
	if enabled {
		v = 1
	}
	return OptimisticUpdate(ctx, db, "accounts", id, expectedVersion,
		Set{Column: "enabled", Value: v})
}

// SetAccountRole rewrites an account's instance-wide role (TASK-013 role
// assignment, SYS-090) using optimistic concurrency.
func SetAccountRole(ctx context.Context, db DBTX, id, role string, expectedVersion int64) (int64, error) {
	return OptimisticUpdate(ctx, db, "accounts", id, expectedVersion,
		Set{Column: "role", Value: role})
}

// CountAccounts returns the number of provisioned accounts (used to decide
// whether first-run bootstrap of an instance-admin account is needed).
func CountAccounts(ctx context.Context, db DBTX) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&n)
	return n, err
}

func isUniqueConstraint(err error) bool {
	// modernc.org/sqlite reports constraint violations as *sqlite.Error with
	// a message containing "UNIQUE constraint failed"; matching on the
	// message avoids an import-only dependency on the driver's error type.
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
