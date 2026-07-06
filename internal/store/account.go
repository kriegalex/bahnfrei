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
	Version      int64
}

// ErrDuplicateUsername means the username is already taken.
var ErrDuplicateUsername = errors.New("username already exists")

// CreateAccount inserts a new account with a fresh ID and version 1.
func CreateAccount(ctx context.Context, db DBTX, a Account) (Account, error) {
	a.ID = NewID()
	a.Version = 1
	_, err := db.ExecContext(ctx, `INSERT INTO accounts
		(id, username, display_name, password_hash, role, version)
		VALUES (?, ?, ?, ?, ?, ?)`,
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
		password_hash, role, version FROM accounts WHERE username = ?`, username))
}

// GetAccountByID looks up an account by its primary key.
func GetAccountByID(ctx context.Context, db DBTX, id string) (Account, error) {
	return scanAccount(db.QueryRowContext(ctx, `SELECT id, username, display_name,
		password_hash, role, version FROM accounts WHERE id = ?`, id))
}

func scanAccount(row *sql.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Username, &a.DisplayName, &a.PasswordHash, &a.Role, &a.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Account{}, ErrNotFound
	case err != nil:
		return Account{}, err
	}
	return a, nil
}

// UpdateAccountPasswordHash rewrites the stored hash (upgrade-on-verify,
// SYS-091) using optimistic concurrency so a concurrent password change
// cannot be silently clobbered.
func UpdateAccountPasswordHash(ctx context.Context, db DBTX, id string, expectedVersion int64, newHash string) (int64, error) {
	return OptimisticUpdate(ctx, db, "accounts", id, expectedVersion,
		Set{Column: "password_hash", Value: newHash})
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
