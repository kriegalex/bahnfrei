// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// Checkout aliases the store type for the web layer (architecture.md §3: web
// never imports internal/store).
type Checkout = store.Checkout

// ErrCheckedOutByAnother means another active holder owns the unit's capture
// lock; the office must override to reassign it (SYS-086).
var ErrCheckedOutByAnother = errors.New("unit is checked out by another device")

// --- Event-unit checkout (SYS-086) ---

// EnsureCheckout guarantees a checkout row exists for the unit, creating a
// first-generation one held by actor if none exists, and otherwise leaving
// the current holder untouched. It is the non-blocking hook the online
// capture flow calls when a field official opens a unit — populating the
// checkout model without churning the generation on every page load.
func (s *ResultsService) EnsureCheckout(ctx context.Context, actor Session, meetID, unitID, deviceLabel string) (Checkout, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	if c, err := store.GetCheckout(ctx, s.db, unitID); err == nil {
		return c, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return Checkout{}, err
	}
	return store.CreateCheckout(ctx, s.db, unitID, actor.AccountID, deviceLabel, store.NewID(), 1, 0)
}

// CheckoutUnit is a field official explicitly taking a unit's capture lock
// before offline capture (SYS-086). A first checkout starts at generation 1;
// re-taking a released lock bumps the generation; a lock actively held by
// another device requires an office override (ErrCheckedOutByAnother).
func (s *ResultsService) CheckoutUnit(ctx context.Context, actor Session, meetID, unitID, deviceLabel string) (Checkout, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	cur, err := store.GetCheckout(ctx, s.db, unitID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return store.CreateCheckout(ctx, s.db, unitID, actor.AccountID, deviceLabel, store.NewID(), 1, 0)
	case err != nil:
		return Checkout{}, err
	}
	if cur.Active && cur.AccountID != actor.AccountID {
		return Checkout{}, ErrCheckedOutByAnother
	}
	if cur.Active && cur.DeviceLabel == deviceLabel {
		return cur, nil // idempotent re-checkout by the same holder/device
	}
	// Same account new device, or re-taking a released lock: reassign (gen+1).
	return store.ReassignCheckout(ctx, s.db, unitID, actor.AccountID, deviceLabel, store.NewID(), cur.Version)
}

// OverrideCheckout is the privileged office action that reassigns a unit's
// capture lock to a new device (e.g. after a lost/dead device), bumping the
// generation so the old device's later replays are provably stale. It is
// written to the append-only audit log (SYS-046).
func (s *ResultsService) OverrideCheckout(ctx context.Context, actor Session, meetID, unitID, deviceLabel, reason string) (Checkout, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return Checkout{}, err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Checkout{}, fmt.Errorf("override checkout: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	cur, err := store.GetCheckout(ctx, tx, unitID)
	if err != nil {
		return Checkout{}, err
	}
	beforeJSON, _ := json.Marshal(map[string]any{"holder": cur.AccountID, "device": cur.DeviceLabel, "generation": cur.Generation})
	co, err := store.ReassignCheckout(ctx, tx, unitID, actor.AccountID, deviceLabel, store.NewID(), cur.Version)
	if err != nil {
		return Checkout{}, err
	}
	afterJSON, _ := json.Marshal(map[string]any{"holder": co.AccountID, "device": co.DeviceLabel, "generation": co.Generation})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "checkout.override",
		EntityType: "checkout", EntityID: unitID,
		Before: string(beforeJSON), After: string(afterJSON), Reason: reason,
	}); err != nil {
		return Checkout{}, fmt.Errorf("audit checkout override: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Checkout{}, fmt.Errorf("override checkout: %w", err)
	}
	return co, nil
}

// ReviseStartList records an office-side start-list change on a checked-out
// unit (SYS-086): the current holder's captures against the old version now
// route to reconciliation. Audited (SYS-046).
func (s *ResultsService) ReviseStartList(ctx context.Context, actor Session, meetID, unitID string) (Checkout, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return Checkout{}, err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Checkout{}, fmt.Errorf("revise start list: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	cur, err := store.GetCheckout(ctx, tx, unitID)
	if err != nil {
		return Checkout{}, err
	}
	co, err := store.BumpStartListVersion(ctx, tx, unitID, cur.Version)
	if err != nil {
		return Checkout{}, err
	}
	after, _ := json.Marshal(map[string]any{"startListVersion": co.StartListVersion})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "checkout.revise_startlist",
		EntityType: "checkout", EntityID: unitID, After: string(after),
	}); err != nil {
		return Checkout{}, fmt.Errorf("audit start-list revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Checkout{}, fmt.Errorf("revise start list: %w", err)
	}
	return co, nil
}

// ReleaseCheckout releases a unit's lock on completion (SYS-086).
func (s *ResultsService) ReleaseCheckout(ctx context.Context, actor Session, meetID, unitID string) (Checkout, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return Checkout{}, err
	}
	cur, err := store.GetCheckout(ctx, s.db, unitID)
	if err != nil {
		return Checkout{}, err
	}
	return store.ReleaseCheckout(ctx, s.db, unitID, cur.Version)
}

// --- Idempotent replay (SYS-085/086, UC-034 #2/#4/#5) ---

// ReplayOp is one queued capture operation replayed from a device: a field
// trial keyed by a client-generated ULID (OpID).
type ReplayOp struct {
	OpID            string
	AthleteID       string
	Seq             int
	Kind            domain.AttemptKind
	Mark            string
	Wind            *float64
	ExpectedVersion int64
}

// ReplayBatch is an ordered batch of queued ops from one device, stamped with
// the checkout it was captured under.
type ReplayBatch struct {
	Token            string
	DeviceLabel      string
	Generation       int64
	StartListVersion int64
	Ops              []ReplayOp
}

// OpOutcome is the per-op result reported to the client: applied, duplicate
// (already applied — no second write), or reconciliation (with the reason).
// Version carries the attempt's authoritative stored version after an applied
// or duplicate decision (0 for reconciliation), so the client SETS the cell's
// optimistic version to the server truth rather than blindly incrementing it —
// a blind increment double-counts when the same op is acknowledged again after
// a page render already reflected the write (SYS-085).
type OpOutcome struct {
	OpID    string
	Status  domain.OpStatus
	Reason  domain.ReconcileReason
	Version int64
}

// ReplayResult carries one outcome per submitted op, in submission order.
type ReplayResult struct {
	Outcomes []OpOutcome
}

// ReplayCaptureBatch applies an ordered batch of queued capture ops exactly
// once (SYS-085): an op already applied is acknowledged, not re-applied; ops
// from a stale checkout or against a changed start list route to
// reconciliation — never silently discarded (SYS-086). Applied ops go through
// the same TASK-008 capture path (validation, standings recompute, SSE
// notify): there is no second write path.
func (s *ResultsService) ReplayCaptureBatch(ctx context.Context, actor Session, meetID, unitID string, batch ReplayBatch) (ReplayResult, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return ReplayResult{}, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return ReplayResult{}, err
	}
	if uc.disc.Family != domain.FamilyFieldHorizontal {
		return ReplayResult{}, fmt.Errorf("unit %s is %s; offline replay covers horizontal field units", unitID, uc.disc.Family)
	}

	var res ReplayResult
	for _, op := range batch.Ops {
		outcome, err := s.replayOne(ctx, actor, meetID, unitID, batch, op)
		if err != nil {
			return ReplayResult{}, err
		}
		res.Outcomes = append(res.Outcomes, outcome)
	}
	return res, nil
}

func (s *ResultsService) replayOne(ctx context.Context, actor Session, meetID, unitID string, batch ReplayBatch, op ReplayOp) (OpOutcome, error) {
	// Exactly-once: a previously decided op id is acknowledged, not re-run.
	// The recorded unit must match: a client reusing an op id across units is
	// a bug that must fail loudly — acknowledging it as "duplicate" would
	// silently drop the second unit's capture, the exact loss mode SYS-086
	// forbids. The client must regenerate the ULID.
	if recordedUnit, status, reason, found, err := store.GetCaptureOp(ctx, s.db, op.OpID); err != nil {
		return OpOutcome{}, err
	} else if found {
		if recordedUnit != unitID {
			return OpOutcome{}, fmt.Errorf("op %s was already recorded for unit %s, not unit %s: op ids are unique per capture — regenerate the ULID (SYS-086)",
				op.OpID, recordedUnit, unitID)
		}
		if status == string(domain.OpReconciliation) {
			return OpOutcome{OpID: op.OpID, Status: domain.OpReconciliation, Reason: domain.ReconcileReason(reason)}, nil
		}
		// Already applied by an earlier batch (a flaky reconnect re-sent it):
		// report the CURRENT authoritative version so the client converges the
		// cell to the server truth instead of over-counting it.
		return OpOutcome{OpID: op.OpID, Status: domain.OpDuplicate, Version: s.currentAttemptVersion(ctx, unitID, op.AthleteID, op.Seq)}, nil
	}

	state := domain.CheckoutState{}
	holderToken := ""
	cur, err := store.GetCheckout(ctx, s.db, unitID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// no live checkout -> not the current holder -> reconcile
	case err != nil:
		return OpOutcome{}, err
	default:
		state = domain.CheckoutState{Generation: cur.Generation, StartListVersion: cur.StartListVersion, Active: cur.Active}
		holderToken = cur.Token
	}

	// Holder authorization (ADR-004 §8: "the server accepts replays only from
	// the unit's current checkout holder"): the opaque token is the
	// credential; the generation is the staleness/audit signal. A batch whose
	// token is not the live checkout's is by definition from a superseded
	// device (tokens are reissued on every reassign/override), whatever
	// generation it claims — route it to reconciliation, never apply.
	if state.Active && batch.Token != holderToken {
		return s.routeReconciliation(ctx, actor, unitID, batch, op, domain.ReasonStaleCheckout)
	}

	switch domain.DecideSync(state, domain.OpStamp{Generation: batch.Generation, StartListVersion: batch.StartListVersion}) {
	case domain.DecisionApply:
		return s.applyReplayOp(ctx, actor, meetID, unitID, batch, op)
	case domain.DecisionStartListChange:
		return s.routeReconciliation(ctx, actor, unitID, batch, op, domain.ReasonStartListChange)
	default: // DecisionStaleCheckout
		return s.routeReconciliation(ctx, actor, unitID, batch, op, domain.ReasonStaleCheckout)
	}
}

// applyReplayOp applies one current-holder op through the shared capture path.
// A version conflict is not a silent overwrite: if the stored attempt already
// equals this op it is an at-least-once re-delivery (duplicate); otherwise the
// captures diverged and the op is surfaced for reconciliation (SYS-086).
func (s *ResultsService) applyReplayOp(ctx context.Context, actor Session, meetID, unitID string, batch ReplayBatch, op ReplayOp) (OpOutcome, error) {
	in := FieldAttemptInput{AthleteID: op.AthleteID, Seq: op.Seq, Kind: op.Kind, Mark: op.Mark, Wind: op.Wind, ExpectedVersion: op.ExpectedVersion}
	rec, err := s.SaveFieldAttempt(ctx, actor, meetID, unitID, in)
	if err == nil {
		if e := store.RecordCaptureOp(ctx, s.db, op.OpID, unitID, string(domain.OpApplied), ""); e != nil {
			return OpOutcome{}, e
		}
		return OpOutcome{OpID: op.OpID, Status: domain.OpApplied, Version: rec.Version}, nil
	}
	var conflict *AttemptConflictError
	if errors.As(err, &conflict) {
		if sameAttempt(conflict.Current, op) {
			if e := store.RecordCaptureOp(ctx, s.db, op.OpID, unitID, string(domain.OpApplied), ""); e != nil {
				return OpOutcome{}, e
			}
			return OpOutcome{OpID: op.OpID, Status: domain.OpDuplicate, Version: conflict.Current.Version}, nil
		}
		return s.routeReconciliation(ctx, actor, unitID, batch, op, domain.ReasonConflict)
	}
	return OpOutcome{}, err
}

// sameAttempt reports whether a stored attempt already equals what an op would
// write — the marker that a version conflict is a harmless re-delivery of an
// op that already landed, not a genuine divergence.
func sameAttempt(stored store.AttemptRecord, op ReplayOp) bool {
	if stored.Kind != op.Kind || stored.Mark != op.Mark {
		return false
	}
	switch {
	case stored.Wind == nil && op.Wind == nil:
		return true
	case stored.Wind == nil || op.Wind == nil:
		return false
	default:
		return *stored.Wind == *op.Wind
	}
}

// routeReconciliation records an unappliable op as a pending reconciliation
// item plus its dedupe-ledger entry, atomically, and audits the routing so
// the "never silently discarded" guarantee is itself traceable (SYS-046/086).
func (s *ResultsService) routeReconciliation(ctx context.Context, actor Session, unitID string, batch ReplayBatch, op ReplayOp, reason domain.ReconcileReason) (OpOutcome, error) {
	payload, _ := json.Marshal(capturePayload{
		AthleteID: op.AthleteID, Seq: op.Seq, Kind: string(op.Kind), Mark: op.Mark, Wind: op.Wind,
	})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OpOutcome{}, fmt.Errorf("route reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	item, err := store.CreateReconciliationItem(ctx, tx, store.ReconciliationItem{
		OpID: op.OpID, UnitID: unitID, AthleteID: op.AthleteID, Reason: string(reason),
		Payload: string(payload), CapturedBy: actor.AccountID, DeviceLabel: batch.DeviceLabel,
		Generation: batch.Generation, StartListVersion: batch.StartListVersion,
	})
	if err != nil {
		return OpOutcome{}, err
	}
	if err := store.RecordCaptureOp(ctx, tx, op.OpID, unitID, string(domain.OpReconciliation), string(reason)); err != nil {
		return OpOutcome{}, err
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "reconciliation.route",
		EntityType: "reconciliation", EntityID: item.ID,
		After: string(payload), Reason: string(reason),
	}); err != nil {
		return OpOutcome{}, fmt.Errorf("audit reconciliation route: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return OpOutcome{}, fmt.Errorf("route reconciliation: %w", err)
	}
	return OpOutcome{OpID: op.OpID, Status: domain.OpReconciliation, Reason: reason}, nil
}

// capturePayload is the stored form of a queued field trial, enough to apply
// or discard it during reconciliation.
type capturePayload struct {
	AthleteID string   `json:"athleteId"`
	Seq       int      `json:"seq"`
	Kind      string   `json:"kind"`
	Mark      string   `json:"mark"`
	Wind      *float64 `json:"wind,omitempty"`
}

// --- Reconciliation view & resolution (UC-034 #4/#5) ---

// ReconciliationEntry is one pending item as the office view shows it.
type ReconciliationEntry struct {
	ItemID      string
	AthleteID   string
	Reason      domain.ReconcileReason
	CapturedBy  string
	DeviceLabel string
	Trial       int
	Value       string // reconstructed grid notation (mark or X/–/r)
}

// ReconciliationGroup groups pending items by unit for the office view.
type ReconciliationGroup struct {
	UnitID         string
	DisciplineCode string
	DisciplineName string
	Entries        []ReconciliationEntry
}

// ReconciliationItems lists a meet's pending reconciliation items grouped by
// unit (UC-034 #4/#5, office-facing).
func (s *ResultsService) ReconciliationItems(ctx context.Context, actor Session, meetID string) ([]ReconciliationGroup, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	rows, err := store.ListPendingReconciliation(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	var groups []ReconciliationGroup
	byUnit := map[string]int{}
	for _, r := range rows {
		idx, ok := byUnit[r.UnitID]
		if !ok {
			name := r.DisciplineCode
			if disc, ok := s.catalog.ByCode(r.DisciplineCode); ok {
				name = disc.Name
			}
			groups = append(groups, ReconciliationGroup{UnitID: r.UnitID, DisciplineCode: r.DisciplineCode, DisciplineName: name})
			idx = len(groups) - 1
			byUnit[r.UnitID] = idx
		}
		var pl capturePayload
		_ = json.Unmarshal([]byte(r.Payload), &pl)
		groups[idx].Entries = append(groups[idx].Entries, ReconciliationEntry{
			ItemID: r.ID, AthleteID: r.AthleteID, Reason: domain.ReconcileReason(r.Reason),
			CapturedBy: r.CapturedBy, DeviceLabel: r.DeviceLabel,
			Trial: pl.Seq, Value: renderAttemptValue(pl),
		})
	}
	return groups, nil
}

// renderAttemptValue reconstructs the grid notation a payload represents.
func renderAttemptValue(pl capturePayload) string {
	switch domain.AttemptKind(pl.Kind) {
	case domain.AttemptFoul:
		return "X"
	case domain.AttemptPass:
		return "–"
	case domain.AttemptRetire:
		return "r"
	default:
		return pl.Mark
	}
}

// ResolveReconciliation applies or discards a pending reconciliation item
// (UC-034 #4/#5). Apply routes through the shared capture path, force-writing
// over the current stored trial; discard is an explicit, audited decision
// (SYS-046) — a capture is never dropped without a recorded operator action.
func (s *ResultsService) ResolveReconciliation(ctx context.Context, actor Session, meetID, itemID string, apply bool) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	item, err := store.GetReconciliationItem(ctx, s.db, itemID)
	if err != nil {
		return err
	}
	if item.Status != "pending" {
		return fmt.Errorf("reconciliation item %s already %s", itemID, item.Status)
	}

	if apply {
		var pl capturePayload
		if err := json.Unmarshal([]byte(item.Payload), &pl); err != nil {
			return fmt.Errorf("reconciliation item %s: bad payload: %w", itemID, err)
		}
		expected := s.currentAttemptVersion(ctx, item.UnitID, pl.AthleteID, pl.Seq)
		in := FieldAttemptInput{AthleteID: pl.AthleteID, Seq: pl.Seq, Kind: domain.AttemptKind(pl.Kind), Mark: pl.Mark, Wind: pl.Wind, ExpectedVersion: expected}
		if _, err := s.SaveFieldAttempt(ctx, actor, meetID, item.UnitID, in); err != nil {
			return fmt.Errorf("apply reconciliation item %s: %w", itemID, err)
		}
	}

	status := "discarded"
	action := "reconciliation.discard"
	if apply {
		status, action = "applied", "reconciliation.apply"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("resolve reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := store.SetReconciliationStatus(ctx, tx, itemID, status, actor.AccountID); err != nil {
		return err
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: action,
		EntityType: "reconciliation", EntityID: itemID, After: item.Payload,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return tx.Commit()
}

// currentAttemptVersion returns the stored version of a trial (0 if none), so
// a reconciliation apply overwrites the current value rather than conflicting.
func (s *ResultsService) currentAttemptVersion(ctx context.Context, unitID, athleteID string, seq int) int64 {
	rec, err := store.GetAttempt(ctx, s.db, unitID, athleteID, seq)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, store.ErrNotFound) {
			return 0
		}
		return 0
	}
	return rec.Version
}
