// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "time"

// ProtestWindow is the SYS-047/UC-015 protest-clock duration: 30 minutes
// from a result list's announcement, and 30 minutes from an amended
// result's announcement for appeals (D8.3).
const ProtestWindow = 30 * time.Minute

// ProtestState is a unit's result-list protest-clock state at a point in
// time (SYS-047, UC-015 #1/#4): whether it has been announced at all, and
// once announced, whether the window is still open (provisional) or has
// elapsed (official).
type ProtestState struct {
	Announced   bool
	AnnouncedAt time.Time
	// Official is true once ProtestWindow has elapsed since AnnouncedAt with
	// no further correction — the result transitions from provisional to
	// official automatically, without operator action (UC-015 #4).
	Official bool
	// Remaining is the time left in the protest window; zero once Official.
	Remaining time.Duration
}

// ComputeProtestState derives a unit's protest-clock state at now from its
// latest announcement (hasAnnouncement is false when the unit's results
// have never been posted — SYS-047).
func ComputeProtestState(announcedAt time.Time, hasAnnouncement bool, now time.Time) ProtestState {
	if !hasAnnouncement {
		return ProtestState{}
	}
	elapsed := now.Sub(announcedAt)
	if elapsed >= ProtestWindow {
		return ProtestState{Announced: true, AnnouncedAt: announcedAt, Official: true}
	}
	return ProtestState{
		Announced: true, AnnouncedAt: announcedAt,
		Remaining: ProtestWindow - elapsed,
	}
}
