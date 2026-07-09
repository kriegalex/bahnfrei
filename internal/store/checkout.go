// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// nowExpr is the SQLite timestamp expression matching the DEFAULTs in the
// migrations (RFC 3339 with milliseconds, UTC) so app-side updates and
// table defaults produce the same encoding.
const nowExpr = `strftime('%Y-%m-%dT%H:%M:%fZ','now')`

// Checkout is a unit's capture-lock state (SYS-086): the single active
// holder, the monotonic per-unit generation, and the start-list version the
// holder captures against.
type Checkout struct {
	UnitID           string
	AccountID        string
	DeviceLabel      string
	Token            string
	Generation       int64
	StartListVersion int64
	Active           bool
	CheckedOutAt     time.Time
	Version          int64
}

// GetCheckout returns the unit's checkout row, or ErrNotFound if the unit was
// never checked out.
func GetCheckout(ctx context.Context, db DBTX, unitID string) (Checkout, error) {
	row := db.QueryRowContext(ctx, `SELECT unit_id, account_id, device_label, token,
		generation, start_list_version, active, checked_out_at, version
		FROM unit_checkouts WHERE unit_id = ?`, unitID)
	var c Checkout
	var active int
	var ts string
	err := row.Scan(&c.UnitID, &c.AccountID, &c.DeviceLabel, &c.Token,
		&c.Generation, &c.StartListVersion, &active, &ts, &c.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Checkout{}, ErrNotFound
	case err != nil:
		return Checkout{}, err
	}
	c.Active = active != 0
	if c.CheckedOutAt, err = time.Parse(time.RFC3339Nano, ts); err != nil {
		return Checkout{}, fmt.Errorf("checkout %s: bad timestamp %q: %w", unitID, ts, err)
	}
	return c, nil
}

// CreateCheckout inserts the first checkout for a unit at the given
// generation and start-list version.
func CreateCheckout(ctx context.Context, db DBTX, unitID, accountID, deviceLabel, token string, generation, startListVersion int64) (Checkout, error) {
	if unitID == "" || accountID == "" || token == "" {
		return Checkout{}, errors.New("checkout: unit id, account id and token are required")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO unit_checkouts
		(unit_id, account_id, device_label, token, generation, start_list_version, active)
		VALUES (?, ?, ?, ?, ?, ?, 1)`,
		unitID, accountID, deviceLabel, token, generation, startListVersion)
	if err != nil {
		return Checkout{}, fmt.Errorf("create checkout %s: %w", unitID, err)
	}
	return GetCheckout(ctx, db, unitID)
}

// ReassignCheckout hands the unit's lock to a new holder and bumps the
// generation (SYS-086 override/reassign, or a device re-taking a released
// lock). The version guard turns concurrent reassigns into ErrVersionConflict
// rather than a lost bump.
func ReassignCheckout(ctx context.Context, db DBTX, unitID, accountID, deviceLabel, token string, expectedVersion int64) (Checkout, error) {
	res, err := db.ExecContext(ctx, `UPDATE unit_checkouts
		SET account_id = ?, device_label = ?, token = ?,
		    generation = generation + 1, active = 1,
		    checked_out_at = `+nowExpr+`, version = version + 1
		WHERE unit_id = ? AND version = ?`,
		accountID, deviceLabel, token, unitID, expectedVersion)
	if err != nil {
		return Checkout{}, fmt.Errorf("reassign checkout %s: %w", unitID, err)
	}
	if err := oneRow(ctx, db, res, unitID); err != nil {
		return Checkout{}, err
	}
	return GetCheckout(ctx, db, unitID)
}

// BumpStartListVersion records that the office revised a unit's start list
// (SYS-086): the current holder's captures against the old version now
// reconcile.
func BumpStartListVersion(ctx context.Context, db DBTX, unitID string, expectedVersion int64) (Checkout, error) {
	res, err := db.ExecContext(ctx, `UPDATE unit_checkouts
		SET start_list_version = start_list_version + 1, version = version + 1
		WHERE unit_id = ? AND version = ?`, unitID, expectedVersion)
	if err != nil {
		return Checkout{}, fmt.Errorf("bump start-list version %s: %w", unitID, err)
	}
	if err := oneRow(ctx, db, res, unitID); err != nil {
		return Checkout{}, err
	}
	return GetCheckout(ctx, db, unitID)
}

// ReleaseCheckout marks the unit's checkout inactive (released on unit
// completion). Later replays from the released holder reconcile.
func ReleaseCheckout(ctx context.Context, db DBTX, unitID string, expectedVersion int64) (Checkout, error) {
	res, err := db.ExecContext(ctx, `UPDATE unit_checkouts
		SET active = 0, version = version + 1
		WHERE unit_id = ? AND version = ?`, unitID, expectedVersion)
	if err != nil {
		return Checkout{}, fmt.Errorf("release checkout %s: %w", unitID, err)
	}
	if err := oneRow(ctx, db, res, unitID); err != nil {
		return Checkout{}, err
	}
	return GetCheckout(ctx, db, unitID)
}

// oneRow maps an UPDATE that changed no row to a version conflict / not-found,
// mirroring OptimisticUpdate's contract for the bespoke checkout updates.
func oneRow(ctx context.Context, db DBTX, res sql.Result, unitID string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	var v int64
	err = db.QueryRowContext(ctx, `SELECT version FROM unit_checkouts WHERE unit_id = ?`, unitID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checkout %s: %w", unitID, ErrNotFound)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("checkout %s: %w", unitID, ErrVersionConflict)
}

// GetCaptureOp returns the recorded terminal decision for a client op id: the
// unit it was recorded against, its status, its reconciliation reason (empty
// unless reconciled) and whether the op was seen before. It is the
// exactly-once dedupe lookup (SYS-085); callers MUST compare unitID against
// their own unit — an op id reused across units is a client bug that must not
// be acknowledged as a duplicate (that would silently drop the second unit's
// capture, SYS-086).
func GetCaptureOp(ctx context.Context, db DBTX, opID string) (unitID, status, reason string, found bool, err error) {
	var r sql.NullString
	err = db.QueryRowContext(ctx, `SELECT unit_id, status, reason FROM capture_ops WHERE op_id = ?`, opID).Scan(&unitID, &status, &r)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", "", "", false, nil
	case err != nil:
		return "", "", "", false, err
	}
	return unitID, status, r.String, true, nil
}

// RecordCaptureOp writes the terminal decision for an op id. A duplicate op id
// (a racing double-record) is ignored: the first decision stands, which is
// exactly the idempotency SYS-085 requires.
func RecordCaptureOp(ctx context.Context, db DBTX, opID, unitID, status, reason string) error {
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO capture_ops (op_id, unit_id, status, reason)
		VALUES (?, ?, ?, nullif(?, ''))`, opID, unitID, status, reason)
	if err != nil {
		return fmt.Errorf("record capture op %s: %w", opID, err)
	}
	return nil
}

// ReconciliationItem is one capture that could not be applied, awaiting an
// office apply/discard decision (SYS-086).
type ReconciliationItem struct {
	ID               string
	OpID             string
	UnitID           string
	AthleteID        string
	Reason           string
	Payload          string
	CapturedBy       string
	DeviceLabel      string
	Generation       int64
	StartListVersion int64
	Status           string
	CreatedAt        time.Time
	ResolvedAt       *time.Time
	ResolvedBy       string
}

// CreateReconciliationItem inserts a pending reconciliation item with a fresh
// ID.
func CreateReconciliationItem(ctx context.Context, db DBTX, it ReconciliationItem) (ReconciliationItem, error) {
	it.ID = NewID()
	if it.Status == "" {
		it.Status = "pending"
	}
	_, err := db.ExecContext(ctx, `INSERT INTO reconciliation_items
		(id, op_id, unit_id, athlete_id, reason, payload_json, captured_by, device_label,
		 generation, start_list_version, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, it.OpID, it.UnitID, it.AthleteID, it.Reason, it.Payload, it.CapturedBy,
		it.DeviceLabel, it.Generation, it.StartListVersion, it.Status)
	if err != nil {
		return ReconciliationItem{}, fmt.Errorf("create reconciliation item: %w", err)
	}
	return it, nil
}

// GetReconciliationItem returns one item by ID.
func GetReconciliationItem(ctx context.Context, db DBTX, id string) (ReconciliationItem, error) {
	row := db.QueryRowContext(ctx, `SELECT id, op_id, unit_id, athlete_id, reason, payload_json,
		captured_by, device_label, generation, start_list_version, status, created_at,
		coalesce(resolved_at, ''), coalesce(resolved_by, '')
		FROM reconciliation_items WHERE id = ?`, id)
	return scanReconciliationRow(row)
}

// SetReconciliationStatus resolves an item (applied/discarded) and stamps who
// resolved it and when.
func SetReconciliationStatus(ctx context.Context, db DBTX, id, status, resolvedBy string) error {
	res, err := db.ExecContext(ctx, `UPDATE reconciliation_items
		SET status = ?, resolved_by = ?, resolved_at = `+nowExpr+`
		WHERE id = ? AND status = 'pending'`, status, resolvedBy, id)
	if err != nil {
		return fmt.Errorf("resolve reconciliation item %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("reconciliation item %s: not pending: %w", id, ErrNotFound)
	}
	return nil
}

// ReconciliationRow is a pending item joined to its unit's discipline for the
// office view.
type ReconciliationRow struct {
	ReconciliationItem
	DisciplineCode string
}

// ListPendingReconciliation returns a meet's pending reconciliation items,
// grouped-ready (ordered by unit then capture time), joined to discipline.
func ListPendingReconciliation(ctx context.Context, db DBTX, meetID string) ([]ReconciliationRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT ri.id, ri.op_id, ri.unit_id, ri.athlete_id, ri.reason,
		ri.payload_json, ri.captured_by, ri.device_label, ri.generation, ri.start_list_version,
		ri.status, ri.created_at, coalesce(ri.resolved_at, ''), coalesce(ri.resolved_by, ''),
		e.discipline_code
		FROM reconciliation_items ri
		JOIN units u ON u.id = ri.unit_id
		JOIN rounds r ON r.id = u.round_id
		JOIN events e ON e.id = r.event_id
		WHERE e.meet_id = ? AND ri.status = 'pending'
		ORDER BY ri.unit_id, ri.created_at, ri.id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReconciliationRow
	for rows.Next() {
		var rr ReconciliationRow
		it, err := scanReconciliation(rows, &rr.DisciplineCode)
		if err != nil {
			return nil, err
		}
		rr.ReconciliationItem = it
		out = append(out, rr)
	}
	return out, rows.Err()
}

// rowScanner unifies *sql.Row and *sql.Rows for the reconciliation scanners.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanReconciliationRow(row rowScanner) (ReconciliationItem, error) {
	return scanReconciliation(row, nil)
}

func scanReconciliation(row rowScanner, disciplineCode *string) (ReconciliationItem, error) {
	var it ReconciliationItem
	var created, resolvedAt string
	dest := []any{&it.ID, &it.OpID, &it.UnitID, &it.AthleteID, &it.Reason, &it.Payload,
		&it.CapturedBy, &it.DeviceLabel, &it.Generation, &it.StartListVersion, &it.Status,
		&created, &resolvedAt, &it.ResolvedBy}
	if disciplineCode != nil {
		dest = append(dest, disciplineCode)
	}
	if err := row.Scan(dest...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReconciliationItem{}, ErrNotFound
		}
		return ReconciliationItem{}, err
	}
	var err error
	if it.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return ReconciliationItem{}, fmt.Errorf("reconciliation %s: bad created_at %q: %w", it.ID, created, err)
	}
	if resolvedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, resolvedAt)
		if err != nil {
			return ReconciliationItem{}, fmt.Errorf("reconciliation %s: bad resolved_at %q: %w", it.ID, resolvedAt, err)
		}
		it.ResolvedAt = &t
	}
	return it, nil
}
