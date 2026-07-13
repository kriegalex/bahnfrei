// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestAthleteConsentPersistsSYS103UC023_2 covers the SYS-103 consent
// flags' storage round trip: CreateAthlete persists them, GetAthlete
// reads them back, and UpdateAthleteConsent changes them under optimistic
// versioning (allow path) while a stale expected version is refused (deny
// path — SYS-083 conflict surfacing, reused here rather than a bespoke
// mechanism).
func TestAthleteConsentPersistsSYS103UC023_2(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	recordedAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2012, Sex: domain.SexFemale,
		Consent: domain.PublicationConsent{
			ResultsPublicationWithdrawn: true,
			PhotoConsentGiven:           true,
			RecordedAt:                  recordedAt,
			RecordedBy:                  "office-1",
		},
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	got, err := GetAthlete(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if !got.Consent.ResultsPublicationWithdrawn || !got.Consent.PhotoConsentGiven {
		t.Fatalf("consent flags not persisted: %+v", got.Consent)
	}
	if got.Consent.ExtendedDataConsentGiven {
		t.Error("ExtendedDataConsentGiven should default false")
	}
	if !got.Consent.RecordedAt.Equal(recordedAt) || got.Consent.RecordedBy != "office-1" {
		t.Errorf("consent audit context = %+v", got.Consent)
	}

	t.Run("allow: update under the correct expected version", func(t *testing.T) {
		newVersion, err := UpdateAthleteConsent(ctx, s.DB(), created.ID, got.Version, domain.PublicationConsent{
			ResultsPublicationWithdrawn: false, RecordedAt: recordedAt.Add(time.Hour), RecordedBy: "office-2",
		})
		if err != nil {
			t.Fatalf("UpdateAthleteConsent: %v", err)
		}
		if newVersion != got.Version+1 {
			t.Errorf("version = %d, want %d", newVersion, got.Version+1)
		}
		reread, err := GetAthlete(ctx, s.DB(), created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reread.Consent.ResultsPublicationWithdrawn {
			t.Error("consent should now be restored (not withdrawn)")
		}
		if reread.Consent.RecordedBy != "office-2" {
			t.Errorf("RecordedBy = %q, want office-2", reread.Consent.RecordedBy)
		}
	})

	t.Run("deny: stale expected version is refused", func(t *testing.T) {
		_, err := UpdateAthleteConsent(ctx, s.DB(), created.ID, got.Version, domain.PublicationConsent{})
		if !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("UpdateAthleteConsent with stale version: err = %v, want ErrVersionConflict", err)
		}
	})

	t.Run("deny: unknown athlete id is refused", func(t *testing.T) {
		_, err := UpdateAthleteConsent(ctx, s.DB(), "does-not-exist", 1, domain.PublicationConsent{})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

// TestAnonymizeAthleteStoreSYS101UC024_2 covers the SYS-101 erasure
// request's storage layer: AnonymizeAthlete persists the domain layer's
// pseudonym/cleared fields and bumps version (allow path); a stale
// expected version is refused (deny path).
func TestAnonymizeAthleteStoreSYS101UC024_2(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bd := time.Date(2010, 3, 4, 0, 0, 0, 0, time.UTC)
	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Anna", LastName: "Muster", BirthDate: &bd, BirthYear: 2010, Sex: domain.SexFemale,
		ExternalIDs: domain.ExternalIDs{domain.NamespaceSwissAthleticsLicence: "SA-1"},
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	a := created.Athlete
	a.AnonymizePersonalData(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	newVersion, err := AnonymizeAthlete(ctx, s.DB(), a, created.Version)
	if err != nil {
		t.Fatalf("AnonymizeAthlete: %v", err)
	}
	if newVersion != created.Version+1 {
		t.Errorf("version = %d, want %d", newVersion, created.Version+1)
	}

	got, err := GetAthlete(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstName == "Anna" || got.LastName == "Muster" {
		t.Error("anonymized athlete must not retain the original name")
	}
	if got.BirthDate != nil {
		t.Error("anonymized athlete's full birth date must be cleared")
	}
	if got.BirthYear != 2010 {
		t.Errorf("BirthYear must survive erasure: got %d", got.BirthYear)
	}
	if len(got.ExternalIDs) != 0 {
		t.Errorf("ExternalIDs must be cleared: got %v", got.ExternalIDs)
	}
	if !got.Anonymized || got.AnonymizedAt == nil {
		t.Error("Anonymized/AnonymizedAt must be set")
	}

	t.Run("deny: stale expected version is refused", func(t *testing.T) {
		if _, err := AnonymizeAthlete(ctx, s.DB(), a, created.Version); !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("err = %v, want ErrVersionConflict", err)
		}
	})
}

// TestFindAthletesOutsideRetentionSYS102UC024_3 covers the SYS-102
// retention finder's core rule (UC-024 #3): an athlete whose every
// participation is in a meet that ended before the cutoff is selected
// (allow); one with at least one participation still inside the retention
// window — even if they also have an old one — is left untouched (deny),
// since they might return this season.
func TestFindAthletesOutsideRetentionSYS102UC024_3(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	oldMeet, err := CreateMeet(ctx, s.DB(), domain.Meet{
		Name: "Alter Wettkampf", Venue: "V", StartDate: day(2020, 1, 1), EndDate: day(2020, 1, 2), Tier: "C-Meeting",
	})
	if err != nil {
		t.Fatal(err)
	}
	recentMeet, err := CreateMeet(ctx, s.DB(), domain.Meet{
		Name: "Neuer Wettkampf", Venue: "V", StartDate: day(2026, 6, 1), EndDate: day(2026, 6, 2), Tier: "C-Meeting",
	})
	if err != nil {
		t.Fatal(err)
	}

	bd := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	onlyOld, err := CreateAthlete(ctx, s.DB(), domain.Athlete{FirstName: "A", LastName: "Old", BirthDate: &bd, BirthYear: 2010, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}
	oldAndRecent, err := CreateAthlete(ctx, s.DB(), domain.Athlete{FirstName: "B", LastName: "Both", BirthDate: &bd, BirthYear: 2010, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}
	onlyRecent, err := CreateAthlete(ctx, s.DB(), domain.Athlete{FirstName: "C", LastName: "Recent", BirthDate: &bd, BirthYear: 2010, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}
	// Already-purged athlete: fully expired, but nothing left to purge.
	alreadyPurged, err := CreateAthlete(ctx, s.DB(), domain.Athlete{FirstName: "D", LastName: "Purged", BirthYear: 2010, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range []struct{ meetID, athleteID string }{
		{oldMeet.ID, onlyOld.ID},
		{oldMeet.ID, oldAndRecent.ID},
		{recentMeet.ID, oldAndRecent.ID},
		{recentMeet.ID, onlyRecent.ID},
		{oldMeet.ID, alreadyPurged.ID},
	} {
		if _, err := RegisterParticipant(ctx, s.DB(), r.meetID, r.athleteID, ""); err != nil {
			t.Fatalf("RegisterParticipant(%s, %s): %v", r.meetID, r.athleteID, err)
		}
	}

	cutoff := day(2025, 1, 1)
	found, err := FindAthletesOutsideRetention(ctx, s.DB(), cutoff)
	if err != nil {
		t.Fatalf("FindAthletesOutsideRetention: %v", err)
	}
	set := map[string]bool{}
	for _, id := range found {
		set[id] = true
	}

	if !set[onlyOld.ID] {
		t.Error("allow: athlete with only an out-of-retention meet must be selected")
	}
	if set[oldAndRecent.ID] {
		t.Error("deny: athlete with a still-current meet must NOT be selected, even with an old one too")
	}
	if set[onlyRecent.ID] {
		t.Error("deny: athlete with only a current meet must NOT be selected")
	}
	if set[alreadyPurged.ID] {
		t.Error("deny: an athlete with nothing left to purge must NOT be re-selected")
	}
}

// TestPurgeAthleteRetentionDataSYS102UC024_3 covers exactly what the
// SYS-102 retention purge clears vs. preserves per athlete: full birth
// date and the consent-recorder identity are cleared; BirthYear and the
// consent flags/timestamp themselves are kept (clearing a withdrawal flag
// would silently re-publish a suppressed athlete — see
// internal/store/privacy.go's rationale).
func TestPurgeAthleteRetentionDataSYS102UC024_3(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bd := time.Date(2010, 5, 6, 0, 0, 0, 0, time.UTC)
	recordedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Anna", LastName: "Muster", BirthDate: &bd, BirthYear: 2010, Sex: domain.SexFemale,
		Consent: domain.PublicationConsent{ResultsPublicationWithdrawn: true, RecordedAt: recordedAt, RecordedBy: "office-1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := PurgeAthleteRetentionData(ctx, s.DB(), created.ID); err != nil {
		t.Fatalf("PurgeAthleteRetentionData: %v", err)
	}
	got, err := GetAthlete(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BirthDate != nil {
		t.Error("full birth date must be purged")
	}
	if got.BirthYear != 2010 {
		t.Errorf("BirthYear must survive the purge: got %d", got.BirthYear)
	}
	if got.Consent.RecordedBy != "" {
		t.Errorf("consent-recorder identity must be purged: got %q", got.Consent.RecordedBy)
	}
	if !got.Consent.ResultsPublicationWithdrawn {
		t.Error("the withdrawal flag itself must survive the purge (else a suppressed athlete would silently re-publish)")
	}
	if !got.Consent.RecordedAt.Equal(recordedAt) {
		t.Error("RecordedAt must survive the purge")
	}

	// Idempotent: a repeat purge on an already-purged athlete is a no-op,
	// not an error.
	if err := PurgeAthleteRetentionData(ctx, s.DB(), created.ID); err != nil {
		t.Fatalf("repeat PurgeAthleteRetentionData: %v", err)
	}
}

// TestRedactAuditPIISYS102UC024_3 covers the audit_log table's one
// blessed exception to its append-only invariant (SYS-046,
// migrations/0001_audit_log.sql): RedactAuditPII replaces the before/
// after JSON of exactly the targeted rows and leaves every other column —
// and every other row — untouched, then restores the append-only trigger
// before the caller can commit (deny path: a plain UPDATE after
// RedactAuditPII returns must still be refused, proving the trigger is
// really back).
func TestRedactAuditPIISYS102UC024_3(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	seq1, err := appendAudit(t, s, AuditEntry{
		Actor: "office-1", Action: "participant.register", EntityType: "participant", EntityID: "p1",
		After: `{"name":"Anna Muster"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	seq2, err := appendAudit(t, s, AuditEntry{
		Actor: "office-1", Action: "participant.register", EntityType: "participant", EntityID: "p2",
		After: `{"name":"Someone Else"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := RedactAuditPII(ctx, tx, "participant", []string{"p1"})
	if err != nil {
		t.Fatalf("RedactAuditPII: %v", err)
	}
	if n != 1 {
		t.Errorf("redacted %d rows, want 1", n)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	trail, err := AuditTrail(ctx, s.DB(), "participant", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 1 || trail[0].After != AuditRedactionMarker {
		t.Fatalf("p1 after_json = %+v, want redaction marker", trail)
	}
	if trail[0].Actor != "office-1" || trail[0].Action != "participant.register" || trail[0].Seq != seq1 {
		t.Errorf("redaction must preserve actor/action/seq: got %+v", trail[0])
	}

	unredacted, err := AuditTrail(ctx, s.DB(), "participant", "p2")
	if err != nil {
		t.Fatal(err)
	}
	if len(unredacted) != 1 || unredacted[0].After != `{"name":"Someone Else"}` || unredacted[0].Seq != seq2 {
		t.Fatalf("p2 (not targeted) must be untouched: got %+v", unredacted)
	}

	t.Run("deny: append-only trigger is restored after redaction", func(t *testing.T) {
		_, err := s.DB().ExecContext(ctx, `UPDATE audit_log SET reason = 'tampered' WHERE seq = ?`, seq2)
		if err == nil {
			t.Fatal("a plain UPDATE after RedactAuditPII must still be refused by the append-only trigger")
		}
	})

	t.Run("no-op on an empty ID list", func(t *testing.T) {
		tx, err := s.DB().BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		n, err := RedactAuditPII(ctx, tx, "participant", nil)
		if err != nil || n != 0 {
			t.Fatalf("RedactAuditPII(nil) = (%d, %v), want (0, nil)", n, err)
		}
	})
}

// TestListParticipationsAndResultsByAthleteSYS101UC024_1 covers the
// SYS-101 subject-access export's own-data-only scoping (UC-024 #1:
// "nothing about other persons"): two athletes share a meet and a unit,
// and each athlete's participation/result query returns only their own
// rows.
func TestListParticipationsAndResultsByAthleteSYS101UC024_1(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meetID, unitID, athleteA := ukcFixture(t, s)

	athleteB, err := CreateAthlete(ctx, s.DB(), domain.Athlete{FirstName: "Bea", LastName: "Beispiel", BirthYear: 2013, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteA, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteB.ID, "2"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unitID, AthleteID: athleteA, Mark: "8.90"}, domain.TimingManual); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unitID, AthleteID: athleteB.ID, Mark: "9.10"}, domain.TimingManual); err != nil {
		t.Fatal(err)
	}

	parts, err := ListParticipationsByAthlete(ctx, s.DB(), athleteA)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].Bib != "1" {
		t.Fatalf("athlete A participations = %+v, want exactly their own bib \"1\"", parts)
	}

	results, err := ListResultsByAthlete(ctx, s.DB(), athleteA)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Mark != "8.90" {
		t.Fatalf("athlete A results = %+v, want exactly their own mark, nothing from athlete B", results)
	}
}

// TestFindExpiredMeetIDsAndMeetPersonalDataEntityIDsSYS102UC024_3 covers
// the meet-scoped half of the retention sweep: an expired meet's
// participant/result/entry row IDs are the exact set RedactAuditPII
// targets. Entries are included since TASK-029 privacy-review finding #2:
// they were previously omitted from the purge sweep entirely (an entry is
// event-scoped, one hop further from the meet than participants/results).
func TestFindExpiredMeetIDsAndMeetPersonalDataEntityIDsSYS102UC024_3(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meetID, _, athleteID := ukcFixture(t, s)
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteID, "1"); err != nil {
		t.Fatal(err)
	}
	entryEvent, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID: meetID, DisciplineCode: "100m", CategoryCodes: []string{"W12"},
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: entryEvent.ID, AthleteID: athleteID}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	m, err := GetMeet(ctx, s.DB(), meetID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("deny: a meet still inside retention is not expired", func(t *testing.T) {
		// cutoff before the meet's end date: the meet has not aged past
		// retention yet.
		ids, err := FindExpiredMeetIDs(ctx, s.DB(), m.EndDate.AddDate(0, 0, -1))
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			if id == meetID {
				t.Fatal("meet ending after cutoff must not be reported expired")
			}
		}
	})

	t.Run("allow: a meet past the cutoff is expired, and its entity IDs are reported", func(t *testing.T) {
		// cutoff after the meet's end date: the meet is now out of
		// retention.
		ids, err := FindExpiredMeetIDs(ctx, s.DB(), m.EndDate.AddDate(0, 0, 1))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, id := range ids {
			if id == meetID {
				found = true
			}
		}
		if !found {
			t.Fatal("meet ending before cutoff must be reported expired")
		}

		participantIDs, resultIDs, entryIDs, err := MeetPersonalDataEntityIDs(ctx, s.DB(), meetID)
		if err != nil {
			t.Fatal(err)
		}
		if len(participantIDs) != 1 {
			t.Errorf("participantIDs = %v, want exactly 1", participantIDs)
		}
		if len(resultIDs) != 0 {
			t.Errorf("resultIDs = %v, want none (no results saved in this fixture)", resultIDs)
		}
		if len(entryIDs) != 1 {
			t.Errorf("entryIDs = %v, want exactly 1 (TASK-029: entries must be swept too)", entryIDs)
		}
	})
}

// TestAthletePersonalDataEntityIDsSYS101UC024_2 covers the SYS-101
// erasure-scoped counterpart to MeetPersonalDataEntityIDs: an athlete's
// participant/result/entry row IDs are returned regardless of which meet
// they belong to (an athlete is instance-global, SYS-010), and another
// athlete's rows in the very same meet/event/unit are never included.
func TestAthletePersonalDataEntityIDsSYS101UC024_2(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meetID, unitID, athleteA := ukcFixture(t, s)

	athleteB, err := CreateAthlete(ctx, s.DB(), domain.Athlete{FirstName: "Bea", LastName: "Beispiel", BirthYear: 2013, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteA, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteB.ID, "2"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unitID, AthleteID: athleteA, Mark: "8.90"}, domain.TimingManual); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unitID, AthleteID: athleteB.ID, Mark: "9.10"}, domain.TimingManual); err != nil {
		t.Fatal(err)
	}
	entryEvent, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID: meetID, DisciplineCode: "100m", CategoryCodes: []string{"W12"},
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	entryA, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: entryEvent.ID, AthleteID: athleteA})
	if err != nil {
		t.Fatalf("CreateEntry (A): %v", err)
	}
	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: entryEvent.ID, AthleteID: athleteB.ID}); err != nil {
		t.Fatalf("CreateEntry (B): %v", err)
	}

	participantIDs, resultIDs, entryIDs, err := AthletePersonalDataEntityIDs(ctx, s.DB(), athleteA)
	if err != nil {
		t.Fatal(err)
	}
	if len(participantIDs) != 1 {
		t.Errorf("participantIDs = %v, want exactly 1 (athlete A's own participant row)", participantIDs)
	}
	if len(resultIDs) != 1 {
		t.Errorf("resultIDs = %v, want exactly 1 (athlete A's own result row)", resultIDs)
	}
	if len(entryIDs) != 1 || entryIDs[0] != entryA.ID {
		t.Errorf("entryIDs = %v, want exactly [%s] (athlete A's own entry, not athlete B's)", entryIDs, entryA.ID)
	}
}
