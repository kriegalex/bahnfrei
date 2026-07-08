// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"errors"
	"fmt"
)

// RoundUpHandTime applies the D5.1 hand-timing conversion (SYS-041): a
// manually captured track time is rounded **up** to the next 0.1 s and
// returned in one-decimal form ("11.32" → "11.4"; an exact tenth stays,
// "11.30" → "11.3"). Road events round up to the whole second instead —
// out of scope until a road discipline enters the catalog (TASK-019).
func RoundUpHandTime(mark string) (string, error) {
	centi, err := ParseCentiMark(mark)
	if err != nil {
		return "", err
	}
	if centi == 0 {
		return "", errors.New("hand time: a time > 0 is required")
	}
	rounded := (centi + 9) / 10 * 10
	return FormatCentiMark(rounded, 1), nil
}

// ValidateFATTime checks a fully-automatic-timing track mark: SYS-040
// requires 0.01 s resolution, which ParseCentiMark's two-decimal limit
// already enforces; the mark just has to be a positive time.
func ValidateFATTime(mark string) (string, error) {
	centi, err := ParseCentiMark(mark)
	if err != nil {
		return "", err
	}
	if centi == 0 {
		return "", errors.New("time: a time > 0 is required")
	}
	return FormatCentiMark(centi, 2), nil
}

// captureStatuses is the subset of the CR 25 result-status vocabulary
// (SYS-045, D5.2) an operator may set directly during capture. Qualification
// codes (Q, q, …) are assigned by round progression (TASK-018), and O/X/–/r
// are per-attempt facts, not settable unit statuses.
var captureStatuses = map[QualificationStatus]bool{
	StatusDNS: true,
	StatusDNF: true,
	StatusDQ:  true,
	StatusNM:  true,
}

// ValidateCaptureStatus checks an operator-set result status against the
// CR 25 vocabulary (SYS-045): only the capture subset is settable, and a DQ
// SHALL carry its rule reference (e.g. "TR16.8") in detail — a DQ without
// one is rejected (UC-010 #3).
func ValidateCaptureStatus(status QualificationStatus, detail string) error {
	if !captureStatuses[status] {
		return fmt.Errorf("status %q is not an operator-settable CR 25 status (SYS-045)", status)
	}
	if status == StatusDQ && detail == "" {
		return errors.New("a DQ requires a rule reference (SYS-045)")
	}
	if status != StatusDQ && detail != "" {
		return fmt.Errorf("status %s carries no rule reference", status)
	}
	return nil
}

// RenderStatus renders a result status per the CR 25 result-list convention
// (SYS-045): the code itself, with a DQ's rule reference appended —
// "DQ (TR16.8)" (UC-010 #3).
func RenderStatus(status QualificationStatus, detail string) string {
	if status == StatusDQ && detail != "" {
		return fmt.Sprintf("DQ (%s)", detail)
	}
	return string(status)
}
