// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"testing"
	"time"
)

// TestComputeProtestStateSYS047UC015_1 covers UC-015 #1: an announced
// unit's protest window counts down for 30 minutes from the announcement
// timestamp (D8.3).
func TestComputeProtestStateSYS047UC015_1(t *testing.T) {
	announced := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

	if st := ComputeProtestState(time.Time{}, false, announced); st.Announced {
		t.Fatalf("no announcement yet must report Announced=false, got %+v", st)
	}

	now := announced.Add(10 * time.Minute)
	st := ComputeProtestState(announced, true, now)
	if !st.Announced || st.Official {
		t.Fatalf("10 minutes in = %+v, want announced/provisional", st)
	}
	if st.Remaining != 20*time.Minute {
		t.Errorf("remaining = %v, want 20m", st.Remaining)
	}
}

// TestComputeProtestStateSYS047UC015_4 covers UC-015 #4: once the 30-minute
// window elapses with no correction, the result becomes official
// automatically — no operator action.
func TestComputeProtestStateSYS047UC015_4(t *testing.T) {
	announced := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

	justBefore := ComputeProtestState(announced, true, announced.Add(29*time.Minute+59*time.Second))
	if justBefore.Official {
		t.Error("29m59s in must still be provisional")
	}

	atBoundary := ComputeProtestState(announced, true, announced.Add(ProtestWindow))
	if !atBoundary.Official || atBoundary.Remaining != 0 {
		t.Errorf("at the 30m boundary = %+v, want official with no remaining time", atBoundary)
	}

	longAfter := ComputeProtestState(announced, true, announced.Add(2*time.Hour))
	if !longAfter.Official {
		t.Error("well past the window must be official")
	}
}
