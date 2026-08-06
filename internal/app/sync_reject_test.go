// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestSyncPerOpRejectionSYS149UC040_1 is the F1 defect regression (SYS-149,
// UC-040 #1/#2): a queued op the server cannot apply for a VALIDATION or
// capture-business-rule reason — an unparseable mark, an unregistered
// athlete, an already-announced unit — comes back as a per-op "rejected"
// outcome inside a normal 200 response, never a batch-wide error. The
// existing applied/duplicate paths (UC-034) are unchanged, table-tested
// alongside the new rejection cases so a regression in either direction
// shows up here.
func TestSyncPerOpRejectionSYS149UC040_1(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name       string
		op         func(anna string) ReplayOp
		announce   bool
		wantStatus domain.OpStatus
		wantReason domain.RejectReason
	}{
		{
			name: "invalid mark rejected, not a batch error",
			op: func(anna string) ReplayOp {
				return ReplayOp{OpID: "01OP00000000000000000R001", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "abc"}
			},
			wantStatus: domain.OpRejected,
			wantReason: domain.RejectInvalidMark,
		},
		{
			name: "zero mark rejected",
			op: func(anna string) ReplayOp {
				return ReplayOp{OpID: "01OP00000000000000000R002", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "0"}
			},
			wantStatus: domain.OpRejected,
			wantReason: domain.RejectInvalidMark,
		},
		{
			name: "trial beyond series rejected",
			op: func(anna string) ReplayOp {
				return ReplayOp{OpID: "01OP00000000000000000R003", AthleteID: anna, Seq: 99, Kind: domain.AttemptValid, Mark: "3.40"}
			},
			wantStatus: domain.OpRejected,
			wantReason: domain.RejectInvalidMark,
		},
		{
			name: "unknown athlete rejected",
			op: func(string) ReplayOp {
				return ReplayOp{OpID: "01OP00000000000000000R004", AthleteID: "01UNKNOWNATHLETE00000000", Seq: 1, Kind: domain.AttemptValid, Mark: "3.40"}
			},
			wantStatus: domain.OpRejected,
			wantReason: domain.RejectUnknownAthlete,
		},
		{
			name: "already-announced unit rejected",
			op: func(anna string) ReplayOp {
				return ReplayOp{OpID: "01OP00000000000000000R005", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.40"}
			},
			announce:   true,
			wantStatus: domain.OpRejected,
			wantReason: domain.RejectAnnounced,
		},
		{
			name: "valid mark still applies (UC-034 unchanged)",
			op: func(anna string) ReplayOp {
				return ReplayOp{OpID: "01OP00000000000000000R006", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "3.40"}
			},
			wantStatus: domain.OpApplied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, st, meetID, unitID, co, anna, _ := syncFixture(t)
			if tc.announce {
				if _, err := results.AnnounceUnitResults(ctx, office, meetID, unitID); err != nil {
					t.Fatalf("AnnounceUnitResults: %v", err)
				}
			}
			op := tc.op(anna)
			res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batchFrom(co, op))
			if err != nil {
				t.Fatalf("ReplayCaptureBatch returned a batch-wide error for a per-op failure: %v (SYS-149: must be a per-op outcome)", err)
			}
			if len(res.Outcomes) != 1 {
				t.Fatalf("outcomes = %d, want 1", len(res.Outcomes))
			}
			got := res.Outcomes[0]
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (outcome %+v)", got.Status, tc.wantStatus, got)
			}
			if tc.wantStatus == domain.OpRejected {
				if got.RejectReason != tc.wantReason {
					t.Errorf("reject reason = %q, want %q", got.RejectReason, tc.wantReason)
				}
				if n := countUnitAttempts(t, st, unitID); n != 0 {
					t.Errorf("attempts after rejection = %d, want 0 (never applied)", n)
				}
			}
		})
	}
}

// TestSyncPerOpRejectionSYS149UC040_2_DoesNotBlockLaterOps proves UC-040 #2:
// one rejected op in a batch never blocks the other queued ops from
// applying — the F1 defect was a poisoned op head-of-line-blocking every
// later capture.
func TestSyncPerOpRejectionSYS149UC040_2_DoesNotBlockLaterOps(t *testing.T) {
	ctx := context.Background()
	results, st, meetID, unitID, co, anna, bea := syncFixture(t)

	batch := batchFrom(co,
		ReplayOp{OpID: "01OP00000000000000000R010", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "abc"}, // poisoned
		ReplayOp{OpID: "01OP00000000000000000R011", AthleteID: bea, Seq: 1, Kind: domain.AttemptValid, Mark: "3.80"}, // valid, must still apply
	)
	res, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batch)
	if err != nil {
		t.Fatalf("ReplayCaptureBatch: %v", err)
	}
	if len(res.Outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2", len(res.Outcomes))
	}
	if res.Outcomes[0].Status != domain.OpRejected {
		t.Errorf("op 0 status = %q, want rejected", res.Outcomes[0].Status)
	}
	if res.Outcomes[1].Status != domain.OpApplied {
		t.Errorf("op 1 status = %q, want applied — a rejection must not block later ops (UC-040 #2)", res.Outcomes[1].Status)
	}
	if n := countUnitAttempts(t, st, unitID); n != 1 {
		t.Fatalf("attempts = %d, want 1 (bea's valid mark applied)", n)
	}

	// Resending the same batch: the rejected op re-acknowledges the same
	// terminal rejection (idempotent, SYS-085) rather than re-validating or
	// erroring; the already-applied op comes back duplicate.
	res, err = results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID, batch)
	if err != nil {
		t.Fatalf("re-replay: %v", err)
	}
	if res.Outcomes[0].Status != domain.OpRejected || res.Outcomes[0].RejectReason != domain.RejectInvalidMark {
		t.Errorf("re-replay op 0 = %+v, want rejected/invalid_mark again", res.Outcomes[0])
	}
	if res.Outcomes[1].Status != domain.OpDuplicate {
		t.Errorf("re-replay op 1 status = %q, want duplicate", res.Outcomes[1].Status)
	}
}

// TestSyncRejectedOpNotQueuedForReconciliationSYS149 proves a rejected op is
// distinct from a reconciliation item: there is nothing for the office to
// apply, so it must never appear in the office reconciliation view.
func TestSyncRejectedOpNotQueuedForReconciliationSYS149(t *testing.T) {
	ctx := context.Background()
	results, _, meetID, unitID, co, anna, _ := syncFixture(t)

	if _, err := results.ReplayCaptureBatch(ctx, fieldOfficial, meetID, unitID,
		batchFrom(co, ReplayOp{OpID: "01OP00000000000000000R020", AthleteID: anna, Seq: 1, Kind: domain.AttemptValid, Mark: "abc"})); err != nil {
		t.Fatalf("ReplayCaptureBatch: %v", err)
	}
	groups, err := results.ReconciliationItems(ctx, office, meetID)
	if err != nil {
		t.Fatalf("ReconciliationItems: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("reconciliation groups = %+v, want none (a rejection is not a reconciliation item)", groups)
	}
}
