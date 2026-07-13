// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// RecordListOption is one wired record list, for a meet-settings selector
// (UC-016: "a loaded meeting-record list").
type RecordListOption struct {
	ID   string
	Name string
}

// RecordListOptions lists every record list this service is wired with
// (SetRecordLists), sorted by ID.
func (s *ResultsService) RecordListOptions() []RecordListOption {
	out := make([]RecordListOption, 0, len(s.recordLists))
	for id, l := range s.recordLists {
		out = append(out, RecordListOption{ID: id, Name: l.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MeetRecordListIDs returns the record-list IDs currently configured for
// meetID.
func (s *ResultsService) MeetRecordListIDs(ctx context.Context, meetID string) ([]string, error) {
	return store.ListMeetRecordListIDs(ctx, s.db, meetID)
}

// SetMeetRecordLists configures which loadable record lists (SYS-049)
// evaluate meetID's results, replacing any previous configuration —
// organizer-level meet setup, mirroring AddEvent's CapOrganizeMeet gate.
func (s *ResultsService) SetMeetRecordLists(ctx context.Context, actor Session, meetID string, listIDs []string) error {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return err
	}
	for _, id := range listIDs {
		if _, ok := s.recordLists[id]; !ok {
			return fmt.Errorf("unknown record list %q", id)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set meet record lists: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := store.ReplaceMeetRecordLists(ctx, tx, meetID, listIDs); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]any{"recordListIds": listIDs})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "meet.record_lists.set",
		EntityType: "meet", EntityID: meetID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit meet record lists: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set meet record lists: %w", err)
	}
	return nil
}

// evaluateRecord resolves every input domain.EvaluateRecord needs for one
// just-captured mark — the meet's configured reference lists filtered to
// this discipline/category (and, for meeting records, this meet), plus the
// athlete's in-system PB/SB history — and returns the full evaluation
// (SYS-049/050). db must be the caller's open handle (same single-
// connection-pool constraint scorePoints documents: every capture call site
// runs this inside its own transaction). excludeUnitID lets a re-capture
// before announcement (a corrected mark) exclude its own prior save from
// counting as history to beat (see
// store.ListAthleteResultsByDiscipline's doc).
func (s *ResultsService) evaluateRecord(ctx context.Context, db store.DBTX, meet store.MeetRecord, disc domain.Discipline,
	athlete domain.Athlete, mark string, timing domain.Timing, wind *float64, excludeUnitID string) (domain.RecordEvaluation, error) {
	scheme, ok := s.schemes[meet.CategorySchemeID]
	if !ok {
		return domain.RecordEvaluation{}, fmt.Errorf("meet %s references unknown category scheme %q", meet.ID, meet.CategorySchemeID)
	}
	// An athlete outside every category band (should not happen for a
	// registered participant, but ResolveDefaultCategory can fail for an
	// edge-of-range age) simply gets no reference-record candidates —
	// PB/SB history-based flagging still applies below.
	cat, _ := scheme.ResolveDefaultCategory(athlete.BirthYear, athlete.Sex, meet.StartDate)

	listIDs, err := store.ListMeetRecordListIDs(ctx, db, meet.ID)
	if err != nil {
		return domain.RecordEvaluation{}, err
	}
	var refs []domain.RecordReference
	for _, id := range listIDs {
		list, ok := s.recordLists[id]
		if !ok {
			continue // configured but not wired into this instance: skip, not an error
		}
		for _, r := range list.Records {
			if r.DisciplineCode != disc.Code || r.CategoryCode != cat.Code {
				continue
			}
			if r.Type == domain.RecordMeeting && r.MeetID != meet.ID {
				continue // a meeting record belongs to one specific meet (SYS-049)
			}
			refs = append(refs, r)
		}
	}

	history, err := store.ListAthleteResultsByDiscipline(ctx, db, athlete.ID, disc.Code, excludeUnitID)
	if err != nil {
		return domain.RecordEvaluation{}, err
	}
	hist := make([]domain.MarkHistory, len(history))
	for i, h := range history {
		hist[i] = domain.MarkHistory{Mark: h.Mark, Wind: h.Wind, WindRelevant: disc.WindRelevant, Year: h.MeetStartDate.Year()}
	}
	best, hasBest, seasonBest, hasSeasonBest := domain.BestMarks(disc.Family, hist, meet.StartDate.Year())

	return domain.EvaluateRecord(domain.RecordEvaluationInput{
		Family:             disc.Family,
		DisciplineCode:     disc.Code,
		Mark:               mark,
		Timing:             timing,
		Wind:               wind,
		WindRelevant:       disc.WindRelevant,
		References:         refs,
		HasPriorBest:       hasBest,
		PriorBest:          best,
		HasPriorSeasonBest: hasSeasonBest,
		PriorSeasonBest:    seasonBest,
	}), nil
}

// RecordChecklistView is the SYS-051 record-documentation checklist for one
// flagged result (UC-016 #4): the Rekordprotokoll fields available
// in-system, with gaps the operator must complete by hand explicitly
// marked.
type RecordChecklistView struct {
	ResultID    string
	AthleteID   string
	RecordFlags []string
	Items       []domain.RecordChecklistItem
}

// RecordChecklist assembles the checklist for one unit/athlete's settled
// result. Callable for any result, not only a flagged one — an empty
// RecordFlags simply means the checklist documents a mark that did not, in
// the end, earn a record flag (UC-016 #3's "reason inspectable" companion:
// the checklist view is where an operator looks up why).
func (s *ResultsService) RecordChecklist(ctx context.Context, meetID, unitID, athleteID string) (RecordChecklistView, error) {
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return RecordChecklistView{}, err
	}
	result, err := store.GetResult(ctx, s.db, unitID, athleteID)
	if err != nil {
		return RecordChecklistView{}, err
	}
	results, err := store.ListUnitResults(ctx, s.db, unitID)
	if err != nil {
		return RecordChecklistView{}, err
	}
	items := domain.BuildRecordChecklist(result.Timing, uc.disc.WindRelevant, result.Wind, len(results))
	return RecordChecklistView{
		ResultID: result.ID, AthleteID: athleteID, RecordFlags: result.RecordFlags, Items: items,
	}, nil
}
