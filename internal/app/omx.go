// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/exchange"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- omx/v1 full-meet export/import (TASK-025, UC-027, UC-021; SYS-073,
// SYS-083, SYS-144). ExportOMXDocument/ExportOMXResultsCSV are the
// organizer-triggered federation-delivery artifacts (SYS-073); both are
// computed synchronously from live data with no background job, so a meet
// closed at time T has its export available immediately at T (UC-027 #3).
// ImportOMXDocument reconstructs a full meet from a Document into
// whatever store it is pointed at — normally a fresh instance (UC-027
// #2's round-trip scenario), but nothing about it requires that; it is a
// plain "create everything this document describes" operation. ---

// ExportOMXDocument builds the full omx/v1 snapshot of a meet (UC-027
// #1): every entity needed to reconstruct its official results.
func (s *ResultsService) ExportOMXDocument(ctx context.Context, actor Session, meetID string) (exchange.Document, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return exchange.Document{}, err
	}
	return BuildOMXDocument(ctx, s.db, meetID, s.now().UTC())
}

// ExportOMXResultsCSV builds the same snapshot's flat CSV derivative
// (SYS-073's spreadsheet-consumer artifact, ADR-005 §2).
func (s *ResultsService) ExportOMXResultsCSV(ctx context.Context, actor Session, meetID string) ([]byte, error) {
	doc, err := s.ExportOMXDocument(ctx, actor, meetID)
	if err != nil {
		return nil, err
	}
	return exchange.EncodeOMXResultsCSV(doc), nil
}

// BuildOMXDocument is ExportOMXDocument's unauthenticated core, exported
// so the round-trip property-test generator (and any future CLI/batch
// export path) can build documents directly.
func BuildOMXDocument(ctx context.Context, db store.DBTX, meetID string, exportedAt time.Time) (exchange.Document, error) {
	meetRec, err := store.GetMeet(ctx, db, meetID)
	if err != nil {
		return exchange.Document{}, fmt.Errorf("build omx document: %w", err)
	}

	doc := exchange.Document{
		SchemaVersion: exchange.SchemaVersionOMXV1,
		ExportedAt:    exportedAt,
		Meet:          meetDocFrom(meetRec.Meet),
	}

	sessions, err := store.ListSessions(ctx, db, meetID)
	if err != nil {
		return exchange.Document{}, err
	}
	for _, sess := range sessions {
		doc.Sessions = append(doc.Sessions, exchange.SessionDoc{ID: sess.ID, Day: sess.Day, Label: sess.Label})
	}

	participants, err := store.ListParticipants(ctx, db, meetID)
	if err != nil {
		return exchange.Document{}, err
	}
	entries, err := store.ListEntriesByMeet(ctx, db, meetID)
	if err != nil {
		return exchange.Document{}, err
	}
	results, err := store.ListMeetResults(ctx, db, meetID)
	if err != nil {
		return exchange.Document{}, err
	}

	athleteIDs := map[string]bool{}
	for _, p := range participants {
		athleteIDs[p.AthleteID] = true
	}
	relayTeamIDs := map[string]bool{}
	for _, e := range entries {
		if e.AthleteID != "" {
			athleteIDs[e.AthleteID] = true
		}
		if e.RelayTeamID != "" {
			relayTeamIDs[e.RelayTeamID] = true
		}
	}
	for _, r := range results {
		athleteIDs[r.AthleteID] = true
	}

	clubIDs := map[string]bool{}
	for _, id := range sortedKeys(athleteIDs) {
		a, err := store.GetAthlete(ctx, db, id)
		if err != nil {
			return exchange.Document{}, fmt.Errorf("build omx document: athlete %s: %w", id, err)
		}
		for _, c := range a.ClubIDs {
			clubIDs[c] = true
		}
		doc.Athletes = append(doc.Athletes, athleteDocFrom(a.Athlete))
	}

	for _, id := range sortedKeys(relayTeamIDs) {
		t, err := store.GetRelayTeam(ctx, db, id)
		if err != nil {
			return exchange.Document{}, fmt.Errorf("build omx document: relay team %s: %w", id, err)
		}
		clubIDs[t.ClubID] = true
		doc.RelayTeams = append(doc.RelayTeams, relayTeamDocFrom(t.RelayTeam))
	}

	for _, id := range sortedKeys(clubIDs) {
		c, err := store.GetClub(ctx, db, id)
		if err != nil {
			return exchange.Document{}, fmt.Errorf("build omx document: club %s: %w", id, err)
		}
		doc.Clubs = append(doc.Clubs, clubDocFrom(c.Club))
	}

	events, err := store.ListEvents(ctx, db, meetID)
	if err != nil {
		return exchange.Document{}, err
	}
	for _, ev := range events {
		doc.Events = append(doc.Events, eventDocFrom(ev.Event))
		rounds, err := store.ListRounds(ctx, db, ev.ID)
		if err != nil {
			return exchange.Document{}, err
		}
		for seq, rd := range rounds {
			doc.Rounds = append(doc.Rounds, exchange.RoundDoc{ID: rd.ID, EventID: rd.EventID, Kind: string(rd.Kind), Seq: seq})

			units, err := store.ListRoundUnits(ctx, db, rd.ID)
			if err != nil {
				return exchange.Document{}, err
			}
			for _, u := range units {
				ud := exchange.UnitDoc{ID: u.ID, RoundID: u.RoundID, Location: u.Location}
				if !u.ScheduledAt.IsZero() {
					t := u.ScheduledAt
					ud.ScheduledAt = &t
				}
				wind, err := store.GetUnitWind(ctx, db, u.ID)
				if err != nil {
					return exchange.Document{}, err
				}
				ud.Wind = wind
				if at, found, err := store.LatestAnnouncement(ctx, db, u.ID); err != nil {
					return exchange.Document{}, err
				} else if found {
					ud.AnnouncedAt = &at
				}
				doc.Units = append(doc.Units, ud)

				assignments, err := store.ListUnitAssignments(ctx, db, u.ID)
				if err != nil {
					return exchange.Document{}, err
				}
				for _, a := range assignments {
					doc.Assignments = append(doc.Assignments, exchange.AssignmentDoc{
						ID: a.ID, UnitID: a.UnitID, EntryID: a.EntryID, SeedRank: a.SeedRank,
						Lane: a.Lane, Qualification: string(a.Qualification), ManualOverride: a.ManualOverride,
					})
				}
			}
		}
	}

	for _, e := range entries {
		doc.Entries = append(doc.Entries, exchange.EntryDoc{
			ID: e.ID, EventID: e.EventID, AthleteID: e.AthleteID, RelayTeamID: e.RelayTeamID,
			SeedPerformance: e.SeedPerformance, Status: string(e.Status), Source: string(e.Source),
			StartedUp: e.StartedUp, StartedDown: e.StartedDown, FailsStandard: e.FailsStandard,
		})
	}

	for _, r := range results {
		doc.Results = append(doc.Results, exchange.ResultDoc{
			ID: r.ID, UnitID: r.UnitID, AthleteID: r.AthleteID, Lane: r.Lane, Mark: r.Mark,
			Timing: string(r.Timing), Wind: r.Wind, Status: string(r.Status), StatusDetail: r.StatusDetail,
			Points: r.Points, Placing: r.Placing, RecordFlags: r.RecordFlags, Source: r.Source,
		})
	}

	if err := exchange.ValidateOMX(doc); err != nil {
		return exchange.Document{}, fmt.Errorf("build omx document: %w", err)
	}
	return doc, nil
}

func meetDocFrom(m domain.Meet) exchange.MeetDoc {
	return exchange.MeetDoc{
		ID: m.ID, Name: m.Name, Venue: m.Venue, HomologationRef: m.HomologationRef,
		StartDate: m.StartDate, EndDate: m.EndDate, Organizer: m.Organizer, Tier: string(m.Tier),
		Status: string(m.Status), CategorySchemeID: m.CategorySchemeID, TemplateID: m.TemplateID,
		ScoringTableID: m.ScoringTableID, ResultsPositioning: string(m.ResultsPositioning),
		OfficialSourceName: m.OfficialSourceName, OfficialSourceURL: m.OfficialSourceURL,
		ExternalIDs: m.ExternalIDs, EntryFeeCents: m.EntryFeeCents, RelayFeeCents: m.RelayFeeCents,
	}
}

func athleteDocFrom(a domain.Athlete) exchange.AthleteDoc {
	return exchange.AthleteDoc{
		ID: a.ID, FirstName: a.FirstName, LastName: a.LastName, BirthDate: a.BirthDate,
		BirthYear: a.BirthYear, Sex: string(a.Sex), Nationality: a.Nationality,
		ClubIDs: a.ClubIDs, ExternalIDs: a.ExternalIDs, ParaClasses: a.ParaClasses,
	}
}

func relayTeamDocFrom(t domain.RelayTeam) exchange.RelayTeamDoc {
	return exchange.RelayTeamDoc{ID: t.ID, ClubID: t.ClubID, Composition: t.Composition, Reserves: t.Reserves}
}

func clubDocFrom(c domain.Club) exchange.ClubDoc {
	return exchange.ClubDoc{ID: c.ID, Name: c.Name, ExternalIDs: c.ExternalIDs}
}

func eventDocFrom(e domain.Event) exchange.EventDoc {
	return exchange.EventDoc{
		ID: e.ID, DisciplineCode: e.DisciplineCode, CategoryCodes: e.CategoryCodes,
		EntryStandard: e.EntryStandard, EntryDeadline: e.EntryDeadline,
		Status: string(e.Status), EntryLimit: e.EntryLimit,
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ImportOMXDocument reconstructs a full meet from doc into db (UC-027 #2:
// "the export and a fresh system... the export is re-imported"), creating
// every entity fresh — every id in the returned meet's storage is new;
// doc's own ids only ever describe cross-references WITHIN doc itself
// (exchange.Document's own doc comment). Runs as one transaction: a
// partially-applied import is never left visible. Returns the new meet's
// id.
func ImportOMXDocument(ctx context.Context, db *sql.DB, doc exchange.Document) (meetID string, err error) {
	if err := exchange.ValidateOMX(doc); err != nil {
		return "", err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("import omx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	meetRec, err := store.CreateMeet(ctx, tx, domain.Meet{
		Name: doc.Meet.Name, Venue: doc.Meet.Venue, HomologationRef: doc.Meet.HomologationRef,
		StartDate: doc.Meet.StartDate, EndDate: doc.Meet.EndDate, Organizer: doc.Meet.Organizer,
		Tier: domain.MeetTier(doc.Meet.Tier), Status: domain.MeetStatus(doc.Meet.Status),
		CategorySchemeID: doc.Meet.CategorySchemeID, TemplateID: doc.Meet.TemplateID,
		ScoringTableID: doc.Meet.ScoringTableID, ResultsPositioning: domain.ResultsPositioning(doc.Meet.ResultsPositioning),
		OfficialSourceName: doc.Meet.OfficialSourceName, OfficialSourceURL: doc.Meet.OfficialSourceURL,
		ExternalIDs: doc.Meet.ExternalIDs, EntryFeeCents: doc.Meet.EntryFeeCents, RelayFeeCents: doc.Meet.RelayFeeCents,
	})
	if err != nil {
		return "", fmt.Errorf("import omx: create meet: %w", err)
	}
	meetID = meetRec.ID

	for _, sd := range doc.Sessions {
		if _, err := store.CreateSession(ctx, tx, domain.Session{MeetID: meetID, Day: sd.Day, Label: sd.Label}); err != nil {
			return "", fmt.Errorf("import omx: session: %w", err)
		}
	}

	clubIDMap := map[string]string{}
	for _, cd := range doc.Clubs {
		c, err := store.CreateClub(ctx, tx, domain.Club{Name: cd.Name, ExternalIDs: cd.ExternalIDs})
		if err != nil {
			return "", fmt.Errorf("import omx: club %s: %w", cd.Name, err)
		}
		clubIDMap[cd.ID] = c.ID
	}

	athleteIDMap := map[string]string{}
	for _, ad := range doc.Athletes {
		a, err := store.CreateAthlete(ctx, tx, domain.Athlete{
			FirstName: ad.FirstName, LastName: ad.LastName, BirthDate: ad.BirthDate, BirthYear: ad.BirthYear,
			Sex: domain.Sex(ad.Sex), Nationality: ad.Nationality, ClubIDs: remapIDs(ad.ClubIDs, clubIDMap),
			ExternalIDs: ad.ExternalIDs, ParaClasses: ad.ParaClasses,
		})
		if err != nil {
			return "", fmt.Errorf("import omx: athlete %s %s: %w", ad.FirstName, ad.LastName, err)
		}
		athleteIDMap[ad.ID] = a.ID
		// Capture requires a participant row to exist (ResultsService's
		// participant() lookup) — every athlete this document carries a
		// result/entry for must be registered. Bib is not part of the
		// omx/v1 official-result contract (OQ-057): re-registering without
		// one is safe, multiple empty bibs never collide
		// (idx_participants_bib excludes '').
		if _, err := store.RegisterParticipant(ctx, tx, meetID, a.ID, ""); err != nil {
			return "", fmt.Errorf("import omx: register participant %s %s: %w", ad.FirstName, ad.LastName, err)
		}
	}

	relayTeamIDMap := map[string]string{}
	for _, rtd := range doc.RelayTeams {
		club, ok := clubIDMap[rtd.ClubID]
		if !ok {
			return "", fmt.Errorf("import omx: relay team %s: unknown club %s", rtd.ID, rtd.ClubID)
		}
		t, err := store.CreateRelayTeam(ctx, tx, domain.RelayTeam{
			ClubID: club, Composition: remapIDs(rtd.Composition, athleteIDMap), Reserves: remapIDs(rtd.Reserves, athleteIDMap),
		})
		if err != nil {
			return "", fmt.Errorf("import omx: relay team: %w", err)
		}
		relayTeamIDMap[rtd.ID] = t.ID
	}

	eventIDMap := map[string]string{}
	for _, ed := range doc.Events {
		e, err := store.CreateEvent(ctx, tx, domain.Event{
			MeetID: meetID, DisciplineCode: ed.DisciplineCode, CategoryCodes: ed.CategoryCodes,
			EntryStandard: ed.EntryStandard, EntryDeadline: ed.EntryDeadline,
			Status: domain.EventStatus(ed.Status), EntryLimit: ed.EntryLimit,
		})
		if err != nil {
			return "", fmt.Errorf("import omx: event %s: %w", ed.DisciplineCode, err)
		}
		eventIDMap[ed.ID] = e.ID
	}

	roundsByEvent := map[string][]exchange.RoundDoc{}
	for _, rd := range doc.Rounds {
		roundsByEvent[rd.EventID] = append(roundsByEvent[rd.EventID], rd)
	}
	roundIDMap := map[string]string{}
	for _, eventKey := range sortedRoundEventKeys(roundsByEvent) {
		list := roundsByEvent[eventKey]
		sort.Slice(list, func(i, j int) bool { return list[i].Seq < list[j].Seq })
		newEventID, ok := eventIDMap[eventKey]
		if !ok {
			return "", fmt.Errorf("import omx: round: unknown event %s", eventKey)
		}
		for i, rd := range list {
			r, err := store.CreateRound(ctx, tx, domain.Round{EventID: newEventID, Kind: domain.RoundKind(rd.Kind)}, i)
			if err != nil {
				return "", fmt.Errorf("import omx: round: %w", err)
			}
			roundIDMap[rd.ID] = r.ID
		}
	}

	unitIDMap := map[string]string{}
	for _, ud := range doc.Units {
		newRoundID, ok := roundIDMap[ud.RoundID]
		if !ok {
			return "", fmt.Errorf("import omx: unit %s: unknown round %s", ud.ID, ud.RoundID)
		}
		u := domain.Unit{RoundID: newRoundID, Location: ud.Location}
		if ud.ScheduledAt != nil {
			u.ScheduledAt = *ud.ScheduledAt
		}
		rec, err := store.CreateUnit(ctx, tx, u)
		if err != nil {
			return "", fmt.Errorf("import omx: unit: %w", err)
		}
		unitIDMap[ud.ID] = rec.ID
		if ud.Wind != nil {
			if err := store.SetUnitWind(ctx, tx, rec.ID, *ud.Wind); err != nil {
				return "", fmt.Errorf("import omx: unit wind: %w", err)
			}
		}
		if ud.AnnouncedAt != nil {
			if _, _, err := store.AnnounceUnit(ctx, tx, rec.ID, "omx-import"); err != nil {
				return "", fmt.Errorf("import omx: announce unit: %w", err)
			}
		}
	}

	entryIDMap := map[string]string{}
	for _, ent := range doc.Entries {
		newEventID, ok := eventIDMap[ent.EventID]
		if !ok {
			return "", fmt.Errorf("import omx: entry %s: unknown event %s", ent.ID, ent.EventID)
		}
		e := domain.Entry{
			EventID: newEventID, SeedPerformance: ent.SeedPerformance,
			Status: domain.EntryStatus(ent.Status), Source: domain.EntrySource(ent.Source),
			StartedUp: ent.StartedUp, StartedDown: ent.StartedDown, FailsStandard: ent.FailsStandard,
		}
		if ent.AthleteID != "" {
			newID, ok := athleteIDMap[ent.AthleteID]
			if !ok {
				return "", fmt.Errorf("import omx: entry %s: unknown athlete %s", ent.ID, ent.AthleteID)
			}
			e.AthleteID = newID
		}
		if ent.RelayTeamID != "" {
			newID, ok := relayTeamIDMap[ent.RelayTeamID]
			if !ok {
				return "", fmt.Errorf("import omx: entry %s: unknown relay team %s", ent.ID, ent.RelayTeamID)
			}
			e.RelayTeamID = newID
		}
		rec, err := store.CreateEntry(ctx, tx, e)
		if err != nil {
			return "", fmt.Errorf("import omx: entry: %w", err)
		}
		entryIDMap[ent.ID] = rec.ID
	}

	for _, a := range doc.Assignments {
		newUnitID, ok := unitIDMap[a.UnitID]
		if !ok {
			return "", fmt.Errorf("import omx: unit assignment %s: unknown unit %s", a.ID, a.UnitID)
		}
		newEntryID, ok := entryIDMap[a.EntryID]
		if !ok {
			return "", fmt.Errorf("import omx: unit assignment %s: unknown entry %s", a.ID, a.EntryID)
		}
		if _, err := store.SaveUnitAssignment(ctx, tx, domain.UnitAssignment{
			UnitID: newUnitID, EntryID: newEntryID, SeedRank: a.SeedRank, Lane: a.Lane,
			Qualification: domain.QualificationStatus(a.Qualification), ManualOverride: a.ManualOverride,
		}); err != nil {
			return "", fmt.Errorf("import omx: unit assignment: %w", err)
		}
	}

	for _, r := range doc.Results {
		newUnitID, ok := unitIDMap[r.UnitID]
		if !ok {
			return "", fmt.Errorf("import omx: result %s: unknown unit %s", r.ID, r.UnitID)
		}
		newAthleteID, ok := athleteIDMap[r.AthleteID]
		if !ok {
			return "", fmt.Errorf("import omx: result %s: unknown athlete %s", r.ID, r.AthleteID)
		}
		source := r.Source
		if source == "" {
			source = "manual"
		}
		if _, err := store.SaveResultWithSource(ctx, tx, domain.Result{
			UnitID: newUnitID, AthleteID: newAthleteID, Lane: r.Lane, Mark: r.Mark,
			Wind: r.Wind, Status: domain.QualificationStatus(r.Status), StatusDetail: r.StatusDetail,
			Points: r.Points, Placing: r.Placing, RecordFlags: r.RecordFlags,
		}, domain.Timing(r.Timing), source); err != nil {
			return "", fmt.Errorf("import omx: result: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("import omx: %w", err)
	}
	return meetID, nil
}

func remapIDs(ids []string, m map[string]string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if nid, ok := m[id]; ok {
			out = append(out, nid)
		}
	}
	return out
}

func sortedRoundEventKeys(m map[string][]exchange.RoundDoc) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
