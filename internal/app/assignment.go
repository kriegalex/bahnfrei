// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// ErrNotFieldOfficial means the target account does not carry
// RoleFieldOfficial, so per-event scoping does not apply to it (TASK-013,
// SYS-090).
var ErrNotFieldOfficial = fmt.Errorf("account is not a field official")

// AssignFieldOfficialUnit grants accountID capture access to unitID within
// meetID (SYS-090's "assignable per meet"): office level and above may make
// the grant (CapAssignUnits), and the target account must carry
// RoleFieldOfficial — scoping a higher role would be a no-op that could
// mislead an operator into thinking it restricts anything. Idempotent;
// audited (SYS-046, UC-022 #2).
func (s *ResultsService) AssignFieldOfficialUnit(ctx context.Context, actor Session, meetID, unitID, accountID string) error {
	if err := Authorize(actor.Role, CapAssignUnits); err != nil {
		return err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return err
	}
	acct, err := store.GetAccountByID(ctx, s.db, accountID)
	if err != nil {
		return err
	}
	if acct.Role != string(RoleFieldOfficial) {
		return ErrNotFieldOfficial
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("assign field official unit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := store.AssignFieldOfficialUnit(ctx, tx, accountID, meetID, unitID); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]string{"account": accountID, "unit": unitID, "meet": meetID})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "field_official.assign",
		EntityType: "field_official_unit", EntityID: accountID + ":" + unitID,
		After: string(after),
	}); err != nil {
		return fmt.Errorf("audit field_official.assign: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("assign field official unit: %w", err)
	}
	return nil
}

// UnassignFieldOfficialUnit revokes accountID's capture access to unitID
// (SYS-090; office level and above). Audited (SYS-046, UC-022 #2).
func (s *ResultsService) UnassignFieldOfficialUnit(ctx context.Context, actor Session, meetID, unitID, accountID string) error {
	if err := Authorize(actor.Role, CapAssignUnits); err != nil {
		return err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("unassign field official unit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := store.UnassignFieldOfficialUnit(ctx, tx, accountID, unitID); err != nil {
		return err
	}
	before, _ := json.Marshal(map[string]string{"account": accountID, "unit": unitID, "meet": meetID})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "field_official.unassign",
		EntityType: "field_official_unit", EntityID: accountID + ":" + unitID,
		Before: string(before),
	}); err != nil {
		return fmt.Errorf("audit field_official.unassign: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("unassign field official unit: %w", err)
	}
	return nil
}

// FieldOfficialRow is one field-official account's assignment state for the
// office's assignment matrix (TASK-013): which of the meet's capturable
// units it currently holds a grant for.
type FieldOfficialRow struct {
	AccountID   string
	Username    string
	DisplayName string
	// AssignedUnitIDs is the set of CaptureUnits.UnitID this account is
	// currently granted for this meet.
	AssignedUnitIDs map[string]bool
}

// FieldOfficialAssignments returns every field-official account (instance-
// wide — accounts are not meet-scoped) alongside its current per-unit grants
// for meetID, for the office's assignment matrix UI (SYS-090, CapAssignUnits).
func (s *ResultsService) FieldOfficialAssignments(ctx context.Context, actor Session, meetID string) ([]FieldOfficialRow, error) {
	if err := Authorize(actor.Role, CapAssignUnits); err != nil {
		return nil, err
	}
	accounts, err := store.ListAccounts(ctx, s.db)
	if err != nil {
		return nil, err
	}
	grants, err := store.ListFieldOfficialAssignments(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	byAccount := map[string]map[string]bool{}
	for _, g := range grants {
		if byAccount[g.AccountID] == nil {
			byAccount[g.AccountID] = map[string]bool{}
		}
		byAccount[g.AccountID][g.UnitID] = true
	}

	var out []FieldOfficialRow
	for _, acc := range accounts {
		if acc.Role != string(RoleFieldOfficial) {
			continue
		}
		row := FieldOfficialRow{AccountID: acc.ID, Username: acc.Username, DisplayName: acc.DisplayName}
		row.AssignedUnitIDs = byAccount[acc.ID]
		if row.AssignedUnitIDs == nil {
			row.AssignedUnitIDs = map[string]bool{}
		}
		out = append(out, row)
	}
	return out, nil
}
