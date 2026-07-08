// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Participant{}, ErrDuplicateParticipant
		}
		return Participant{}, fmt.Errorf("register participant: %w", err)
	}
	return p, nil
}

// ListParticipants returns a meet's participants with their athlete data,
// ordered by bib then name for stable start lists.
func ListParticipants(ctx context.Context, db DBTX, meetID string) ([]ParticipantRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT
		p.id, p.meet_id, p.athlete_id, p.bib, p.version,
		a.id, a.first_name, a.last_name, a.birth_date, a.birth_year, a.sex,
		a.nationality, a.club_ids, a.external_ids, a.version
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
		var birthDate sql.NullString
		var sex, clubs, ext string
		if err := rows.Scan(&r.ID, &r.MeetID, &r.AthleteID, &r.Bib, &r.Version,
			&a.ID, &a.FirstName, &a.LastName, &birthDate, &a.BirthYear, &sex,
			&a.Nationality, &clubs, &ext, &a.Version); err != nil {
			return nil, err
		}
		dec, err := decodeAthlete(a, birthDate, sex, clubs, ext)
		if err != nil {
			return nil, err
		}
		r.Athlete = dec.Athlete
		out = append(out, r)
	}
	return out, rows.Err()
}
