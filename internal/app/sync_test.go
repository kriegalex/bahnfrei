// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// syncFixture creates a UKC meet with a checked-out zone long jump unit and
// two registered athletes, returning the meet, unit and the field official's
// checkout stamp.
func syncFixture(t *testing.T) (*ResultsService, *store.Store, string, string, store.Checkout, string, string) {
	t.Helper()
	meets, results, st := newTestResults(t)
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, meets, rec.ID, "ZoneLJ")
	anna := register(t, results, rec.ID, ParticipantInput{FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101"})
	bea := register(t, results, rec.ID, ParticipantInput{FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102"})
	co, err := results.CheckoutUnit(context.Background(), fieldOfficial, rec.ID, unitID, "tablet-A")
	if err != nil {
		t.Fatalf("CheckoutUnit: %v", err)
	}
	return results, st, rec.ID, unitID, co, anna.AthleteID, bea.AthleteID
}

func batchFrom(co store.Checkout, ops ...ReplayOp) ReplayBatch {
	return ReplayBatch{Token: co.Token, DeviceLabel: "tablet-A", Generation: co.Generation, StartListVersion: co.StartListVersion, Ops: ops}
}

func countUnitAttempts(t *testing.T, st *store.Store, unitID string) int {
	t.Helper()
	all, err := store.ListUnitAttempts(context.Background(), st.DB(), unitID)
	if err != nil {
		t.Fatalf("ListUnitAttempts: %v", err)
	}
	return len(all)
}

// TestUC034_2_IdempotentReplay proves UC-034 #2 / SYS-085: an ordered batch
// applies in order, and repeated flaky reconnects re-sending the same batch
// produce no duplicates — every op is exactly-once.
func TestUC034_2_IdempotentReplay(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, co, anna, bea := syncFixture(t)

	batch := batchFrom(co,
		ReplayOp{OpID: "01OP0000000000000000000001", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"},
		ReplayOp{OpID: "01OP0000000000000000000002", AthleteID: anna, Seq: 2, Kind: domain.AttemptFoul},
		ReplayOp{OpID: "01OP0000000000000000000003", AthleteID: bea, Seq: 1, Kind: domain.AttemptValid, Mark: "3.50"},
	)

	res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batch)
	if err != nil {
		t.Fatalf("ReplayCaptureBatch: %v", err)
	}
	if len(res.Outcomes) != 3 {
		t.Fatalf("outcomes = %d, want 3", len(res.Outcomes))
	}
	for i, o := range res.Outcomes {
		if o.Status != domain.OpApplied {
			t.Errorf("op %d status = %q, want applied", i, o.Status)
		}
		if o.OpID != batch.Ops[i].OpID {
			t.Errorf("outcome %d order = %q, want %q (ordered apply)", i, o.OpID, batch.Ops[i].OpID)
		}
	}
	if n := countUnitAttempts(t, st, unitID); n != 3 {
		t.Fatalf("attempts after first replay = %d, want 3", n)
	}

	// Re-send the identical batch twice more: every op is a duplicate, and the
	// stored attempt count never grows.
	for round := 0; round < 2; round++ {
		res, err = results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batch)
		if err != nil {
			t.Fatalf("re-replay round %d: %v", round, err)
		}
		for i, o := range res.Outcomes {
			if o.Status != domain.OpDuplicate {
				t.Errorf("re-replay op %d status = %q, want duplicate (SYS-085)", i, o.Status)
			}
		}
		if n := countUnitAttempts(t, st, unitID); n != 3 {
			t.Fatalf("attempts after re-replay round %d = %d, want 3 (no duplicates)", round, n)
		}
	}

	// The applied series settled a standing (shared capture path, not a fork).
	view, err := results.UnitCapture(ctx, meetID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	if len(view.Standings) != 2 || view.Standings[0].AthleteID != bea {
		t.Errorf("standings = %+v, want Bea (3.50) first", view.Standings)
	}
}

// TestUC034_4_StartListChangeRoutesToReconciliation proves UC-034 #4 /
// SYS-086: when the office changes the start list of a checked-out unit while
// the device is offline, the device's captures are surfaced for
// reconciliation, never silently applied or discarded (the Web.TEC 2 loss
// mode C2.1 anti-pattern).
func TestUC034_4_StartListChangeRoutesToReconciliation(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, co, anna, _ := syncFixture(t)

	// Office revises the start list after the device checked out.
	if _, err := results.ReviseStartList(ctx, office, meetID, unitID); err != nil {
		t.Fatalf("ReviseStartList: %v", err)
	}

	// The device replays ops stamped with the pre-change start-list version.
	batch := batchFrom(co, ReplayOp{OpID: "01OP0000000000000000000010", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"})
	res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batch)
	if err != nil {
		t.Fatalf("ReplayCaptureBatch: %v", err)
	}
	if got := res.Outcomes[0]; got.Status != domain.OpReconciliation || got.Reason != domain.ReasonStartListChange {
		t.Fatalf("outcome = %+v, want reconciliation/start_list_change", got)
	}
	if n := countUnitAttempts(t, st, unitID); n != 0 {
		t.Errorf("attempts = %d, want 0 (never silently applied)", n)
	}
	groups, err := results.ReconciliationItems(ctx, office, meetID)
	if err != nil {
		t.Fatalf("ReconciliationItems: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Entries) != 1 || groups[0].Entries[0].Reason != domain.ReasonStartListChange {
		t.Fatalf("reconciliation groups = %+v, want one start-list-change item", groups)
	}
	// The routing itself is audited (SYS-046): the never-discard guarantee is
	// traceable.
	trail, err := store.AuditTrail(ctx, st.DB(), "reconciliation", groups[0].Entries[0].ItemID)
	if err != nil || len(trail) == 0 {
		t.Fatalf("audit trail for reconciliation item = %v (err %v), want a route entry", trail, err)
	}
}

// TestUC034_5_StaleCheckoutNeverSilentlyApplied proves UC-034 #5 / SYS-086:
// after the office overrides a checkout (audited), capture resumes on another
// device; the original device's later replay lands in reconciliation, not
// silently applied.
func TestUC034_5_StaleCheckoutNeverSilentlyApplied(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, deviceA, anna, _ := syncFixture(t)

	// Office overrides: the lock moves to a new device and the generation bumps.
	deviceB, err := results.OverrideCheckout(ctx, office, meetID, unitID, "tablet-B", "device A lost")
	if err != nil {
		t.Fatalf("OverrideCheckout: %v", err)
	}
	if deviceB.Generation != deviceA.Generation+1 {
		t.Fatalf("override generation = %d, want %d (bumped)", deviceB.Generation, deviceA.Generation+1)
	}
	if trail, err := store.AuditTrail(ctx, st.DB(), "checkout", unitID); err != nil || len(trail) == 0 {
		t.Fatalf("override audit trail = %v (err %v), want an override entry (SYS-046)", trail, err)
	}

	// The original device (stale generation) replays: reconciliation, not applied.
	staleBatch := batchFrom(deviceA, ReplayOp{OpID: "01OP0000000000000000000020", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"})
	res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, staleBatch)
	if err != nil {
		t.Fatalf("stale replay: %v", err)
	}
	if got := res.Outcomes[0]; got.Status != domain.OpReconciliation || got.Reason != domain.ReasonStaleCheckout {
		t.Fatalf("stale outcome = %+v, want reconciliation/stale_checkout", got)
	}
	if n := countUnitAttempts(t, st, unitID); n != 0 {
		t.Errorf("attempts = %d, want 0 (stale never applied)", n)
	}

	// The new device (current generation) captures normally.
	freshBatch := ReplayBatch{Token: deviceB.Token, DeviceLabel: "tablet-B", Generation: deviceB.Generation, StartListVersion: deviceB.StartListVersion,
		Ops: []ReplayOp{{OpID: "01OP0000000000000000000021", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"}}}
	res, err = results.ReplayCaptureBatch(ctx, office, meetID, unitID, freshBatch)
	if err != nil {
		t.Fatalf("fresh replay: %v", err)
	}
	if res.Outcomes[0].Status != domain.OpApplied {
		t.Fatalf("fresh outcome = %+v, want applied on the current generation", res.Outcomes[0])
	}
	if n := countUnitAttempts(t, st, unitID); n != 1 {
		t.Errorf("attempts = %d, want 1 (only the current-generation capture)", n)
	}
}

// TestUC034_5_WrongTokenNeverApplies proves the holder-authorization rule
// (ADR-004 §8: "the server accepts replays only from the unit's current
// checkout holder"): a batch claiming the CURRENT generation but presenting a
// token that is not the live checkout's routes to reconciliation — a client
// cannot write as the holder by misreporting its generation.
func TestUC034_5_WrongTokenNeverApplies(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, co, anna, _ := syncFixture(t)

	forged := ReplayBatch{
		Token: "01FORGEDTOKEN0000000000000", DeviceLabel: "tablet-X",
		Generation: co.Generation, StartListVersion: co.StartListVersion, // current values, wrong credential
		Ops: []ReplayOp{{OpID: "01OP0000000000000000000040", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"}},
	}
	res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, forged)
	if err != nil {
		t.Fatalf("ReplayCaptureBatch: %v", err)
	}
	if got := res.Outcomes[0]; got.Status != domain.OpReconciliation || got.Reason != domain.ReasonStaleCheckout {
		t.Fatalf("wrong-token outcome = %+v, want reconciliation/stale_checkout", got)
	}
	if n := countUnitAttempts(t, st, unitID); n != 0 {
		t.Errorf("attempts = %d, want 0 (wrong token never applied)", n)
	}

	// The genuine holder token still applies.
	res, err = results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID,
		batchFrom(co, ReplayOp{OpID: "01OP0000000000000000000041", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"}))
	if err != nil {
		t.Fatalf("holder replay: %v", err)
	}
	if res.Outcomes[0].Status != domain.OpApplied {
		t.Fatalf("holder outcome = %+v, want applied", res.Outcomes[0])
	}
}

// TestUC034_2_OpIDReuseAcrossUnitsFails proves the exactly-once ledger is
// per-op-id AND per-unit (SYS-086): a buggy client reusing an op id on a
// different unit gets an explicit error — never a false "duplicate" ack that
// would silently drop the second unit's capture.
func TestUC034_2_OpIDReuseAcrossUnitsFails(t *testing.T) {
	ctx := context.Background()
	meets, results, st := newTestResults(t)
	rec := createUKCMeet(t, meets)
	ljUnit := unitOf(t, meets, rec.ID, "ZoneLJ")
	ballUnit := unitOf(t, meets, rec.ID, "BallThrow200g")
	anna := register(t, results, rec.ID, ParticipantInput{FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101"})

	ljCo, err := results.CheckoutUnit(ctx, fieldOfficial, rec.ID, ljUnit, "tablet-A")
	if err != nil {
		t.Fatalf("CheckoutUnit(LJ): %v", err)
	}
	ballCo, err := results.CheckoutUnit(ctx, fieldOfficial, rec.ID, ballUnit, "tablet-A")
	if err != nil {
		t.Fatalf("CheckoutUnit(ball): %v", err)
	}

	const reusedID = "01OP0000000000000000000050"
	res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, rec.ID, ljUnit,
		batchFrom(ljCo, ReplayOp{OpID: reusedID, AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"}))
	if err != nil || res.Outcomes[0].Status != domain.OpApplied {
		t.Fatalf("LJ replay = %+v (err %v), want applied", res, err)
	}

	// The same op id replayed against the ball-throw unit: an explicit error,
	// not a silent "duplicate" that drops the capture.
	_, err = results.ReplayCaptureBatch(ctx, fieldOfficial, rec.ID, ballUnit,
		ReplayBatch{Token: ballCo.Token, DeviceLabel: "tablet-A", Generation: ballCo.Generation, StartListVersion: ballCo.StartListVersion,
			Ops: []ReplayOp{{OpID: reusedID, AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "32.50"}}})
	if err == nil {
		t.Fatal("cross-unit op-id reuse must fail loudly, not ack a false duplicate (SYS-086)")
	}
	if n := countUnitAttempts(t, st, ballUnit); n != 0 {
		t.Errorf("ball unit attempts = %d, want 0 (the erroneous op must not apply either)", n)
	}
}

// TestUC034_ReconciliationResolveApplyAndDiscard proves the office resolution
// paths behind UC-034 #4/#5: apply lands the capture through the shared path;
// discard is an explicit, audited drop.
func TestUC034_ReconciliationResolveApplyAndDiscard(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, co, anna, bea := syncFixture(t)

	// Force two ops to reconciliation via a start-list change.
	if _, err := results.ReviseStartList(ctx, office, meetID, unitID); err != nil {
		t.Fatalf("ReviseStartList: %v", err)
	}
	batch := batchFrom(co,
		ReplayOp{OpID: "01OP0000000000000000000030", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"},
		ReplayOp{OpID: "01OP0000000000000000000031", AthleteID: bea, Seq: 1, Kind: domain.AttemptValid, Mark: "3.50"},
	)
	if _, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batch); err != nil {
		t.Fatalf("ReplayCaptureBatch: %v", err)
	}
	groups, err := results.ReconciliationItems(ctx, office, meetID)
	if err != nil || len(groups) != 1 || len(groups[0].Entries) != 2 {
		t.Fatalf("reconciliation groups = %+v (err %v), want two items", groups, err)
	}
	var annaItem, beaItem string
	for _, e := range groups[0].Entries {
		if e.AthleteID == anna {
			annaItem = e.ItemID
		} else {
			beaItem = e.ItemID
		}
	}

	// Apply Anna's item: it lands as a real attempt.
	if err := results.ResolveReconciliation(ctx, office, meetID, annaItem, true); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if n := countUnitAttempts(t, st, unitID); n != 1 {
		t.Fatalf("attempts after apply = %d, want 1", n)
	}
	// Discard Bea's item: no attempt, but an audit record of the drop.
	if err := results.ResolveReconciliation(ctx, office, meetID, beaItem, false); err != nil {
		t.Fatalf("discard reconciliation: %v", err)
	}
	if n := countUnitAttempts(t, st, unitID); n != 1 {
		t.Errorf("attempts after discard = %d, want 1 (discard applies nothing)", n)
	}
	if trail, err := store.AuditTrail(ctx, st.DB(), "reconciliation", beaItem); err != nil || len(trail) < 2 {
		t.Fatalf("discard audit = %v (err %v), want route + discard entries", trail, err)
	}
	// The queue is now empty (both resolved).
	groups, err = results.ReconciliationItems(ctx, office, meetID)
	if err != nil {
		t.Fatalf("ReconciliationItems: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("pending groups after resolution = %d, want 0", len(groups))
	}
	// A resolved item cannot be resolved again.
	if err := results.ResolveReconciliation(ctx, office, meetID, annaItem, true); err == nil {
		t.Error("resolving an already-resolved item should fail")
	}
}

// TestUC034_CheckoutByAnotherRequiresOverride proves the single-holder rule
// (SYS-086): a second account cannot take an actively held lock without an
// office override.
func TestUC034_CheckoutByAnotherRequiresOverride(t *testing.T) {
	ctx := context.Background()
	results, _, meetID, unitID, _, _, _ := syncFixture(t)

	other := Session{AccountID: "01OTHER", Username: "other", Role: RoleFieldOfficial}
	_, err := results.CheckoutUnit(ctx, other, meetID, unitID, "tablet-C")
	if err != ErrCheckedOutByAnother {
		t.Fatalf("second checkout err = %v, want ErrCheckedOutByAnother", err)
	}
}
