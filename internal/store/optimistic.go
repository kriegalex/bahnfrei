// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrVersionConflict means the row changed since it was read: the caller must
// surface the conflict for resolution, never overwrite (ADR-004 §2, SYS-083).
var ErrVersionConflict = errors.New("version conflict: entity was modified concurrently")

// ErrNotFound means the target row does not exist.
var ErrNotFound = errors.New("entity not found")

// Set is one column assignment for OptimisticUpdate.
type Set struct {
	Column string
	Value  any
}

var sqlIdent = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// OptimisticUpdate applies assignments to the row of table whose id matches,
// but only if its version column still equals expected; on success the row's
// version is incremented and the new version returned. Tables using this
// helper need `id` (TEXT PRIMARY KEY) and `version` (INTEGER NOT NULL)
// columns. Identifiers are validated because they are interpolated.
func OptimisticUpdate(ctx context.Context, db DBTX, table, id string, expected int64, sets ...Set) (int64, error) {
	if !sqlIdent.MatchString(table) {
		return 0, fmt.Errorf("invalid table identifier %q", table)
	}
	if len(sets) == 0 {
		return 0, errors.New("no assignments")
	}
	assign := make([]string, 0, len(sets)+1)
	args := make([]any, 0, len(sets)+2)
	for _, s := range sets {
		if !sqlIdent.MatchString(s.Column) || s.Column == "id" || s.Column == "version" {
			return 0, fmt.Errorf("invalid column identifier %q", s.Column)
		}
		assign = append(assign, s.Column+" = ?")
		args = append(args, s.Value)
	}
	assign = append(assign, "version = version + 1")
	args = append(args, id, expected)

	res, err := db.ExecContext(ctx, fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = ? AND version = ?",
		table, strings.Join(assign, ", ")), args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		return expected + 1, nil
	}

	// Nothing updated: distinguish "gone" from "moved on".
	var current int64
	err = db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT version FROM %s WHERE id = ?", table), id).Scan(&current)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("%s id=%s: %w", table, id, ErrNotFound)
	case err != nil:
		return 0, err
	default:
		return current, fmt.Errorf("%s id=%s: expected version %d, found %d: %w",
			table, id, expected, current, ErrVersionConflict)
	}
}
