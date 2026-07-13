// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// PrivacyService hosts the SYS-101 data-subject-rights (subject-access
// export, erasure) and SYS-102 retention-purge use-cases (TASK-023,
// UC-024). It is a separate service from ResultsService (which owns
// SYS-103 consent — see ResultsService.SetConsent — since consent is
// captured as part of the entry/roster flow those handlers already gate)
// because erasure and retention purge are instance-administration-adjacent
// actions with their own authorization tier and no natural home in the
// capture/standings surface.
type PrivacyService struct {
	db  *sql.DB
	now Clock
}

// NewPrivacyService wires a PrivacyService against the instance's store.
func NewPrivacyService(db *sql.DB) *PrivacyService {
	return &PrivacyService{db: db, now: time.Now}
}

// WithClock overrides the time source (tests only) — the SYS-102 retention
// purge's clock-injected test suite needs this to place a fixture meet on
// either side of the retention cutoff deterministically.
func (p *PrivacyService) WithClock(now Clock) *PrivacyService {
	p.now = now
	return p
}

// ErrAthleteNotFound aliases the store sentinel for web handlers (every
// store entity lookup, including store.GetAthlete, returns the same
// store.ErrNotFound — see internal/app/meet.go's identical ErrMeetNotFound
// alias for the established pattern).
var ErrAthleteNotFound = store.ErrNotFound

// ErrAthleteAnonymized means the athlete's personal data was already
// erased (SYS-101): a repeat erasure is refused rather than silently
// re-writing already-cleared fields (the pseudonym would otherwise churn
// on every call, which is harmless but pointless — refusing loudly is
// clearer for the office operator acting on a second request).
var ErrAthleteAnonymized = errors.New("athlete: already anonymized")

// DefaultRetentionDays is the privacy-protective default for SYS-102
// retention: contact/consent-recorder metadata and full birth dates become
// purgeable this many days after the last meet an athlete participated in
// ends. UC-024 #3's own acceptance example ("retention configured to purge
// contact data 90 days post-meet") is adopted as the shipped default
// (SYS-102: "defaults SHALL be documented and privacy-protective") — see
// docs/requirements/open-questions-and-assumptions.md OQ-042 for whether a
// federation-specific results-challenge window argues for a different
// number once one is confirmed.
const DefaultRetentionDays = 90

// AthleteDataExport is the SYS-101 subject-access export's shape (UC-024
// #1): every piece of personal data this system holds about one athlete,
// machine-readable, and — by construction, since every query here filters
// on athlete_id — nothing about any other person.
type AthleteDataExport struct {
	AthleteID      string                       `json:"athlete_id"`
	FirstName      string                       `json:"first_name"`
	LastName       string                       `json:"last_name"`
	BirthDate      string                       `json:"birth_date,omitempty"`
	BirthYear      int                          `json:"birth_year"`
	Sex            string                       `json:"sex"`
	Nationality    string                       `json:"nationality,omitempty"`
	Clubs          []string                     `json:"clubs,omitempty"`
	ExternalIDs    map[string]string            `json:"external_ids,omitempty"`
	Consent        ConsentExport                `json:"consent"`
	Anonymized     bool                         `json:"anonymized"`
	Participations []store.AthleteParticipation `json:"participations,omitempty"`
	Results        []store.AthleteResult        `json:"results,omitempty"`
	ExportedAt     time.Time                    `json:"exported_at"`
}

// ConsentExport is the export's JSON-friendly projection of
// domain.PublicationConsent (field names spelled out for a human reading
// the export, not the internal Go field names).
type ConsentExport struct {
	ResultsPublicationWithdrawn bool      `json:"results_publication_withdrawn"`
	PhotoConsentGiven           bool      `json:"photo_consent_given"`
	ExtendedDataConsentGiven    bool      `json:"extended_data_consent_given"`
	RecordedAt                  time.Time `json:"recorded_at,omitempty"`
}

// ExportAthleteData assembles the SYS-101 subject-access export for one
// athlete (UC-024 #1). Office level and above (CapPrivacyActions); the
// export itself is audited (metadata only — an export doesn't mutate
// anything, but SYS-046 treats every data-subject-rights action as
// privileged and worth a trail entry, and UC-024 expects the action
// "documented" the same way erasure is).
func (p *PrivacyService) ExportAthleteData(ctx context.Context, actor Session, athleteID string) (AthleteDataExport, error) {
	if err := Authorize(actor.Role, CapPrivacyActions); err != nil {
		return AthleteDataExport{}, err
	}
	rec, err := store.GetAthlete(ctx, p.db, athleteID)
	if err != nil {
		return AthleteDataExport{}, err
	}
	clubNames, err := store.ClubNames(ctx, p.db, rec.ClubIDs)
	if err != nil {
		return AthleteDataExport{}, err
	}
	clubs := make([]string, 0, len(rec.ClubIDs))
	for _, id := range rec.ClubIDs {
		if name, ok := clubNames[id]; ok {
			clubs = append(clubs, name)
		}
	}
	participations, err := store.ListParticipationsByAthlete(ctx, p.db, athleteID)
	if err != nil {
		return AthleteDataExport{}, err
	}
	results, err := store.ListResultsByAthlete(ctx, p.db, athleteID)
	if err != nil {
		return AthleteDataExport{}, err
	}

	out := AthleteDataExport{
		AthleteID:   rec.ID,
		FirstName:   rec.FirstName,
		LastName:    rec.LastName,
		BirthYear:   rec.BirthYear,
		Sex:         string(rec.Sex),
		Nationality: rec.Nationality,
		Clubs:       clubs,
		ExternalIDs: rec.ExternalIDs,
		Consent: ConsentExport{
			ResultsPublicationWithdrawn: rec.Consent.ResultsPublicationWithdrawn,
			PhotoConsentGiven:           rec.Consent.PhotoConsentGiven,
			ExtendedDataConsentGiven:    rec.Consent.ExtendedDataConsentGiven,
			RecordedAt:                  rec.Consent.RecordedAt,
		},
		Anonymized:     rec.Anonymized,
		Participations: participations,
		Results:        results,
		ExportedAt:     p.now(),
	}
	if rec.BirthDate != nil {
		out.BirthDate = rec.BirthDate.Format("2006-01-02")
	}

	if err := p.audit(ctx, actor.AccountID, "athlete.export", athleteID, "", ""); err != nil {
		return out, err
	}
	return out, nil
}

// EraseAthlete implements the SYS-101 erasure/pseudonymization request
// (UC-024 #2): it applies domain.Athlete.AnonymizePersonalData and
// persists the result. Office level and above (CapPrivacyActions).
//
// The athlete.erase audit row deliberately documents ONLY that an erasure
// happened (actor, timestamp, reason, which field categories were
// cleared) — never the pre-erasure name. The audit_log table is
// append-only by design (SYS-046, migrations/0001_audit_log.sql): if the
// real name were written into before_json here "for the record", it would
// sit in that immutable log forever, permanently defeating the very
// erasure this action performs. UC-024 #2's "the action is documented" is
// satisfied by recording that erasure occurred, not by re-embedding what
// it erased.
//
// That leaves the OLDER audit rows a prior action already wrote with the
// name in plain text: participant.register (internal/app/results.go) and
// entry.submit (internal/app/entry.go) both embed the athlete's name in
// their after_json at the time, before this erasure ever ran. TASK-029
// privacy-review finding #1: those rows must be redacted too, or the
// erasure is cosmetic — the subject's name survives, just one hop away in
// the same table. This reuses RedactAuditPII (the SYS-102 retention
// purge's own transactional trigger-drop/restore mechanism,
// internal/store/privacy.go) rather than inventing a second redaction
// path — erasure and retention purge both need "redact exactly these
// audit_log rows, leave actor/action/entity/timestamp intact" and nothing
// more.
func (p *PrivacyService) EraseAthlete(ctx context.Context, actor Session, athleteID, reason string) error {
	if err := Authorize(actor.Role, CapPrivacyActions); err != nil {
		return err
	}
	rec, err := store.GetAthlete(ctx, p.db, athleteID)
	if err != nil {
		return err
	}
	if rec.Anonymized {
		return ErrAthleteAnonymized
	}

	a := rec.Athlete
	a.AnonymizePersonalData(p.now())

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("erase athlete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.AnonymizeAthlete(ctx, tx, a, rec.Version); err != nil {
		return err
	}

	participantIDs, resultIDs, entryIDs, err := store.AthletePersonalDataEntityIDs(ctx, tx, athleteID)
	if err != nil {
		return fmt.Errorf("erase athlete: find audit entity ids: %w", err)
	}
	var redacted int64
	for _, scope := range []struct {
		entityType string
		ids        []string
	}{
		{"participant", participantIDs},
		{"result", resultIDs},
		{"entry", entryIDs},
	} {
		n, err := store.RedactAuditPII(ctx, tx, scope.entityType, scope.ids)
		if err != nil {
			return fmt.Errorf("erase athlete: redact audit PII (%s): %w", scope.entityType, err)
		}
		redacted += n
	}

	after, _ := json.Marshal(map[string]any{
		"anonymized":          true,
		"fields_cleared":      []string{"first_name", "last_name", "birth_date", "external_ids"},
		"audit_rows_redacted": redacted,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "athlete.erase",
		EntityType: "athlete", EntityID: athleteID, After: string(after), Reason: reason,
	}); err != nil {
		return fmt.Errorf("audit erasure: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erase athlete: %w", err)
	}
	return nil
}

// audit appends one privacy-action audit row inside its own short
// transaction (read-only PrivacyService calls like ExportAthleteData have
// no other transaction to piggyback on).
func (p *PrivacyService) audit(ctx context.Context, actorID, action, entityID, after, reason string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actorID, Action: action, EntityType: "athlete", EntityID: entityID, After: after, Reason: reason,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return tx.Commit()
}

// PurgeReport summarizes one SYS-102 retention-purge run (UC-024 #3's
// "verification query finds no out-of-retention data" is exactly what
// FindAthletesOutsideRetention re-running with zero results proves).
type PurgeReport struct {
	Cutoff            time.Time `json:"cutoff"`
	AthletesPurged    int       `json:"athletes_purged"`
	AuditRowsRedacted int       `json:"audit_rows_redacted"`
}

// PurgeExpired runs the SYS-102 retention purge on manual admin trigger
// (UC-024 #3's route: an office/admin action, gated CapManageRetention —
// instance-admin, the same tier as the one-action backup, since a purge is
// also whole-instance and irreversible).
func (p *PrivacyService) PurgeExpired(ctx context.Context, actor Session, retentionDays int) (PurgeReport, error) {
	if err := Authorize(actor.Role, CapManageRetention); err != nil {
		return PurgeReport{}, err
	}
	return p.purge(ctx, actor.AccountID, retentionDays)
}

// PurgeExpiredAtStartup runs the same SYS-102 sweep with no authenticated
// actor (cmd/bahnfrei wires this once per process start, SYS-102: "SHALL
// be automatically purgeable" — a purely manual trigger would not satisfy
// "automatically"). The audit trail records the actor as "system" so a
// startup-triggered purge is distinguishable from an admin-triggered one.
func (p *PrivacyService) PurgeExpiredAtStartup(ctx context.Context, retentionDays int) (PurgeReport, error) {
	return p.purge(ctx, systemPurgeActor, retentionDays)
}

// systemPurgeActor is the audit-log actor recorded for a startup-triggered
// (as opposed to admin-triggered) retention purge.
const systemPurgeActor = "system"

func (p *PrivacyService) purge(ctx context.Context, actorID string, retentionDays int) (PurgeReport, error) {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}
	cutoff := p.now().AddDate(0, 0, -retentionDays)
	report := PurgeReport{Cutoff: cutoff}

	athleteIDs, err := store.FindAthletesOutsideRetention(ctx, p.db, cutoff)
	if err != nil {
		return report, fmt.Errorf("retention purge: find expired athletes: %w", err)
	}
	for _, id := range athleteIDs {
		if err := store.PurgeAthleteRetentionData(ctx, p.db, id); err != nil {
			return report, fmt.Errorf("retention purge: %w", err)
		}
		report.AthletesPurged++
	}

	meetIDs, err := store.FindExpiredMeetIDs(ctx, p.db, cutoff)
	if err != nil {
		return report, fmt.Errorf("retention purge: find expired meets: %w", err)
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("retention purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var redacted int64
	for _, meetID := range meetIDs {
		participantIDs, resultIDs, entryIDs, err := store.MeetPersonalDataEntityIDs(ctx, tx, meetID)
		if err != nil {
			return report, fmt.Errorf("retention purge: meet %s: %w", meetID, err)
		}
		n, err := store.RedactAuditPII(ctx, tx, "participant", participantIDs)
		if err != nil {
			return report, fmt.Errorf("retention purge: %w", err)
		}
		redacted += n
		n, err = store.RedactAuditPII(ctx, tx, "result", resultIDs)
		if err != nil {
			return report, fmt.Errorf("retention purge: %w", err)
		}
		redacted += n
		// TASK-029 privacy-review finding #2: entries (entity_type "entry",
		// entry.submit's audit row) were previously omitted from this sweep
		// entirely — online-entry audit names never aged out.
		n, err = store.RedactAuditPII(ctx, tx, "entry", entryIDs)
		if err != nil {
			return report, fmt.Errorf("retention purge: %w", err)
		}
		redacted += n
	}
	n, err := store.RedactAuditPII(ctx, tx, "athlete", athleteIDs)
	if err != nil {
		return report, fmt.Errorf("retention purge: %w", err)
	}
	redacted += n
	report.AuditRowsRedacted = int(redacted)

	after, _ := json.Marshal(report)
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actorID, Action: "privacy.retention_purge",
		EntityType: "instance", EntityID: "retention", After: string(after),
	}); err != nil {
		return report, fmt.Errorf("audit retention purge: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("retention purge: %w", err)
	}
	return report, nil
}
