// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"sync"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// TestUC034_ConcurrentResendNeverDoubleApplies proves the server half of the
// exactly-once guarantee (SYS-085) holds under CONCURRENCY, not just serial
// re-sends: a correction (an attempt re-captured under the version it already
// carries) whose sync is re-sent many times at once — as a flapping network or
// a browser-level connection-kill retry can produce — must apply exactly once.
// However many identical batches race, the stored version settles at 2 (one
// insert + one correction), never higher: the dedupe ledger plus the optimistic
// version guard together stop a second apply even though the ledger check and
// the write are not one transaction (the version guard turns the losing racer
// into a harmless duplicate, not a second increment). This is the server-side
// invariant behind the chaos-m1 e2e run; the client's optimistic version is
// converged to this authoritative value by OpOutcome.Version.
func TestUC034_ConcurrentResendNeverDoubleApplies(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, co, anna, _ := syncFixture(t)

	// Initial capture: anna trial 1 = 3.40 -> version 1.
	if _, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID,
		batchFrom(co, ReplayOp{OpID: "01OP000000000000000000INIT", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.40"})); err != nil {
		t.Fatalf("initial replay: %v", err)
	}
	if v := attemptVersion(t, st, unitID, anna, 1); v != 1 {
		t.Fatalf("version after initial = %d, want 1", v)
	}

	// The correction op: reused opId, expected version 1, new value 3.45.
	corr := ReplayOp{OpID: "01OP000000000000000000CORR", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.45", ExpectedVersion: 1}

	const n = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied, dup, other := 0, 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batchFrom(co, corr))
			if err != nil {
				return
			}
			mu.Lock()
			switch res.Outcomes[0].Status {
			case domain.OpApplied:
				applied++
			case domain.OpDuplicate:
				dup++
			default:
				other++
			}
			// Every applied/duplicate ack reports the authoritative version so
			// the client can SET (not blindly increment) the cell.
			if res.Outcomes[0].Version != 2 {
				t.Errorf("ack version = %d, want 2 (authoritative)", res.Outcomes[0].Version)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if v := attemptVersion(t, st, unitID, anna, 1); v != 2 {
		t.Fatalf("stored version after %d concurrent correction re-sends = %d, want 2 (no double-apply)", n, v)
	}
	if mark := attemptMark(t, st, unitID, anna, 1); mark != "3.45" {
		t.Fatalf("stored mark = %q, want 3.45", mark)
	}
	if applied != 1 {
		t.Errorf("applied acks = %d, want exactly 1 (exactly-once)", applied)
	}
	if other != 0 {
		t.Errorf("unexpected non-applied/duplicate outcomes = %d", other)
	}
}

func attemptVersion(t *testing.T, st *store.Store, unitID, athleteID string, seq int) int64 {
	t.Helper()
	rec, err := store.GetAttempt(context.Background(), st.DB(), unitID, athleteID, seq)
	if err != nil {
		t.Fatalf("GetAttempt: %v", err)
	}
	return rec.Version
}

func attemptMark(t *testing.T, st *store.Store, unitID, athleteID string, seq int) string {
	t.Helper()
	rec, err := store.GetAttempt(context.Background(), st.DB(), unitID, athleteID, seq)
	if err != nil {
		t.Fatalf("GetAttempt: %v", err)
	}
	return rec.Mark
}
