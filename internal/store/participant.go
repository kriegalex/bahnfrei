// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// Participant is an athlete's registration at one meet with their start
// number (bib) — the "list of participants with their number" every meet
// runs on (research C7.3, UC-033 #4).
type Participant struct {
	ID        string
	MeetID    string
	AthleteID string
	Bib       string
	Version   int64
	// OutOfCompetition marks a registration ausser Konkurrenz/hors concours
	// (TASK-036, DEC-016/OQ-020 investigation, 0020_out_of_competition.sql):
	// the athlete's marks are still captured and shown, but they never hold
	// a numeric rank in standings, provisional or final.
	OutOfCompetition bool
}

// ParticipantRow is a participant joined with the athlete person data that
// standings and start lists present (bib, name, club, birth year — C7.3).
type ParticipantRow struct {
	Participant
	Athlete domain.Athlete
}

// ErrDuplicateParticipant means the athlete is already registered for the
// meet, or the bib is taken by someone else.
var ErrDuplicateParticipant = errors.New("participant already registered or bib taken")

// RegisterParticipant registers an athlete for a meet with an optional bib.
func RegisterParticipant(ctx context.Context, db DBTX, meetID, athleteID, bib string) (Participant, error) {
	p := Participant{ID: NewID(), MeetID: meetID, AthleteID: athleteID, Bib: bib, Version: 1}
	if meetID == "" || athleteID == "" {
		return Participant{}, errors.New("participant: meet id and athlete id are required")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO participants (id, meet_id, athlete_id, bib, version)
		VALUES (?, ?, ?, ?, 1)`, p.ID, p.MeetID, p.AthleteID, p.Bib)
	if err != nil {
		if isUniqueViolation(err) {
			return Participant{}, ErrDuplicateParticipant
		}
		return Participant{}, fmt.Errorf("register participant: %w", err)
	}
	return p, nil
}

// GetParticipant looks up one participant row by its own ID, regardless of
// meet (TASK-049, SYS-150/UC-043): the roster-edit form and its POST
// handler both need the row's meet/athlete linkage and current version
// before mutating identity data, and only have the participant ID from the
// route — callers MUST still check the returned MeetID against the meet
// they expected (an ID from another meet's roster must never be editable
// through this one's URL).
func GetParticipant(ctx context.Context, db DBTX, id string) (Participant, error) {
	var p Participant
	err := db.QueryRowContext(ctx, `SELECT id, meet_id, athlete_id, bib, version, out_of_competition
		FROM participants WHERE id = ?`, id).
		Scan(&p.ID, &p.MeetID, &p.AthleteID, &p.Bib, &p.Version, &p.OutOfCompetition)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Participant{}, ErrNotFound
	case err != nil:
		return Participant{}, err
	}
	return p, nil
}

// GetParticipantByAthlete looks up an athlete's participant row at meetID.
func GetParticipantByAthlete(ctx context.Context, db DBTX, meetID, athleteID string) (Participant, error) {
	var p Participant
	err := db.QueryRowContext(ctx, `SELECT id, meet_id, athlete_id, bib, version, out_of_competition
		FROM participants WHERE meet_id = ? AND athlete_id = ?`, meetID, athleteID).
		Scan(&p.ID, &p.MeetID, &p.AthleteID, &p.Bib, &p.Version, &p.OutOfCompetition)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Participant{}, ErrNotFound
	case err != nil:
		return Participant{}, err
	}
	return p, nil
}

// EnsureParticipant idempotently registers athleteID as a participant of
// meetID with no bib yet (TASK-016, SYS-018): an online entry needs its
// athlete(s) to hold a meet-wide participant row so bib assignment stays
// meet-scoped rather than per-event, regardless of how many events (or, for
// a relay, how many entries) the athlete appears in.
func EnsureParticipant(ctx context.Context, db DBTX, meetID, athleteID string) (Participant, error) {
	p, err := RegisterParticipant(ctx, db, meetID, athleteID, "")
	if errors.Is(err, ErrDuplicateParticipant) {
		return GetParticipantByAthlete(ctx, db, meetID, athleteID)
	}
	return p, err
}

// UpdateParticipantBib assigns or edits a participant's bib under optimistic
// concurrency (SYS-018): the meet-wide UNIQUE(meet_id, bib) index
// (0004_athletes_results.sql) rejects a duplicate manual assignment as
// ErrDuplicateParticipant (UC-006 #2).
func UpdateParticipantBib(ctx context.Context, db DBTX, id string, expectedVersion int64, bib string) (int64, error) {
	v, err := OptimisticUpdate(ctx, db, "participants", id, expectedVersion,
		Set{Column: "bib", Value: bib})
	if isUniqueViolation(err) {
		return 0, ErrDuplicateParticipant
	}
	return v, err
}

// UpdateParticipantOutOfCompetition sets a participant's ausser
// Konkurrenz/hors concours flag under optimistic concurrency (TASK-036,
// DEC-016/OQ-020 investigation; 0020_out_of_competition.sql).
func UpdateParticipantOutOfCompetition(ctx context.Context, db DBTX, id string, expectedVersion int64, outOfCompetition bool) (int64, error) {
	return OptimisticUpdate(ctx, db, "participants", id, expectedVersion,
		Set{Column: "out_of_competition", Value: outOfCompetition})
}

// ListParticipants returns a meet's participants with their athlete data
// including SYS-103 consent flags, ordered by bib then name for stable
// start lists.
func ListParticipants(ctx context.Context, db DBTX, meetID string) ([]ParticipantRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT
		p.id, p.meet_id, p.athlete_id, p.bib, p.version, p.out_of_competition,
		a.id, a.first_name, a.last_name, a.birth_date, a.birth_year, a.sex,
		a.nationality, a.club_ids, a.external_ids,
		a.results_publication_withdrawn, a.photo_consent_given, a.extended_data_consent_given,
		a.consent_recorded_at, a.consent_recorded_by, a.anonymized, a.anonymized_at, a.version
		FROM participants p JOIN athletes a ON a.id = p.athlete_id
		WHERE p.meet_id = ?
		ORDER BY CAST(p.bib AS INTEGER), p.bib, a.last_name, a.first_name, a.id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ParticipantRow
	for rows.Next() {
		var r ParticipantRow
		var a AthleteRecord
		var birthDate, consentRecordedAt, anonymizedAt sql.NullString
		var sex, clubs, ext string
		if err := rows.Scan(&r.ID, &r.MeetID, &r.AthleteID, &r.Bib, &r.Version, &r.OutOfCompetition,
			&a.ID, &a.FirstName, &a.LastName, &birthDate, &a.BirthYear, &sex,
			&a.Nationality, &clubs, &ext,
			&a.Consent.ResultsPublicationWithdrawn, &a.Consent.PhotoConsentGiven, &a.Consent.ExtendedDataConsentGiven,
			&consentRecordedAt, &a.Consent.RecordedBy, &a.Anonymized, &anonymizedAt, &a.Version); err != nil {
			return nil, err
		}
		dec, err := decodeAthlete(a, birthDate, sex, clubs, ext, consentRecordedAt, anonymizedAt)
		if err != nil {
			return nil, err
		}
		r.Athlete = dec.Athlete
		out = append(out, r)
	}
	return out, rows.Err()
}
