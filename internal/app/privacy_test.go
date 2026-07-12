// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

var admin = Session{AccountID: "01ADM", Username: "admin", Role: RoleInstanceAdmin}

// TestSetConsentSYS103UC023_2_3 covers the SYS-103 consent-toggle
// use-case: an office-level actor can withdraw/restore an athlete's
// publication consent (allow path), a field official cannot (deny path,
// SYS-090 least privilege), the change is reflected in the very next
// Standings() read (UC-023 #3: "within one publication cycle" — nothing
// here caches consent state), and the audit trail documents the change
// without embedding the athlete's name in the log body.
func TestSetConsentSYS103UC023_2_3(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	meet := createUKCMeet(t, meets)

	p, err := results.RegisterParticipant(ctx, office, meet.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}

	t.Run("deny: field official cannot change consent", func(t *testing.T) {
		err := results.SetConsent(ctx, fieldOfficial, p.AthleteID, true)
		if _, ok := err.(ErrForbidden); !ok {
			t.Fatalf("SetConsent by field official: err = %v, want ErrForbidden", err)
		}
	})

	t.Run("allow: office withdraws consent, reflected on the next Standings() read", func(t *testing.T) {
		if err := results.SetConsent(ctx, office, p.AthleteID, true); err != nil {
			t.Fatalf("SetConsent: %v", err)
		}
		standings, err := results.Standings(ctx, meet.ID)
		if err != nil {
			t.Fatal(err)
		}
		row := findStandingRow(t, standings, p.AthleteID)
		if !row.Consent.ResultsPublicationWithdrawn {
			t.Fatal("UC-023 #3: withdrawn consent must show up on the very next Standings() read")
		}
	})

	t.Run("allow: office restores consent", func(t *testing.T) {
		if err := results.SetConsent(ctx, office, p.AthleteID, false); err != nil {
			t.Fatalf("SetConsent: %v", err)
		}
		standings, err := results.Standings(ctx, meet.ID)
		if err != nil {
			t.Fatal(err)
		}
		row := findStandingRow(t, standings, p.AthleteID)
		if row.Consent.ResultsPublicationWithdrawn {
			t.Fatal("restored consent must show up on the very next Standings() read")
		}
	})

	t.Run("audit row documents the change without the athlete's name", func(t *testing.T) {
		trail, err := store.AuditTrail(ctx, st.DB(), "athlete", p.AthleteID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range trail {
			if e.Action != "athlete.consent.update" {
				continue
			}
			found = true
			if strings.Contains(e.After, "Anna") || strings.Contains(e.After, "Muster") {
				t.Fatalf("consent audit row must not embed the athlete's name: %q", e.After)
			}
		}
		if !found {
			t.Fatal("expected at least one athlete.consent.update audit row")
		}
	})

	t.Run("deny: unknown athlete id", func(t *testing.T) {
		if err := results.SetConsent(ctx, office, "does-not-exist", true); err == nil {
			t.Fatal("expected an error for an unknown athlete id")
		}
	})
}

func findStandingRow(t *testing.T, standings MeetStandings, athleteID string) StandingRow {
	t.Helper()
	for _, div := range standings.Divisions {
		for _, row := range div.Rows {
			if row.AthleteID == athleteID {
				return row
			}
		}
	}
	t.Fatalf("athlete %s not found in standings", athleteID)
	return StandingRow{}
}

// TestExportAthleteDataSYS101UC024_1 covers the SYS-101 subject-access
// export: office level and above may export one athlete's full stored
// data (allow), a field official may not (deny), and the export never
// includes another athlete's participation/result rows (UC-024 #1:
// "nothing about other persons").
func TestExportAthleteDataSYS101UC024_1(t *testing.T) {
	meets, results, st := newTestResults(t)
	privacy := NewPrivacyService(st.DB())
	ctx := context.Background()
	meet := createUKCMeet(t, meets)

	a, err := results.RegisterParticipant(ctx, office, meet.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := results.RegisterParticipant(ctx, office, meet.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := results.SaveResult(ctx, office2Capture(), meet.ID, ResultInput{AthleteID: a.AthleteID, DisciplineCode: "60m", Timing: domain.TimingManual, Mark: "9.50"}); err != nil {
		t.Fatal(err)
	}
	if _, err := results.SaveResult(ctx, office2Capture(), meet.ID, ResultInput{AthleteID: b.AthleteID, DisciplineCode: "60m", Timing: domain.TimingManual, Mark: "10.10"}); err != nil {
		t.Fatal(err)
	}

	t.Run("deny: field official cannot export", func(t *testing.T) {
		if _, err := privacy.ExportAthleteData(ctx, fieldOfficial, a.AthleteID); err == nil {
			t.Fatal("expected ErrForbidden")
		}
	})

	t.Run("allow: office exports the athlete's own data only", func(t *testing.T) {
		export, err := privacy.ExportAthleteData(ctx, office, a.AthleteID)
		if err != nil {
			t.Fatalf("ExportAthleteData: %v", err)
		}
		if export.FirstName != "Anna" || export.LastName != "Muster" {
			t.Errorf("export identity = %+v", export)
		}
		if len(export.Participations) != 1 || export.Participations[0].Bib != "1" {
			t.Fatalf("Participations = %+v, want exactly bib 1", export.Participations)
		}
		if len(export.Results) != 1 || export.Results[0].Mark != "9.50" {
			t.Fatalf("Results = %+v, want exactly the athlete's own mark", export.Results)
		}
		for _, p := range export.Participations {
			if p.Bib == "2" {
				t.Fatal("export leaked another athlete's participation")
			}
		}
		for _, r := range export.Results {
			if r.Mark == "10.10" {
				t.Fatal("export leaked another athlete's result")
			}
		}
	})

	t.Run("deny: unknown athlete id", func(t *testing.T) {
		if _, err := privacy.ExportAthleteData(ctx, office, "does-not-exist"); err == nil {
			t.Fatal("expected an error for an unknown athlete id")
		}
	})
}

// office2Capture is a competition-office session used to save results in
// these tests — CapCaptureResults only requires RoleFieldOfficial and
// above, and office already carries that.
func office2Capture() Session { return office }

// TestEraseAthleteSYS101UC024_2 covers the SYS-101 erasure use-case: an
// office-level actor can erase an athlete's identifying data (allow), a
// field official cannot (deny), a second erasure of an already-anonymized
// athlete is refused (deny), and — the UC-024 #2 acceptance criterion —
// the sporting result itself (mark, points, standings membership) survives
// unchanged while the identity is pseudonymized. The audit row documents
// that an erasure happened without embedding the pre-erasure name.
func TestEraseAthleteSYS101UC024_2(t *testing.T) {
	meets, results, st := newTestResults(t)
	privacy := NewPrivacyService(st.DB())
	ctx := context.Background()
	meet := createUKCMeet(t, meets)

	p, err := results.RegisterParticipant(ctx, office, meet.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := results.SaveResult(ctx, office, meet.ID, ResultInput{AthleteID: p.AthleteID, DisciplineCode: "60m", Timing: domain.TimingManual, Mark: "9.50"}); err != nil {
		t.Fatal(err)
	}

	t.Run("deny: field official cannot erase", func(t *testing.T) {
		if err := privacy.EraseAthlete(ctx, fieldOfficial, p.AthleteID, "test"); err == nil {
			t.Fatal("expected ErrForbidden")
		}
	})

	t.Run("allow: office erases the athlete's identity", func(t *testing.T) {
		if err := privacy.EraseAthlete(ctx, office, p.AthleteID, "subject request"); err != nil {
			t.Fatalf("EraseAthlete: %v", err)
		}
		got, err := store.GetAthlete(ctx, st.DB(), p.AthleteID)
		if err != nil {
			t.Fatal(err)
		}
		if got.FirstName == "Anna" || got.LastName == "Muster" {
			t.Error("erased athlete must not retain the original name")
		}
		if !got.Anonymized {
			t.Error("Anonymized flag must be set")
		}
	})

	t.Run("UC-024 #2: the sporting result survives unchanged", func(t *testing.T) {
		standings, err := results.Standings(ctx, meet.ID)
		if err != nil {
			t.Fatal(err)
		}
		row := findStandingRow(t, standings, p.AthleteID)
		if row.FirstName == "Anna" {
			t.Error("standings must reflect the pseudonym, not the erased name")
		}
		found := false
		for _, m := range row.Marks {
			if m.Mark == "9.50" {
				found = true
			}
		}
		if !found {
			t.Fatalf("the mark 9.50 must survive erasure intact: got %+v", row.Marks)
		}
	})

	t.Run("deny: repeat erasure of an already-anonymized athlete is refused", func(t *testing.T) {
		if err := privacy.EraseAthlete(ctx, office, p.AthleteID, "again"); err != ErrAthleteAnonymized {
			t.Fatalf("err = %v, want ErrAthleteAnonymized", err)
		}
	})

	t.Run("audit row documents erasure without the pre-erasure name", func(t *testing.T) {
		trail, err := store.AuditTrail(ctx, st.DB(), "athlete", p.AthleteID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range trail {
			if e.Action != "athlete.erase" {
				continue
			}
			found = true
			if strings.Contains(e.After, "Anna") || strings.Contains(e.After, "Muster") || strings.Contains(e.Before, "Anna") {
				t.Fatalf("erasure audit row must never embed the pre-erasure name: before=%q after=%q", e.Before, e.After)
			}
			if e.Reason != "subject request" {
				t.Errorf("Reason = %q, want %q", e.Reason, "subject request")
			}
		}
		if !found {
			t.Fatal("expected one athlete.erase audit row")
		}
	})
}

// TestPurgeExpiredSYS102UC024_3 covers the SYS-102 retention purge:
// personal data belonging to a meet that ended more than the configured
// retention period ago is purged (allow), data belonging to a still-
// current meet is left untouched (deny), the action requires
// instance-admin (deny for office), and a repeat verification query finds
// no remaining out-of-retention data (UC-024 #3's own acceptance wording).
// The clock is injected (PrivacyService.WithClock) so the cutoff is
// deterministic regardless of when the test suite runs.
func TestPurgeExpiredSYS102UC024_3(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	privacy := NewPrivacyService(st.DB()).WithClock(func() time.Time { return now })

	oldMeet, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Alter Wettkampf", Venue: "V", HomologationRef: "H",
		StartDate: now.AddDate(0, 0, -200), EndDate: now.AddDate(0, 0, -200),
		Tier: "C-Meeting", CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet (old): %v", err)
	}
	recentMeet, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Neuer Wettkampf", Venue: "V", HomologationRef: "H",
		StartDate: now.AddDate(0, 0, -10), EndDate: now.AddDate(0, 0, -10),
		Tier: "C-Meeting", CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet (recent): %v", err)
	}

	oldParticipant, err := results.RegisterParticipant(ctx, office, oldMeet.ID, ParticipantInput{
		FirstName: "Old", LastName: "Athlete", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	recentParticipant, err := results.RegisterParticipant(ctx, office, recentMeet.ID, ParticipantInput{
		FirstName: "Recent", LastName: "Athlete", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("deny: office cannot trigger the instance-wide purge", func(t *testing.T) {
		if _, err := privacy.PurgeExpired(ctx, office, DefaultRetentionDays); err == nil {
			t.Fatal("expected ErrForbidden")
		}
	})

	t.Run("allow: instance-admin runs the purge", func(t *testing.T) {
		report, err := privacy.PurgeExpired(ctx, admin, DefaultRetentionDays)
		if err != nil {
			t.Fatalf("PurgeExpired: %v", err)
		}
		if report.AthletesPurged != 1 {
			t.Errorf("AthletesPurged = %d, want 1 (only the old-meet-only athlete)", report.AthletesPurged)
		}
		if report.AuditRowsRedacted == 0 {
			t.Error("expected at least one redacted audit row (the old meet's participant.register entry)")
		}

		oldGot, err := store.GetAthlete(ctx, st.DB(), oldParticipant.AthleteID)
		if err != nil {
			t.Fatal(err)
		}
		if oldGot.Consent.RecordedBy != "" {
			t.Error("old-meet-only athlete's consent-recorder identity must be purged")
		}

		recentGot, err := store.GetAthlete(ctx, st.DB(), recentParticipant.AthleteID)
		if err != nil {
			t.Fatal(err)
		}
		if recentGot.Consent.RecordedBy == "" {
			t.Error("recent-meet athlete's consent-recorder identity must survive (deny: not out of retention)")
		}
	})

	t.Run("verification query finds no remaining out-of-retention data", func(t *testing.T) {
		remaining, err := store.FindAthletesOutsideRetention(ctx, st.DB(), now.AddDate(0, 0, -DefaultRetentionDays))
		if err != nil {
			t.Fatal(err)
		}
		if len(remaining) != 0 {
			t.Errorf("FindAthletesOutsideRetention after purge = %v, want none", remaining)
		}
	})
}

// TestPurgeExpiredAtStartupAuditsAsSystemSYS102 covers the process-start
// sweep's audit trail: it runs with no authenticated actor and records
// "system" so it is distinguishable from an admin-triggered purge.
func TestPurgeExpiredAtStartupAuditsAsSystemSYS102(t *testing.T) {
	_, _, st := newTestResults(t)
	ctx := context.Background()
	privacy := NewPrivacyService(st.DB())

	if _, err := privacy.PurgeExpiredAtStartup(ctx, DefaultRetentionDays); err != nil {
		t.Fatalf("PurgeExpiredAtStartup: %v", err)
	}
	events, err := store.ListAudit(ctx, st.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Action == "privacy.retention_purge" {
			found = true
			if e.Actor != "system" {
				t.Errorf("Actor = %q, want %q", e.Actor, "system")
			}
		}
	}
	if !found {
		t.Fatal("expected a privacy.retention_purge audit row")
	}
}

// TestEntryFlowsCollectConsentSYS103UC023 covers TASK-023's UC-023
// requirement that every online entry flow (TASK-016: individual, club
// bulk, relay legs) collects the SYS-103 publication-consent choice at
// submission time: a withdrawal checked on the form lands on the created
// athlete's stored consent (deny path for later public rendering), an
// unchecked one records the not-withdrawn baseline (allow path), and both
// carry the submitting actor + timestamp as the consent's audit context.
func TestEntryFlowsCollectConsentSYS103UC023(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()

	assertConsent := func(t *testing.T, athleteID string, wantWithdrawn bool) {
		t.Helper()
		a, err := store.GetAthlete(ctx, f.st.DB(), athleteID)
		if err != nil {
			t.Fatalf("GetAthlete: %v", err)
		}
		if a.Consent.ResultsPublicationWithdrawn != wantWithdrawn {
			t.Errorf("withdrawn = %v, want %v", a.Consent.ResultsPublicationWithdrawn, wantWithdrawn)
		}
		if a.Consent.RecordedBy != entrySubmitter.AccountID {
			t.Errorf("RecordedBy = %q, want the submitting actor %q", a.Consent.RecordedBy, entrySubmitter.AccountID)
		}
		if a.Consent.RecordedAt.IsZero() {
			t.Error("RecordedAt must be stamped at submission time")
		}
	}

	t.Run("individual entry: withdrawal collected", func(t *testing.T) {
		detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
			EventID: f.eventID, FirstName: "Wanda", LastName: "Withdrawn",
			BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "13.50",
			PublicationWithdrawn: true,
		})
		if err != nil {
			t.Fatalf("SubmitIndividualEntry: %v", err)
		}
		assertConsent(t, detail.AthleteID, true)
	})

	t.Run("bulk entry: per-line consent, never per batch", func(t *testing.T) {
		details, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, BulkEntryInput{
			Club: "LC Bulk",
			Lines: []BulkEntryLine{
				{FirstName: "Paula", LastName: "Public", BirthYear: 2011, Sex: domain.SexFemale,
					EventID: f.eventID, SeedPerformance: "13.60"},
				{FirstName: "Nora", LastName: "NichtOeffentlich", BirthYear: 2011, Sex: domain.SexFemale,
					EventID: f.eventID, SeedPerformance: "13.70", PublicationWithdrawn: true},
			},
		})
		if err != nil {
			t.Fatalf("SubmitClubBulkEntries: %v", err)
		}
		assertConsent(t, details[0].AthleteID, false)
		assertConsent(t, details[1].AthleteID, true)
	})

	t.Run("relay legs: per-leg consent", func(t *testing.T) {
		relayEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "4x100m", CategoryCodes: []string{"U16 W"}})
		detail, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
			EventID: relayEventID, Club: "LC Staffel",
			Composition: []RelayLegInput{
				{FirstName: "Lea", LastName: "LegOne", BirthYear: 2011, Sex: domain.SexFemale, PublicationWithdrawn: true},
				{FirstName: "Mia", LastName: "LegTwo", BirthYear: 2011, Sex: domain.SexFemale},
			},
		})
		if err != nil {
			t.Fatalf("SubmitRelayEntry: %v", err)
		}
		if detail.RelayTeam == nil || len(detail.RelayTeam.RelayTeamRecord.Composition) != 2 {
			t.Fatalf("relay detail = %+v, want a 2-leg team", detail.RelayTeam)
		}
		// RelayTeamDetail's own Composition field holds display names; the
		// embedded record's Composition holds the leg athlete IDs.
		assertConsent(t, detail.RelayTeam.RelayTeamRecord.Composition[0], true)
		assertConsent(t, detail.RelayTeam.RelayTeamRecord.Composition[1], false)
	})
}

// TestImportPreservesConsentSYS103UC023 covers the TASK-017 CSV-import
// path's consent semantics (TASK-023): an import file carries no consent
// artifact, so (allow/default path) a newly import-created athlete gets
// the not-withdrawn baseline with NO RecordedBy/RecordedAt claiming a
// consent interaction that never happened; and (deny path) re-importing a
// row that matches an EXISTING withdrawn athlete never silently
// re-publishes them — their withdrawal survives the import untouched.
func TestImportPreservesConsentSYS103UC023(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	// An existing, publication-withdrawn athlete (as the office privacy
	// worklist would have recorded it).
	existing, err := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{
		FirstName: "Wanda", LastName: "Withdrawn", BirthYear: 2001, Sex: domain.SexFemale,
		Consent: domain.PublicationConsent{
			ResultsPublicationWithdrawn: true,
			RecordedAt:                  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			RecordedBy:                  "office-1",
		},
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	csvBody := systemNativeCSVHeader + "\n" +
		systemNativeCSVRow("Wanda", "Withdrawn", "2001", "W", "LC Test", "", "100m/Women", "", "12.80") + "\n" +
		systemNativeCSVRow("Nina", "Neu", "2001", "W", "LC Test", "", "100m/Women", "", "12.90") + "\n"
	report, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("CommitCSVImport: %v", err)
	}
	if report.Accepted != 2 {
		t.Fatalf("accepted = %d, want 2 (%+v)", report.Accepted, report)
	}

	t.Run("deny: a matched existing athlete's withdrawal survives re-import", func(t *testing.T) {
		got, err := store.GetAthlete(ctx, f.st.DB(), existing.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Consent.ResultsPublicationWithdrawn {
			t.Error("import must never silently re-publish a withdrawn athlete")
		}
		if got.Consent.RecordedBy != "office-1" {
			t.Errorf("consent audit context clobbered by import: RecordedBy = %q", got.Consent.RecordedBy)
		}
	})

	t.Run("default: an import-created athlete is not withdrawn, with no fabricated consent interaction", func(t *testing.T) {
		created, err := store.FindAthleteByNaturalKey(ctx, f.st.DB(), "Nina", "Neu", 2001, domain.SexFemale)
		if err != nil {
			t.Fatalf("FindAthleteByNaturalKey: %v", err)
		}
		if created.Consent.ResultsPublicationWithdrawn {
			t.Error("import-created athlete must default to not withdrawn")
		}
		if created.Consent.RecordedBy != "" || !created.Consent.RecordedAt.IsZero() {
			t.Errorf("import must not fabricate a consent interaction: %+v", created.Consent)
		}
	})
}
