// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Heat seeding and lane draws (TASK-018, UC-008, SYS-026/027/028):
// serpentine heat distribution and TR20.4 lane draws, driven generically by
// the versioned domain.SeedingRules data (D2.2-D2.4) — see
// internal/domain/heatseeding.go for the interpreter. ---

// defaultMaxHeatSize is used when GenerateHeatsRequest does not specify one
// (falls back to the request's TrackLanes if that is set).
const defaultMaxHeatSize = 8

// ErrRoundNotFound means the requested round id does not belong to the
// requested event.
var ErrRoundNotFound = errors.New("round not found for this event")

// HeatSheetRow is one entry's line in a generated/regenerated heat sheet
// (UC-008): identity, seed rank/mark, drawn lane, and qualification code
// once progression has run (UC-009).
type HeatSheetRow struct {
	EntryID string
	// Version is the unit-assignment row's optimistic-concurrency version —
	// callers must round-trip it into OverrideAssignment (SYS-028/083).
	Version        int64
	AthleteName    string
	ClubName       string
	SeedRank       int
	SeedMark       string
	Lane           int
	Qualification  domain.QualificationStatus
	ManualOverride bool
}

// HeatSheetUnit is one heat/flight of a round's seeding.
type HeatSheetUnit struct {
	UnitID string
	Rows   []HeatSheetRow
}

// HeatSheet is a round's full seeding state, plus the rule set applied —
// exposed so the operator can see which rules produced it (D2.2: "the
// applied rule set SHALL be visible to the operator").
type HeatSheet struct {
	RoundID       string
	RoundKind     domain.RoundKind
	Units         []HeatSheetUnit
	RulesID       string
	RulesVersion  string
	Distribution  string
	ClubSeparated bool
}

// GenerateHeatsRequest configures one heat-generation run (SYS-026/027).
// MaxHeatSize bounds heat size (commonly the venue's usable lane count); 0
// falls back to TrackLanes, then to defaultMaxHeatSize. TrackLanes is the
// track's physical lane count for the lane draw; 0 means no lane draw is
// performed (off-track/flight-style rounds, SYS-030).
type GenerateHeatsRequest struct {
	MaxHeatSize int
	TrackLanes  int
}

func loadSeedingRules() (*domain.SeedingRules, error) {
	return domain.BuiltinSeedingRules(domain.SeedingRulesTR20)
}

// entryClubID resolves the club a confirmed entry competes for (individual
// athlete's first club, or the relay team's club).
func entryClubID(ctx context.Context, db store.DBTX, entry store.EntryRecord) (string, error) {
	if entry.AthleteID != "" {
		athlete, err := store.GetAthlete(ctx, db, entry.AthleteID)
		if err != nil {
			return "", err
		}
		if len(athlete.ClubIDs) > 0 {
			return athlete.ClubIDs[0], nil
		}
		return "", nil
	}
	if entry.RelayTeamID != "" {
		team, err := store.GetRelayTeam(ctx, db, entry.RelayTeamID)
		if err != nil {
			return "", err
		}
		return team.ClubID, nil
	}
	return "", nil
}

// entryDisplayName resolves an entry's athlete/team display name.
func entryDisplayName(ctx context.Context, db store.DBTX, entry store.EntryRecord) (string, error) {
	if entry.AthleteID != "" {
		a, err := store.GetAthlete(ctx, db, entry.AthleteID)
		if err != nil {
			return "", err
		}
		return a.FirstName + " " + a.LastName, nil
	}
	if entry.RelayTeamID != "" {
		team, err := store.GetRelayTeam(ctx, db, entry.RelayTeamID)
		if err != nil {
			return "", err
		}
		clubName := team.ClubID
		if names, err := store.ClubNames(ctx, db, []string{team.ClubID}); err == nil && names[team.ClubID] != "" {
			clubName = names[team.ClubID]
		}
		return clubName + " (relay)", nil
	}
	return "", nil
}

// candidatePool resolves the seeding candidates for one round: confirmed
// entries for the event's first round (SYS-026: "generate heats from
// confirmed entries"), or the previous round's qualifiers re-seeded by
// their settled mark from that round otherwise (D2.2/D2.4). isFirstRound
// tells the caller which source was used, for messaging.
func (s *ResultsService) candidatePool(ctx context.Context, event store.EventRecord, rounds []domain.Round, roundIdx int) ([]domain.SeedCandidate, error) {
	if roundIdx == 0 {
		entries, err := store.ListEntriesByEvent(ctx, s.db, event.ID)
		if err != nil {
			return nil, err
		}
		var pool []domain.SeedCandidate
		for _, e := range entries {
			if e.Status != domain.EntryConfirmed {
				continue
			}
			clubID, err := entryClubID(ctx, s.db, e)
			if err != nil {
				return nil, err
			}
			pool = append(pool, domain.SeedCandidate{EntryID: e.ID, ClubID: clubID, Mark: e.SeedPerformance})
		}
		return pool, nil
	}

	prevRoundID := rounds[roundIdx-1].ID
	assignments, err := store.ListRoundAssignments(ctx, s.db, prevRoundID)
	if err != nil {
		return nil, err
	}
	var pool []domain.SeedCandidate
	for _, a := range assignments {
		if a.Qualification == domain.StatusNone {
			continue
		}
		entry, err := store.GetEntry(ctx, s.db, a.EntryID)
		if err != nil {
			return nil, err
		}
		clubID, err := entryClubID(ctx, s.db, entry)
		if err != nil {
			return nil, err
		}
		mark := ""
		if entry.AthleteID != "" {
			if res, err := store.GetResult(ctx, s.db, a.UnitID, entry.AthleteID); err == nil {
				mark = res.Mark
			}
		}
		pool = append(pool, domain.SeedCandidate{EntryID: entry.ID, ClubID: clubID, Mark: mark})
	}
	return pool, nil
}

// GenerateHeats (re)generates a round's heat sheet (UC-008): candidates are
// ranked by seed mark, distributed by serpentine into as many heats as
// req.MaxHeatSize requires, and — for lane-run disciplines with
// req.TrackLanes set — lanes are drawn per the TR20.4 grouped method (or by
// lot for non-grouped disciplines/lane counts). Entries with an existing
// manual override (SYS-028) keep their heat and lane; regeneration seeds
// only the remaining ("free") entries around them, and drops stale
// assignments for entries no longer in the pool (e.g. scratched since the
// last generation).
func (s *ResultsService) GenerateHeats(ctx context.Context, actor Session, meetID, eventID, roundID string, req GenerateHeatsRequest) (HeatSheet, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return HeatSheet{}, err
	}
	event, err := eventOfMeet(ctx, s.db, meetID, eventID)
	if err != nil {
		return HeatSheet{}, err
	}
	rounds, err := store.ListRounds(ctx, s.db, eventID)
	if err != nil {
		return HeatSheet{}, err
	}
	roundIdx := -1
	for i, r := range rounds {
		if r.ID == roundID {
			roundIdx = i
			break
		}
	}
	if roundIdx < 0 {
		return HeatSheet{}, ErrRoundNotFound
	}
	round := rounds[roundIdx]

	pool, err := s.candidatePool(ctx, event, rounds, roundIdx)
	if err != nil {
		return HeatSheet{}, err
	}
	if len(pool) == 0 {
		return HeatSheet{}, fmt.Errorf("generate heats: no confirmed/qualified entries to seed")
	}

	rules, err := loadSeedingRules()
	if err != nil {
		return HeatSheet{}, err
	}

	maxHeatSize := req.MaxHeatSize
	if maxHeatSize <= 0 {
		maxHeatSize = req.TrackLanes
	}
	if maxHeatSize <= 0 {
		maxHeatSize = defaultMaxHeatSize
	}

	// Existing manual overrides: locked in place, excluded from the pool
	// the serpentine distributor sees.
	existing, err := store.ListRoundAssignments(ctx, s.db, roundID)
	if err != nil {
		return HeatSheet{}, err
	}
	lockedUnit := map[string]string{} // entryID -> unitID
	lockedLane := map[string]int{}    // entryID -> lane
	for _, a := range existing {
		if a.ManualOverride {
			lockedUnit[a.EntryID] = a.UnitID
			lockedLane[a.EntryID] = a.Lane
		}
	}

	var freePool []domain.SeedCandidate
	for _, c := range pool {
		if _, locked := lockedUnit[c.EntryID]; !locked {
			freePool = append(freePool, c)
		}
	}
	ranked := domain.RankCandidates(freePool)

	// heatCount accounts for the locked entries too: the round needs enough
	// units to seat everyone, including whichever heat each locked entry
	// already occupies.
	heatCount, err := domain.ChooseHeatCount(len(pool), maxHeatSize)
	if err != nil {
		return HeatSheet{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HeatSheet{}, fmt.Errorf("generate heats: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	units, err := store.EnsureRoundUnitCount(ctx, tx, roundID, heatCount)
	if err != nil {
		return HeatSheet{}, err
	}
	unitIDs := make([]string, len(units))
	for i, u := range units {
		unitIDs[i] = u.ID
	}

	heats, err := domain.SeedHeats(ranked, heatCount, rules.ClubSeparation)
	if err != nil {
		return HeatSheet{}, err
	}

	// Drop stale assignments for entries that fell out of the pool (e.g.
	// scratched since the last generation) — a manual override (SYS-028)
	// protects an entry's heat/lane from being *reshuffled* by the
	// algorithm while it is still an active confirmed/qualified entrant; it
	// does not keep a scratched entry's placement around.
	inPool := map[string]bool{}
	for _, c := range pool {
		inPool[c.EntryID] = true
	}
	for _, a := range existing {
		if !inPool[a.EntryID] {
			if err := store.DeleteUnitAssignment(ctx, tx, a.UnitID, a.EntryID); err != nil {
				return HeatSheet{}, err
			}
		}
	}

	// Clear every free (non-manual) entry's previously-drawn lane before
	// assigning new ones: SQLite checks the (unit_id, lane) uniqueness
	// index immediately per statement, so reshuffling lanes among rows that
	// already exist (a regeneration, as opposed to a first generation into
	// empty units) could otherwise collide transiently — entry A's new
	// lane might still be held by entry B a few statements away from being
	// updated. Lane 0 is exempt from the index, so staging every rewritten
	// row through it first makes the whole reshuffle collision-free
	// regardless of the new permutation.
	for _, a := range existing {
		if !a.ManualOverride && inPool[a.EntryID] && a.Lane != 0 {
			cleared := a.UnitAssignment
			cleared.Lane = 0
			if _, err := store.SaveUnitAssignment(ctx, tx, cleared); err != nil {
				return HeatSheet{}, err
			}
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	laned := req.TrackLanes > 0 && rules.IsLaneRace(event.DisciplineCode)

	for hi, heat := range heats {
		unitID := unitIDs[hi]
		lockedInHeat := map[string]bool{}
		usedLanes := map[int]bool{}
		for entryID, uID := range lockedUnit {
			if uID == unitID {
				lockedInHeat[entryID] = true
				if lockedLane[entryID] != 0 {
					usedLanes[lockedLane[entryID]] = true
				}
			}
		}

		var lanes map[string]int
		if laned {
			ids := make([]string, len(heat))
			for i, c := range heat {
				ids[i] = c.EntryID
			}
			switch {
			case len(lockedInHeat) == 0:
				if groups, ok := rules.LaneGroupsFor(len(heat)); ok {
					// The group table's lane numbers are relative to a field
					// of exactly len(heat); shift onto the track's physical
					// numbering when it has more lanes than the field uses
					// (D2.3's unused-inner-lane rule, UC-008 #4).
					lanes, err = domain.DrawLanesGrouped(ids, groups, rng)
					if err == nil {
						if unused := req.TrackLanes - len(heat); unused > 0 {
							lanes = domain.ShiftForUnusedLanes(lanes, unused)
						}
					}
				} else {
					// By-lot over the full physical lane range — already
					// absolute lane numbers, no shift needed.
					lanes, err = domain.DrawLanesByLot(ids, domain.SequentialLanes(req.TrackLanes), rng)
				}
			default:
				// A partially manually-seeded heat: fall back to a by-lot
				// draw among the physical lanes the locked entries did not
				// already take, so no collision is possible (OQ: full
				// TR20.4 group fidelity around manual overrides is
				// deferred).
				var avail []int
				for _, l := range domain.SequentialLanes(req.TrackLanes) {
					if !usedLanes[l] {
						avail = append(avail, l)
					}
				}
				lanes, err = domain.DrawLanesByLot(ids, avail, rng)
			}
			if err != nil {
				return HeatSheet{}, err
			}
		}

		for rank, c := range heat {
			lane := 0
			if lanes != nil {
				lane = lanes[c.EntryID]
			}
			if _, err := store.SaveUnitAssignment(ctx, tx, domain.UnitAssignment{
				UnitID: unitID, EntryID: c.EntryID, SeedRank: rank + 1, Lane: lane,
			}); err != nil {
				return HeatSheet{}, err
			}
		}
	}

	after, _ := json.Marshal(map[string]any{"round": roundID, "heats": heatCount, "entries": len(pool)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "seeding.generate",
		EntityType: "round", EntityID: roundID, After: string(after),
	}); err != nil {
		return HeatSheet{}, fmt.Errorf("audit seeding.generate: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return HeatSheet{}, fmt.Errorf("generate heats: %w", err)
	}
	s.notifyChanged(meetID)
	return s.buildHeatSheet(ctx, round, rules)
}

// HeatSheetFor returns a round's current seeding state without regenerating
// it (initial view, or after a manual edit).
func (s *ResultsService) HeatSheetFor(ctx context.Context, actor Session, meetID, eventID, roundID string) (HeatSheet, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return HeatSheet{}, err
	}
	if _, err := eventOfMeet(ctx, s.db, meetID, eventID); err != nil {
		return HeatSheet{}, err
	}
	rounds, err := store.ListRounds(ctx, s.db, eventID)
	if err != nil {
		return HeatSheet{}, err
	}
	var round domain.Round
	found := false
	for _, r := range rounds {
		if r.ID == roundID {
			round, found = r, true
			break
		}
	}
	if !found {
		return HeatSheet{}, ErrRoundNotFound
	}
	rules, err := loadSeedingRules()
	if err != nil {
		return HeatSheet{}, err
	}
	return s.buildHeatSheet(ctx, round, rules)
}

func (s *ResultsService) buildHeatSheet(ctx context.Context, round domain.Round, rules *domain.SeedingRules) (HeatSheet, error) {
	sheet := HeatSheet{
		RoundID: round.ID, RoundKind: round.Kind,
		RulesID: rules.ID, RulesVersion: rules.Version,
		Distribution: rules.Distribution, ClubSeparated: rules.ClubSeparation,
	}
	unitRows, err := store.ListRoundAssignments(ctx, s.db, round.ID)
	if err != nil {
		return HeatSheet{}, err
	}
	byUnit := map[string][]store.UnitAssignmentRecord{}
	var unitOrder []string
	seen := map[string]bool{}
	for _, a := range unitRows {
		byUnit[a.UnitID] = append(byUnit[a.UnitID], a)
		if !seen[a.UnitID] {
			seen[a.UnitID] = true
			unitOrder = append(unitOrder, a.UnitID)
		}
	}
	for _, unitID := range unitOrder {
		u := HeatSheetUnit{UnitID: unitID}
		for _, a := range byUnit[unitID] {
			entry, err := store.GetEntry(ctx, s.db, a.EntryID)
			if err != nil {
				return HeatSheet{}, err
			}
			name, err := entryDisplayName(ctx, s.db, entry)
			if err != nil {
				return HeatSheet{}, err
			}
			clubID, err := entryClubID(ctx, s.db, entry)
			if err != nil {
				return HeatSheet{}, err
			}
			clubName := ""
			if clubID != "" {
				if names, err := store.ClubNames(ctx, s.db, []string{clubID}); err == nil {
					clubName = names[clubID]
				}
			}
			u.Rows = append(u.Rows, HeatSheetRow{
				EntryID: a.EntryID, Version: a.Version, AthleteName: name, ClubName: clubName,
				SeedRank: a.SeedRank, SeedMark: entry.SeedPerformance, Lane: a.Lane,
				Qualification: a.Qualification, ManualOverride: a.ManualOverride,
			})
		}
		sheet.Units = append(sheet.Units, u)
	}
	return sheet, nil
}

// OverrideAssignment applies an operator's manual heat/lane edit (UC-008
// #5, SYS-028): moving entryID (currently seeded somewhere in roundID) to
// targetUnitID with an optional lane (0 = no lane). The row is marked
// manual_override so a later regeneration leaves it in place unless
// ReleaseOverride is called first.
func (s *ResultsService) OverrideAssignment(ctx context.Context, actor Session, meetID, eventID, roundID, entryID, targetUnitID string, lane int, expectedVersion int64) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	if _, err := eventOfMeet(ctx, s.db, meetID, eventID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("override assignment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The entry may already be seeded elsewhere in this round (the normal
	// swap case — move its existing row to targetUnitID) or not seeded at
	// all yet (a first manual placement — insert one).
	assignments, err := store.ListRoundAssignments(ctx, tx, roundID)
	if err != nil {
		return err
	}
	var id, currentUnitID string
	found := false
	prevLane := 0
	for _, a := range assignments {
		if a.EntryID == entryID {
			id, found, prevLane, currentUnitID = a.ID, true, a.Lane, a.UnitID
			break
		}
	}

	// If the requested lane is already held by a different entry in the
	// target unit, swap the two lanes (UC-008 #5: "the operator swaps two
	// athletes") instead of failing the unique-lane constraint. The moving
	// entry vacates through lane 0 first (exempt from the uniqueness
	// index) so the two updates never collide mid-transaction.
	var occupant *store.UnitAssignmentRecord
	if lane != 0 {
		for i, a := range assignments {
			if a.UnitID == targetUnitID && a.Lane == lane && a.EntryID != entryID {
				occupant = &assignments[i]
				break
			}
		}
	}
	if occupant != nil && found {
		if _, err := store.UpdateUnitAssignmentManual(ctx, tx, id, expectedVersion, currentUnitID, 0); err != nil {
			return err
		}
		expectedVersion++ // the row we just vacated bumped its own version
	}
	if occupant != nil {
		if _, err := store.UpdateUnitAssignmentManual(ctx, tx, occupant.ID, occupant.Version, occupant.UnitID, prevLane); err != nil {
			return err
		}
	}

	if found {
		if _, err := store.UpdateUnitAssignmentManual(ctx, tx, id, expectedVersion, targetUnitID, lane); err != nil {
			return err
		}
	} else {
		rec, err := store.SaveUnitAssignment(ctx, tx, domain.UnitAssignment{UnitID: targetUnitID, EntryID: entryID, Lane: lane, ManualOverride: true})
		if err != nil {
			return err
		}
		id = rec.ID
	}

	after, _ := json.Marshal(map[string]any{"unit": targetUnitID, "lane": lane})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "seeding.override",
		EntityType: "unit_entry", EntityID: id, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit seeding.override: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("override assignment: %w", err)
	}
	s.notifyChanged(meetID)
	return nil
}

// EventHeatSheet is one event's seeded round(s), for the public start-list
// page (SYS-070, UC-008): unauthenticated, mirrors HeatSheetFor's shape.
type EventHeatSheet struct {
	EventID        string
	DisciplineName string
	CategoryCodes  []string
	Rounds         []HeatSheet
}

// PublicHeatSheets lists every event of a meet that has at least one
// generated round (heat seeding has run), each with its round(s)' heat
// sheets — the public start-list page's heat/lane breakdown alongside the
// existing flat roster (SYS-070/074). Events with no seeded round are
// omitted, not shown empty.
func (s *ResultsService) PublicHeatSheets(ctx context.Context, meetID string) ([]EventHeatSheet, error) {
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	rules, err := loadSeedingRules()
	if err != nil {
		return nil, err
	}

	var out []EventHeatSheet
	for _, ev := range events {
		rounds, err := store.ListRounds(ctx, s.db, ev.ID)
		if err != nil {
			return nil, err
		}
		var sheets []HeatSheet
		for _, round := range rounds {
			assignments, err := store.ListRoundAssignments(ctx, s.db, round.ID)
			if err != nil {
				return nil, err
			}
			if len(assignments) == 0 {
				continue
			}
			sheet, err := s.buildHeatSheet(ctx, round, rules)
			if err != nil {
				return nil, err
			}
			sheets = append(sheets, sheet)
		}
		if len(sheets) == 0 {
			continue
		}
		disc, _ := s.catalog.ByCode(ev.DisciplineCode)
		out = append(out, EventHeatSheet{
			EventID: ev.ID, DisciplineName: disc.Name, CategoryCodes: ev.CategoryCodes, Rounds: sheets,
		})
	}
	return out, nil
}
