// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestRoundUpHandTime is UC-010 #2 / SYS-041 (D5.1): hand times round UP to
// the next 0.1 s; exact tenths stay.
func TestRoundUpHandTime(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"11.32", "11.4", false},
		{"11.30", "11.3", false},
		{"11.3", "11.3", false},
		{"11.31", "11.4", false},
		{"11.39", "11.4", false},
		{"8.01", "8.1", false},
		{"9,95", "10.0", false}, // comma decimal, carry over the second
		{"0", "", true},
		{"", "", true},
		{"abc", "", true},
	}
	for _, tc := range cases {
		got, err := RoundUpHandTime(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("RoundUpHandTime(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("RoundUpHandTime(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateFATTime(t *testing.T) {
	if got, err := ValidateFATTime("10.87"); err != nil || got != "10.87" {
		t.Errorf("ValidateFATTime(10.87) = %q, %v", got, err)
	}
	if got, err := ValidateFATTime("10.8"); err != nil || got != "10.80" {
		t.Errorf("ValidateFATTime(10.8) = %q, %v (want canonical 0.01 s form)", got, err)
	}
	for _, bad := range []string{"0", "", "x"} {
		if _, err := ValidateFATTime(bad); err == nil {
			t.Errorf("ValidateFATTime(%q) accepted", bad)
		}
	}
}

// TestValidateCaptureStatus is the SYS-045 vocabulary gate: only the CR 25
// capture subset is settable and a DQ without a rule reference is rejected
// (UC-010 #3).
func TestValidateCaptureStatus(t *testing.T) {
	cases := []struct {
		status  QualificationStatus
		detail  string
		wantErr bool
	}{
		{StatusDNS, "", false},
		{StatusDNF, "", false},
		{StatusNM, "", false},
		{StatusDQ, "TR16.8", false},
		{StatusDQ, "", true},        // DQ requires a rule reference
		{StatusDNF, "TR16.8", true}, // rule reference only qualifies a DQ
		{StatusQ, "", true},         // progression codes are not capture statuses
		{StatusX, "", true},         // per-attempt symbols are not unit statuses
		{"banana", "", true},
	}
	for _, tc := range cases {
		err := ValidateCaptureStatus(tc.status, tc.detail)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateCaptureStatus(%q, %q) error = %v, wantErr %v", tc.status, tc.detail, err, tc.wantErr)
		}
	}
}

func TestRenderStatus(t *testing.T) {
	if got := RenderStatus(StatusDQ, "TR16.8"); got != "DQ (TR16.8)" {
		t.Errorf("RenderStatus(DQ, TR16.8) = %q", got)
	}
	if got := RenderStatus(StatusDNF, ""); got != "DNF" {
		t.Errorf("RenderStatus(DNF) = %q", got)
	}
}
