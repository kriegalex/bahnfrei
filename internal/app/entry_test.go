// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

var entrySubmitter = Session{AccountID: "01SUB", Username: "submitter", Role: RoleEntrySubmitter}

// entryFixture is a published meet with one open 100m/U16 W event, ready to
// accept online entries (UC-003 #1's "published meet with open entries").
type entryFixture struct {
	meets   *MeetService
	results *ResultsService
	st      *store.Store
	meetID  string
	eventID string
}

func newEntryFixture(t *testing.T) entryFixture {
	t.Helper()
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if err := meets.PublishMeet(ctx, organizer, rec.ID, rec.Version); err != nil {
		t.Fatalf("PublishMeet: %v", err)
	}
	return entryFixture{meets: meets, results: results, st: st, meetID: rec.ID, eventID: ev.ID}
}

// addEvent adds a further event to the fixture's meet (already published),
// returning its ID.
func (f entryFixture) addEvent(t *testing.T, req AddEventRequest) string {
	t.Helper()
	ev, err := f.meets.AddEvent(context.Background(), organizer, f.meetID, req)
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	return ev.ID
}

// TestPublishMeetOpensEntries proves the wired draft→published transition
// (added to unblock UC-003 #1's "published meet" precondition, which had no
// prior path to that status): a draft meet has no open entry events, a
// published one does, and publishing twice is rejected.
func TestPublishMeetOpensEntries(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	draft, err := f.meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := f.meets.AddEvent(ctx, organizer, draft.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	opts, err := f.results.OpenEntryEvents(ctx, draft.ID)
	if err != nil {
		t.Fatalf("OpenEntryEvents: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("a draft meet must offer no open entry events, got %d", len(opts))
	}

	if err := f.meets.PublishMeet(ctx, organizer, draft.ID, draft.Version); err != nil {
		t.Fatalf("PublishMeet: %v", err)
	}
	opts, err = f.results.OpenEntryEvents(ctx, draft.ID)
	if err != nil {
		t.Fatalf("OpenEntryEvents: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("a published meet must offer its open event, got %d", len(opts))
	}

	if err := f.meets.PublishMeet(ctx, organizer, draft.ID, draft.Version+1); !errors.Is(err, ErrMeetNotDraft) {
		t.Errorf("re-publishing an already-published meet = %v, want ErrMeetNotDraft", err)
	}
}

// TestOnlineEntryIndividualSYS011UC003_1 covers UC-003 #1: an athlete's
// online entry with a seed performance before the deadline is stored with
// source online, status entered, and is visible to the submitter.
func TestOnlineEntryIndividualSYS011UC003_1(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()

	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011,
		Sex: domain.SexFemale, Club: "LC Test", SeedPerformance: "13.50",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Status != domain.EntryEntered {
		t.Errorf("status = %q, want entered", detail.Status)
	}
	if detail.Source != domain.EntrySourceOnline {
		t.Errorf("source = %q, want online", detail.Source)
	}
	if detail.SubmittedBy != entrySubmitter.AccountID {
		t.Errorf("submittedBy = %q, want %q", detail.SubmittedBy, entrySubmitter.AccountID)
	}

	mine, err := f.results.MyEntries(ctx, entrySubmitter, f.meetID)
	if err != nil {
		t.Fatalf("MyEntries: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != detail.ID {
		t.Fatalf("MyEntries = %+v, want exactly the submitted entry", mine)
	}

	// The athlete also holds a meet-wide participant slot for later bib
	// assignment (SYS-018).
	participants, err := f.results.Participants(ctx, f.meetID)
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("participants = %+v, want exactly one", participants)
	}
}

// TestSubmitIndividualEntryValidatesAthleteInputSYS010 covers createAthlete's
// input validation (SYS-010): a missing name, a missing/invalid birth year
// and an invalid sex value are all rejected before anything is persisted.
func TestSubmitIndividualEntryValidatesAthleteInputSYS010(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	base := IndividualEntryInput{EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "13.50"}

	t.Run("empty first name", func(t *testing.T) {
		in := base
		in.FirstName = "  "
		if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, in); err == nil {
			t.Error("expected an error for a blank first name")
		}
	})
	t.Run("empty last name", func(t *testing.T) {
		in := base
		in.LastName = ""
		if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, in); err == nil {
			t.Error("expected an error for a blank last name")
		}
	})
	t.Run("missing birth year", func(t *testing.T) {
		in := base
		in.BirthYear = 0
		if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, in); err == nil {
			t.Error("expected an error for a missing birth year")
		}
	})
	t.Run("invalid sex", func(t *testing.T) {
		in := base
		in.Sex = domain.Sex("X")
		if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, in); err == nil {
			t.Error("expected an error for an invalid sex value")
		}
	})
}

// TestOnlineEntryLicenceSetsHasLicenceSYS014DEC023TASK039 covers DEC-023/
// OQ-033's core promise: an online individual entry that supplies a valid
// licence number is stored as the athlete's domain.NamespaceSwissAthleticsLicence
// external id and immediately evaluates HasLicence=true (SYS-014) — a
// licence-required tier (B-Meeting) no longer blocks the entry, closing the
// operator round-trip OQ-033 described.
func TestOnlineEntryLicenceSetsHasLicenceSYS014DEC023TASK039(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()

	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Lena", LastName: "Keller", BirthYear: 1998,
		Sex: domain.SexFemale, SeedPerformance: "12.40", LicenceNo: "SA-1234",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Eligibility.EffectiveOutcome() != domain.EligibilityEligible {
		t.Fatalf("EffectiveOutcome = %q, want eligible (flags %+v)", detail.Eligibility.EffectiveOutcome(), detail.Eligibility.Flags)
	}
	athlete, err := store.GetAthlete(ctx, f.st.DB(), detail.AthleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if id, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-1234" {
		t.Fatalf("licence external id = (%q, %v), want (\"SA-1234\", true)", id, ok)
	}
}

// TestOnlineEntryEmptyLicenceUnchangedBehaviourDEC023TASK039 proves the
// optional field's empty case is a true no-op: an online entry with no
// licence number behaves exactly as before TASK-039 (blocked at a
// licence-required tier, no external id recorded).
func TestOnlineEntryEmptyLicenceUnchangedBehaviourDEC023TASK039(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()

	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Nina", LastName: "Frei", BirthYear: 1998,
		Sex: domain.SexFemale, SeedPerformance: "12.40",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Eligibility.EffectiveOutcome() != domain.EligibilityBlocked {
		t.Fatalf("EffectiveOutcome = %q, want blocked (no licence given)", detail.Eligibility.EffectiveOutcome())
	}
	athlete, err := store.GetAthlete(ctx, f.st.DB(), detail.AthleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if _, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); ok {
		t.Fatal("athlete unexpectedly carries a licence external id when none was submitted")
	}
}

// TestOnlineEntryMalformedLicenceRejectedDEC023TASK039 covers both the
// individual and bulk paths: a licence value that fails
// domain.ValidLicenceNo (here, an embedded newline) is rejected with
// ErrInvalidLicenceNo rather than silently stored as a join key that could
// never match a real licence.
func TestOnlineEntryMalformedLicenceRejectedDEC023TASK039(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()

	t.Run("individual", func(t *testing.T) {
		_, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
			EventID: f.eventID, FirstName: "Rea", LastName: "Nyman", BirthYear: 2000,
			Sex: domain.SexFemale, SeedPerformance: "13.00", LicenceNo: "bad\nlicence",
		})
		if !errors.Is(err, ErrInvalidLicenceNo) {
			t.Fatalf("SubmitIndividualEntry with a malformed licence = %v, want ErrInvalidLicenceNo", err)
		}
	})
	t.Run("bulk", func(t *testing.T) {
		in := BulkEntryInput{Club: "LC Bulk", Lines: []BulkEntryLine{
			{FirstName: "Rea", LastName: "Nyman", BirthYear: 2000, Sex: domain.SexFemale,
				EventID: f.eventID, SeedPerformance: "13.00", LicenceNo: strings.Repeat("x", 41)},
		}}
		if _, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, in); !errors.Is(err, ErrInvalidLicenceNo) {
			t.Fatalf("SubmitClubBulkEntries with a malformed licence = %v, want ErrInvalidLicenceNo", err)
		}
	})
}

// TestClubBulkEntryLicenceSYS014DEC023TASK039 covers the bulk-entry form's
// licence field (UC-003 #2): a valid per-line licence number is stored and
// feeds HasLicence exactly like the individual path.
func TestClubBulkEntryLicenceSYS014DEC023TASK039(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()

	in := BulkEntryInput{Club: "LC Bulk", Lines: []BulkEntryLine{
		{FirstName: "Timo", LastName: "Aeby", BirthYear: 1999, Sex: domain.SexFemale,
			EventID: f.eventID, SeedPerformance: "12.60", LicenceNo: "SA-7788"},
	}}
	details, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, in)
	if err != nil {
		t.Fatalf("SubmitClubBulkEntries: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("created %d entries, want 1", len(details))
	}
	if details[0].Eligibility.EffectiveOutcome() != domain.EligibilityEligible {
		t.Fatalf("EffectiveOutcome = %q, want eligible", details[0].Eligibility.EffectiveOutcome())
	}
	athlete, err := store.GetAthlete(ctx, f.st.DB(), details[0].AthleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if id, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-7788" {
		t.Fatalf("licence external id = (%q, %v), want (\"SA-7788\", true)", id, ok)
	}
}

// TestOnlineEntryLicenceJoinKeyParityWithNaturalKeyMatchDEC023TASK039 proves
// the online path's licence resolution mirrors the CSV import path's
// resolveImportAthlete (internal/app/import.go) — same join-key semantics,
// same enrichment behaviour (see TestCSVImportAddsLicenceToExistingNaturalKeyMatch
// for the CSV-side counterpart this test parallels): a first online entry
// with no licence creates an athlete; a second online entry for the same
// person (same natural key) that DOES supply a licence enriches that SAME
// athlete record via store.SetAthleteExternalID rather than creating a
// duplicate.
func TestOnlineEntryLicenceJoinKeyParityWithNaturalKeyMatchDEC023TASK039(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	otherEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "200m", CategoryCodes: []string{"U16 W"}})

	first, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Sina", LastName: "Roth", BirthYear: 2011,
		Sex: domain.SexFemale, SeedPerformance: "13.50",
	})
	if err != nil {
		t.Fatalf("first SubmitIndividualEntry: %v", err)
	}
	athlete, err := store.GetAthlete(ctx, f.st.DB(), first.AthleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if _, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); ok {
		t.Fatal("athlete unexpectedly already carries a licence number")
	}

	second, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: otherEventID, FirstName: "Sina", LastName: "Roth", BirthYear: 2011,
		Sex: domain.SexFemale, SeedPerformance: "27.50", LicenceNo: "SA-4242",
	})
	if err != nil {
		t.Fatalf("second SubmitIndividualEntry: %v", err)
	}
	if second.AthleteID != first.AthleteID {
		t.Fatalf("second entry resolved athlete %q, want the same athlete %q as the first (natural-key enrichment, no duplicate)", second.AthleteID, first.AthleteID)
	}
	athlete, err = store.GetAthlete(ctx, f.st.DB(), first.AthleteID)
	if err != nil {
		t.Fatalf("GetAthlete after second entry: %v", err)
	}
	if id, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-4242" {
		t.Fatalf("licence external id = (%q, %v), want (\"SA-4242\", true) backfilled onto the matched athlete", id, ok)
	}
}

// TestOnlineEntryLicenceJoinKeyParityWithCSVImportDEC023TASK039 is the
// stronger cross-path parity proof TASK-039 calls for: an athlete first
// created via the CSV import path carrying a licence number, then entered
// online by a submitter who supplies the SAME licence number (different
// submitted name/data possible in practice, but proven here with the exact
// same natural key too) — both paths resolve to the identical stored
// athlete, because both use the same domain.NamespaceSwissAthleticsLicence
// join key with identical (trim-only) normalization.
func TestOnlineEntryLicenceJoinKeyParityWithCSVImportDEC023TASK039(t *testing.T) {
	f := newImportFixture(t) // C-Meeting/Women/100m, office-role CSV import
	ctx := context.Background()

	csv := strings.Join([]string{
		systemNativeCSVHeader,
		systemNativeCSVRow("Elin", "Baumann", "1997", "W", "LC Import", "SA-5150", "100m/Women", "", "12.70"),
	}, "\n")
	report, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("CommitCSVImport: %v", err)
	}
	if report.Accepted != 1 {
		t.Fatalf("import report = %+v, want 1 accepted", report)
	}
	importedAthleteID := report.Rows[0].AthleteID

	otherEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "200m", CategoryCodes: []string{"Women"}})
	onlineDetail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: otherEventID, FirstName: "Elin", LastName: "Baumann", BirthYear: 1997,
		Sex: domain.SexFemale, SeedPerformance: "26.50", LicenceNo: "SA-5150",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if onlineDetail.AthleteID != importedAthleteID {
		t.Fatalf("online entry resolved athlete %q, want the CSV-imported athlete %q (same licence join key)", onlineDetail.AthleteID, importedAthleteID)
	}
	// Same stored state: the athlete record carries exactly one external id
	// for the namespace, unchanged by the online entry (already matched, so
	// resolveEntryAthlete's licence-first branch never re-writes it).
	athlete, err := store.GetAthlete(ctx, f.st.DB(), importedAthleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if id, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-5150" {
		t.Fatalf("licence external id = (%q, %v), want (\"SA-5150\", true)", id, ok)
	}
}

// TestSubmitIndividualEntryUnknownMeet covers the not-found path: a meetID
// that does not exist is refused rather than silently creating orphaned
// data.
func TestSubmitIndividualEntryUnknownMeet(t *testing.T) {
	f := newEntryFixture(t)
	if _, err := f.results.SubmitIndividualEntry(context.Background(), entrySubmitter, "no-such-meet", IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "13.50",
	}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SubmitIndividualEntry(unknown meet) = %v, want store.ErrNotFound", err)
	}
}

// TestSubmitIndividualEntryRejectsEventFromDifferentMeet covers UC-003 #3's
// request-forgery angle from a different direction than the deadline test:
// an eventID that is real but belongs to a different meet must not be
// usable to enter athletes into this meet.
func TestSubmitIndividualEntryRejectsEventFromDifferentMeet(t *testing.T) {
	f := newEntryFixture(t)
	other := newEntryFixture(t)
	if _, err := f.results.SubmitIndividualEntry(context.Background(), entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: other.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "13.50",
	}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SubmitIndividualEntry with another meet's event = %v, want store.ErrNotFound", err)
	}
}

// TestSubmitIndividualEntryRejectedWhenMeetNotPublished covers
// validateEntryEvent's meet-status gate: a draft meet (never published)
// refuses entries server-side even given a real event id.
func TestSubmitIndividualEntryRejectedWhenMeetNotPublished(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	draft, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, draft.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if _, err := results.SubmitIndividualEntry(ctx, entrySubmitter, draft.ID, IndividualEntryInput{
		EventID: ev.ID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "13.50",
	}); !errors.Is(err, ErrEntriesClosed) {
		t.Errorf("SubmitIndividualEntry on a draft meet = %v, want ErrEntriesClosed", err)
	}
}

// TestSubmitIndividualEntryRejectedWhenEventClosed covers the event-status
// (as opposed to meet-status or deadline) gate: an event explicitly closed
// by the office is excluded from OpenEntryEvents and refuses a direct
// SubmitIndividualEntry call naming it.
func TestSubmitIndividualEntryRejectedWhenEventClosed(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	closedEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "200m", CategoryCodes: []string{"U16 W"}})
	if _, err := f.st.DB().ExecContext(ctx, `UPDATE events SET status = 'closed' WHERE id = ?`, closedEventID); err != nil {
		t.Fatalf("close event: %v", err)
	}

	opts, err := f.results.OpenEntryEvents(ctx, f.meetID)
	if err != nil {
		t.Fatalf("OpenEntryEvents: %v", err)
	}
	for _, o := range opts {
		if o.EventID == closedEventID {
			t.Fatal("OpenEntryEvents must not offer a closed event")
		}
	}
	if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: closedEventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "27.00",
	}); !errors.Is(err, ErrEntriesClosed) {
		t.Errorf("SubmitIndividualEntry on a closed event = %v, want ErrEntriesClosed", err)
	}
}

// TestOnlineEntryClubBulkSYS011UC003_2 covers UC-003 #2: a club submitter
// enters 15 athletes across 6 events in one bulk operation, and all 15
// entries exist afterwards.
func TestOnlineEntryClubBulkSYS011UC003_2(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	codes := []string{"100m", "200m", "300m", "400m", "800m", "1500m"}
	var eventIDs []string
	for _, code := range codes[1:] { // f.eventID already covers "100m"
		eventIDs = append(eventIDs, f.addEvent(t, AddEventRequest{DisciplineCode: code, CategoryCodes: []string{"U16 W"}}))
	}
	eventIDs = append([]string{f.eventID}, eventIDs...)

	in := BulkEntryInput{Club: "LC Bulk"}
	for i := 0; i < 15; i++ {
		in.Lines = append(in.Lines, BulkEntryLine{
			FirstName: "Athlete", LastName: string(rune('A' + i)), BirthYear: 2010 + (i % 5),
			Sex: domain.SexFemale, EventID: eventIDs[i%len(eventIDs)], SeedPerformance: "60.00",
		})
	}
	details, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, in)
	if err != nil {
		t.Fatalf("SubmitClubBulkEntries: %v", err)
	}
	if len(details) != 15 {
		t.Fatalf("created %d entries, want 15", len(details))
	}

	mine, err := f.results.MyEntries(ctx, entrySubmitter, f.meetID)
	if err != nil {
		t.Fatalf("MyEntries: %v", err)
	}
	if len(mine) != 15 {
		t.Fatalf("per-submitter entry list has %d entries, want 15", len(mine))
	}
	for _, e := range mine {
		if e.ClubName != "LC Bulk" {
			t.Errorf("entry club = %q, want LC Bulk", e.ClubName)
		}
	}
}

// TestOnlineEntryBulkAtomicSYS011: one invalid line in a bulk submission
// aborts the whole operation — "all N entries exist" never holds partially.
func TestOnlineEntryBulkAtomicSYS011(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	in := BulkEntryInput{Club: "LC Bulk", Lines: []BulkEntryLine{
		{FirstName: "A", LastName: "One", BirthYear: 2011, Sex: domain.SexFemale, EventID: f.eventID, SeedPerformance: "13.50"},
		{FirstName: "B", LastName: "Two", BirthYear: 2011, Sex: domain.SexFemale, EventID: "no-such-event", SeedPerformance: "13.50"},
	}}
	if _, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, in); err == nil {
		t.Fatal("expected an error for an invalid line")
	}
	mine, err := f.results.MyEntries(ctx, entrySubmitter, f.meetID)
	if err != nil {
		t.Fatalf("MyEntries: %v", err)
	}
	if len(mine) != 0 {
		t.Fatalf("MyEntries = %+v, want none (atomic rollback)", mine)
	}
}

// TestOnlineEntryDeadlinePassedRejectedSYS011UC003_3 covers UC-003 #3: an
// online entry attempted after the event's configured deadline is rejected
// server-side — including a "direct request forgery" that names an event ID
// the submission form would never have offered.
func TestOnlineEntryDeadlinePassedRejectedSYS011UC003_3(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	closedEventID := f.addEvent(t, AddEventRequest{
		DisciplineCode: "200m", CategoryCodes: []string{"U16 W"}, EntryDeadline: &past,
	})

	// The submission form itself would never offer this event.
	opts, err := f.results.OpenEntryEvents(ctx, f.meetID)
	if err != nil {
		t.Fatalf("OpenEntryEvents: %v", err)
	}
	for _, o := range opts {
		if o.EventID == closedEventID {
			t.Fatal("OpenEntryEvents must not offer an event past its deadline")
		}
	}

	// A direct POST naming that event ID anyway is rejected server-side.
	_, err = f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: closedEventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011,
		Sex: domain.SexFemale, SeedPerformance: "27.00",
	})
	if !errors.Is(err, ErrEntryDeadlinePassed) {
		t.Errorf("SubmitIndividualEntry past deadline = %v, want ErrEntryDeadlinePassed", err)
	}
}

// TestSubmitClubBulkEntriesDenialPaths covers SubmitClubBulkEntries' input
// guards: an under-privileged actor, a missing club, an empty line list and
// an unknown meet are all refused before any entry is written.
func TestSubmitClubBulkEntriesDenialPaths(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	validLine := BulkEntryLine{FirstName: "A", LastName: "One", BirthYear: 2011, Sex: domain.SexFemale, EventID: f.eventID, SeedPerformance: "13.50"}

	t.Run("requires role", func(t *testing.T) {
		public := Session{Role: RolePublic}
		var forbidden ErrForbidden
		if _, err := f.results.SubmitClubBulkEntries(ctx, public, f.meetID, BulkEntryInput{Club: "LC Bulk", Lines: []BulkEntryLine{validLine}}); !errors.As(err, &forbidden) {
			t.Errorf("SubmitClubBulkEntries by public = %v, want ErrForbidden", err)
		}
	})
	t.Run("requires a club", func(t *testing.T) {
		if _, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, BulkEntryInput{Lines: []BulkEntryLine{validLine}}); err == nil {
			t.Error("expected an error for a missing club")
		}
	})
	t.Run("requires at least one line", func(t *testing.T) {
		if _, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, f.meetID, BulkEntryInput{Club: "LC Bulk"}); err == nil {
			t.Error("expected an error for an empty line list")
		}
	})
	t.Run("unknown meet", func(t *testing.T) {
		if _, err := f.results.SubmitClubBulkEntries(ctx, entrySubmitter, "no-such-meet", BulkEntryInput{Club: "LC Bulk", Lines: []BulkEntryLine{validLine}}); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("SubmitClubBulkEntries(unknown meet) = %v, want store.ErrNotFound", err)
		}
	})
}

// TestOnlineEntryRelaySYS012UC003_4 covers UC-003 #4: a club enters a team
// of 4 named athletes in order plus 2 reserves; the ordered composition is
// stored and can be revised before the deadline, but rejected after it.
func TestOnlineEntryRelaySYS012UC003_4(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	relayEventID := f.addEvent(t, AddEventRequest{
		DisciplineCode: "4x100m", CategoryCodes: []string{"U16 W"}, EntryDeadline: &future,
	})

	leg := func(first string) RelayLegInput {
		return RelayLegInput{FirstName: first, LastName: "Runner", BirthYear: 2011, Sex: domain.SexFemale}
	}
	detail, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
		EventID: relayEventID, Club: "LC Relay",
		Composition: []RelayLegInput{leg("Leg1"), leg("Leg2"), leg("Leg3"), leg("Leg4")},
		Reserves:    []RelayLegInput{leg("Res1"), leg("Res2")},
	})
	if err != nil {
		t.Fatalf("SubmitRelayEntry: %v", err)
	}
	if detail.RelayTeam == nil {
		t.Fatal("expected a relay team detail")
	}
	wantOrder := []string{"Leg1 Runner", "Leg2 Runner", "Leg3 Runner", "Leg4 Runner"}
	for i, name := range wantOrder {
		if detail.RelayTeam.Composition[i] != name {
			t.Fatalf("composition = %v, want ordered %v", detail.RelayTeam.Composition, wantOrder)
		}
	}
	if len(detail.RelayTeam.Reserves) != 2 {
		t.Fatalf("reserves = %v, want 2", detail.RelayTeam.Reserves)
	}

	// Revising the composition before the deadline succeeds.
	revised, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, f.meetID, detail.ID, detail.RelayTeam.Version,
		[]RelayLegInput{leg("NewLeg1"), leg("Leg2"), leg("Leg3"), leg("Leg4")}, nil)
	if err != nil {
		t.Fatalf("UpdateRelayComposition: %v", err)
	}
	if revised.RelayTeam.Composition[0] != "NewLeg1 Runner" {
		t.Errorf("revised composition = %v", revised.RelayTeam.Composition)
	}

	// After the deadline, the same edit is rejected.
	expiringEventID := f.addEvent(t, AddEventRequest{
		DisciplineCode: "4x200m", CategoryCodes: []string{"U16 W"}, EntryDeadline: timePtr(time.Now().Add(time.Hour)),
	})
	closedDetail, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
		EventID: expiringEventID, Club: "LC Relay",
		Composition: []RelayLegInput{leg("A"), leg("B"), leg("C"), leg("D")},
	})
	if err != nil {
		t.Fatalf("SubmitRelayEntry (second team): %v", err)
	}
	// Now the deadline passes.
	expired := time.Now().Add(-time.Hour)
	if _, err := f.st.DB().ExecContext(ctx, `UPDATE events SET entry_deadline = ? WHERE id = ?`,
		expired.UTC().Format(time.RFC3339Nano), expiringEventID); err != nil {
		t.Fatalf("expire deadline: %v", err)
	}
	if _, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, f.meetID, closedDetail.ID, closedDetail.RelayTeam.Version,
		[]RelayLegInput{leg("X"), leg("Y"), leg("Z"), leg("W")}, nil); !errors.Is(err, ErrEntryDeadlinePassed) {
		t.Errorf("post-deadline composition edit = %v, want ErrEntryDeadlinePassed", err)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// TestSubmitRelayEntryDenialPaths covers SubmitRelayEntry's input guards: an
// under-privileged actor, a missing club, an empty composition and an
// unknown meet are all refused.
func TestSubmitRelayEntryDenialPaths(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	relayEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "4x100m", CategoryCodes: []string{"U16 W"}})
	leg := RelayLegInput{FirstName: "L", LastName: "Runner", BirthYear: 2011, Sex: domain.SexFemale}

	t.Run("requires role", func(t *testing.T) {
		public := Session{Role: RolePublic}
		var forbidden ErrForbidden
		if _, err := f.results.SubmitRelayEntry(ctx, public, f.meetID, RelayEntryInput{EventID: relayEventID, Club: "LC Relay", Composition: []RelayLegInput{leg}}); !errors.As(err, &forbidden) {
			t.Errorf("SubmitRelayEntry by public = %v, want ErrForbidden", err)
		}
	})
	t.Run("requires a club", func(t *testing.T) {
		if _, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{EventID: relayEventID, Composition: []RelayLegInput{leg}}); err == nil {
			t.Error("expected an error for a missing club")
		}
	})
	t.Run("requires a non-empty composition", func(t *testing.T) {
		if _, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{EventID: relayEventID, Club: "LC Relay"}); err == nil {
			t.Error("expected an error for an empty composition")
		}
	})
	t.Run("unknown meet", func(t *testing.T) {
		if _, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, "no-such-meet", RelayEntryInput{EventID: relayEventID, Club: "LC Relay", Composition: []RelayLegInput{leg}}); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("SubmitRelayEntry(unknown meet) = %v, want store.ErrNotFound", err)
		}
	})
}

// TestUpdateRelayCompositionDenialPaths covers UpdateRelayComposition's
// guards beyond the deadline check already covered by
// TestOnlineEntryRelaySYS012UC003_4: an under-privileged actor, an empty
// composition, a non-relay entry, an unknown entry id, an entry belonging to
// a different meet, and a stale optimistic-concurrency version.
func TestUpdateRelayCompositionDenialPaths(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	relayEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "4x100m", CategoryCodes: []string{"U16 W"}})
	leg := func(first string) RelayLegInput {
		return RelayLegInput{FirstName: first, LastName: "Runner", BirthYear: 2011, Sex: domain.SexFemale}
	}
	detail, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
		EventID: relayEventID, Club: "LC Relay",
		Composition: []RelayLegInput{leg("A"), leg("B"), leg("C"), leg("D")},
	})
	if err != nil {
		t.Fatalf("SubmitRelayEntry: %v", err)
	}

	t.Run("requires role", func(t *testing.T) {
		public := Session{Role: RolePublic}
		var forbidden ErrForbidden
		if _, err := f.results.UpdateRelayComposition(ctx, public, f.meetID, detail.ID, detail.RelayTeam.Version,
			[]RelayLegInput{leg("X")}, nil); !errors.As(err, &forbidden) {
			t.Errorf("UpdateRelayComposition by public = %v, want ErrForbidden", err)
		}
	})
	t.Run("requires a non-empty composition", func(t *testing.T) {
		if _, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, f.meetID, detail.ID, detail.RelayTeam.Version,
			nil, nil); err == nil {
			t.Error("expected an error for an empty composition")
		}
	})
	t.Run("unknown entry id", func(t *testing.T) {
		if _, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, f.meetID, "no-such-entry", 1,
			[]RelayLegInput{leg("X")}, nil); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("UpdateRelayComposition(unknown entry) = %v, want store.ErrNotFound", err)
		}
	})
	t.Run("non-relay entry", func(t *testing.T) {
		individual, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
			EventID: f.eventID, FirstName: "Solo", LastName: "Runner", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "13.50",
		})
		if err != nil {
			t.Fatalf("SubmitIndividualEntry: %v", err)
		}
		if _, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, f.meetID, individual.ID, individual.Version,
			[]RelayLegInput{leg("X")}, nil); !errors.Is(err, ErrNotRelayEntry) {
			t.Errorf("UpdateRelayComposition on an individual entry = %v, want ErrNotRelayEntry", err)
		}
	})
	t.Run("entry belongs to a different meet", func(t *testing.T) {
		other := newEntryFixture(t)
		if _, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, other.meetID, detail.ID, detail.RelayTeam.Version,
			[]RelayLegInput{leg("X")}, nil); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("UpdateRelayComposition against the wrong meet = %v, want store.ErrNotFound", err)
		}
	})
	t.Run("stale version is an optimistic-concurrency conflict", func(t *testing.T) {
		if _, err := f.results.UpdateRelayComposition(ctx, entrySubmitter, f.meetID, detail.ID, detail.RelayTeam.Version+999,
			[]RelayLegInput{leg("X"), leg("Y"), leg("Z"), leg("W")}, nil); !errors.Is(err, store.ErrVersionConflict) {
			t.Errorf("UpdateRelayComposition with a stale version = %v, want store.ErrVersionConflict", err)
		}
	})
}

// TestOnlineEntryFailsStandardSYS015UC003_5 is the UC-003 #5 fixture: an
// event with entry standard "12.20" (100 m) marks a "12.85" seed as failing
// the standard, and it appears on the organizer's exception report.
func TestOnlineEntryFailsStandardSYS015UC003_5(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	standardEventID := f.addEvent(t, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"}, EntryStandard: "12.20",
	})

	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: standardEventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011,
		Sex: domain.SexFemale, SeedPerformance: "12.85",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if !detail.FailsStandard {
		t.Error("expected FailsStandard = true for a 12.85 seed against a 12.20 standard")
	}
	if detail.Status != domain.EntryEntered {
		t.Errorf("a failing-standard entry is still stored (not rejected): status = %q", detail.Status)
	}

	exceptions, err := f.results.EntryExceptions(ctx, office, f.meetID)
	if err != nil {
		t.Fatalf("EntryExceptions: %v", err)
	}
	if len(exceptions) != 1 || exceptions[0].ID != detail.ID {
		t.Fatalf("EntryExceptions = %+v, want exactly the failing entry", exceptions)
	}
}

// TestOnlineEntrySeedRequiredSYS015 covers SYS-015's "required
// seed-performance information": an entry without a seed is rejected.
func TestOnlineEntrySeedRequiredSYS015(t *testing.T) {
	f := newEntryFixture(t)
	_, err := f.results.SubmitIndividualEntry(context.Background(), entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011, Sex: domain.SexFemale,
	})
	if !errors.Is(err, ErrSeedPerformanceRequired) {
		t.Errorf("missing seed = %v, want ErrSeedPerformanceRequired", err)
	}
}

// TestOnlineEntryLimitReachedSYS015 covers SYS-015's per-event entry limit.
func TestOnlineEntryLimitReachedSYS015(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	limitedEventID := f.addEvent(t, AddEventRequest{
		DisciplineCode: "200m", CategoryCodes: []string{"U16 W"}, EntryLimit: 1,
	})
	first := IndividualEntryInput{EventID: limitedEventID, FirstName: "A", LastName: "One", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "27.00"}
	if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, first); err != nil {
		t.Fatalf("first SubmitIndividualEntry: %v", err)
	}
	second := IndividualEntryInput{EventID: limitedEventID, FirstName: "B", LastName: "Two", BirthYear: 2011, Sex: domain.SexFemale, SeedPerformance: "27.50"}
	if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, second); !errors.Is(err, ErrEntryLimitReached) {
		t.Errorf("over-limit entry = %v, want ErrEntryLimitReached", err)
	}
}

// TestSubmitEntryRequiresRoleSYS011: an unauthenticated/under-privileged
// actor cannot submit online entries (SYS-090 least privilege).
func TestSubmitEntryRequiresRoleSYS011(t *testing.T) {
	f := newEntryFixture(t)
	public := Session{Role: RolePublic}
	var forbidden ErrForbidden
	_, err := f.results.SubmitIndividualEntry(context.Background(), public, f.meetID, IndividualEntryInput{EventID: f.eventID})
	if !errors.As(err, &forbidden) {
		t.Errorf("public SubmitIndividualEntry = %v, want ErrForbidden", err)
	}
}

// TestMyEntriesUnknownMeet and TestEntryExceptionsUnknownMeet cover the
// not-found lookup path both read surfaces share; TestEntryExceptionsRequiresOfficeCapability
// covers the SYS-090 least-privilege gate that distinguishes EntryExceptions
// (office-only) from MyEntries (any submitter, over their own entries).
func TestMyEntriesUnknownMeet(t *testing.T) {
	f := newEntryFixture(t)
	if _, err := f.results.MyEntries(context.Background(), entrySubmitter, "no-such-meet"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("MyEntries(unknown meet) = %v, want store.ErrNotFound", err)
	}
}

func TestEntryExceptionsUnknownMeet(t *testing.T) {
	f := newEntryFixture(t)
	if _, err := f.results.EntryExceptions(context.Background(), office, "no-such-meet"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("EntryExceptions(unknown meet) = %v, want store.ErrNotFound", err)
	}
}

// TestMyEntriesIncludesRelayEntry covers enrichEntries' relay branch, which
// the individual-only entry fixtures never reach: a submitted relay entry
// shows up in MyEntries with its team composition/reserve names, club and
// relay fee resolved.
func TestMyEntriesIncludesRelayEntry(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	relayEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "4x100m", CategoryCodes: []string{"U16 W"}})
	leg := func(first string) RelayLegInput {
		return RelayLegInput{FirstName: first, LastName: "Runner", BirthYear: 2011, Sex: domain.SexFemale}
	}
	submitted, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
		EventID: relayEventID, Club: "LC Relay",
		Composition: []RelayLegInput{leg("A"), leg("B"), leg("C"), leg("D")},
		Reserves:    []RelayLegInput{leg("E")},
	})
	if err != nil {
		t.Fatalf("SubmitRelayEntry: %v", err)
	}

	mine, err := f.results.MyEntries(ctx, entrySubmitter, f.meetID)
	if err != nil {
		t.Fatalf("MyEntries: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != submitted.ID {
		t.Fatalf("MyEntries = %+v, want exactly the submitted relay entry", mine)
	}
	got := mine[0]
	if got.ClubName != "LC Relay" {
		t.Errorf("ClubName = %q, want LC Relay", got.ClubName)
	}
	if got.RelayTeam == nil || len(got.RelayTeam.Composition) != 4 || len(got.RelayTeam.Reserves) != 1 {
		t.Fatalf("RelayTeam = %+v, want 4 composition + 1 reserve names resolved", got.RelayTeam)
	}
	if got.RelayTeam.Composition[0] != "A Runner" {
		t.Errorf("Composition[0] = %q, want %q", got.RelayTeam.Composition[0], "A Runner")
	}

	// Same relay-branch enrichment through the office-only EntryExceptions
	// surface (only reached if the relay entry also fails standard — relay
	// entries never carry FailsStandard, so it must be empty here, exercising
	// the "no failing entries" early-return alongside individual coverage).
	exceptions, err := f.results.EntryExceptions(ctx, office, f.meetID)
	if err != nil {
		t.Fatalf("EntryExceptions: %v", err)
	}
	if len(exceptions) != 0 {
		t.Errorf("EntryExceptions = %+v, want none (relay entries never fail a seed standard)", exceptions)
	}
}

func TestEntryExceptionsRequiresOfficeCapability(t *testing.T) {
	f := newEntryFixture(t)
	var forbidden ErrForbidden
	if _, err := f.results.EntryExceptions(context.Background(), entrySubmitter, f.meetID); !errors.As(err, &forbidden) {
		t.Errorf("EntryExceptions by an entry-submitter = %v, want ErrForbidden", err)
	}
}

// TestBibAssignmentSYS018UC006_1 covers UC-006 #1: bibs assigned from a
// starting number per club give every athlete exactly one unique bib.
func TestBibAssignmentSYS018UC006_1(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	club, err := store.CreateClub(ctx, f.st.DB(), domain.Club{Name: "LC Bib"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	for i := 0; i < 3; i++ {
		a, err := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{
			FirstName: "Athlete", LastName: string(rune('A' + i)), BirthYear: 2011, Sex: domain.SexFemale, ClubIDs: []string{club.ID},
		})
		if err != nil {
			t.Fatalf("CreateAthlete: %v", err)
		}
		if _, err := store.EnsureParticipant(ctx, f.st.DB(), f.meetID, a.ID); err != nil {
			t.Fatalf("EnsureParticipant: %v", err)
		}
	}

	n, err := f.results.BulkAssignBibsByClub(ctx, organizer, f.meetID, club.ID, 100)
	if err != nil {
		t.Fatalf("BulkAssignBibsByClub: %v", err)
	}
	if n != 3 {
		t.Fatalf("assigned %d bibs, want 3", n)
	}
	participants, err := f.results.Participants(ctx, f.meetID)
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range participants {
		if p.Bib == "" {
			t.Errorf("participant %s has no bib", p.ID)
		}
		if seen[p.Bib] {
			t.Errorf("duplicate bib %q", p.Bib)
		}
		seen[p.Bib] = true
	}
	if !seen["100"] || !seen["101"] || !seen["102"] {
		t.Errorf("bibs = %v, want 100/101/102", seen)
	}
}

// TestBibAssignmentDuplicateRejectedSYS018UC006_2 covers UC-006 #2: a manual
// duplicate bib assignment is rejected.
func TestBibAssignmentDuplicateRejectedSYS018UC006_2(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	a1, _ := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{FirstName: "A", LastName: "One", BirthYear: 2011, Sex: domain.SexFemale})
	a2, _ := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{FirstName: "B", LastName: "Two", BirthYear: 2011, Sex: domain.SexFemale})
	p1, err := store.EnsureParticipant(ctx, f.st.DB(), f.meetID, a1.ID)
	if err != nil {
		t.Fatalf("EnsureParticipant: %v", err)
	}
	p2, err := store.EnsureParticipant(ctx, f.st.DB(), f.meetID, a2.ID)
	if err != nil {
		t.Fatalf("EnsureParticipant: %v", err)
	}
	if err := f.results.AssignBib(ctx, organizer, f.meetID, p1.ID, p1.Version, "42"); err != nil {
		t.Fatalf("AssignBib: %v", err)
	}
	if err := f.results.AssignBib(ctx, organizer, f.meetID, p2.ID, p2.Version, "42"); !errors.Is(err, ErrDuplicateParticipant) {
		t.Errorf("duplicate bib = %v, want ErrDuplicateParticipant", err)
	}
	if err := f.results.AssignBib(ctx, organizer, f.meetID, p2.ID, p2.Version, ""); !errors.Is(err, ErrBibRequired) {
		t.Errorf("empty bib = %v, want ErrBibRequired", err)
	}
}

// TestAssignBibUnknownParticipant covers the not-found lookup path: a
// participantID that does not exist is refused rather than silently
// creating a dangling bib assignment.
func TestAssignBibUnknownParticipant(t *testing.T) {
	f := newEntryFixture(t)
	if err := f.results.AssignBib(context.Background(), organizer, f.meetID, "no-such-participant", 1, "42"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("AssignBib(unknown participant) = %v, want store.ErrNotFound", err)
	}
}

// TestBulkAssignBibsByClubDenialAndEdgePaths covers checkEntryLimit-adjacent
// input guards on the bulk bib flow: a non-positive starting number is
// rejected, and a club with no unbibbed participants assigns zero bibs
// (not an error).
func TestBulkAssignBibsByClubDenialAndEdgePaths(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()

	t.Run("requires a positive starting number", func(t *testing.T) {
		if _, err := f.results.BulkAssignBibsByClub(ctx, organizer, f.meetID, "any-club", 0); err == nil {
			t.Error("expected an error for a non-positive starting bib number")
		}
	})
	t.Run("no unbibbed participants in the club: zero assigned, no error", func(t *testing.T) {
		club, err := store.CreateClub(ctx, f.st.DB(), domain.Club{Name: "LC Empty"})
		if err != nil {
			t.Fatalf("CreateClub: %v", err)
		}
		n, err := f.results.BulkAssignBibsByClub(ctx, organizer, f.meetID, club.ID, 100)
		if err != nil {
			t.Fatalf("BulkAssignBibsByClub: %v", err)
		}
		if n != 0 {
			t.Errorf("assigned = %d, want 0 for a club with no participants", n)
		}
	})
}

// TestFeeSummarySYS017UC006_3 covers UC-006 #3: per-club totals equal the
// entries × schedule arithmetic, verified against a fixture.
func TestFeeSummarySYS017UC006_3(t *testing.T) {
	f := newEntryFixture(t)
	ctx := context.Background()
	relayEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "4x100m", CategoryCodes: []string{"U16 W"}})

	current, err := f.meets.Meet(ctx, f.meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	if err := f.meets.SetFeeSchedule(ctx, organizer, f.meetID, current.Version, 1000, 3000); err != nil {
		t.Fatalf("SetFeeSchedule: %v", err)
	}

	// LC A: 2 individual entries.
	for i := 0; i < 2; i++ {
		if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
			EventID: f.eventID, FirstName: "A", LastName: string(rune('A' + i)), BirthYear: 2011,
			Sex: domain.SexFemale, Club: "LC A", SeedPerformance: "13.50",
		}); err != nil {
			t.Fatalf("SubmitIndividualEntry: %v", err)
		}
	}
	// LC B: 1 individual + 1 relay entry.
	if _, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "B", LastName: "One", BirthYear: 2011,
		Sex: domain.SexFemale, Club: "LC B", SeedPerformance: "13.60",
	}); err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	leg := func(first string) RelayLegInput {
		return RelayLegInput{FirstName: first, LastName: "Runner", BirthYear: 2011, Sex: domain.SexFemale}
	}
	if _, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
		EventID: relayEventID, Club: "LC B",
		Composition: []RelayLegInput{leg("L1"), leg("L2"), leg("L3"), leg("L4")},
	}); err != nil {
		t.Fatalf("SubmitRelayEntry: %v", err)
	}

	sum, err := f.results.FeeSummary(ctx, organizer, f.meetID)
	if err != nil {
		t.Fatalf("FeeSummary: %v", err)
	}
	if sum.EntryFeeCents != 1000 || sum.RelayFeeCents != 3000 {
		t.Fatalf("schedule = (%d, %d), want (1000, 3000)", sum.EntryFeeCents, sum.RelayFeeCents)
	}
	byClub := map[string]ClubFeeSummary{}
	for _, c := range sum.Clubs {
		byClub[c.ClubName] = c
	}
	a := byClub["LC A"]
	if a.IndividualEntries != 2 || a.RelayEntries != 0 || a.TotalCents != 2*1000 {
		t.Errorf("LC A summary = %+v, want 2 individual, 0 relay, 2000 total", a)
	}
	b := byClub["LC B"]
	if b.IndividualEntries != 1 || b.RelayEntries != 1 || b.TotalCents != 1000+3000 {
		t.Errorf("LC B summary = %+v, want 1 individual, 1 relay, 4000 total", b)
	}
	wantTotal := a.TotalCents + b.TotalCents
	if sum.TotalCents != wantTotal {
		t.Errorf("grand total = %d, want %d", sum.TotalCents, wantTotal)
	}
}

// TestFeeSummaryRequiresOrganizerRoleSYS017 / TestBibAssignRequiresOrganizerRoleSYS018
// cover the SYS-090 least-privilege gate on the organizer-only surfaces.
func TestFeeSummaryRequiresOrganizerRoleSYS017(t *testing.T) {
	f := newEntryFixture(t)
	var forbidden ErrForbidden
	if _, err := f.results.FeeSummary(context.Background(), entrySubmitter, f.meetID); !errors.As(err, &forbidden) {
		t.Errorf("entry-submitter FeeSummary = %v, want ErrForbidden", err)
	}
}

func TestBibAssignRequiresOrganizerRoleSYS018(t *testing.T) {
	f := newEntryFixture(t)
	var forbidden ErrForbidden
	if _, err := f.results.BulkAssignBibsByClub(context.Background(), entrySubmitter, f.meetID, "any", 1); !errors.As(err, &forbidden) {
		t.Errorf("entry-submitter BulkAssignBibsByClub = %v, want ErrForbidden", err)
	}
}
