// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestDecideSync_Routing proves the pure routing rule behind UC-034 #2/#4/#5:
// current-holder ops apply, stale/absent-holder ops and start-list-change ops
// reconcile, with the stale check taking precedence.
func TestDecideSync_Routing(t *testing.T) {
	tests := []struct {
		name    string
		current CheckoutState
		op      OpStamp
		want    SyncDecision
	}{
		{
			name:    "current holder, same start list applies",
			current: CheckoutState{Generation: 1, StartListVersion: 0, Active: true},
			op:      OpStamp{Generation: 1, StartListVersion: 0},
			want:    DecisionApply,
		},
		{
			name:    "older generation is stale (override happened)",
			current: CheckoutState{Generation: 2, StartListVersion: 0, Active: true},
			op:      OpStamp{Generation: 1, StartListVersion: 0},
			want:    DecisionStaleCheckout,
		},
		{
			name:    "no live checkout is stale",
			current: CheckoutState{Generation: 1, StartListVersion: 0, Active: false},
			op:      OpStamp{Generation: 1, StartListVersion: 0},
			want:    DecisionStaleCheckout,
		},
		{
			name:    "current holder but start list revised reconciles",
			current: CheckoutState{Generation: 1, StartListVersion: 2, Active: true},
			op:      OpStamp{Generation: 1, StartListVersion: 1},
			want:    DecisionStartListChange,
		},
		{
			name:    "stale generation wins over start-list mismatch",
			current: CheckoutState{Generation: 3, StartListVersion: 5, Active: true},
			op:      OpStamp{Generation: 1, StartListVersion: 1},
			want:    DecisionStaleCheckout,
		},
		{
			name:    "a newer generation than current is also stale (defensive)",
			current: CheckoutState{Generation: 1, StartListVersion: 0, Active: true},
			op:      OpStamp{Generation: 2, StartListVersion: 0},
			want:    DecisionStaleCheckout,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideSync(tc.current, tc.op); got != tc.want {
				t.Errorf("DecideSync(%+v, %+v) = %q, want %q", tc.current, tc.op, got, tc.want)
			}
		})
	}
}
