// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// TestExportOMXDocumentAndCSVSYS073UC027_1 exercises the authorized,
// service-level export entry points (ExportOMXDocument/ExportOMXResultsCSV)
// end to end: office role required, and a captured result is present in
// both artifacts with the fields UC-027 #1 names.
func TestExportOMXDocumentAndCSVSYS073UC027_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"})

	if _, err := results.ExportOMXDocument(ctx, fieldOfficial, rec.ID); err == nil {
		t.Error("ExportOMXDocument as a field official: want authorization error (office capability required)")
	}

	doc, err := results.ExportOMXDocument(ctx, office, rec.ID)
	if err != nil {
		t.Fatalf("ExportOMXDocument: %v", err)
	}
	if doc.Meet.ID != rec.ID || len(doc.Results) != 1 || doc.Results[0].AthleteID != anna.AthleteID {
		t.Fatalf("exported document = %+v, want the captured Anna result", doc)
	}
	if doc.Results[0].Mark != "3.42" {
		t.Errorf("exported mark = %q, want 3.42", doc.Results[0].Mark)
	}

	if _, err := results.ExportOMXResultsCSV(ctx, fieldOfficial, rec.ID); err == nil {
		t.Error("ExportOMXResultsCSV as a field official: want authorization error")
	}
	csvBytes, err := results.ExportOMXResultsCSV(ctx, office, rec.ID)
	if err != nil {
		t.Fatalf("ExportOMXResultsCSV: %v", err)
	}
	if !strings.Contains(string(csvBytes), "Muster") || !strings.Contains(string(csvBytes), "3.42") {
		t.Errorf("exported CSV missing expected fields: %s", csvBytes)
	}
}

// TestExportOMXDocumentAvailableImmediatelyAtCloseSYS073UC027_3 proves
// UC-027 #3: a meet closed at time T has its export available immediately
// at T — no background job, no post-processing delay. ExportOMXDocument is
// a synchronous read; this asserts the closed status is reflected in the
// very next call with no polling/retry needed.
func TestExportOMXDocumentAvailableImmediatelyAtCloseSYS073UC027_3(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	if _, err := store.SetMeetStatus(ctx, st.DB(), rec.ID, rec.Version, domain.MeetClosed); err != nil {
		t.Fatalf("SetMeetStatus: %v", err)
	}
	doc, err := results.ExportOMXDocument(ctx, office, rec.ID)
	if err != nil {
		t.Fatalf("ExportOMXDocument immediately after close: %v", err)
	}
	if doc.Meet.Status != string(domain.MeetClosed) {
		t.Errorf("exported meet status = %q, want closed", doc.Meet.Status)
	}
}

// TestOMXRelayTeamRoundTrip covers the relay-roster half of a full-meet
// export/import (BuildOMXDocument/ImportOMXDocument's relay-team path,
// otherwise unexercised by the individual-entry-only property generator):
// a relay team's club and leg composition survive export and
// re-import, remapped to the fresh system's own athlete/club ids.
func TestOMXRelayTeamRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := openFreshStore(t)

	meet, err := store.CreateMeet(ctx, src.DB(), domain.Meet{
		Name: "Relay Meet", Venue: "Track", StartDate: ukcDay(), EndDate: ukcDay(), Status: domain.MeetClosed,
	})
	if err != nil {
		t.Fatal(err)
	}
	club, err := store.CreateClub(ctx, src.DB(), domain.Club{Name: "LC Relay"})
	if err != nil {
		t.Fatal(err)
	}
	var legIDs []string
	for i := 0; i < 4; i++ {
		a, err := store.CreateAthlete(ctx, src.DB(), domain.Athlete{
			FirstName: "Runner", LastName: "Leg", BirthYear: 2010, Sex: domain.SexMale, ClubIDs: []string{club.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		legIDs = append(legIDs, a.ID)
		if _, err := store.RegisterParticipant(ctx, src.DB(), meet.ID, a.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	team, err := store.CreateRelayTeam(ctx, src.DB(), domain.RelayTeam{ClubID: club.ID, Composition: legIDs})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := store.CreateEvent(ctx, src.DB(), domain.Event{
		MeetID: meet.ID, DisciplineCode: "4x100m", CategoryCodes: []string{"U18 M"}, Status: domain.EventClosed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEntry(ctx, src.DB(), domain.Entry{EventID: ev.ID, RelayTeamID: team.ID, Status: domain.EntryConfirmed}); err != nil {
		t.Fatal(err)
	}

	doc, err := BuildOMXDocument(ctx, src.DB(), meet.ID, ukcDay())
	if err != nil {
		t.Fatalf("BuildOMXDocument: %v", err)
	}
	if len(doc.RelayTeams) != 1 || len(doc.RelayTeams[0].Composition) != 4 {
		t.Fatalf("exported relay teams = %+v", doc.RelayTeams)
	}
	if len(doc.Entries) != 1 || doc.Entries[0].RelayTeamID != team.ID || doc.Entries[0].AthleteID != "" {
		t.Fatalf("exported relay entry = %+v", doc.Entries)
	}

	dst := openFreshStore(t)
	newMeetID, err := ImportOMXDocument(ctx, dst.DB(), doc)
	if err != nil {
		t.Fatalf("ImportOMXDocument: %v", err)
	}
	imported, err := store.ListEntriesByMeet(ctx, dst.DB(), newMeetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 1 || imported[0].RelayTeamID == "" {
		t.Fatalf("imported entries = %+v, want one relay entry with a remapped relay team id", imported)
	}
	importedTeam, err := store.GetRelayTeam(ctx, dst.DB(), imported[0].RelayTeamID)
	if err != nil {
		t.Fatalf("GetRelayTeam(reimported): %v", err)
	}
	if len(importedTeam.Composition) != 4 {
		t.Errorf("reimported relay composition = %v, want 4 legs", importedTeam.Composition)
	}
}
