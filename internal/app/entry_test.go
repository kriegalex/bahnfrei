// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
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
