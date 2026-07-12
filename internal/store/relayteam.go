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

// RelayTeamRecord is a persisted domain.RelayTeam plus its version.
type RelayTeamRecord struct {
	domain.RelayTeam
	Version int64
}

// CreateRelayTeam inserts a new relay team (SYS-012) with a fresh ID.
func CreateRelayTeam(ctx context.Context, db DBTX, t domain.RelayTeam) (RelayTeamRecord, error) {
	t.ID = NewID()
	if err := t.Validate(); err != nil {
		return RelayTeamRecord{}, err
	}
	comp, err := json.Marshal(t.Composition)
	if err != nil {
		return RelayTeamRecord{}, fmt.Errorf("encode relay composition: %w", err)
	}
	reserves, err := json.Marshal(orEmptySlice(t.Reserves))
	if err != nil {
		return RelayTeamRecord{}, fmt.Errorf("encode relay reserves: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO relay_teams
		(id, club_id, composition_json, reserves_json, version)
		VALUES (?, ?, ?, ?, 1)`, t.ID, t.ClubID, string(comp), string(reserves)); err != nil {
		return RelayTeamRecord{}, fmt.Errorf("create relay team for club %s: %w", t.ClubID, err)
	}
	return RelayTeamRecord{RelayTeam: t, Version: 1}, nil
}

// GetRelayTeam looks up one relay team by ID.
func GetRelayTeam(ctx context.Context, db DBTX, id string) (RelayTeamRecord, error) {
	return scanRelayTeam(db.QueryRowContext(ctx, `SELECT id, club_id, composition_json, reserves_json, version
		FROM relay_teams WHERE id = ?`, id))
}

// UpdateRelayTeamComposition replaces a relay team's leg composition and
// reserves under optimistic concurrency (SYS-012, UC-003 #4: "permits
// changes until the configured deadline" — deadline enforcement is the
// app layer's job, this is the storage write).
func UpdateRelayTeamComposition(ctx context.Context, db DBTX, id string, expectedVersion int64, composition, reserves []string) (int64, error) {
	comp, err := json.Marshal(orEmptySlice(composition))
	if err != nil {
		return 0, fmt.Errorf("encode relay composition: %w", err)
	}
	res, err := json.Marshal(orEmptySlice(reserves))
	if err != nil {
		return 0, fmt.Errorf("encode relay reserves: %w", err)
	}
	return OptimisticUpdate(ctx, db, "relay_teams", id, expectedVersion,
		Set{Column: "composition_json", Value: string(comp)},
		Set{Column: "reserves_json", Value: string(res)})
}

func scanRelayTeam(row *sql.Row) (RelayTeamRecord, error) {
	var t RelayTeamRecord
	var comp, reserves string
	err := row.Scan(&t.ID, &t.ClubID, &comp, &reserves, &t.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return RelayTeamRecord{}, ErrNotFound
	case err != nil:
		return RelayTeamRecord{}, err
	}
	if err := json.Unmarshal([]byte(comp), &t.Composition); err != nil {
		return RelayTeamRecord{}, fmt.Errorf("relay team %s: bad composition %q: %w", t.ID, comp, err)
	}
	if err := json.Unmarshal([]byte(reserves), &t.Reserves); err != nil {
		return RelayTeamRecord{}, fmt.Errorf("relay team %s: bad reserves %q: %w", t.ID, reserves, err)
	}
	return t, nil
}
