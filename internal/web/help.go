// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import "strings"

// Contextual-input-help registry (TASK-031, SYS-115, UC-037 #1).
//
// SYS-115 requires that "which inputs qualify [for a contextual-help
// affordance] SHALL be maintained as a reviewable, machine-readable
// help-content registry". This file is that registry: one entry per
// (screen, input) pair that carries a help icon, bound to the route that
// serves the screen and to the i18n key of its localized help text.
//
// The registry is the single source of truth the UC-037 #1 coverage test
// (help_test.go) verifies mechanically in BOTH directions: every
// registered input's rendered screen must show its help icon adjacent to
// the input, and every help icon rendered on a registered screen must
// have a registry entry — so the registry can neither silently rot behind
// the templates nor the templates grow unreviewed help content.
//
// What qualifies (SYS-115): all domain-specific fields (wind reading,
// seeding parameters, bar-height configuration, publication/consent
// toggles) and fields with a non-obvious format or consequence.
// Universally familiar fields (name, e-mail, password) do not get an
// icon; hard constraints (formats, units, bounds) additionally stay
// permanently visible as `.hint` text (SYS-117) and are never moved into
// the popup — the popup carries supplementary explanation only (NN/g
// tooltip guideline 1; docs/research/ux-contextual-help-and-design-
// quality.md).
//
// Dense capture grids (field-attempt cells, vertical-jump trial cells,
// bulk-entry table rows) are deliberately not in the registry: a per-cell
// icon would be noise. Those surfaces explain their symbols once per
// screen (`capture.value_help`, `capture.vertical.trial_help`) and every
// cell carries an aria-label; the screen-level explanation is their
// SYS-115 affordance.
type helpEntry struct {
	// Screen is the stable fixture id of the rendered screen state the
	// coverage test renders (help_test.go maps each Screen to a template
	// + fixture view). One route can expose more than one screen state
	// (e.g. the vertical-jump heights form before and after initial
	// configuration), so Screen — not Route — is the registry's binding
	// for the mechanical test.
	Screen string
	// Route is the URL pattern that serves the screen, for reviewers
	// tracing an entry back to the live surface.
	Route string
	// Input is the input/select `name` attribute the help icon belongs to.
	Input string
	// Key names the entry's help text: the catalog key is "help." + Key.
	// Two entries may share a Key when the same input appears on two
	// screens (e.g. the publication-withdrawal consent toggle).
	Key string
}

// helpRegistry is the SYS-115 help-content registry. Keep it grouped by
// Screen; every Key needs a "help.<Key>" text in de.json AND fr.json
// (help_test.go fails the build otherwise).
var helpRegistry = []helpEntry{
	{Screen: "capture-track", Route: "/meets/{id}/capture/{unit}", Input: "wind", Key: "capture.wind"},
	{Screen: "entries", Route: "/meets/{id}/entries", Input: "seed", Key: "entries.seed"},
	{Screen: "entries", Route: "/meets/{id}/entries", Input: "publication_withdrawn", Key: "publication_withdrawn"},
	{Screen: "meet-detail", Route: "/meets/{id}", Input: "entry_deadline", Key: "programme.entry_deadline"},
	{Screen: "meet-detail", Route: "/meets/{id}", Input: "entry_standard", Key: "programme.entry_standard"},
	{Screen: "meet-detail", Route: "/meets/{id}", Input: "entry_limit", Key: "programme.entry_limit"},
	{Screen: "meet-form", Route: "/meets/new", Input: "homologation_ref", Key: "meet.homologation"},
	{Screen: "meet-form", Route: "/meets/new", Input: "tier", Key: "meet.tier"},
	{Screen: "meet-form", Route: "/meets/new", Input: "scheme", Key: "meet.scheme"},
	{Screen: "meet-form", Route: "/meets/new", Input: "results_positioning", Key: "meet.results_positioning"},
	{Screen: "roster", Route: "/meets/{id}/roster", Input: "publication_withdrawn", Key: "publication_withdrawn"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "max_heat_size", Key: "seeding.max_heat_size"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "track_lanes", Key: "seeding.track_lanes"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "top_n", Key: "seeding.advance.top_n"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "fastest_k", Key: "seeding.advance.fastest_k"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "standard", Key: "seeding.advance.standard"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "finals_capacity", Key: "seeding.advance.capacity"},
	{Screen: "seeding", Route: "/meets/{id}/events/{eid}/rounds/{rid}/seeding", Input: "better_direction", Key: "seeding.advance.better_direction"},
	{Screen: "vertical-capture-initial", Route: "/meets/{id}/capture/{unit}", Input: "heights", Key: "capture.vertical.heights"},
	{Screen: "vertical-capture-extend", Route: "/meets/{id}/capture/{unit}", Input: "add_height", Key: "capture.vertical.add_height"},
}

// helpTextKey returns the catalog key of a registry entry's help text.
func helpTextKey(key string) string {
	return "help." + key
}

// helpContentID returns the DOM id of a help popup, referenced by the
// trigger's aria-describedby (help.templ). Keys are dotted; ids use
// dashes so the id stays a plain CSS-selectable token.
func helpContentID(key string) string {
	return "help-" + strings.ReplaceAll(key, ".", "-")
}
