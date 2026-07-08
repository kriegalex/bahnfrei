// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	if _, err := db.ExecContext(ctx, `INSERT INTO athletes
		(id, first_name, last_name, birth_date, birth_year, sex, nationality, club_ids, external_ids, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		a.ID, a.FirstName, a.LastName, birthDate, a.BirthYear, string(a.Sex),
		a.Nationality, string(clubs), string(ext)); err != nil {
		return AthleteRecord{}, fmt.Errorf("create athlete %s %s: %w", a.FirstName, a.LastName, err)
	}
	return AthleteRecord{Athlete: a, Version: 1}, nil
}

// GetAthlete looks up one athlete by ID.
func GetAthlete(ctx context.Context, db DBTX, id string) (AthleteRecord, error) {
	var a AthleteRecord
	var birthDate sql.NullString
	var sex, clubs, ext string
	err := db.QueryRowContext(ctx, `SELECT id, first_name, last_name, birth_date,
		birth_year, sex, nationality, club_ids, external_ids, version
		FROM athletes WHERE id = ?`, id).Scan(&a.ID, &a.FirstName, &a.LastName,
		&birthDate, &a.BirthYear, &sex, &a.Nationality, &clubs, &ext, &a.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return AthleteRecord{}, ErrNotFound
	case err != nil:
		return AthleteRecord{}, err
	}
	return decodeAthlete(a, birthDate, sex, clubs, ext)
}

func decodeAthlete(a AthleteRecord, birthDate sql.NullString, sex, clubs, ext string) (AthleteRecord, error) {
	a.Sex = domain.Sex(sex)
	if birthDate.Valid {
		d, err := time.Parse(dayFormat, birthDate.String)
		if err != nil {
			return AthleteRecord{}, fmt.Errorf("athlete %s: bad birth date %q: %w", a.ID, birthDate.String, err)
		}
		a.BirthDate = &d
	}
	if err := json.Unmarshal([]byte(clubs), &a.ClubIDs); err != nil {
		return AthleteRecord{}, fmt.Errorf("athlete %s: bad club ids: %w", a.ID, err)
	}
	if err := json.Unmarshal([]byte(ext), &a.ExternalIDs); err != nil {
		return AthleteRecord{}, fmt.Errorf("athlete %s: bad external ids: %w", a.ID, err)
	}
	return a, nil
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
