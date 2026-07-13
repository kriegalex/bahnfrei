// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestAssignTimingUnitNumbersIsIdempotentSYS060 checks the "assigned once,
// never recomputed" invariant 0018_timing_exchange.sql documents: a second
// assignment call for the same unit is a no-op that returns the original
// numbers, even if different numbers are requested.
func TestAssignTimingUnitNumbersIsIdempotentSYS060(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)

	first, err := AssignTimingUnitNumbers(ctx, s.DB(), unit.ID, 1, 1, 1)
	if err != nil {
		t.Fatalf("AssignTimingUnitNumbers: %v", err)
	}
	second, err := AssignTimingUnitNumbers(ctx, s.DB(), unit.ID, 9, 9, 9)
	if err != nil {
		t.Fatalf("AssignTimingUnitNumbers (repeat): %v", err)
	}
	if second != first {
		t.Fatalf("second assignment = %+v, want unchanged %+v", second, first)
	}

	got, err := GetTimingUnitNumbers(ctx, s.DB(), unit.ID)
	if err != nil {
		t.Fatalf("GetTimingUnitNumbers: %v", err)
	}
	if got.EventNumber != 1 || got.RoundNumber != 1 || got.HeatNumber != 1 {
		t.Fatalf("got %+v, want 1/1/1", got)
	}
}

// TestListRoundUnitsReturnsScheduleSYS060 covers ListRoundUnits' scheduled/
// unscheduled decode branches (0018_timing_exchange.sql's numbering reuses
// this same ordering): a unit UpdateUnitSchedule has stamped decodes its
// ScheduledAt/Location, and one that was never scheduled comes back zero-
// valued rather than erroring.
func TestListRoundUnitsReturnsScheduleSYS060(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	round, unit := seedingFixtureRound(t, s)
	unscheduled, err := CreateUnit(ctx, s.DB(), domain.Unit{RoundID: round.ID})
	if err != nil {
		t.Fatalf("CreateUnit: %v", err)
	}

	when := time.Date(2027, 6, 12, 9, 30, 0, 0, time.UTC)
	if _, err := UpdateUnitSchedule(ctx, s.DB(), unit.ID, unit.Version, when, "Bahn 1"); err != nil {
		t.Fatalf("UpdateUnitSchedule: %v", err)
	}

	units, err := ListRoundUnits(ctx, s.DB(), round.ID)
	if err != nil {
		t.Fatalf("ListRoundUnits: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	byID := map[string]UnitRecord{}
	for _, u := range units {
		byID[u.ID] = u
	}
	scheduled := byID[unit.ID]
	if !scheduled.ScheduledAt.Equal(when) || scheduled.Location != "Bahn 1" {
		t.Errorf("scheduled unit = %+v, want ScheduledAt=%v Location=Bahn 1", scheduled, when)
	}
	if got := byID[unscheduled.ID]; !got.ScheduledAt.IsZero() || got.Location != "" {
		t.Errorf("never-scheduled unit = %+v, want zero ScheduledAt and empty Location", got)
	}
}

// TestGetTimingImportBatchUnknownIDSYS061 covers scanTimingImportBatch's
// not-found branch directly.
func TestGetTimingImportBatchUnknownIDSYS061(t *testing.T) {
	s := openTest(t)
	if _, err := GetTimingImportBatch(context.Background(), s.DB(), "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTimingImportBatch(unknown) err = %v, want ErrNotFound", err)
	}
}

// TestCreateTimingAgentTokenDuplicateHashADR006 covers
// CreateTimingAgentToken's isUniqueViolation branch: token_hash is UNIQUE
// (0018_timing_exchange.sql) so a hash collision — astronomically unlikely
// for a real random token, but exercised here directly — is reported as
// ErrDuplicateTimingAgentToken, not a generic SQL error the caller cannot
// distinguish from any other failure.
func TestCreateTimingAgentTokenDuplicateHashADR006(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)
	hash := sha256Hex("colliding-token")

	if _, err := CreateTimingAgentToken(ctx, s.DB(), TimingAgentToken{
		MeetID: meet.ID, Label: "first", TokenHash: hash, CreatedBy: "org-account",
	}); err != nil {
		t.Fatalf("CreateTimingAgentToken (first): %v", err)
	}
	if _, err := CreateTimingAgentToken(ctx, s.DB(), TimingAgentToken{
		MeetID: meet.ID, Label: "second", TokenHash: hash, CreatedBy: "org-account",
	}); !errors.Is(err, ErrDuplicateTimingAgentToken) {
		t.Errorf("CreateTimingAgentToken (duplicate hash) err = %v, want ErrDuplicateTimingAgentToken", err)
	}
}

// TestFindUnitByTimingNumbersSYS061 covers the .lif import path's unit
// resolution: a matched triple resolves, an unmatched one is ErrNotFound —
// the "unresolved_unit" conflict trigger.
func TestFindUnitByTimingNumbersSYS061(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)
	if _, err := AssignTimingUnitNumbers(ctx, s.DB(), unit.ID, 5, 2, 3); err != nil {
		t.Fatalf("AssignTimingUnitNumbers: %v", err)
	}

	got, err := FindUnitByTimingNumbers(ctx, s.DB(), unitMeetID(t, s, unit), 5, 2, 3)
	if err != nil {
		t.Fatalf("FindUnitByTimingNumbers: %v", err)
	}
	if got != unit.ID {
		t.Fatalf("got unit %s, want %s", got, unit.ID)
	}

	if _, err := FindUnitByTimingNumbers(ctx, s.DB(), unitMeetID(t, s, unit), 5, 2, 4); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindUnitByTimingNumbers (no match): err = %v, want ErrNotFound", err)
	}
}

// unitMeetID resolves a unit's owning meet id via its round/event chain —
// a small test-only helper (production code reaches the meet from the
// event/round context it already holds).
func unitMeetID(t *testing.T, s *Store, unit UnitRecord) string {
	t.Helper()
	var meetID string
	err := s.DB().QueryRowContext(context.Background(), `SELECT e.meet_id
		FROM units u JOIN rounds r ON r.id = u.round_id JOIN events e ON e.id = r.event_id
		WHERE u.id = ?`, unit.ID).Scan(&meetID)
	if err != nil {
		t.Fatalf("resolve meet id for unit %s: %v", unit.ID, err)
	}
	return meetID
}

// TestTimingImportBatchAndConflictLifecycleSYS061UC014_4 covers the
// conflict-queue round trip: create a batch, queue a conflict, resolve it
// once (never twice — resolving an already-resolved conflict is an
// ErrNotFound, not a silent success).
func TestTimingImportBatchAndConflictLifecycleSYS061UC014_4(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)
	_, unit := seedingFixtureRound(t, s)

	batch, err := CreateTimingImportBatch(ctx, s.DB(), TimingImportBatch{
		MeetID: meet.ID, Format: "lif", Filename: "Men_100 (1-1-1).lif", UnitID: unit.ID, ImportedBy: "agent:timing-pc-1",
	})
	if err != nil {
		t.Fatalf("CreateTimingImportBatch: %v", err)
	}
	if batch.Applied != 0 || batch.Conflicted != 0 || batch.Format != "lif" {
		t.Fatalf("batch = %+v", batch)
	}

	conflict, err := CreateTimingImportConflict(ctx, s.DB(), TimingImportConflict{
		BatchID: batch.ID, UnitID: unit.ID, Bib: "999", Lane: 3,
		Reason: "unknown_bib", PayloadJSON: `{"lane":3,"time":"11.4"}`,
	})
	if err != nil {
		t.Fatalf("CreateTimingImportConflict: %v", err)
	}
	if conflict.Status != "pending" || conflict.Reason != "unknown_bib" {
		t.Fatalf("conflict = %+v", conflict)
	}

	list, err := ListTimingImportConflicts(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("ListTimingImportConflicts: %v", err)
	}
	if len(list) != 1 || list[0].ID != conflict.ID {
		t.Fatalf("got %+v, want one conflict %s", list, conflict.ID)
	}

	if err := ResolveTimingImportConflict(ctx, s.DB(), conflict.ID, "discarded", "office-account"); err != nil {
		t.Fatalf("ResolveTimingImportConflict: %v", err)
	}
	resolved, err := GetTimingImportConflict(ctx, s.DB(), conflict.ID)
	if err != nil {
		t.Fatalf("GetTimingImportConflict: %v", err)
	}
	if resolved.Status != "discarded" || resolved.ResolvedBy != "office-account" || resolved.ResolvedAt == nil {
		t.Fatalf("resolved = %+v", resolved)
	}

	// Resolving twice is refused, never a silent second application
	// (UC-014 #4/#5: "no silent overwrite").
	if err := ResolveTimingImportConflict(ctx, s.DB(), conflict.ID, "kept", "office-account"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second resolve: err = %v, want ErrNotFound", err)
	}

	batches, err := ListTimingImportBatches(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("ListTimingImportBatches: %v", err)
	}
	if len(batches) != 1 || batches[0].ID != batch.ID {
		t.Fatalf("got %+v, want one batch %s", batches, batch.ID)
	}
}

// TestTimingAgentTokenLifecycleADR006 covers issuing, resolving-by-hash and
// revoking an agent credential — a revoked token authenticates nothing
// (ADR-006's hub-first timing-agent amendment).
func TestTimingAgentTokenLifecycleADR006(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)

	hash := sha256Hex("super-secret-agent-token")
	tok, err := CreateTimingAgentToken(ctx, s.DB(), TimingAgentToken{
		MeetID: meet.ID, Label: "timing PC", TokenHash: hash, CreatedBy: "org-account",
	})
	if err != nil {
		t.Fatalf("CreateTimingAgentToken: %v", err)
	}
	if tok.RevokedAt != nil {
		t.Fatalf("fresh token has RevokedAt = %v, want nil", tok.RevokedAt)
	}

	got, err := GetTimingAgentTokenByHash(ctx, s.DB(), hash)
	if err != nil {
		t.Fatalf("GetTimingAgentTokenByHash: %v", err)
	}
	if got.ID != tok.ID || got.MeetID != meet.ID {
		t.Fatalf("got %+v", got)
	}

	list, err := ListTimingAgentTokens(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("ListTimingAgentTokens: %v", err)
	}
	if len(list) != 1 || list[0].ID != tok.ID {
		t.Fatalf("got %+v", list)
	}

	if err := RevokeTimingAgentToken(ctx, s.DB(), tok.ID); err != nil {
		t.Fatalf("RevokeTimingAgentToken: %v", err)
	}
	if _, err := GetTimingAgentTokenByHash(ctx, s.DB(), hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTimingAgentTokenByHash after revoke: err = %v, want ErrNotFound", err)
	}
	// Revoking again is a no-op, not an error.
	if err := RevokeTimingAgentToken(ctx, s.DB(), tok.ID); err != nil {
		t.Fatalf("RevokeTimingAgentToken (second call): %v", err)
	}
}

// TestSaveResultWithSourceTracksProvenanceSYS041 checks results.source
// defaults to "manual" via SaveResult and can be overridden via
// SaveResultWithSource for the import pipeline — the conflict check's
// "existing manual result" vs "previous import" distinction.
func TestSaveResultWithSourceTracksProvenanceSYS041(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	athlete := entryFixtureAthlete(t, s, "Ana")
	_, unit := seedingFixtureRound(t, s)

	manual, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unit.ID, AthleteID: athlete.ID, Mark: "11.4"}, "manual")
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
	if manual.Source != "manual" {
		t.Fatalf("manual.Source = %q, want manual", manual.Source)
	}

	imported, err := SaveResultWithSource(ctx, s.DB(),
		domain.Result{UnitID: unit.ID, AthleteID: athlete.ID, Mark: "11.3"}, "electronic", "import_lif")
	if err != nil {
		t.Fatalf("SaveResultWithSource: %v", err)
	}
	if imported.Source != "import_lif" || imported.Mark != "11.3" {
		t.Fatalf("imported = %+v", imported)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
