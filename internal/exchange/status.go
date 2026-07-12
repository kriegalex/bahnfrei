// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import (
	"strconv"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// LynxStatuses is the vendor-documented status vocabulary a FinishLynx
// Place field may carry instead of a finishing place (Database Files,
// "What are the allowable status codes?", fetched 2026-07-13): DNS, FS,
// DNF, DQ, SC, ADV.
var lynxStatuses = map[string]bool{
	"DNS": true, "FS": true, "DNF": true, "DQ": true, "SC": true, "ADV": true,
}

// PlaceKind classifies a parsed .lif Place field.
type PlaceKind int

const (
	// PlaceEmpty means the lane had no result at all (a DNS-by-omission —
	// the meettrax sample's blank-place, lane-only rows for unassigned
	// lanes, or a heat that has not been run/imported yet).
	PlaceEmpty PlaceKind = iota
	// PlaceNumber means Place is a finishing place ("1", "2", … — ties
	// share a place per the sample data).
	PlaceNumber
	// PlaceMappedStatus means Place is a Lynx status code this system's CR
	// 25 capture vocabulary (SYS-045) has a direct equivalent for (DNS,
	// DNF, DQ) — see MapLynxStatus.
	PlaceMappedStatus
	// PlaceUnmappedStatus means Place is a recognised Lynx status code
	// (FS, SC, ADV) with no CR 25 capture-vocabulary equivalent (OQ-048):
	// the import pipeline must surface it as a conflict for the operator
	// to resolve manually, never guess a mapping.
	PlaceUnmappedStatus
	// PlaceUnrecognized means Place is neither a number nor one of the
	// six documented status codes — also a conflict, never silently
	// dropped or coerced.
	PlaceUnrecognized
)

// ClassifyPlace inspects a .lif Place field and, for PlaceNumber, returns
// the parsed 1-based place.
func ClassifyPlace(place string) (kind PlaceKind, placeNumber int) {
	trimmed := strings.TrimSpace(place)
	if trimmed == "" {
		return PlaceEmpty, 0
	}
	if n, err := strconv.Atoi(trimmed); err == nil && n > 0 {
		return PlaceNumber, n
	}
	upper := strings.ToUpper(trimmed)
	if !lynxStatuses[upper] {
		return PlaceUnrecognized, 0
	}
	if _, ok := MapLynxStatus(upper); ok {
		return PlaceMappedStatus, 0
	}
	return PlaceUnmappedStatus, 0
}

// MapLynxStatus maps a Lynx status code to this system's CR 25
// operator-settable capture vocabulary (SYS-045, domain.ValidateCaptureStatus):
//
//	DNS -> domain.StatusDNS   (direct equivalent)
//	DNF -> domain.StatusDNF   (direct equivalent)
//	DQ  -> domain.StatusDQ    (direct equivalent; StatusDetail is left for
//	                           the office to fill in via the correction
//	                           flow — FinishLynx carries no CR25 rule
//	                           reference, OQ-048)
//
// FS (false start), SC (scratched) and ADV (advanced/qualified) have no
// direct CR 25 result-status equivalent in this system (OQ-048: FS is
// arguably a DQ-class outcome, SC is an Entry-level not Result-level
// status, ADV is a round-progression concept TASK-018 already owns) — ok
// is false for these and for any code MapLynxStatus does not recognise;
// callers must surface an unmapped status as an import conflict rather
// than guess.
func MapLynxStatus(lynxCode string) (domain.QualificationStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(lynxCode)) {
	case "DNS":
		return domain.StatusDNS, true
	case "DNF":
		return domain.StatusDNF, true
	case "DQ":
		return domain.StatusDQ, true
	default:
		return domain.StatusNone, false
	}
}
