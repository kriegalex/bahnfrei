// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// checkinFixture is a published meet with one 100m event and 8 entered
// (not yet confirmed) athletes, for check-in tests (UC-007).
type checkinFixture struct {
	results *ResultsService
	meetID  string
	eventID string
	entries []store.EntryRecord
}

func newCheckinFixture(t *testing.T, n int) checkinFixture {
	t.Helper()
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	meet, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, meet.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	var entries []store.EntryRecord
	for i := 0; i < n; i++ {
		a, err := store.CreateAthlete(ctx, st.DB(), domain.Athlete{
			FirstName: "Athlete", LastName: string(rune('A' + i)), BirthYear: 2010, Sex: domain.SexFemale,
		})
		if err != nil {
			t.Fatalf("CreateAthlete: %v", err)
		}
		rec, err := store.CreateEntry(ctx, st.DB(), domain.Entry{
			EventID: ev.ID, AthleteID: a.ID, SeedPerformance: "13.50",
		})
		if err != nil {
			t.Fatalf("CreateEntry: %v", err)
		}
		entries = append(entries, rec)
	}
	return checkinFixture{results: results, meetID: meet.ID, eventID: ev.ID, entries: entries}
}

// TestCheckInCloseMarksUnconfirmedDNSSYS025UC007_1 reproduces UC-007 #1: 8
// entered athletes, check-in open; 7 confirm and the deadline passes; the
// operator's one action (CloseCheckIn) marks the unconfirmed athlete DNS,
// and the roster shows 7 confirmed + 1 DNS.
func TestCheckInCloseMarksUnconfirmedDNSSYS025UC007_1(t *testing.T) {
	f := newCheckinFixture(t, 8)
	ctx := context.Background()

	for i := 0; i < 7; i++ {
		if err := f.results.ConfirmCheckIn(ctx, office, f.meetID, f.entries[i].ID, f.entries[i].Version); err != nil {
			t.Fatalf("ConfirmCheckIn entry %d: %v", i, err)
		}
	}
	n, err := f.results.CloseCheckIn(ctx, office, f.meetID, f.eventID)
	if err != nil {
		t.Fatalf("CloseCheckIn: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly 1 entry marked DNS, got %d", n)
	}

	roster, err := f.results.CheckInRoster(ctx, office, f.meetID, f.eventID)
	if err != nil {
		t.Fatalf("CheckInRoster: %v", err)
	}
	var confirmed, dns int
	for _, row := range roster {
		switch row.Status {
		case domain.EntryConfirmed:
			confirmed++
		case domain.EntryDNS:
			dns++
		}
	}
	if confirmed != 7 {
		t.Errorf("expected 7 confirmed starters, got %d", confirmed)
	}
	if dns != 1 {
		t.Errorf("expected 1 DNS, got %d", dns)
	}
}

// TestReinstateEntrySYS025UC007_2 reproduces UC-007 #2: a DNS-marked
// athlete reinstated by the referee before the start returns to the start
// list, and the change is audited.
func TestReinstateEntrySYS025UC007_2(t *testing.T) {
	f := newCheckinFixture(t, 2)
	ctx := context.Background()

	if _, err := f.results.CloseCheckIn(ctx, office, f.meetID, f.eventID); err != nil {
		t.Fatalf("CloseCheckIn: %v", err)
	}
	roster, err := f.results.CheckInRoster(ctx, office, f.meetID, f.eventID)
	if err != nil {
		t.Fatalf("CheckInRoster: %v", err)
	}
	var dnsEntry EntryDetail
	for _, row := range roster {
		if row.Status == domain.EntryDNS {
			dnsEntry = row
			break
		}
	}
	if dnsEntry.ID == "" {
		t.Fatal("expected at least one DNS entry to reinstate")
	}

	if err := f.results.ReinstateEntry(ctx, office, f.meetID, dnsEntry.ID, dnsEntry.Version); err != nil {
		t.Fatalf("ReinstateEntry: %v", err)
	}
	got, err := store.GetEntry(ctx, mustStoreDB(t, f), dnsEntry.ID)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if got.Status != domain.EntryConfirmed {
		t.Errorf("expected reinstated entry to be confirmed, got %q", got.Status)
	}

	logged, err := store.ListAudit(ctx, mustStoreDB(t, f), 50)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	found := false
	for _, e := range logged {
		if e.Action == "checkin.reinstate" && e.EntityID == dnsEntry.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected a checkin.reinstate audit entry")
	}
}

// TestConfirmCheckInRejectsAlreadyConfirmedSYS025 is a denial/edge-path
// test: only an "entered" entry can be confirmed.
func TestConfirmCheckInRejectsAlreadyConfirmedSYS025(t *testing.T) {
	f := newCheckinFixture(t, 1)
	ctx := context.Background()
	e := f.entries[0]
	if err := f.results.ConfirmCheckIn(ctx, office, f.meetID, e.ID, e.Version); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	if err := f.results.ConfirmCheckIn(ctx, office, f.meetID, e.ID, e.Version+1); !errors.Is(err, ErrEntryNotConfirmable) {
		t.Errorf("expected ErrEntryNotConfirmable on a second confirm, got %v", err)
	}
}

// TestReinstateEntryRejectsNonDNSSYS025 is a denial/edge-path test.
func TestReinstateEntryRejectsNonDNSSYS025(t *testing.T) {
	f := newCheckinFixture(t, 1)
	ctx := context.Background()
	e := f.entries[0]
	if err := f.results.ReinstateEntry(ctx, office, f.meetID, e.ID, e.Version); !errors.Is(err, ErrEntryNotDNS) {
		t.Errorf("expected ErrEntryNotDNS for a never-DNS entry, got %v", err)
	}
}

// TestCheckInRequiresOfficeCapabilitySYS025 is a denial/edge-path test:
// an entry-submitter (below competition-office) cannot run check-in
// actions (SYS-090 least privilege).
func TestCheckInRequiresOfficeCapabilitySYS025(t *testing.T) {
	f := newCheckinFixture(t, 1)
	ctx := context.Background()
	e := f.entries[0]
	unauthorized := Session{AccountID: "01SUB", Role: RoleEntrySubmitter}
	if err := f.results.ConfirmCheckIn(ctx, unauthorized, f.meetID, e.ID, e.Version); err == nil {
		t.Error("expected an authorization error for an entry-submitter running check-in")
	}
}

// mustStoreDB is a small test helper exposing the fixture's *sql.DB via the
// results service (checkinFixture keeps only the service, not the raw
// store, mirroring how the app layer itself never leaks *sql.DB upward).
func mustStoreDB(t *testing.T, f checkinFixture) store.DBTX {
	t.Helper()
	return f.results.db
}
