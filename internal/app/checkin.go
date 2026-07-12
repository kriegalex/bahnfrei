// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Check-in / call-room and DNS handling (TASK-018, UC-007, SYS-025):
// confirm or DNS entries per event before seeding. Reuses entry.go's
// EntryDetail/enrichEntries (TASK-016 precedent) so the check-in roster
// carries the same athlete/club display data the entries pages already
// show. ---

// ErrEntryWrongMeet means the target entry does not belong to the meet the
// request scoped it to.
var ErrEntryWrongMeet = errors.New("entry does not belong to this meet")

// ErrEntryNotConfirmable means the entry's current status cannot transition
// to confirmed directly (only "entered" confirms; a scratched entry never
// checks in).
var ErrEntryNotConfirmable = errors.New("entry is not in a confirmable state")

// ErrEntryNotDNS means a reinstatement was attempted on an entry that is not
// currently marked DNS.
var ErrEntryNotDNS = errors.New("entry is not marked DNS")

// eventOfMeet resolves eventID and checks it belongs to meetID, the common
// guard every check-in/seeding/progression entry point needs.
func eventOfMeet(ctx context.Context, db store.DBTX, meetID, eventID string) (store.EventRecord, error) {
	event, err := store.GetEvent(ctx, db, eventID)
	if err != nil {
		return store.EventRecord{}, err
	}
	if event.MeetID != meetID {
		return store.EventRecord{}, store.ErrNotFound
	}
	return event, nil
}

// entryOfMeet resolves entryID, checks it belongs to meetID (via its
// event), and returns both.
func (s *ResultsService) entryOfMeet(ctx context.Context, meetID, entryID string) (store.EntryRecord, store.EventRecord, error) {
	entry, err := store.GetEntry(ctx, s.db, entryID)
	if err != nil {
		return store.EntryRecord{}, store.EventRecord{}, err
	}
	event, err := store.GetEvent(ctx, s.db, entry.EventID)
	if err != nil {
		return store.EntryRecord{}, store.EventRecord{}, err
	}
	if event.MeetID != meetID {
		return store.EntryRecord{}, store.EventRecord{}, ErrEntryWrongMeet
	}
	return entry, event, nil
}

// EventDetail resolves one event for display — the discipline/family
// context header the check-in and seeding workspace pages share.
func (s *ResultsService) EventDetail(ctx context.Context, actor Session, meetID, eventID string) (ProgrammeEvent, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return ProgrammeEvent{}, err
	}
	event, err := eventOfMeet(ctx, s.db, meetID, eventID)
	if err != nil {
		return ProgrammeEvent{}, err
	}
	pe := ProgrammeEvent{EventRecord: event}
	if disc, ok := s.catalog.ByCode(event.DisciplineCode); ok {
		pe.DisciplineName, pe.Family = disc.Name, disc.Family
	}
	if pe.Rounds, err = store.ListRounds(ctx, s.db, eventID); err != nil {
		return ProgrammeEvent{}, err
	}
	return pe, nil
}

// CheckInRoster lists one event's entries with their current check-in state
// (UC-007): entered (awaiting check-in), confirmed, scratched, dns.
func (s *ResultsService) CheckInRoster(ctx context.Context, actor Session, meetID, eventID string) ([]EntryDetail, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	if _, err := eventOfMeet(ctx, s.db, meetID, eventID); err != nil {
		return nil, err
	}
	recs, err := store.ListEntriesByEvent(ctx, s.db, eventID)
	if err != nil {
		return nil, err
	}
	return s.enrichEntries(ctx, recs)
}

// ConfirmCheckIn confirms one entry's check-in (entered -> confirmed,
// UC-007 #1). Audited.
func (s *ResultsService) ConfirmCheckIn(ctx context.Context, actor Session, meetID, entryID string, expectedVersion int64) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	entry, _, err := s.entryOfMeet(ctx, meetID, entryID)
	if err != nil {
		return err
	}
	if entry.Status != domain.EntryEntered {
		return ErrEntryNotConfirmable
	}
	return s.setEntryStatus(ctx, actor, entryID, expectedVersion, domain.EntryConfirmed, "checkin.confirm")
}

// CloseCheckIn is the operator's one action that closes check-in for an
// event: every entry still "entered" (never confirmed) is marked DNS, and
// the final start list shows the confirmed starters plus every DNS
// (UC-007 #1, SYS-025). Returns the number of entries marked DNS.
func (s *ResultsService) CloseCheckIn(ctx context.Context, actor Session, meetID, eventID string) (int, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return 0, err
	}
	if _, err := eventOfMeet(ctx, s.db, meetID, eventID); err != nil {
		return 0, err
	}
	recs, err := store.ListEntriesByEvent(ctx, s.db, eventID)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, e := range recs {
		if e.Status != domain.EntryEntered {
			continue
		}
		if err := s.setEntryStatus(ctx, actor, e.ID, e.Version, domain.EntryDNS, "checkin.close_dns"); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ReinstateEntry reverses a DNS back to confirmed — a referee override
// before the start (UC-007 #2), audited so the change is traceable.
func (s *ResultsService) ReinstateEntry(ctx context.Context, actor Session, meetID, entryID string, expectedVersion int64) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	entry, _, err := s.entryOfMeet(ctx, meetID, entryID)
	if err != nil {
		return err
	}
	if entry.Status != domain.EntryDNS {
		return ErrEntryNotDNS
	}
	return s.setEntryStatus(ctx, actor, entryID, expectedVersion, domain.EntryConfirmed, "checkin.reinstate")
}

// setEntryStatus applies an entry status transition under optimistic
// concurrency and audits it (SYS-046).
func (s *ResultsService) setEntryStatus(ctx context.Context, actor Session, entryID string, expectedVersion int64, status domain.EntryStatus, action string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.UpdateEntryStatus(ctx, tx, entryID, expectedVersion, status); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]string{"status": string(status)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: action,
		EntityType: "entry", EntityID: entryID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}
