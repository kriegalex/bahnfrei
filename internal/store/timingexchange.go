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

// ListRoundUnits returns a round's units in stable (creation/id) order —
// the same ordering EnsureRoundUnitCount (seeding.go) relies on, reused
// here so the timing-exchange numbering assignment (SYS-060) sees heats in
// the same order the seeding sheet does.
func ListRoundUnits(ctx context.Context, db DBTX, roundID string) ([]UnitRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, round_id, scheduled_at, location, version
		FROM units WHERE round_id = ? ORDER BY id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnitRecord
	for rows.Next() {
		var u UnitRecord
		var sched sql.NullString
		if err := rows.Scan(&u.ID, &u.RoundID, &sched, &u.Location, &u.Version); err != nil {
			return nil, err
		}
		if sched.Valid {
			t, err := time.Parse(instantFormat, sched.String)
			if err != nil {
				return nil, fmt.Errorf("unit %s: bad scheduled_at %q: %w", u.ID, sched.String, err)
			}
			u.ScheduledAt = t
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// --- timing_unit_numbers: stable FinishLynx event/round/heat numeric
// handles (TASK-020, SYS-060; see 0018_timing_exchange.sql for why these
// are assigned once and never recomputed). ---

// TimingUnitNumbers is a unit's assigned FinishLynx numeric identity.
type TimingUnitNumbers struct {
	UnitID      string
	EventNumber int
	RoundNumber int
	HeatNumber  int
}

// AssignTimingUnitNumbers records unitID's numbers the first time it is
// exported. Calling it again for the same unitID is a no-op returning the
// existing assignment (idempotent — a caller does not need to check
// existence first).
func AssignTimingUnitNumbers(ctx context.Context, db DBTX, unitID string, event, round, heat int) (TimingUnitNumbers, error) {
	if existing, err := GetTimingUnitNumbers(ctx, db, unitID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return TimingUnitNumbers{}, err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO timing_unit_numbers
		(unit_id, event_number, round_number, heat_number) VALUES (?, ?, ?, ?)`,
		unitID, event, round, heat)
	if err != nil {
		return TimingUnitNumbers{}, fmt.Errorf("assign timing unit numbers for unit %s: %w", unitID, err)
	}
	return TimingUnitNumbers{UnitID: unitID, EventNumber: event, RoundNumber: round, HeatNumber: heat}, nil
}

// GetTimingUnitNumbers looks up a unit's assigned numbers.
func GetTimingUnitNumbers(ctx context.Context, db DBTX, unitID string) (TimingUnitNumbers, error) {
	var n TimingUnitNumbers
	n.UnitID = unitID
	err := db.QueryRowContext(ctx, `SELECT event_number, round_number, heat_number
		FROM timing_unit_numbers WHERE unit_id = ?`, unitID).Scan(&n.EventNumber, &n.RoundNumber, &n.HeatNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return TimingUnitNumbers{}, ErrNotFound
	}
	if err != nil {
		return TimingUnitNumbers{}, err
	}
	return n, nil
}

// FindUnitByTimingNumbers resolves a unit from the (event, round, heat)
// triple a .lif/.evt file header carries (SYS-061's "matching them to the
// correct unit"). Returns ErrNotFound if no unit has been assigned that
// triple yet.
func FindUnitByTimingNumbers(ctx context.Context, db DBTX, meetID string, event, round, heat int) (string, error) {
	var unitID string
	err := db.QueryRowContext(ctx, `SELECT tn.unit_id
		FROM timing_unit_numbers tn
		JOIN units u ON u.id = tn.unit_id
		JOIN rounds r ON r.id = u.round_id
		JOIN events e ON e.id = r.event_id
		WHERE e.meet_id = ? AND tn.event_number = ? AND tn.round_number = ? AND tn.heat_number = ?`,
		meetID, event, round, heat).Scan(&unitID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return unitID, nil
}

// ListTimingUnitNumbersForMeet returns every unit of meetID that already
// has assigned numbers, keyed by unit id — the export path's "has this
// unit been numbered yet" lookup.
func ListTimingUnitNumbersForMeet(ctx context.Context, db DBTX, meetID string) (map[string]TimingUnitNumbers, error) {
	rows, err := db.QueryContext(ctx, `SELECT tn.unit_id, tn.event_number, tn.round_number, tn.heat_number
		FROM timing_unit_numbers tn
		JOIN units u ON u.id = tn.unit_id
		JOIN rounds r ON r.id = u.round_id
		JOIN events e ON e.id = r.event_id
		WHERE e.meet_id = ?`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]TimingUnitNumbers{}
	for rows.Next() {
		var n TimingUnitNumbers
		if err := rows.Scan(&n.UnitID, &n.EventNumber, &n.RoundNumber, &n.HeatNumber); err != nil {
			return nil, err
		}
		out[n.UnitID] = n
	}
	return out, rows.Err()
}

// --- timing_import_batches / timing_import_conflicts (TASK-020, SYS-061,
// UC-014 #4/#5). ---

// TimingImportBatch is one ingested .lif/.csv file.
type TimingImportBatch struct {
	ID         string
	MeetID     string
	Format     string
	Filename   string
	UnitID     string // "" if the file's unit could not be resolved
	Applied    int
	Conflicted int
	ImportedBy string
	CreatedAt  time.Time
}

// CreateTimingImportBatch records one ingest run.
func CreateTimingImportBatch(ctx context.Context, db DBTX, b TimingImportBatch) (TimingImportBatch, error) {
	b.ID = NewID()
	var unitID any
	if b.UnitID != "" {
		unitID = b.UnitID
	}
	_, err := db.ExecContext(ctx, `INSERT INTO timing_import_batches
		(id, meet_id, format, filename, unit_id, applied, conflicted, imported_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.MeetID, b.Format, b.Filename, unitID, b.Applied, b.Conflicted, b.ImportedBy)
	if err != nil {
		return TimingImportBatch{}, fmt.Errorf("create timing import batch: %w", err)
	}
	return GetTimingImportBatch(ctx, db, b.ID)
}

// GetTimingImportBatch looks up one batch.
func GetTimingImportBatch(ctx context.Context, db DBTX, id string) (TimingImportBatch, error) {
	return scanTimingImportBatch(db.QueryRowContext(ctx, `SELECT id, meet_id, format, filename,
		COALESCE(unit_id, ''), applied, conflicted, imported_by, created_at
		FROM timing_import_batches WHERE id = ?`, id))
}

// ListTimingImportBatches returns a meet's ingest history, most recent
// first (the office's "what came in" view).
func ListTimingImportBatches(ctx context.Context, db DBTX, meetID string) ([]TimingImportBatch, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, meet_id, format, filename,
		COALESCE(unit_id, ''), applied, conflicted, imported_by, created_at
		FROM timing_import_batches WHERE meet_id = ? ORDER BY created_at DESC, id DESC`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimingImportBatch
	for rows.Next() {
		b, err := scanTimingImportBatchRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanTimingImportBatch(row *sql.Row) (TimingImportBatch, error) {
	var b TimingImportBatch
	var created string
	err := row.Scan(&b.ID, &b.MeetID, &b.Format, &b.Filename, &b.UnitID, &b.Applied, &b.Conflicted, &b.ImportedBy, &created)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return TimingImportBatch{}, ErrNotFound
	case err != nil:
		return TimingImportBatch{}, err
	}
	b.CreatedAt, err = time.Parse(instantFormat, created)
	if err != nil {
		return TimingImportBatch{}, fmt.Errorf("batch %s: bad created_at %q: %w", b.ID, created, err)
	}
	return b, nil
}

func scanTimingImportBatchRow(rows *sql.Rows) (TimingImportBatch, error) {
	var b TimingImportBatch
	var created string
	if err := rows.Scan(&b.ID, &b.MeetID, &b.Format, &b.Filename, &b.UnitID, &b.Applied, &b.Conflicted, &b.ImportedBy, &created); err != nil {
		return TimingImportBatch{}, err
	}
	var err error
	b.CreatedAt, err = time.Parse(instantFormat, created)
	if err != nil {
		return TimingImportBatch{}, fmt.Errorf("batch %s: bad created_at %q: %w", b.ID, created, err)
	}
	return b, nil
}

// TimingImportConflict is one unresolved (or resolved) row from an ingest
// batch (UC-014 #4/#5): never silently applied or dropped.
type TimingImportConflict struct {
	ID          string
	BatchID     string
	UnitID      string
	AthleteID   string
	Bib         string
	Lane        int
	Reason      string
	PayloadJSON string
	Status      string
	CreatedAt   time.Time
	ResolvedAt  *time.Time
	ResolvedBy  string
}

// CreateTimingImportConflict queues one unresolved row.
func CreateTimingImportConflict(ctx context.Context, db DBTX, c TimingImportConflict) (TimingImportConflict, error) {
	c.ID = NewID()
	if c.Status == "" {
		c.Status = "pending"
	}
	var unitID, athleteID any
	if c.UnitID != "" {
		unitID = c.UnitID
	}
	if c.AthleteID != "" {
		athleteID = c.AthleteID
	}
	_, err := db.ExecContext(ctx, `INSERT INTO timing_import_conflicts
		(id, batch_id, unit_id, athlete_id, bib, lane, reason, payload_json, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.BatchID, unitID, athleteID, c.Bib, c.Lane, c.Reason, c.PayloadJSON, c.Status)
	if err != nil {
		return TimingImportConflict{}, fmt.Errorf("create timing import conflict: %w", err)
	}
	return GetTimingImportConflict(ctx, db, c.ID)
}

// GetTimingImportConflict looks up one conflict row.
func GetTimingImportConflict(ctx context.Context, db DBTX, id string) (TimingImportConflict, error) {
	return scanTimingImportConflict(db.QueryRowContext(ctx, `SELECT id, batch_id,
		COALESCE(unit_id, ''), COALESCE(athlete_id, ''), bib, lane, reason, payload_json, status,
		created_at, resolved_at, COALESCE(resolved_by, '')
		FROM timing_import_conflicts WHERE id = ?`, id))
}

// ListTimingImportConflicts returns a meet's conflicts across every batch,
// pending first, most recent first — the office's resolution queue
// (UC-014 #4/#5).
func ListTimingImportConflicts(ctx context.Context, db DBTX, meetID string) ([]TimingImportConflict, error) {
	rows, err := db.QueryContext(ctx, `SELECT c.id, c.batch_id,
		COALESCE(c.unit_id, ''), COALESCE(c.athlete_id, ''), c.bib, c.lane, c.reason, c.payload_json, c.status,
		c.created_at, c.resolved_at, COALESCE(c.resolved_by, '')
		FROM timing_import_conflicts c
		JOIN timing_import_batches b ON b.id = c.batch_id
		WHERE b.meet_id = ?
		ORDER BY (c.status = 'pending') DESC, c.created_at DESC, c.id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimingImportConflict
	for rows.Next() {
		c, err := scanTimingImportConflictRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ResolveTimingImportConflict marks a conflict resolved (kept/replaced/
// merged/discarded — never a second "pending", resolution is one-shot).
func ResolveTimingImportConflict(ctx context.Context, db DBTX, id, status, resolvedBy string) error {
	res, err := db.ExecContext(ctx, `UPDATE timing_import_conflicts
		SET status = ?, resolved_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'), resolved_by = ?
		WHERE id = ? AND status = 'pending'`, status, resolvedBy, id)
	if err != nil {
		return fmt.Errorf("resolve timing import conflict %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("conflict %s: %w (already resolved, or does not exist)", id, ErrNotFound)
	}
	return nil
}

func scanTimingImportConflict(row *sql.Row) (TimingImportConflict, error) {
	var c TimingImportConflict
	var created string
	var resolved sql.NullString
	err := row.Scan(&c.ID, &c.BatchID, &c.UnitID, &c.AthleteID, &c.Bib, &c.Lane, &c.Reason,
		&c.PayloadJSON, &c.Status, &created, &resolved, &c.ResolvedBy)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return TimingImportConflict{}, ErrNotFound
	case err != nil:
		return TimingImportConflict{}, err
	}
	return decodeTimingImportConflict(c, created, resolved)
}

func scanTimingImportConflictRow(rows *sql.Rows) (TimingImportConflict, error) {
	var c TimingImportConflict
	var created string
	var resolved sql.NullString
	if err := rows.Scan(&c.ID, &c.BatchID, &c.UnitID, &c.AthleteID, &c.Bib, &c.Lane, &c.Reason,
		&c.PayloadJSON, &c.Status, &created, &resolved, &c.ResolvedBy); err != nil {
		return TimingImportConflict{}, err
	}
	return decodeTimingImportConflict(c, created, resolved)
}

func decodeTimingImportConflict(c TimingImportConflict, created string, resolved sql.NullString) (TimingImportConflict, error) {
	var err error
	c.CreatedAt, err = time.Parse(instantFormat, created)
	if err != nil {
		return TimingImportConflict{}, fmt.Errorf("conflict %s: bad created_at %q: %w", c.ID, created, err)
	}
	if resolved.Valid {
		t, err := time.Parse(instantFormat, resolved.String)
		if err != nil {
			return TimingImportConflict{}, fmt.Errorf("conflict %s: bad resolved_at %q: %w", c.ID, resolved.String, err)
		}
		c.ResolvedAt = &t
	}
	return c, nil
}

// --- timing_agent_tokens (TASK-020, ADR-006's hub-first timing-agent
// amendment). ---

// TimingAgentToken is a persisted agent credential (never the plaintext
// token itself — see CreateTimingAgentToken).
type TimingAgentToken struct {
	ID        string
	MeetID    string
	Label     string
	TokenHash string
	CreatedBy string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// ErrDuplicateTimingAgentToken means the generated token's hash already
// exists (astronomically unlikely for a 256-bit random token; the caller
// generates a fresh one and retries — see cmd/bahnfrei/timingagent.go's
// caller in internal/app/exchange.go).
var ErrDuplicateTimingAgentToken = errors.New("timing agent token hash collision")

// CreateTimingAgentToken stores a new agent credential's hash (the caller
// generates and hands the plaintext token to the operator exactly once —
// this function never sees or returns it).
func CreateTimingAgentToken(ctx context.Context, db DBTX, t TimingAgentToken) (TimingAgentToken, error) {
	t.ID = NewID()
	_, err := db.ExecContext(ctx, `INSERT INTO timing_agent_tokens
		(id, meet_id, label, token_hash, created_by) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.MeetID, t.Label, t.TokenHash, t.CreatedBy)
	if err != nil {
		if isUniqueViolation(err) {
			return TimingAgentToken{}, ErrDuplicateTimingAgentToken
		}
		return TimingAgentToken{}, fmt.Errorf("create timing agent token: %w", err)
	}
	return GetTimingAgentTokenByID(ctx, db, t.ID)
}

// GetTimingAgentTokenByID looks up a token row by its own id (management
// views — list/revoke).
func GetTimingAgentTokenByID(ctx context.Context, db DBTX, id string) (TimingAgentToken, error) {
	return scanTimingAgentToken(db.QueryRowContext(ctx, `SELECT id, meet_id, label, token_hash, created_by, created_at, revoked_at
		FROM timing_agent_tokens WHERE id = ?`, id))
}

// GetTimingAgentTokenByHash resolves a presented bearer token's hash to its
// row (the agent-auth middleware's lookup) — revoked tokens are excluded,
// so a revoked credential authenticates nothing.
func GetTimingAgentTokenByHash(ctx context.Context, db DBTX, tokenHash string) (TimingAgentToken, error) {
	return scanTimingAgentToken(db.QueryRowContext(ctx, `SELECT id, meet_id, label, token_hash, created_by, created_at, revoked_at
		FROM timing_agent_tokens WHERE token_hash = ? AND revoked_at IS NULL`, tokenHash))
}

// ListTimingAgentTokens returns every token issued for a meet, including
// revoked ones (the audit-visible management view never hides history).
func ListTimingAgentTokens(ctx context.Context, db DBTX, meetID string) ([]TimingAgentToken, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, meet_id, label, token_hash, created_by, created_at, revoked_at
		FROM timing_agent_tokens WHERE meet_id = ? ORDER BY created_at DESC, id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimingAgentToken
	for rows.Next() {
		t, err := scanTimingAgentTokenRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeTimingAgentToken disables a token immediately (idempotent —
// revoking an already-revoked token is a no-op, not an error).
func RevokeTimingAgentToken(ctx context.Context, db DBTX, id string) error {
	_, err := db.ExecContext(ctx, `UPDATE timing_agent_tokens
		SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ? AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("revoke timing agent token %s: %w", id, err)
	}
	return nil
}

func scanTimingAgentToken(row *sql.Row) (TimingAgentToken, error) {
	var t TimingAgentToken
	var created string
	var revoked sql.NullString
	err := row.Scan(&t.ID, &t.MeetID, &t.Label, &t.TokenHash, &t.CreatedBy, &created, &revoked)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return TimingAgentToken{}, ErrNotFound
	case err != nil:
		return TimingAgentToken{}, err
	}
	return decodeTimingAgentToken(t, created, revoked)
}

func scanTimingAgentTokenRow(rows *sql.Rows) (TimingAgentToken, error) {
	var t TimingAgentToken
	var created string
	var revoked sql.NullString
	if err := rows.Scan(&t.ID, &t.MeetID, &t.Label, &t.TokenHash, &t.CreatedBy, &created, &revoked); err != nil {
		return TimingAgentToken{}, err
	}
	return decodeTimingAgentToken(t, created, revoked)
}

func decodeTimingAgentToken(t TimingAgentToken, created string, revoked sql.NullString) (TimingAgentToken, error) {
	var err error
	t.CreatedAt, err = time.Parse(instantFormat, created)
	if err != nil {
		return TimingAgentToken{}, fmt.Errorf("agent token %s: bad created_at %q: %w", t.ID, created, err)
	}
	if revoked.Valid {
		rt, err := time.Parse(instantFormat, revoked.String)
		if err != nil {
			return TimingAgentToken{}, fmt.Errorf("agent token %s: bad revoked_at %q: %w", t.ID, revoked.String, err)
		}
		t.RevokedAt = &rt
	}
	return t, nil
}
