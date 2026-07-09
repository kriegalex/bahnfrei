// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

// Offline capture sync semantics (UC-034, SYS-085/086; ADR-004 §8). This is
// the pure routing rule the app layer applies to every replayed capture op:
// given the server's current checkout facts for a unit and the stamp an op
// claims it was captured against, decide whether the op may be applied or
// must be surfaced for reconciliation — it is never silently discarded (the
// documented Web.TEC 2 loss mode C2.1 is the named anti-pattern).

// CheckoutState is the server's current checkout facts for a unit that a
// replayed op is validated against.
type CheckoutState struct {
	// Generation is the unit's current, monotonically increasing checkout
	// generation. An override/reassign bumps it, so a device holding an older
	// generation is provably stale (SYS-086).
	Generation int64
	// StartListVersion is the unit's current start-list version; an
	// office-side revision bumps it (SYS-086: "device captures affected by an
	// office-side start-list change").
	StartListVersion int64
	// Active is false when no live checkout holds the unit (e.g. released on
	// completion, or never checked out). A replay against no live holder is
	// not from the current holder, so it reconciles.
	Active bool
}

// OpStamp is what a replayed capture op claims it was captured against: the
// checkout generation and start-list version live on the device at capture
// time.
type OpStamp struct {
	Generation       int64
	StartListVersion int64
}

// SyncDecision is the terminal routing of one replayed op.
type SyncDecision string

const (
	// DecisionApply means the op is from the current holder against the
	// current start list and may be applied through the normal capture path.
	DecisionApply SyncDecision = "apply"
	// DecisionStaleCheckout means the op is from a superseded/absent holder:
	// route to reconciliation, never apply (SYS-086).
	DecisionStaleCheckout SyncDecision = "stale_checkout"
	// DecisionStartListChange means the op is from the current holder but the
	// office changed the start list since checkout: route to reconciliation.
	DecisionStartListChange SyncDecision = "start_list_change"
)

// DecideSync routes one replayed op. Order matters: a stale checkout is
// decided before a start-list mismatch, because a stale device's start-list
// version is not comparable to the current holder's.
func DecideSync(current CheckoutState, op OpStamp) SyncDecision {
	if !current.Active || op.Generation != current.Generation {
		return DecisionStaleCheckout
	}
	if op.StartListVersion != current.StartListVersion {
		return DecisionStartListChange
	}
	return DecisionApply
}

// OpStatus is the per-op status reported back to the replaying client.
type OpStatus string

const (
	// OpApplied: the op was applied to the unit this call.
	OpApplied OpStatus = "applied"
	// OpDuplicate: the op was already applied by an earlier batch; a flaky
	// reconnect re-sent it and it produced no second write (SYS-085).
	OpDuplicate OpStatus = "duplicate"
	// OpReconciliation: the op was routed to the office reconciliation queue.
	OpReconciliation OpStatus = "reconciliation"
)

// ReconcileReason names why a capture could not be applied. It travels to the
// office reconciliation view so an operator sees what happened.
type ReconcileReason string

const (
	// ReasonStaleCheckout: a superseded or absent checkout holder (SYS-086).
	ReasonStaleCheckout ReconcileReason = "stale_checkout"
	// ReasonStartListChange: the office revised the start list after checkout.
	ReasonStartListChange ReconcileReason = "start_list_change"
	// ReasonConflict: the op is from the current holder but applying it would
	// overwrite a diverging attempt captured meanwhile (surfaced, not merged).
	ReasonConflict ReconcileReason = "conflict"
)
