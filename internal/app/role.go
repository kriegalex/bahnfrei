// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package app hosts use-case services that orchestrate domain logic, storage
// and audit (architecture.md §3): authorization (SYS-090), session and
// credential handling (SYS-091). The web layer never touches internal/store
// directly — it calls through here.
package app

import "fmt"

// Role is one of the coarse, instance-wide RBAC roles enumerated by SYS-090.
// Per-meet capability scoping (assignable per meet, least privilege) is a
// later layer (TASK-013); this primitive is the shell it builds on.
type Role string

const (
	// RoleInstanceAdmin administers the whole instance: accounts, roles,
	// instance configuration.
	RoleInstanceAdmin Role = "instance_admin"
	// RoleMeetOrganizer sets up and runs meets end to end.
	RoleMeetOrganizer Role = "meet_organizer"
	// RoleCompetitionOffice runs day-of-competition office actions:
	// check-in, corrections, overrides.
	RoleCompetitionOffice Role = "competition_office"
	// RoleFieldOfficial captures results, scoped to assigned events.
	RoleFieldOfficial Role = "field_official"
	// RoleEntrySubmitter is a club/athlete submitting entries.
	RoleEntrySubmitter Role = "entry_submitter"
	// RolePublic is the unauthenticated public-read visitor.
	RolePublic Role = "public"
)

// ErrInvalidRole means the given string is not one of the enumerated roles.
type ErrInvalidRole struct{ Value string }

func (e ErrInvalidRole) Error() string { return fmt.Sprintf("invalid role %q", e.Value) }

// roleOrder ranks roles from least to most privileged for the coarse,
// instance-wide primitive this package implements. Per-meet nuance (a field
// official is not "less privileged" than an entry submitter in every
// dimension) is intentionally out of scope here — TASK-013 replaces this
// total order with real per-meet capability grants.
var roleOrder = map[Role]int{
	RolePublic:            0,
	RoleEntrySubmitter:    1,
	RoleFieldOfficial:     2,
	RoleCompetitionOffice: 3,
	RoleMeetOrganizer:     4,
	RoleInstanceAdmin:     5,
}

// ParseRole validates a role string, e.g. as read from storage or a request.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if _, ok := roleOrder[r]; !ok {
		return "", ErrInvalidRole{Value: s}
	}
	return r, nil
}

// Valid reports whether r is one of the enumerated roles.
func (r Role) Valid() bool {
	_, ok := roleOrder[r]
	return ok
}

// AtLeast reports whether r meets or exceeds the privilege of min on the
// coarse instance-wide ordering. Least-privilege capability checks
// (SYS-090) should prefer this over ad-hoc role equality checks.
func (r Role) AtLeast(min Role) bool {
	rv, ok1 := roleOrder[r]
	mv, ok2 := roleOrder[min]
	return ok1 && ok2 && rv >= mv
}

// Capability is a named privileged action, distinct from a role, so
// authorization checks read as "can this actor do X" (SYS-090) rather than
// "is this actor role Y" — the indirection is what lets TASK-013 later
// swap in per-meet grants without touching call sites.
type Capability string

const (
	CapManageAccounts   Capability = "manage_accounts"
	CapOrganizeMeet     Capability = "organize_meet"
	CapOfficeActions    Capability = "office_actions"
	CapCaptureResults   Capability = "capture_results"
	CapSubmitEntries    Capability = "submit_entries"
	CapViewPublicResult Capability = "view_public_results"
)

// capabilityMinRole is the least-privileged role each capability requires.
var capabilityMinRole = map[Capability]Role{
	CapManageAccounts:   RoleInstanceAdmin,
	CapOrganizeMeet:     RoleMeetOrganizer,
	CapOfficeActions:    RoleCompetitionOffice,
	CapCaptureResults:   RoleFieldOfficial,
	CapSubmitEntries:    RoleEntrySubmitter,
	CapViewPublicResult: RolePublic,
}

// ErrForbidden means the actor's role does not carry the required capability.
type ErrForbidden struct {
	Role       Role
	Capability Capability
}

func (e ErrForbidden) Error() string {
	return fmt.Sprintf("role %q lacks capability %q", e.Role, e.Capability)
}

// Authorize reports an error unless r carries cap (least privilege, SYS-090).
func Authorize(r Role, cap Capability) error {
	min, ok := capabilityMinRole[cap]
	if !ok {
		return fmt.Errorf("unknown capability %q", cap)
	}
	if !r.AtLeast(min) {
		return ErrForbidden{Role: r, Capability: cap}
	}
	return nil
}
