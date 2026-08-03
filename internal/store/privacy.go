// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- privacy: SYS-101 data-subject rights and SYS-102 retention purge
// storage primitives (TASK-023, UC-023/UC-024). ---

// UpdateAthleteConsent persists the athlete's SYS-103 publication-consent
// flags via the shared OptimisticUpdate helper (like every other athlete
// mutation) and stamps who/when recorded the change.
func UpdateAthleteConsent(ctx context.Context, db DBTX, id string, expectedVersion int64,
	consent domain.PublicationConsent) (int64, error) {
	return OptimisticUpdate(ctx, db, "athletes", id, expectedVersion,
		Set{Column: "results_publication_withdrawn", Value: consent.ResultsPublicationWithdrawn},
		Set{Column: "photo_consent_given", Value: consent.PhotoConsentGiven},
		Set{Column: "extended_data_consent_given", Value: consent.ExtendedDataConsentGiven},
		Set{Column: "consent_recorded_at", Value: consent.RecordedAt.UTC().Format(time.RFC3339Nano)},
		Set{Column: "consent_recorded_by", Value: consent.RecordedBy},
	)
}

// AnonymizeAthlete persists a completed SYS-101 erasure request: the
// pseudonymized name, cleared birth date and external IDs the domain
// layer's Athlete.AnonymizePersonalData computed, plus the
// anonymized/anonymized_at markers. It is idempotent at the storage level
// (a second call just re-applies the same pseudonym) — the app layer is
// what refuses to re-erase (internal/app/privacy.go).
func AnonymizeAthlete(ctx context.Context, db DBTX, a domain.Athlete, expectedVersion int64) (int64, error) {
	ext, err := json.Marshal(orEmptyMap(a.ExternalIDs))
	if err != nil {
		return 0, fmt.Errorf("anonymize athlete %s: %w", a.ID, err)
	}
	var anonymizedAt any
	if a.AnonymizedAt != nil {
		anonymizedAt = a.AnonymizedAt.UTC().Format(time.RFC3339Nano)
	}
	return OptimisticUpdate(ctx, db, "athletes", a.ID, expectedVersion,
		Set{Column: "first_name", Value: a.FirstName},
		Set{Column: "last_name", Value: a.LastName},
		Set{Column: "birth_date", Value: nil},
		Set{Column: "external_ids", Value: string(ext)},
		Set{Column: "anonymized", Value: true},
		Set{Column: "anonymized_at", Value: anonymizedAt},
	)
}

// AthleteParticipation is one meet an athlete registered for — the
// SYS-101 subject-access export's per-meet line (UC-024 #1).
type AthleteParticipation struct {
	MeetID   string
	MeetName string
	Bib      string
}

// ListParticipationsByAthlete returns every meet the athlete registered
// for, across the whole instance (an athlete is instance-global, SYS-010 —
// a subject-access export must cover all of it, not one meet, UC-024 #1).
func ListParticipationsByAthlete(ctx context.Context, db DBTX, athleteID string) ([]AthleteParticipation, error) {
	rows, err := db.QueryContext(ctx, `SELECT p.meet_id, m.name, p.bib
		FROM participants p JOIN meets m ON m.id = p.meet_id
		WHERE p.athlete_id = ? ORDER BY m.start_date, m.name`, athleteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AthleteParticipation
	for rows.Next() {
		var p AthleteParticipation
		if err := rows.Scan(&p.MeetID, &p.MeetName, &p.Bib); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AthleteResult is one of the athlete's own settled results — the
// SYS-101 export's per-result line (UC-024 #1: "nothing about other
// persons" — this query filters by athlete_id, never returns a unit's
// other competitors).
type AthleteResult struct {
	MeetID         string
	MeetName       string
	DisciplineCode string
	Mark           string
	Status         domain.QualificationStatus
	Points         *int
	Placing        *int
}

// ListResultsByAthlete returns every settled result belonging to the
// athlete, across every meet, for the SYS-101 subject-access export.
func ListResultsByAthlete(ctx context.Context, db DBTX, athleteID string) ([]AthleteResult, error) {
	rows, err := db.QueryContext(ctx, `SELECT m.id, m.name, e.discipline_code, r.mark, r.status, r.points, r.placing
		FROM results r
		JOIN units u ON u.id = r.unit_id
		JOIN rounds rd ON rd.id = u.round_id
		JOIN events e ON e.id = rd.event_id
		JOIN meets m ON m.id = e.meet_id
		WHERE r.athlete_id = ?
		ORDER BY m.start_date, e.discipline_code`, athleteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AthleteResult
	for rows.Next() {
		var r AthleteResult
		var status string
		if err := rows.Scan(&r.MeetID, &r.MeetName, &r.DisciplineCode, &r.Mark, &status, &r.Points, &r.Placing); err != nil {
			return nil, err
		}
		r.Status = domain.QualificationStatus(status)
		out = append(out, r)
	}
	return out, rows.Err()
}

// FindAthletesOutsideRetention returns the IDs of athletes whose every
// meet participation ended before cutoff (SYS-102: "personal data not
// needed for the permanent sporting record ... automatically purgeable
// after a configured period post-meet"). An athlete with no participation
// at all, or with at least one participation still inside the retention
// window (they might return this season), is left untouched. Only athletes
// that still hold purgeable data (a full birth date, or consent-recorder
// metadata) are returned, so a repeat sweep is a cheap no-op.
func FindAthletesOutsideRetention(ctx context.Context, db DBTX, cutoff time.Time) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id FROM athletes a
		WHERE EXISTS (SELECT 1 FROM participants p WHERE p.athlete_id = a.id)
		  AND NOT EXISTS (
		      SELECT 1 FROM participants p
		      JOIN meets m ON m.id = p.meet_id
		      WHERE p.athlete_id = a.id AND m.end_date >= ?
		  )
		  AND (a.birth_date IS NOT NULL OR a.consent_recorded_by <> '')`,
		cutoff.UTC().Format(dayFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PurgeAthleteRetentionData clears the SYS-102 retention-scoped personal
// data for one athlete that FindAthletesOutsideRetention selected: the
// full birth date (BirthYear, needed for category/eligibility integrity,
// is kept — see domain.Athlete.AnonymizePersonalData's identical
// rationale) and the consent-recorder identity (RecordedAt and the flags
// themselves are kept: zeroing a withdrawal flag would silently
// re-publish a suppressed athlete). Idempotent: a repeat call on an
// already-purged athlete is a no-op (FindAthletesOutsideRetention would
// not have selected it again either).
func PurgeAthleteRetentionData(ctx context.Context, db DBTX, athleteID string) error {
	_, err := db.ExecContext(ctx, `UPDATE athletes SET
		birth_date = NULL, consent_recorded_by = '', version = version + 1
		WHERE id = ?`, athleteID)
	if err != nil {
		return fmt.Errorf("purge retention data for athlete %s: %w", athleteID, err)
	}
	return nil
}

// FindExpiredMeetIDs returns the IDs of meets that ended before cutoff —
// the SYS-102 retention purge's per-meet audit-redaction scope (a meet's
// participant/result audit rows are redacted once the meet itself is out
// of retention, independent of whether any of its athletes also qualify
// for FindAthletesOutsideRetention).
func FindExpiredMeetIDs(ctx context.Context, db DBTX, cutoff time.Time) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM meets WHERE end_date < ?`, cutoff.UTC().Format(dayFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MeetPersonalDataEntityIDs returns the participant, result and entry row
// IDs belonging to meetID — the audit_log (entity_type, entity_id) pairs
// the SYS-102 retention purge redacts once the meet is out of retention
// (RedactAuditPII's callers). Entries are event-scoped, not directly
// meet-scoped (entries.event_id -> events.meet_id), unlike
// participants/results which carry a direct or one-hop meet reference —
// TASK-029 privacy-review finding #2: online-entry audit rows
// (entity_type "entry", written by internal/app/entry.go's
// auditEntrySubmit) were omitted from the purge sweep entirely.
func MeetPersonalDataEntityIDs(ctx context.Context, db DBTX, meetID string) (participantIDs, resultIDs, entryIDs []string, err error) {
	if participantIDs, err = queryIDColumn(ctx, db, `SELECT id FROM participants WHERE meet_id = ?`, meetID); err != nil {
		return nil, nil, nil, err
	}
	if resultIDs, err = queryIDColumn(ctx, db, `SELECT r.id FROM results r
		JOIN units u ON u.id = r.unit_id
		JOIN rounds rd ON rd.id = u.round_id
		JOIN events e ON e.id = rd.event_id
		WHERE e.meet_id = ?`, meetID); err != nil {
		return nil, nil, nil, err
	}
	entryIDs, err = queryIDColumn(ctx, db, `SELECT e.id FROM entries e
		JOIN events ev ON ev.id = e.event_id
		WHERE ev.meet_id = ?`, meetID)
	return participantIDs, resultIDs, entryIDs, err
}

// AthletePersonalDataEntityIDs returns the participant, result and entry
// row IDs belonging to athleteID, across every meet the athlete ever
// touched — the audit_log (entity_type, entity_id) pairs the SYS-101
// erasure request redacts (RedactAuditPII's caller in
// internal/app/privacy.go PrivacyService.EraseAthlete). Unlike
// MeetPersonalDataEntityIDs (meet-scoped, for the SYS-102 retention purge
// sweep), erasure is athlete-scoped: an athlete is instance-global
// (SYS-010), so their audit-payload PII can be sitting in any meet's
// participant.register/entry.submit rows, not just the one the erasure
// request happened to be filed from — TASK-029 privacy-review finding #1.
func AthletePersonalDataEntityIDs(ctx context.Context, db DBTX, athleteID string) (participantIDs, resultIDs, entryIDs []string, err error) {
	if participantIDs, err = queryIDColumn(ctx, db, `SELECT id FROM participants WHERE athlete_id = ?`, athleteID); err != nil {
		return nil, nil, nil, err
	}
	if resultIDs, err = queryIDColumn(ctx, db, `SELECT id FROM results WHERE athlete_id = ?`, athleteID); err != nil {
		return nil, nil, nil, err
	}
	entryIDs, err = queryIDColumn(ctx, db, `SELECT id FROM entries WHERE athlete_id = ?`, athleteID)
	return participantIDs, resultIDs, entryIDs, err
}

// queryIDColumn runs a single-column `id`-shaped query and collects the
// results — the shared scan loop behind FindExpiredMeetIDs,
// MeetPersonalDataEntityIDs and AthletePersonalDataEntityIDs.
func queryIDColumn(ctx context.Context, db DBTX, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AuditRedactionMarker replaces a redacted audit_log row's before/after
// JSON payload — never NULL, so a redacted row is visibly distinct from a
// row that legitimately never had a payload (e.g. no Before on create).
const AuditRedactionMarker = `{"redacted":"SYS-102 retention purge"}`

// RedactAuditPII implements the audit_log table's documented exception
// (see migrations/0001_audit_log.sql's header comment): the table's
// UPDATE/DELETE triggers make it append-only for every ordinary write path
// so corrections are always new rows (SYS-046) — but a retained
// before/after JSON snapshot can itself carry personal data (SYS-102 names
// "audit PII" explicitly), and once that data is out of retention it must
// be purgeable too. This is the one blessed, transactional exception: it
// drops the no-update trigger, redacts (never deletes) the before/after
// payload of exactly the given (entity_type, entity_id) rows to
// AuditRedactionMarker — leaving actor/action/entity/timestamp intact, so
// the audit trail's "who did what to which entity, when" shape (SYS-046)
// survives — then reinstates the trigger before the transaction commits.
// No other code path may touch audit_log with UPDATE; this function is the
// only place that ever does, and it is itself audited by the caller
// (internal/app/privacy.go PrivacyService.PurgeExpired) via a normal
// AppendAudit row documenting that a redaction ran.
func RedactAuditPII(ctx context.Context, tx *sql.Tx, entityType string, entityIDs []string) (int64, error) {
	if len(entityIDs) == 0 {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, `DROP TRIGGER audit_log_no_update`); err != nil {
		return 0, fmt.Errorf("redact audit PII: lift append-only trigger: %w", err)
	}
	// Trigger is dropped for the remainder of this transaction only —
	// restored unconditionally below (including on the early-return error
	// paths) before the caller can commit.
	restore := func() error {
		_, err := tx.ExecContext(ctx, `CREATE TRIGGER audit_log_no_update
			BEFORE UPDATE ON audit_log
			BEGIN
			    SELECT RAISE(ABORT, 'audit log is append-only (SYS-046)');
			END`)
		return err
	}

	placeholders := make([]string, len(entityIDs))
	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, entityType)
	for i, id := range entityIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	// #nosec G202 -- placeholders is a slice of the fixed literal "?" (one per entityIDs
	// element, set above), never an interpolated value; every entityIDs value itself is
	// passed as a bound arg via execArgs below, not concatenated into the query text.
	query := `UPDATE audit_log SET before_json = ?, after_json = ?
		WHERE entity_type = ? AND entity_id IN (` + strings.Join(placeholders, ",") + `)
		AND (before_json IS NOT NULL OR after_json IS NOT NULL)`
	execArgs := append([]any{AuditRedactionMarker, AuditRedactionMarker}, args...)
	res, err := tx.ExecContext(ctx, query, execArgs...)
	if err != nil {
		_ = restore()
		return 0, fmt.Errorf("redact audit PII: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		_ = restore()
		return 0, err
	}
	if err := restore(); err != nil {
		return 0, fmt.Errorf("redact audit PII: restore append-only trigger: %w", err)
	}
	return n, nil
}
