// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
)

// TestCheckoutLifecycle covers the SYS-086 checkout state machine at the
// storage layer: create at generation 1, reassign bumps the generation,
// start-list revision and release are recorded, and a stale version guard
// surfaces a conflict rather than a lost update.
func TestCheckoutLifecycle(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, _ := ukcFixture(t, s)

	c, err := CreateCheckout(ctx, s.DB(), unitID, "acctA", "tablet-A", "tok1", 1, 0)
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}
	if c.Generation != 1 || !c.Active || c.AccountID != "acctA" {
		t.Fatalf("created checkout = %+v", c)
	}

	// Reassign bumps generation and moves the holder.
	c2, err := ReassignCheckout(ctx, s.DB(), unitID, "acctB", "tablet-B", "tok2", c.Version)
	if err != nil {
		t.Fatalf("ReassignCheckout: %v", err)
	}
	if c2.Generation != 2 || c2.AccountID != "acctB" || c2.Token != "tok2" {
		t.Fatalf("reassigned checkout = %+v", c2)
	}

	// A stale version guard is a conflict, not a silent overwrite.
	if _, err := ReassignCheckout(ctx, s.DB(), unitID, "acctC", "tablet-C", "tok3", c.Version); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale reassign err = %v, want ErrVersionConflict", err)
	}

	// Start-list revision bumps only the start-list version.
	c3, err := BumpStartListVersion(ctx, s.DB(), unitID, c2.Version)
	if err != nil {
		t.Fatalf("BumpStartListVersion: %v", err)
	}
	if c3.StartListVersion != 1 || c3.Generation != 2 {
		t.Fatalf("after start-list bump = %+v", c3)
	}

	// Release deactivates.
	c4, err := ReleaseCheckout(ctx, s.DB(), unitID, c3.Version)
	if err != nil {
		t.Fatalf("ReleaseCheckout: %v", err)
	}
	if c4.Active {
		t.Errorf("released checkout still active = %+v", c4)
	}

	if _, err := GetCheckout(ctx, s.DB(), "no-such-unit"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCheckout(missing) err = %v, want ErrNotFound", err)
	}
}

// TestCaptureOpDedupeLedger proves the exactly-once ledger primitive: a
// recorded op id is found on lookup, and re-recording it is a no-op (the
// first decision stands) — the SYS-085 idempotency backbone.
func TestCaptureOpDedupeLedger(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, _ := ukcFixture(t, s)

	if _, _, _, found, err := GetCaptureOp(ctx, s.DB(), "op1"); err != nil || found {
		t.Fatalf("unseen op = (found %v, err %v), want not found", found, err)
	}
	if err := RecordCaptureOp(ctx, s.DB(), "op1", unitID, "applied", ""); err != nil {
		t.Fatalf("RecordCaptureOp: %v", err)
	}
	// Re-recording with a different decision must not overwrite the first.
	if err := RecordCaptureOp(ctx, s.DB(), "op1", unitID, "reconciliation", "stale_checkout"); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	gotUnit, status, reason, found, err := GetCaptureOp(ctx, s.DB(), "op1")
	if err != nil || !found || status != "applied" || reason != "" {
		t.Fatalf("op1 = (%q, %q, found %v, err %v), want applied/empty", status, reason, found, err)
	}
	// The lookup reports which unit the op was recorded for, so callers can
	// reject a cross-unit op-id reuse instead of acking a false duplicate
	// (SYS-086 never-silent-discard).
	if gotUnit != unitID {
		t.Fatalf("op1 unit = %q, want %q", gotUnit, unitID)
	}
}

// TestReconciliationItemsRoundTrip covers create → list (joined to discipline)
// → resolve → excluded from the pending list (UC-034 #4/#5 storage).
func TestReconciliationItemsRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meetID, unitID, athleteID := ukcFixture(t, s)

	it, err := CreateReconciliationItem(ctx, s.DB(), ReconciliationItem{
		OpID: "op9", UnitID: unitID, AthleteID: athleteID, Reason: "stale_checkout",
		Payload: `{"seq":1,"mark":"3.42"}`, CapturedBy: "acctA", DeviceLabel: "tablet-A",
		Generation: 1, StartListVersion: 0,
	})
	if err != nil {
		t.Fatalf("CreateReconciliationItem: %v", err)
	}

	rows, err := ListPendingReconciliation(ctx, s.DB(), meetID)
	if err != nil {
		t.Fatalf("ListPendingReconciliation: %v", err)
	}
	if len(rows) != 1 || rows[0].DisciplineCode != "60m" || rows[0].AthleteID != athleteID {
		t.Fatalf("pending rows = %+v, want one 60m item", rows)
	}

	if err := SetReconciliationStatus(ctx, s.DB(), it.ID, "discarded", "office1"); err != nil {
		t.Fatalf("SetReconciliationStatus: %v", err)
	}
	// Resolving twice fails (not pending anymore).
	if err := SetReconciliationStatus(ctx, s.DB(), it.ID, "applied", "office1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("double resolve err = %v, want ErrNotFound", err)
	}
	rows, err = ListPendingReconciliation(ctx, s.DB(), meetID)
	if err != nil {
		t.Fatalf("ListPendingReconciliation: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("pending rows after resolve = %d, want 0", len(rows))
	}

	got, err := GetReconciliationItem(ctx, s.DB(), it.ID)
	if err != nil || got.Status != "discarded" || got.ResolvedBy != "office1" || got.ResolvedAt == nil {
		t.Fatalf("resolved item = %+v (err %v)", got, err)
	}
}
