// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// AthleteRecord is a persisted domain.Athlete plus its version.
type AthleteRecord struct {
	domain.Athlete
	Version int64
}

// ClubRecord is a persisted domain.Club plus its version.
type ClubRecord struct {
	domain.Club
	Version int64
}

// CreateClub inserts a club with a fresh ID. Club names are unique; use
// GetClubByName first when re-registering members of a known club.
func CreateClub(ctx context.Context, db DBTX, c domain.Club) (ClubRecord, error) {
	c.ID = NewID()
	if c.Name == "" {
		return ClubRecord{}, errors.New("club: name is required")
	}
	ext, err := json.Marshal(orEmptyMap(c.ExternalIDs))
	if err != nil {
		return ClubRecord{}, fmt.Errorf("create club %q: %w", c.Name, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO clubs (id, name, external_ids, version)
		VALUES (?, ?, ?, 1)`, c.ID, c.Name, string(ext)); err != nil {
		return ClubRecord{}, fmt.Errorf("create club %q: %w", c.Name, err)
	}
	return ClubRecord{Club: c, Version: 1}, nil
}

// GetClub looks a club up by ID (the omx/v1 full-meet export's club
// enumeration, TASK-025 — every other club lookup in this file is by name
// or by a batch of ids-to-names, neither of which returns the full record
// incl. ExternalIDs a document needs).
func GetClub(ctx context.Context, db DBTX, id string) (ClubRecord, error) {
	var c ClubRecord
	var ext string
	err := db.QueryRowContext(ctx, `SELECT id, name, external_ids, version
		FROM clubs WHERE id = ?`, id).Scan(&c.ID, &c.Name, &ext, &c.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ClubRecord{}, ErrNotFound
	case err != nil:
		return ClubRecord{}, err
	}
	if err := json.Unmarshal([]byte(ext), &c.ExternalIDs); err != nil {
		return ClubRecord{}, fmt.Errorf("club %s: bad external ids: %w", c.ID, err)
	}
	return c, nil
}

// GetClubByName looks a club up by its unique name.
func GetClubByName(ctx context.Context, db DBTX, name string) (ClubRecord, error) {
	var c ClubRecord
	var ext string
	err := db.QueryRowContext(ctx, `SELECT id, name, external_ids, version
		FROM clubs WHERE name = ?`, name).Scan(&c.ID, &c.Name, &ext, &c.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ClubRecord{}, ErrNotFound
	case err != nil:
		return ClubRecord{}, err
	}
	if err := json.Unmarshal([]byte(ext), &c.ExternalIDs); err != nil {
		return ClubRecord{}, fmt.Errorf("club %s: bad external ids: %w", c.ID, err)
	}
	return c, nil
}

// ClubNames resolves club IDs to their names (presentation joins).
func ClubNames(ctx context.Context, db DBTX, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM clubs WHERE id = ?`, id).Scan(&name)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return nil, err
		}
		out[id] = name
	}
	return out, nil
}

// CreateAthlete inserts an athlete with a fresh ID (SYS-010 person data).
// Consent (SYS-103) defaults to the privacy-protective baseline documented
// on domain.PublicationConsent when the caller leaves it zero-valued.
func CreateAthlete(ctx context.Context, db DBTX, a domain.Athlete) (AthleteRecord, error) {
	a.ID = NewID()
	if err := a.Validate(); err != nil {
		return AthleteRecord{}, err
	}
	clubs, err := json.Marshal(orEmptySlice(a.ClubIDs))
	if err != nil {
		return AthleteRecord{}, fmt.Errorf("create athlete: %w", err)
	}
	ext, err := json.Marshal(orEmptyMap(a.ExternalIDs))
	if err != nil {
		return AthleteRecord{}, fmt.Errorf("create athlete: %w", err)
	}
	var birthDate any
	if a.BirthDate != nil {
		birthDate = a.BirthDate.Format(dayFormat)
	}
	var consentRecordedAt any
	if !a.Consent.RecordedAt.IsZero() {
		consentRecordedAt = a.Consent.RecordedAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO athletes
		(id, first_name, last_name, birth_date, birth_year, sex, nationality, club_ids, external_ids,
		 results_publication_withdrawn, photo_consent_given, extended_data_consent_given,
		 consent_recorded_at, consent_recorded_by, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		a.ID, a.FirstName, a.LastName, birthDate, a.BirthYear, string(a.Sex),
		a.Nationality, string(clubs), string(ext),
		a.Consent.ResultsPublicationWithdrawn, a.Consent.PhotoConsentGiven, a.Consent.ExtendedDataConsentGiven,
		consentRecordedAt, a.Consent.RecordedBy); err != nil {
		return AthleteRecord{}, fmt.Errorf("create athlete %s %s: %w", a.FirstName, a.LastName, err)
	}
	return AthleteRecord{Athlete: a, Version: 1}, nil
}

const athleteColumns = `id, first_name, last_name, birth_date, birth_year, sex, nationality,
	club_ids, external_ids, results_publication_withdrawn, photo_consent_given,
	extended_data_consent_given, consent_recorded_at, consent_recorded_by,
	anonymized, anonymized_at, version`

// GetAthlete looks up one athlete by ID.
func GetAthlete(ctx context.Context, db DBTX, id string) (AthleteRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT `+athleteColumns+` FROM athletes WHERE id = ?`, id)
	a, err := scanAthlete(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AthleteRecord{}, ErrNotFound
	}
	return a, err
}

// athleteScanner is the common surface of *sql.Row and *sql.Rows, so
// scanAthlete and rows-based callers (ListParticipants) share one decode.
type athleteScanner interface {
	Scan(dest ...any) error
}

func scanAthlete(row athleteScanner) (AthleteRecord, error) {
	var a AthleteRecord
	var birthDate, consentRecordedAt, anonymizedAt sql.NullString
	var sex, clubs, ext string
	if err := row.Scan(&a.ID, &a.FirstName, &a.LastName, &birthDate, &a.BirthYear, &sex,
		&a.Nationality, &clubs, &ext,
		&a.Consent.ResultsPublicationWithdrawn, &a.Consent.PhotoConsentGiven, &a.Consent.ExtendedDataConsentGiven,
		&consentRecordedAt, &a.Consent.RecordedBy, &a.Anonymized, &anonymizedAt, &a.Version); err != nil {
		return AthleteRecord{}, err
	}
	return decodeAthlete(a, birthDate, sex, clubs, ext, consentRecordedAt, anonymizedAt)
}

func decodeAthlete(a AthleteRecord, birthDate sql.NullString, sex, clubs, ext string,
	consentRecordedAt, anonymizedAt sql.NullString) (AthleteRecord, error) {
	a.Sex = domain.Sex(sex)
	if birthDate.Valid {
		d, err := time.Parse(dayFormat, birthDate.String)
		if err != nil {
			return AthleteRecord{}, fmt.Errorf("athlete %s: bad birth date %q: %w", a.ID, birthDate.String, err)
		}
		a.BirthDate = &d
	}
	if consentRecordedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, consentRecordedAt.String)
		if err != nil {
			return AthleteRecord{}, fmt.Errorf("athlete %s: bad consent timestamp %q: %w", a.ID, consentRecordedAt.String, err)
		}
		a.Consent.RecordedAt = t
	}
	if anonymizedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, anonymizedAt.String)
		if err != nil {
			return AthleteRecord{}, fmt.Errorf("athlete %s: bad anonymized timestamp %q: %w", a.ID, anonymizedAt.String, err)
		}
		a.AnonymizedAt = &t
	}
	if err := json.Unmarshal([]byte(clubs), &a.ClubIDs); err != nil {
		return AthleteRecord{}, fmt.Errorf("athlete %s: bad club ids: %w", a.ID, err)
	}
	if err := json.Unmarshal([]byte(ext), &a.ExternalIDs); err != nil {
		return AthleteRecord{}, fmt.Errorf("athlete %s: bad external ids: %w", a.ID, err)
	}
	return a, nil
}

// FindAthleteByExternalID looks up an athlete by one of their namespaced
// external identifiers (ADR-005 §6) — e.g. a Swiss Athletics licence number
// (domain.NamespaceSwissAthleticsLicence). Used by the CSV/Alabus import
// path (SYS-013) to match a re-imported row to its existing athlete instead
// of creating a duplicate (UC-004 #2 idempotency). Returns ErrNotFound if no
// athlete carries that (namespace, id) pair.
func FindAthleteByExternalID(ctx context.Context, db DBTX, namespace, id string) (AthleteRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT `+athleteColumns+`
		FROM athletes WHERE json_extract(external_ids, '$.' || ?) = ? LIMIT 1`,
		jsonKeyPath(namespace), id)
	a, err := scanAthlete(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AthleteRecord{}, ErrNotFound
	}
	return a, err
}

// jsonKeyPath quotes namespace as a SQLite JSON path object-key segment
// (json_extract(doc, '$."namespace:with:colons"')) — external-ID namespaces
// contain colons (e.g. "swiss-athletics:licence"), which are not valid bare
// path tokens.
func jsonKeyPath(namespace string) string {
	return `"` + strings.ReplaceAll(namespace, `"`, `\"`) + `"`
}

// FindAthleteByNaturalKey looks up an athlete by (first name, last name,
// birth year, sex), case-insensitively on the names — the fallback
// idempotency match for import rows that carry no licence number (UC-004
// #2). Returns ErrNotFound if no athlete matches, or the first match if the
// natural key is ambiguous (rare at PoC scale; a licence number
// disambiguates when present).
func FindAthleteByNaturalKey(ctx context.Context, db DBTX, firstName, lastName string, birthYear int, sex domain.Sex) (AthleteRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT `+athleteColumns+`
		FROM athletes
		WHERE lower(first_name) = lower(?) AND lower(last_name) = lower(?)
		  AND birth_year = ? AND sex = ?
		LIMIT 1`, firstName, lastName, birthYear, string(sex))
	a, err := scanAthlete(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AthleteRecord{}, ErrNotFound
	}
	return a, err
}

// SetAthleteExternalID adds/overwrites one namespaced external identifier on
// an existing athlete (e.g. attaching a licence number discovered on
// re-import, UC-004 #2) without disturbing any other field, under
// optimistic concurrency.
func SetAthleteExternalID(ctx context.Context, db DBTX, athleteID string, expectedVersion int64, namespace, id string) (int64, error) {
	rec, err := GetAthlete(ctx, db, athleteID)
	if err != nil {
		return 0, err
	}
	ext := orEmptyMap(rec.ExternalIDs)
	if ext[namespace] == id {
		return rec.Version, nil // already set — idempotent no-op
	}
	ext.Set(namespace, id)
	data, err := json.Marshal(ext)
	if err != nil {
		return 0, fmt.Errorf("set athlete external id: %w", err)
	}
	return OptimisticUpdate(ctx, db, "athletes", athleteID, expectedVersion,
		Set{Column: "external_ids", Value: string(data)})
}

func orEmptyMap(m domain.ExternalIDs) domain.ExternalIDs {
	if m == nil {
		return domain.ExternalIDs{}
	}
	return m
}

func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
