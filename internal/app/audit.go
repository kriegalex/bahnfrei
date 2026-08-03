// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// AuditEvent is one privileged action surfaced to office/organizer operators
// (TASK-013, SYS-091/UC-022 #2): who, when, what, on which entity, and any
// recorded reason. It is a read-only projection of the append-only
// audit_log (SYS-046) — nothing here can be written back.
type AuditEvent struct {
	Seq     int64
	TS      time.Time
	ActorID string
	// ActorName is the actor's current display name, resolved at read time;
	// it falls back to ActorID if the account is unknown (e.g. seeded
	// directly in a test, or a future erasure — SYS-101 — pseudonymizes
	// but never deletes the audit row itself, ADR-004 §3).
	ActorName  string
	Action     string
	EntityType string
	EntityID   string
	Reason     string
}

// privilegedAuditActions is the allow-list of audit_log action names
// surfaced by PrivilegedAuditLog (UC-022 #2: "role grant, override,
// correction"). Routine, high-frequency capture events (attempt.save,
// result.settle, result.capture, reconciliation.route, participant
// registration, meet/event/unit setup writes) are deliberately excluded:
// they are not privileged decisions, and including them would flood the
// office view and bury the actions this surface exists to make visible.
var privilegedAuditActions = map[string]bool{
	"account.create":                        true,
	"account.bootstrap":                     true,
	"account.disable":                       true,
	"account.enable":                        true,
	"account.role_change":                   true,
	"field_official.assign":                 true,
	"field_official.unassign":               true,
	"checkout.override":                     true,
	"checkout.revise_startlist":             true,
	"entry.eligibility_override":            true,
	"reconciliation.apply":                  true,
	"reconciliation.discard":                true,
	"result.save":                           true,
	"meet.update":                           true,
	"meet.archive":                          true,
	"timetable.publish":                     true,
	"capture.access_denied":                 true,
	"participant.out_of_competition.update": true,
}

// auditFetchWindow bounds how far back PrivilegedAuditLog scans the flat
// audit trail before filtering to the privileged subset; generous enough
// that routine capture traffic between two privileged actions does not
// starve the view, small enough to keep the query cheap (PoC scope).
const auditFetchWindow = 1000

// PrivilegedAuditLog returns the most recent privileged actions (account
// changes, role grants, result corrections, meet configuration changes,
// per-event scoping grants, and denied scoped-capture attempts), newest
// first, capped at limit rows — the office/organizer-visible surface over
// the append-only audit log (TASK-013, SYS-091, UC-022 #2).
func (a *AuthService) PrivilegedAuditLog(ctx context.Context, actor Session, limit int) ([]AuditEvent, error) {
	if err := Authorize(actor.Role, CapViewAudit); err != nil {
		return nil, err
	}
	entries, err := store.ListAudit(ctx, a.db, auditFetchWindow)
	if err != nil {
		return nil, fmt.Errorf("privileged audit log: %w", err)
	}
	accounts, err := store.ListAccounts(ctx, a.db)
	if err != nil {
		return nil, fmt.Errorf("privileged audit log: %w", err)
	}
	names := make(map[string]string, len(accounts))
	for _, acc := range accounts {
		names[acc.ID] = acc.DisplayName
	}

	var out []AuditEvent
	for _, e := range entries {
		if !privilegedAuditActions[e.Action] {
			continue
		}
		name := names[e.Actor]
		if name == "" {
			name = e.Actor
		}
		out = append(out, AuditEvent{
			Seq: e.Seq, TS: e.TS, ActorID: e.Actor, ActorName: name,
			Action: e.Action, EntityType: e.EntityType, EntityID: e.EntityID, Reason: e.Reason,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
