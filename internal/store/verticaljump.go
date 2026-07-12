// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// VerticalTrialRecord is a persisted domain.VerticalTrial within its unit,
// plus its optimistic-concurrency version.
type VerticalTrialRecord struct {
	domain.VerticalTrial
	ID        string
	UnitID    string
	AthleteID string
	Version   int64
}

// VerticalUnitConfig is a unit's configured bar-height progression
// (SYS-043: "office-configurable, with defaults per spec"), ascending, plus
// its optimistic-concurrency version.
type VerticalUnitConfig struct {
	UnitID  string
	Heights []string
	Version int64
}

// GetVerticalUnitConfig returns unitID's configured height progression, if
// one has been set.
func GetVerticalUnitConfig(ctx context.Context, db DBTX, unitID string) (VerticalUnitConfig, error) {
	var heightsJSON string
	cfg := VerticalUnitConfig{UnitID: unitID}
	err := db.QueryRowContext(ctx, `SELECT heights, version FROM vertical_unit_config WHERE unit_id = ?`, unitID).
		Scan(&heightsJSON, &cfg.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return VerticalUnitConfig{}, ErrNotFound
	case err != nil:
		return VerticalUnitConfig{}, err
	}
	if err := json.Unmarshal([]byte(heightsJSON), &cfg.Heights); err != nil {
		return VerticalUnitConfig{}, fmt.Errorf("vertical unit config %s: decode heights: %w", unitID, err)
	}
	return cfg, nil
}

// SetVerticalUnitConfig writes unitID's height progression: expectedVersion
// 0 inserts a new configuration, a positive value updates an existing one
// under optimistic concurrency (SYS-043's office-configurable progression,
// including appending a jump-off height to an already-configured unit).
func SetVerticalUnitConfig(ctx context.Context, db DBTX, unitID string, heights []string, expectedVersion int64) (VerticalUnitConfig, error) {
	if unitID == "" {
		return VerticalUnitConfig{}, errors.New("vertical unit config: unit id is required")
	}
	if len(heights) == 0 {
		return VerticalUnitConfig{}, errors.New("vertical unit config: at least one height is required")
	}
	heightsJSON, err := json.Marshal(heights)
	if err != nil {
		return VerticalUnitConfig{}, err
	}
	if expectedVersion > 0 {
		// vertical_unit_config is keyed by unit_id, not id, so
		// OptimisticUpdate (which hardcodes an "id" primary key) does not
		// apply — the same compare-and-swap written out directly.
		res, err := db.ExecContext(ctx,
			`UPDATE vertical_unit_config SET heights = ?, version = version + 1 WHERE unit_id = ? AND version = ?`,
			string(heightsJSON), unitID, expectedVersion)
		if err != nil {
			return VerticalUnitConfig{}, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return VerticalUnitConfig{}, err
		}
		if n != 1 {
			current, getErr := GetVerticalUnitConfig(ctx, db, unitID)
			if errors.Is(getErr, ErrNotFound) {
				return VerticalUnitConfig{}, fmt.Errorf("vertical unit config unit_id=%s: %w", unitID, ErrNotFound)
			}
			if getErr != nil {
				return VerticalUnitConfig{}, getErr
			}
			return VerticalUnitConfig{}, fmt.Errorf("vertical unit config unit_id=%s: expected version %d, found %d: %w",
				unitID, expectedVersion, current.Version, ErrVersionConflict)
		}
		return VerticalUnitConfig{UnitID: unitID, Heights: heights, Version: expectedVersion + 1}, nil
	}
	_, err = db.ExecContext(ctx, `INSERT INTO vertical_unit_config (unit_id, heights, version) VALUES (?, ?, 1)
		ON CONFLICT(unit_id) DO UPDATE SET heights = excluded.heights, version = vertical_unit_config.version + 1`,
		unitID, string(heightsJSON))
	if err != nil {
		return VerticalUnitConfig{}, fmt.Errorf("set vertical unit config %s: %w", unitID, err)
	}
	return GetVerticalUnitConfig(ctx, db, unitID)
}

// SaveVerticalTrial writes one trial of a vertical-jump event (SYS-043).
// expectedVersion 0 inserts a new trial; a positive value re-captures an
// existing one under optimistic concurrency, mirroring SaveAttempt's
// conflict handling (UC-021 #2, SYS-086).
func SaveVerticalTrial(ctx context.Context, db DBTX, unitID, athleteID string, trial domain.VerticalTrial, expectedVersion int64) (VerticalTrialRecord, error) {
	if unitID == "" || athleteID == "" {
		return VerticalTrialRecord{}, errors.New("vertical trial: unit id and athlete id are required")
	}
	if expectedVersion > 0 {
		newVersion, err := OptimisticUpdate(ctx, db, "vertical_trials",
			verticalTrialID(ctx, db, unitID, athleteID, trial.HeightIdx, trial.Seq), expectedVersion,
			Set{Column: "kind", Value: string(trial.Kind)})
		if err != nil {
			return verticalTrialConflict(ctx, db, unitID, athleteID, trial.HeightIdx, trial.Seq, err)
		}
		rec, err := GetVerticalTrial(ctx, db, unitID, athleteID, trial.HeightIdx, trial.Seq)
		if err != nil {
			return VerticalTrialRecord{}, err
		}
		rec.Version = newVersion
		return rec, nil
	}

	rec := VerticalTrialRecord{VerticalTrial: trial, ID: NewID(), UnitID: unitID, AthleteID: athleteID, Version: 1}
	_, err := db.ExecContext(ctx, `INSERT INTO vertical_trials
		(id, unit_id, athlete_id, height_idx, seq, kind, version)
		VALUES (?, ?, ?, ?, ?, ?, 1)`,
		rec.ID, unitID, athleteID, trial.HeightIdx, trial.Seq, string(trial.Kind))
	if err != nil {
		if isUniqueViolation(err) {
			return verticalTrialConflict(ctx, db, unitID, athleteID, trial.HeightIdx, trial.Seq, ErrVersionConflict)
		}
		return VerticalTrialRecord{}, fmt.Errorf("save vertical trial unit %s athlete %s height %d trial %d: %w",
			unitID, athleteID, trial.HeightIdx, trial.Seq, err)
	}
	return rec, nil
}

func verticalTrialID(ctx context.Context, db DBTX, unitID, athleteID string, heightIdx, seq int) string {
	var id string
	_ = db.QueryRowContext(ctx, `SELECT id FROM vertical_trials
		WHERE unit_id = ? AND athlete_id = ? AND height_idx = ? AND seq = ?`,
		unitID, athleteID, heightIdx, seq).Scan(&id)
	return id
}

func verticalTrialConflict(ctx context.Context, db DBTX, unitID, athleteID string, heightIdx, seq int, cause error) (VerticalTrialRecord, error) {
	current, err := GetVerticalTrial(ctx, db, unitID, athleteID, heightIdx, seq)
	if err != nil {
		return VerticalTrialRecord{}, cause
	}
	return current, fmt.Errorf("vertical trial unit %s athlete %s height %d trial %d: %w", unitID, athleteID, heightIdx, seq, cause)
}

// GetVerticalTrial returns the stored trial (unit, athlete, height, seq).
func GetVerticalTrial(ctx context.Context, db DBTX, unitID, athleteID string, heightIdx, seq int) (VerticalTrialRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, height_idx, seq, kind, version
		FROM vertical_trials WHERE unit_id = ? AND athlete_id = ? AND height_idx = ? AND seq = ?`,
		unitID, athleteID, heightIdx, seq)
	if err != nil {
		return VerticalTrialRecord{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return VerticalTrialRecord{}, err
		}
		return VerticalTrialRecord{}, ErrNotFound
	}
	return scanVerticalTrial(rows)
}

// ListUnitVerticalTrials returns every trial captured for a unit, per
// athlete in height-then-seq order.
func ListUnitVerticalTrials(ctx context.Context, db DBTX, unitID string) ([]VerticalTrialRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, height_idx, seq, kind, version
		FROM vertical_trials WHERE unit_id = ? ORDER BY athlete_id, height_idx, seq`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VerticalTrialRecord
	for rows.Next() {
		rec, err := scanVerticalTrial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanVerticalTrial(rows *sql.Rows) (VerticalTrialRecord, error) {
	var rec VerticalTrialRecord
	var kind string
	if err := rows.Scan(&rec.ID, &rec.UnitID, &rec.AthleteID, &rec.HeightIdx, &rec.Seq, &kind, &rec.Version); err != nil {
		return VerticalTrialRecord{}, err
	}
	rec.Kind = domain.QualificationStatus(kind)
	return rec, nil
}
