// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAndGetAccount(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	acct, err := CreateAccount(ctx, s.DB(), Account{
		Username:     "alice",
		DisplayName:  "Alice Athlete",
		PasswordHash: "hash-placeholder",
		Role:         "meet_organizer",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acct.ID == "" {
		t.Error("CreateAccount did not assign an ID")
	}
	if acct.Version != 1 {
		t.Errorf("Version = %d, want 1", acct.Version)
	}

	byName, err := GetAccountByUsername(ctx, s.DB(), "alice")
	if err != nil {
		t.Fatalf("GetAccountByUsername: %v", err)
	}
	if byName != acct {
		t.Errorf("GetAccountByUsername = %+v, want %+v", byName, acct)
	}

	byID, err := GetAccountByID(ctx, s.DB(), acct.ID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if byID != acct {
		t.Errorf("GetAccountByID = %+v, want %+v", byID, acct)
	}
}

func TestCreateAccountDuplicateUsername(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	if _, err := CreateAccount(ctx, s.DB(), Account{Username: "bob", DisplayName: "Bob", PasswordHash: "h", Role: "public"}); err != nil {
		t.Fatal(err)
	}
	_, err := CreateAccount(ctx, s.DB(), Account{Username: "bob", DisplayName: "Bob Two", PasswordHash: "h2", Role: "public"})
	if !errors.Is(err, ErrDuplicateUsername) {
		t.Errorf("second CreateAccount error = %v, want ErrDuplicateUsername", err)
	}
}

func TestGetAccountNotFound(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	if _, err := GetAccountByUsername(ctx, s.DB(), "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAccountByUsername(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := GetAccountByID(ctx, s.DB(), "nonexistent-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAccountByID(missing) error = %v, want ErrNotFound", err)
	}
}

func TestUpdateAccountPasswordHash(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	acct, err := CreateAccount(ctx, s.DB(), Account{Username: "carol", DisplayName: "Carol", PasswordHash: "old-hash", Role: "public"})
	if err != nil {
		t.Fatal(err)
	}

	newVersion, err := UpdateAccountPasswordHash(ctx, s.DB(), acct.ID, acct.Version, "new-hash")
	if err != nil {
		t.Fatalf("UpdateAccountPasswordHash: %v", err)
	}
	if newVersion != acct.Version+1 {
		t.Errorf("newVersion = %d, want %d", newVersion, acct.Version+1)
	}

	got, err := GetAccountByID(ctx, s.DB(), acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash != "new-hash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "new-hash")
	}

	// Stale version must be rejected (optimistic concurrency, ADR-004 §2).
	if _, err := UpdateAccountPasswordHash(ctx, s.DB(), acct.ID, acct.Version, "stale-hash"); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale update error = %v, want ErrVersionConflict", err)
	}
}

func TestCountAccounts(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	n, err := CountAccounts(ctx, s.DB())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("CountAccounts on fresh store = %d, want 0", n)
	}

	if _, err := CreateAccount(ctx, s.DB(), Account{Username: "dave", DisplayName: "Dave", PasswordHash: "h", Role: "public"}); err != nil {
		t.Fatal(err)
	}
	n, err = CountAccounts(ctx, s.DB())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("CountAccounts after one insert = %d, want 1", n)
	}
}
