// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/exchange"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Timing exchange & timing agent (TASK-020, UC-014, SYS-060-062,
// ADR-006): FinishLynx .ppl/.sch/.evt export, .lif/generic-CSV import with
// conflict resolution (never a silent overwrite, SYS-061), and the
// meet-scoped bearer credential the unattended timing agent authenticates
// with. Track units only — the FinishLynx file family is a photo-finish
// timing integration, so field (horizontal/vertical) units, which have no
// timing-hardware counterpart, are out of scope here (documented
// assumption, OQ-049). Relay-team entries are also out of scope for this
// slice (OQ-049): only individual entries (entry.AthleteID != "") are
// exported/matched. ---

// ErrTimingUnitUnresolved means a .lif/.csv row's event/round/heat numbers
// match no unit of this meet (SYS-061's "matching them to the correct
// unit" failure path) — surfaced as an unresolved_unit conflict, never
// guessed.
var ErrTimingUnitUnresolved = errors.New("no unit is numbered with this event/round/heat")

// assignTimingNumbers assigns SYS-060's stable per-unit FinishLynx numeric
// identity to every track unit of meetID that does not have one yet, in
// event -> round (seq order, ListRounds) -> heat (id order, ListRoundUnits)
// order. Already-numbered units are untouched (store.AssignTimingUnitNumbers
// is idempotent) so a later-added event's units get fresh numbers appended
// after the existing ones, never renumbering what a timing PC already has
// on disk.
func (s *ResultsService) assignTimingNumbers(ctx context.Context, meetID string) error {
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return err
	}
	for ei, ev := range events {
		disc, ok := s.catalog.ByCode(ev.DisciplineCode)
		if !ok || disc.Family != domain.FamilyTrack {
			continue
		}
		rounds, err := store.ListRounds(ctx, s.db, ev.ID)
		if err != nil {
			return err
		}
		for ri, round := range rounds {
			units, err := store.ListRoundUnits(ctx, s.db, round.ID)
			if err != nil {
				return err
			}
			for hi, u := range units {
				if _, err := store.AssignTimingUnitNumbers(ctx, s.db, u.ID, ei+1, ri+1, hi+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// timingRoster is one track unit's numbered identity plus its seeded
// start-list rows (unit_entries -> entries -> athlete/participant), the
// shared input ExportTimingFiles/ExportGenericCSV both build from.
type timingRoster struct {
	Numbers        store.TimingUnitNumbers
	DisciplineName string
	CategoryLabel  string
	Rows           []timingRosterRow
}

type timingRosterRow struct {
	AthleteID   string
	Bib         string
	Lane        int
	LastName    string
	FirstName   string
	Affiliation string
}

// timingRosters builds every numbered track unit's start list for meetID,
// event -> round -> heat order — the common input to every export format.
// Units with no seeded entries yet (heat seeding has not run, TASK-018)
// are omitted, matching the "start list" concept: there is nothing to
// export until entries are placed in a unit.
func (s *ResultsService) timingRosters(ctx context.Context, meetID string) ([]timingRoster, error) {
	if err := s.assignTimingNumbers(ctx, meetID); err != nil {
		return nil, err
	}
	numbered, err := store.ListTimingUnitNumbersForMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	participants, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	bibByAthlete := map[string]string{}
	for _, p := range participants {
		bibByAthlete[p.AthleteID] = p.Bib
	}
	clubs, err := s.ClubNamesFor(ctx, participants)
	if err != nil {
		return nil, err
	}

	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	var out []timingRoster
	for _, ev := range events {
		disc, ok := s.catalog.ByCode(ev.DisciplineCode)
		if !ok || disc.Family != domain.FamilyTrack {
			continue
		}
		rounds, err := store.ListRounds(ctx, s.db, ev.ID)
		if err != nil {
			return nil, err
		}
		for _, round := range rounds {
			units, err := store.ListRoundUnits(ctx, s.db, round.ID)
			if err != nil {
				return nil, err
			}
			for _, u := range units {
				numbers, ok := numbered[u.ID]
				if !ok {
					continue
				}
				assignments, err := store.ListUnitAssignments(ctx, s.db, u.ID)
				if err != nil {
					return nil, err
				}
				if len(assignments) == 0 {
					continue
				}
				roster := timingRoster{
					Numbers: numbers, DisciplineName: disc.Name,
					CategoryLabel: strings.Join(ev.CategoryCodes, "/"),
				}
				for _, a := range assignments {
					entry, err := store.GetEntry(ctx, s.db, a.EntryID)
					if err != nil {
						return nil, err
					}
					if entry.AthleteID == "" {
						continue // relay entries are out of scope for this slice, OQ-049
					}
					athlete, err := store.GetAthlete(ctx, s.db, entry.AthleteID)
					if err != nil {
						return nil, err
					}
					affiliation := ""
					if len(athlete.ClubIDs) > 0 {
						affiliation = clubs[athlete.ClubIDs[0]]
					}
					roster.Rows = append(roster.Rows, timingRosterRow{
						AthleteID: entry.AthleteID, Bib: bibByAthlete[entry.AthleteID], Lane: a.Lane,
						LastName: athlete.LastName, FirstName: athlete.FirstName, Affiliation: affiliation,
					})
				}
				out = append(out, roster)
			}
		}
	}
	return out, nil
}

// ExportTimingFiles renders the meet's current start lists as the
// FinishLynx file family (SYS-060, UC-014 #1/#2): lynx.ppl, lynx.sch and
// lynx.evt, always computed fresh from live data — ADR-006's "regenerate
// on start-list changes" is satisfied by never caching a stale copy, not
// by change-detection on the write side (the timing agent's polling loop,
// cmd/bahnfrei/timingagent.go, detects the change on its side via a
// content hash).
func (s *ResultsService) ExportTimingFiles(ctx context.Context, actor Session, meetID string) (ppl, sch, evt []byte, err error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, nil, nil, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return nil, nil, nil, err
	}
	rosters, err := s.timingRosters(ctx, meetID)
	if err != nil {
		return nil, nil, nil, err
	}

	peopleSeen := map[string]bool{}
	var people []exchange.Person
	var events []exchange.Event
	var sched []exchange.ScheduleEntry
	for _, r := range rosters {
		ev := exchange.Event{Number: r.Numbers.EventNumber, Round: r.Numbers.RoundNumber, Heat: r.Numbers.HeatNumber,
			Name: r.DisciplineName + " " + r.CategoryLabel}
		for _, row := range r.Rows {
			id := lynxIDFor(row.Bib)
			ev.Competitors = append(ev.Competitors, exchange.CompetitorRow{
				ID: id, Lane: row.Lane, LastName: row.LastName, FirstName: row.FirstName, Affiliation: row.Affiliation,
			})
			if !peopleSeen[row.AthleteID] {
				peopleSeen[row.AthleteID] = true
				people = append(people, exchange.Person{ID: id, LastName: row.LastName, FirstName: row.FirstName, Affiliation: row.Affiliation})
			}
		}
		sort.Slice(ev.Competitors, func(i, j int) bool { return ev.Competitors[i].Lane < ev.Competitors[j].Lane })
		events = append(events, ev)
		sched = append(sched, exchange.ScheduleEntry{EventNumber: ev.Number, Round: ev.Round, Heat: ev.Heat, Comment: ev.Name})
	}
	sort.Slice(people, func(i, j int) bool { return people[i].ID < people[j].ID })
	sort.Slice(events, func(i, j int) bool {
		if events[i].Number != events[j].Number {
			return events[i].Number < events[j].Number
		}
		if events[i].Round != events[j].Round {
			return events[i].Round < events[j].Round
		}
		return events[i].Heat < events[j].Heat
	})
	sort.Slice(sched, func(i, j int) bool {
		if sched[i].EventNumber != sched[j].EventNumber {
			return sched[i].EventNumber < sched[j].EventNumber
		}
		if sched[i].Round != sched[j].Round {
			return sched[i].Round < sched[j].Round
		}
		return sched[i].Heat < sched[j].Heat
	})

	return exchange.EncodePPL(people), exchange.EncodeSCH(sched), exchange.EncodeEVT(events), nil
}

// lynxIDFor renders a participant bib as a FinishLynx ID field (Database
// Files: "To indicate no ID number you must put a zero in the ID field").
func lynxIDFor(bib string) string {
	if bib == "" {
		return "0"
	}
	return bib
}

// ExportGenericCSV renders the meet's start lists plus any already-settled
// results as the documented generic CSV fallback (SYS-062, UC-014 #6).
func (s *ResultsService) ExportGenericCSV(ctx context.Context, actor Session, meetID string) ([]byte, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	rosters, err := s.timingRosters(ctx, meetID)
	if err != nil {
		return nil, err
	}
	var rows []exchange.GenericRow
	for _, r := range rosters {
		results, err := store.ListUnitResults(ctx, s.db, unitIDFor(r))
		if err != nil {
			return nil, err
		}
		byAthlete := map[string]store.ResultRecord{}
		for _, res := range results {
			byAthlete[res.AthleteID] = res
		}
		for _, row := range r.Rows {
			g := exchange.GenericRow{
				EventNumber: r.Numbers.EventNumber, Round: r.Numbers.RoundNumber, Heat: r.Numbers.HeatNumber,
				Bib: row.Bib, LastName: row.LastName, FirstName: row.FirstName, Affiliation: row.Affiliation, Lane: row.Lane,
			}
			if res, ok := byAthlete[row.AthleteID]; ok {
				g.Mark, g.Timing, g.Status, g.StatusDetail = res.Mark, string(res.Timing), string(res.Status), res.StatusDetail
				if res.Wind != nil {
					g.Wind = fmt.Sprintf("%+.1f", *res.Wind)
				}
			}
			rows = append(rows, g)
		}
	}
	return exchange.EncodeCSV(rows), nil
}

// unitIDFor recovers a roster's unit id — timingRoster does not carry it
// directly (only its numbers, the export-facing identity), so this helper
// resolves it back for the one CSV export path that also needs settled
// results.
func unitIDFor(r timingRoster) string {
	return r.Numbers.UnitID
}

// TimingImportSummary is the outcome of one .lif/.csv ingest (UC-014
// #3/#4/#5): how many rows applied cleanly vs. need operator resolution.
type TimingImportSummary struct {
	BatchID   string
	Applied   int
	Conflicts int
}

// timingConflictPayload is the JSON shape stored in
// timing_import_conflicts.payload_json — everything ResolveTimingConflict
// needs to apply the row later without re-parsing the original file.
type timingConflictPayload struct {
	EventNumber, Round, Heat int
	Bib                      string
	Lane                     int
	LastName, FirstName      string
	Mark                     string
	Timing                   string
	Status                   string
	StatusDetail             string
	RawStatus                string // the untranslated source status code, if any (unmapped_status conflicts)
	Wind                     string
	ReactionTime             string
}

// timingRow is the ingest pipeline's shared intermediate row shape — .lif
// (via exchange.ClassifyPlace/MapLynxStatus) and generic CSV (already
// carrying explicit CR25 codes) both normalize into this before the same
// matching/conflict logic runs.
type timingRow struct {
	EventNumber, Round, Heat int
	Bib                      string
	Lane                     int
	LastName, FirstName      string
	Mark                     string
	Timing                   domain.Timing
	Status                   domain.QualificationStatus
	StatusDetail             string
	RawStatus                string // set (and Status left StatusNone) when the source status has no CR25 equivalent
	Unmapped                 bool
	Empty                    bool // no ID and no place/status/time at all — nothing to report (an unused lane)
	Wind                     string
	ReactionTime             string
}

// rowsFromLIF normalizes a parsed .lif file into the shared ingest shape.
func rowsFromLIF(ev exchange.Event) []timingRow {
	rows := make([]timingRow, 0, len(ev.Competitors))
	for _, c := range ev.Competitors {
		r := timingRow{
			EventNumber: ev.Number, Round: ev.Round, Heat: ev.Heat,
			Bib: c.ID, Lane: c.Lane, LastName: c.LastName, FirstName: c.FirstName,
			Mark: c.Time, Timing: domain.TimingElectronic, ReactionTime: c.ReacTime, Wind: ev.Wind,
		}
		kind, _ := exchange.ClassifyPlace(c.Place)
		switch kind {
		case exchange.PlaceEmpty:
			if c.ID == "" || c.ID == "0" {
				r.Empty = true
			}
		case exchange.PlaceMappedStatus:
			r.Status, _ = exchange.MapLynxStatus(c.Place)
			r.Mark = ""
		case exchange.PlaceUnmappedStatus, exchange.PlaceUnrecognized:
			r.Unmapped = true
			r.RawStatus = c.Place
			r.Mark = ""
		}
		rows = append(rows, r)
	}
	return rows
}

// rowsFromCSV normalizes the generic CSV fallback (SYS-062) into the
// shared ingest shape.
func rowsFromCSV(generic []exchange.GenericRow) []timingRow {
	rows := make([]timingRow, 0, len(generic))
	for _, g := range generic {
		r := timingRow{
			EventNumber: g.EventNumber, Round: g.Round, Heat: g.Heat,
			Bib: g.Bib, Lane: g.Lane, LastName: g.LastName, FirstName: g.FirstName,
			Mark: g.Mark, Timing: domain.Timing(g.Timing), StatusDetail: g.StatusDetail,
			Wind: g.Wind, ReactionTime: g.ReactionTime,
		}
		if g.Status != "" {
			status := domain.QualificationStatus(g.Status)
			if _, detailErr := statusIsOperatorSettable(status); detailErr {
				r.Status = status
			} else {
				r.Unmapped = true
				r.RawStatus = g.Status
			}
			r.Mark = ""
		}
		if r.Mark == "" && r.Status == domain.StatusNone && !r.Unmapped {
			r.Empty = true
		}
		rows = append(rows, r)
	}
	return rows
}

// statusIsOperatorSettable reports whether status is one this system's CR
// 25 capture vocabulary recognizes (SYS-045) — used to classify a generic
// CSV row's Status column the same way MapLynxStatus classifies a Lynx
// Place code.
func statusIsOperatorSettable(status domain.QualificationStatus) (domain.QualificationStatus, bool) {
	switch status {
	case domain.StatusDNS, domain.StatusDNF, domain.StatusDQ, domain.StatusNM:
		return status, true
	default:
		return domain.StatusNone, false
	}
}

// ImportTimingFile ingests one .lif or generic-CSV file (SYS-061, UC-014
// #3/#4/#5). format is "lif" or "csv". Every row that resolves
// unambiguously to a not-yet-announced unit/athlete with no conflicting
// existing manual result is applied immediately; everything else is
// queued in timing_import_conflicts for ResolveTimingConflict — never
// silently applied, never silently dropped.
func (s *ResultsService) ImportTimingFile(ctx context.Context, actor Session, meetID, filename, format string, data []byte) (TimingImportSummary, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return TimingImportSummary{}, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return TimingImportSummary{}, err
	}

	var rows []timingRow
	switch format {
	case "lif":
		ev, err := exchange.ParseLIF(data)
		if err != nil {
			return TimingImportSummary{}, fmt.Errorf("parse .lif file %q: %w", filename, err)
		}
		rows = rowsFromLIF(ev)
	case "csv":
		generic, err := exchange.ParseCSV(data)
		if err != nil {
			return TimingImportSummary{}, fmt.Errorf("parse generic CSV file %q: %w", filename, err)
		}
		rows = rowsFromCSV(generic)
	default:
		return TimingImportSummary{}, fmt.Errorf("unknown timing import format %q", format)
	}

	batch, err := store.CreateTimingImportBatch(ctx, s.db, store.TimingImportBatch{
		MeetID: meetID, Format: format, Filename: filename, ImportedBy: actor.AccountID,
	})
	if err != nil {
		return TimingImportSummary{}, err
	}

	summary := TimingImportSummary{BatchID: batch.ID}
	participants, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return TimingImportSummary{}, err
	}
	athleteByBib := map[string]string{}
	for _, p := range participants {
		if p.Bib != "" {
			athleteByBib[p.Bib] = p.AthleteID
		}
	}

	for _, row := range rows {
		if row.Empty {
			continue
		}
		unitID, uErr := store.FindUnitByTimingNumbers(ctx, s.db, meetID, row.EventNumber, row.Round, row.Heat)
		if errors.Is(uErr, store.ErrNotFound) {
			summary.Conflicts++
			if _, err := s.queueTimingConflict(ctx, batch.ID, "", "", row, "unresolved_unit"); err != nil {
				return TimingImportSummary{}, err
			}
			continue
		}
		if uErr != nil {
			return TimingImportSummary{}, uErr
		}

		athleteID := athleteByBib[row.Bib]
		if athleteID == "" {
			summary.Conflicts++
			if _, err := s.queueTimingConflict(ctx, batch.ID, unitID, "", row, "unknown_bib"); err != nil {
				return TimingImportSummary{}, err
			}
			continue
		}

		if row.Unmapped {
			summary.Conflicts++
			if _, err := s.queueTimingConflict(ctx, batch.ID, unitID, athleteID, row, "unmapped_status"); err != nil {
				return TimingImportSummary{}, err
			}
			continue
		}

		conflictReason, err := s.timingWriteConflict(ctx, unitID, athleteID)
		if err != nil {
			return TimingImportSummary{}, err
		}
		if conflictReason != "" {
			summary.Conflicts++
			if _, err := s.queueTimingConflict(ctx, batch.ID, unitID, athleteID, row, conflictReason); err != nil {
				return TimingImportSummary{}, err
			}
			continue
		}

		source := "import_" + format
		if _, err := s.applyTimingRow(ctx, actor, meetID, unitID, athleteID, row, source); err != nil {
			return TimingImportSummary{}, err
		}
		summary.Applied++
	}

	if err := s.updateBatchCounts(ctx, batch.ID, summary.Applied, summary.Conflicts); err != nil {
		return TimingImportSummary{}, err
	}
	return summary, nil
}

// updateBatchCounts records the batch's as-ingested applied/conflicted
// counts (a snapshot — later conflict resolutions do not rewrite it; the
// conflicts table's own status column is the live source of truth for
// "how many are still pending", see ListTimingImportConflicts).
func (s *ResultsService) updateBatchCounts(ctx context.Context, batchID string, applied, conflicted int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE timing_import_batches SET applied = ?, conflicted = ? WHERE id = ?`,
		applied, conflicted, batchID)
	return err
}

// timingWriteConflict decides, for a row that resolved to a specific
// unit+athlete, whether it can be applied directly or must be queued
// (SYS-061's "no silent overwrite", and TASK-019's announce/correction
// guard): "unit_announced" wins over "existing_manual_result" when both
// are true, since the announcement guard is the stronger invariant (any
// further write, manual-sourced or not, needs the audited correction
// path once a unit is posted).
func (s *ResultsService) timingWriteConflict(ctx context.Context, unitID, athleteID string) (reason string, err error) {
	state, err := s.protestState(ctx, unitID)
	if err != nil {
		return "", err
	}
	if state.Announced {
		return "unit_announced", nil
	}
	existing, err := store.GetResult(ctx, s.db, unitID, athleteID)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if existing.Source == "manual" && (existing.Mark != "" || existing.Status != domain.StatusNone) {
		return "existing_manual_result", nil
	}
	return "", nil
}

// queueTimingConflict persists one unresolved row.
func (s *ResultsService) queueTimingConflict(ctx context.Context, batchID, unitID, athleteID string, row timingRow, reason string) (store.TimingImportConflict, error) {
	payload, _ := json.Marshal(timingConflictPayload{
		EventNumber: row.EventNumber, Round: row.Round, Heat: row.Heat, Bib: row.Bib, Lane: row.Lane,
		LastName: row.LastName, FirstName: row.FirstName, Mark: row.Mark, Timing: string(row.Timing),
		Status: string(row.Status), StatusDetail: row.StatusDetail, RawStatus: row.RawStatus,
		Wind: row.Wind, ReactionTime: row.ReactionTime,
	})
	return store.CreateTimingImportConflict(ctx, s.db, store.TimingImportConflict{
		BatchID: batchID, UnitID: unitID, AthleteID: athleteID, Bib: row.Bib, Lane: row.Lane,
		Reason: reason, PayloadJSON: string(payload),
	})
}

// applyTimingRow writes one non-conflicting (or freshly-resolved) row's
// result (SYS-040/041, wind/lane auto-population mirroring
// SaveTrackResult) tagged with its import provenance (source). Unlike
// SaveTrackResult it does not enforce SYS-045's "a DQ requires a rule
// reference": raw timing-device data structurally cannot supply one
// (OQ-048) — the office completes it later via the ordinary correction
// flow (TASK-019, CorrectResult), which does enforce it.
func (s *ResultsService) applyTimingRow(ctx context.Context, actor Session, meetID, unitID, athleteID string, row timingRow, source string) (store.ResultRecord, error) {
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	result := domain.Result{UnitID: unitID, AthleteID: athleteID, Status: row.Status, StatusDetail: row.StatusDetail}
	timing := domain.TimingNone
	if row.Status == domain.StatusNone {
		mark, terr := parseTimingMark(row.Mark, row.Timing)
		if terr != nil {
			return store.ResultRecord{}, terr
		}
		result.Mark = mark
		timing = row.Timing
		p, err := s.participant(ctx, meetID, athleteID)
		if err != nil {
			return store.ResultRecord{}, err
		}
		if result.Points, err = s.scorePoints(ctx, s.db, uc.meet, uc.disc.Code, timing, p.Athlete.Sex, result.Mark); err != nil {
			return store.ResultRecord{}, err
		}
	}
	if uc.disc.WindRelevant {
		if result.Wind, err = store.GetUnitWind(ctx, s.db, unitID); err != nil {
			return store.ResultRecord{}, err
		}
	}
	if lanes, err := s.laneByAthlete(ctx, unitID); err != nil {
		return store.ResultRecord{}, err
	} else if lane, ok := lanes[athleteID]; ok {
		result.Lane = lane
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ResultRecord{}, fmt.Errorf("apply timing row: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, err := store.SaveResultWithSource(ctx, tx, result, timing, source)
	if err != nil {
		return store.ResultRecord{}, err
	}
	after, _ := json.Marshal(map[string]any{
		"mark": rec.Mark, "timing": timing, "status": row.Status, "source": source,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.import", EntityType: "result", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return store.ResultRecord{}, fmt.Errorf("audit result.import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.ResultRecord{}, fmt.Errorf("apply timing row: %w", err)
	}
	s.notifyChanged(meetID)
	return rec, nil
}

// parseTimingMark applies the same SYS-040/041 encoding rules
// SaveTrackResult uses (hand-time round-up for manual, 0.01 s for
// electronic) to an imported mark.
func parseTimingMark(mark string, timing domain.Timing) (string, error) {
	switch timing {
	case domain.TimingManual:
		return domain.RoundUpHandTime(mark)
	case domain.TimingElectronic:
		return domain.ValidateFATTime(mark)
	default:
		return "", fmt.Errorf("a timing method is required for a time (SYS-041)")
	}
}

// ListTimingImportBatches returns a meet's ingest history (office view).
func (s *ResultsService) ListTimingImportBatches(ctx context.Context, actor Session, meetID string) ([]store.TimingImportBatch, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	return store.ListTimingImportBatches(ctx, s.db, meetID)
}

// ListTimingImportConflicts returns a meet's conflict-resolution queue.
func (s *ResultsService) ListTimingImportConflicts(ctx context.Context, actor Session, meetID string) ([]store.TimingImportConflict, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	return store.ListTimingImportConflicts(ctx, s.db, meetID)
}

// ResolveTimingConflictInput is one operator decision on a queued conflict
// (UC-014 #4/#5).
type ResolveTimingConflictInput struct {
	// Action is "keep" (leave any existing result as-is), "replace" (apply
	// the imported/overridden data), "merge" (fill only the existing
	// result's blank fields from the imported data) or "discard" (drop the
	// row, nothing was or will be written).
	Action string
	// ReassignAthleteID resolves an unknown_bib conflict to a specific
	// athlete when Action is "replace" or "merge".
	ReassignAthleteID string
	// OverrideStatus resolves an unmapped_status conflict (FS/SC/ADV or an
	// unrecognized code, OQ-048) to a CR 25 code when Action is "replace".
	OverrideStatus       domain.QualificationStatus
	OverrideStatusDetail string
	// Reason is required when the target unit is announced (SYS-046,
	// mirrors CorrectResult's requirement).
	Reason string
	// Escalation is required when the target unit's protest window has
	// already elapsed (SYS-047, mirrors CorrectResult's ErrEscalationRequired).
	Escalation string
}

// ErrTimingConflictReassignRequired means resolving an unknown_bib
// conflict with "replace"/"merge" needs ReassignAthleteID.
var ErrTimingConflictReassignRequired = errors.New("resolving an unknown-bib conflict requires ReassignAthleteID")

// ErrTimingConflictStatusRequired means resolving an unmapped_status
// conflict with "replace" needs OverrideStatus.
var ErrTimingConflictStatusRequired = errors.New("resolving an unmapped-status conflict requires OverrideStatus")

// ErrTimingConflictUnitUnresolved means the conflict has no unit at all
// (an unresolved_unit conflict) — this slice only supports discarding
// those; manually pointing a whole file at a different unit is a follow-up
// (OQ-049).
var ErrTimingConflictUnitUnresolved = errors.New("this conflict's file matched no unit; only \"discard\" is supported (OQ-049)")

// ResolveTimingConflict applies an operator's decision on one queued
// conflict (UC-014 #4/#5): office-only, one-shot (the store layer refuses
// a second resolution), and every write path — direct (unit not
// announced) or through the audited correction flow (unit announced) —
// goes through the same guards ordinary capture/correction does.
func (s *ResultsService) ResolveTimingConflict(ctx context.Context, actor Session, meetID, conflictID string, in ResolveTimingConflictInput) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	conflict, err := store.GetTimingImportConflict(ctx, s.db, conflictID)
	if err != nil {
		return err
	}
	batch, err := store.GetTimingImportBatch(ctx, s.db, conflict.BatchID)
	if err != nil {
		return err
	}
	if batch.MeetID != meetID {
		return fmt.Errorf("conflict %s does not belong to meet %s: %w", conflictID, meetID, store.ErrNotFound)
	}

	if in.Action == "discard" || in.Action == "keep" {
		status := "discarded"
		if in.Action == "keep" {
			status = "kept"
		}
		return store.ResolveTimingImportConflict(ctx, s.db, conflictID, status, actor.AccountID)
	}
	if in.Action != "replace" && in.Action != "merge" {
		return fmt.Errorf("unknown resolution action %q", in.Action)
	}
	if conflict.UnitID == "" {
		return ErrTimingConflictUnitUnresolved
	}

	var payload timingConflictPayload
	if err := json.Unmarshal([]byte(conflict.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("conflict %s: decode payload: %w", conflictID, err)
	}

	athleteID := conflict.AthleteID
	if athleteID == "" {
		athleteID = in.ReassignAthleteID
		if athleteID == "" {
			return ErrTimingConflictReassignRequired
		}
	}
	status := domain.QualificationStatus(payload.Status)
	statusDetail := payload.StatusDetail
	if conflict.Reason == "unmapped_status" {
		if in.OverrideStatus == domain.StatusNone {
			return ErrTimingConflictStatusRequired
		}
		status, statusDetail = in.OverrideStatus, in.OverrideStatusDetail
	}

	row := timingRow{
		EventNumber: payload.EventNumber, Round: payload.Round, Heat: payload.Heat, Bib: payload.Bib, Lane: payload.Lane,
		LastName: payload.LastName, FirstName: payload.FirstName, Mark: payload.Mark, Timing: domain.Timing(payload.Timing),
		Status: status, StatusDetail: statusDetail, Wind: payload.Wind, ReactionTime: payload.ReactionTime,
	}

	if in.Action == "merge" {
		existing, err := store.GetResult(ctx, s.db, conflict.UnitID, athleteID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if existing.Mark != "" {
			row.Mark, row.Timing = existing.Mark, existing.Timing
		}
		if existing.Status != domain.StatusNone {
			row.Status, row.StatusDetail = existing.Status, existing.StatusDetail
		}
	}

	state, err := s.protestState(ctx, conflict.UnitID)
	if err != nil {
		return err
	}
	source := "import_" + batch.Format
	if state.Announced {
		if strings.TrimSpace(in.Reason) == "" {
			return ErrCorrectionReasonRequired
		}
		_, err = s.CorrectResult(ctx, actor, meetID, conflict.UnitID, athleteID, CorrectionInput{
			Mark: row.Mark, Timing: row.Timing, Status: row.Status, StatusDetail: row.StatusDetail,
			Reason: in.Reason, Escalation: in.Escalation,
		})
	} else {
		_, err = s.applyTimingRow(ctx, actor, meetID, conflict.UnitID, athleteID, row, source)
	}
	if err != nil {
		return err
	}

	resolvedStatus := "replaced"
	if in.Action == "merge" {
		resolvedStatus = "merged"
	}
	return store.ResolveTimingImportConflict(ctx, s.db, conflictID, resolvedStatus, actor.AccountID)
}

// --- Timing agent tokens (ADR-006's hub-first amendment). ---

// timingAgentTokenBytes is the random credential length (32 bytes = 256
// bits, matching the session-token entropy convention in session.go).
const timingAgentTokenBytes = 32

// CreateTimingAgentToken issues a new bearer credential for the unattended
// timing agent (office-level action, audited). The plaintext token is
// returned exactly once — only its SHA-256 hash is persisted (see
// 0018_timing_exchange.sql), the same trade-off session tokens already
// make, kept because the agent must keep authenticating across hub
// restarts.
func (s *ResultsService) CreateTimingAgentToken(ctx context.Context, actor Session, meetID, label string) (plaintext string, id string, err error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return "", "", err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return "", "", err
	}
	buf := make([]byte, timingAgentTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate timing agent token: %w", err)
	}
	plaintext = hex.EncodeToString(buf)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("create timing agent token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tok, err := store.CreateTimingAgentToken(ctx, tx, store.TimingAgentToken{
		MeetID: meetID, Label: label, TokenHash: hashTimingAgentToken(plaintext), CreatedBy: actor.AccountID,
	})
	if err != nil {
		return "", "", err
	}
	after, _ := json.Marshal(map[string]string{"meet": meetID, "label": label})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "timing_agent_token.create",
		EntityType: "timing_agent_token", EntityID: tok.ID, After: string(after),
	}); err != nil {
		return "", "", fmt.Errorf("audit timing_agent_token.create: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("create timing agent token: %w", err)
	}
	return plaintext, tok.ID, nil
}

// ListTimingAgentTokens returns a meet's issued agent tokens (never the
// plaintext — only label/creation/revocation metadata).
func (s *ResultsService) ListTimingAgentTokens(ctx context.Context, actor Session, meetID string) ([]store.TimingAgentToken, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	return store.ListTimingAgentTokens(ctx, s.db, meetID)
}

// RevokeTimingAgentToken disables a token immediately (audited).
func (s *ResultsService) RevokeTimingAgentToken(ctx context.Context, actor Session, meetID, tokenID string) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	tok, err := store.GetTimingAgentTokenByID(ctx, s.db, tokenID)
	if err != nil {
		return err
	}
	if tok.MeetID != meetID {
		return fmt.Errorf("token %s does not belong to meet %s: %w", tokenID, meetID, store.ErrNotFound)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("revoke timing agent token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := store.RevokeTimingAgentToken(ctx, tx, tokenID); err != nil {
		return err
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "timing_agent_token.revoke",
		EntityType: "timing_agent_token", EntityID: tokenID,
	}); err != nil {
		return fmt.Errorf("audit timing_agent_token.revoke: %w", err)
	}
	return tx.Commit()
}

// ErrTimingAgentUnauthorized means the presented bearer token does not
// match any live (non-revoked) agent credential.
var ErrTimingAgentUnauthorized = errors.New("invalid or revoked timing agent token")

// AuthenticateTimingAgent resolves a presented bearer token to the meet it
// is scoped to and a synthetic Session the rest of ResultsService's
// office-level methods (ExportTimingFiles, ImportTimingFile) can be called
// with — the timing agent, once authenticated via its meet-scoped token,
// acts with competition-office trust for that meet's timing exchange only
// (it never gets a real account or broader capabilities).
func (s *ResultsService) AuthenticateTimingAgent(ctx context.Context, token string) (Session, string, error) {
	tok, err := store.GetTimingAgentTokenByHash(ctx, s.db, hashTimingAgentToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return Session{}, "", ErrTimingAgentUnauthorized
	}
	if err != nil {
		return Session{}, "", err
	}
	return Session{AccountID: "timing-agent:" + tok.Label, Username: tok.Label, Role: RoleCompetitionOffice}, tok.MeetID, nil
}

func hashTimingAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
