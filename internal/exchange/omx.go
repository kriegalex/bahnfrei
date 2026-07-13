// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// SchemaVersionOMXV1 is the omx/v1 schema identifier every Document must
// carry (ADR-005 §2, SYS-144). See schema/omx-v1.schema.json's own
// "description" for the versioning/deprecation policy this constant is
// part of.
const SchemaVersionOMXV1 = "omx/v1"

//go:embed schema/omx-v1.schema.json
var omxV1Schema []byte

// OMXV1Schema returns the published omx/v1 JSON Schema document verbatim
// (SYS-144: "versioned and documented in-repo") — the same bytes
// ValidateOMXSchema checks an export against, so the two can never drift.
func OMXV1Schema() []byte {
	return omxV1Schema
}

// Document is one full-meet omx/v1 snapshot (ADR-005 §1/§2): every entity
// SYS-073's "reconstruct official results" needs, shaped after the SyRS §2
// canonical domain model. IDs are the entities' own identifiers WITHIN
// THIS DOCUMENT ONLY: a consumer re-importing it assigns its own storage
// identifiers, so round-trip equivalence (UC-027 #2) is judged by content
// (athlete natural key, discipline/category, mark/status/wind/...), never
// by literal id equality across the two systems.
type Document struct {
	SchemaVersion string          `json:"schemaVersion"`
	ExportedAt    time.Time       `json:"exportedAt"`
	Meet          MeetDoc         `json:"meet"`
	Sessions      []SessionDoc    `json:"sessions,omitempty"`
	Clubs         []ClubDoc       `json:"clubs,omitempty"`
	Athletes      []AthleteDoc    `json:"athletes"`
	RelayTeams    []RelayTeamDoc  `json:"relayTeams,omitempty"`
	Events        []EventDoc      `json:"events"`
	Rounds        []RoundDoc      `json:"rounds"`
	Units         []UnitDoc       `json:"units"`
	Entries       []EntryDoc      `json:"entries,omitempty"`
	Assignments   []AssignmentDoc `json:"unitAssignments,omitempty"`
	Results       []ResultDoc     `json:"results"`
}

// MeetDoc is the omx/v1 shape of domain.Meet.
type MeetDoc struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Venue              string            `json:"venue"`
	HomologationRef    string            `json:"homologationRef,omitempty"`
	StartDate          time.Time         `json:"startDate"`
	EndDate            time.Time         `json:"endDate"`
	Organizer          string            `json:"organizer,omitempty"`
	Tier               string            `json:"tier,omitempty"`
	Status             string            `json:"status"`
	CategorySchemeID   string            `json:"categorySchemeId,omitempty"`
	TemplateID         string            `json:"templateId,omitempty"`
	ScoringTableID     string            `json:"scoringTableId,omitempty"`
	ResultsPositioning string            `json:"resultsPositioning,omitempty"`
	OfficialSourceName string            `json:"officialSourceName,omitempty"`
	OfficialSourceURL  string            `json:"officialSourceUrl,omitempty"`
	ExternalIDs        map[string]string `json:"externalIds,omitempty"`
	EntryFeeCents      int64             `json:"entryFeeCents,omitempty"`
	RelayFeeCents      int64             `json:"relayFeeCents,omitempty"`
}

// SessionDoc is the omx/v1 shape of domain.Session (MeetID is implicit —
// one Document is always one meet).
type SessionDoc struct {
	ID    string    `json:"id"`
	Day   time.Time `json:"day"`
	Label string    `json:"label"`
}

// ClubDoc is the omx/v1 shape of domain.Club.
type ClubDoc struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	ExternalIDs map[string]string `json:"externalIds,omitempty"`
}

// AthleteDoc is the omx/v1 shape of domain.Athlete. Consent/anonymization
// state is deliberately excluded: those are instance-local operational
// flags, not part of the official-result interchange contract (OQ-056),
// and this export is an organizer-authorized federation-delivery artifact,
// not a public surface — SYS-100 public-minimization does not apply to it
// (the internal official result stays intact and exportable to the
// federation even for a withdrawn athlete, per UC-023 #2's own carve-out).
type AthleteDoc struct {
	ID          string            `json:"id"`
	FirstName   string            `json:"firstName,omitempty"`
	LastName    string            `json:"lastName"`
	BirthDate   *time.Time        `json:"birthDate,omitempty"`
	BirthYear   int               `json:"birthYear"`
	Sex         string            `json:"sex"`
	Nationality string            `json:"nationality,omitempty"`
	ClubIDs     []string          `json:"clubIds,omitempty"`
	ExternalIDs map[string]string `json:"externalIds,omitempty"`
	ParaClasses []string          `json:"paraClasses,omitempty"`
}

// RelayTeamDoc is the omx/v1 shape of domain.RelayTeam.
type RelayTeamDoc struct {
	ID          string   `json:"id"`
	ClubID      string   `json:"clubId"`
	Composition []string `json:"composition"`
	Reserves    []string `json:"reserves,omitempty"`
}

// EventDoc is the omx/v1 shape of domain.Event.
type EventDoc struct {
	ID             string     `json:"id"`
	DisciplineCode string     `json:"disciplineCode"`
	CategoryCodes  []string   `json:"categoryCodes"`
	EntryStandard  string     `json:"entryStandard,omitempty"`
	EntryDeadline  *time.Time `json:"entryDeadline,omitempty"`
	Status         string     `json:"status"`
	EntryLimit     int        `json:"entryLimit,omitempty"`
}

// RoundDoc is the omx/v1 shape of domain.Round, plus Seq (the progression
// order store.CreateRound persists but domain.Round itself does not carry).
type RoundDoc struct {
	ID      string `json:"id"`
	EventID string `json:"eventId"`
	Kind    string `json:"kind"`
	Seq     int    `json:"seq"`
}

// UnitDoc is the omx/v1 shape of domain.Unit, plus the unit's latest
// result-list announcement (SYS-047) and wind reading (SYS-040) — both
// unit-scoped state UC-027 #1 requires ("announcement timestamps").
type UnitDoc struct {
	ID              string     `json:"id"`
	RoundID         string     `json:"roundId"`
	ScheduledAt     *time.Time `json:"scheduledAt,omitempty"`
	Location        string     `json:"location,omitempty"`
	AnnouncedAt     *time.Time `json:"announcedAt,omitempty"`
	AnnouncementSeq int        `json:"announcementSeq,omitempty"`
	Wind            *float64   `json:"wind,omitempty"`
}

// EntryDoc is the omx/v1 shape of domain.Entry.
type EntryDoc struct {
	ID              string `json:"id"`
	EventID         string `json:"eventId"`
	AthleteID       string `json:"athleteId,omitempty"`
	RelayTeamID     string `json:"relayTeamId,omitempty"`
	SeedPerformance string `json:"seedPerformance,omitempty"`
	Status          string `json:"status"`
	Source          string `json:"source,omitempty"`
	StartedUp       bool   `json:"startedUp,omitempty"`
	StartedDown     bool   `json:"startedDown,omitempty"`
	FailsStandard   bool   `json:"failsStandard,omitempty"`
}

// AssignmentDoc is the omx/v1 shape of domain.UnitAssignment.
type AssignmentDoc struct {
	ID             string `json:"id"`
	UnitID         string `json:"unitId"`
	EntryID        string `json:"entryId"`
	SeedRank       int    `json:"seedRank,omitempty"`
	Lane           int    `json:"lane,omitempty"`
	Qualification  string `json:"qualification,omitempty"`
	ManualOverride bool   `json:"manualOverride,omitempty"`
}

// ResultDoc is the omx/v1 shape of domain.Result (the settled outcome —
// UC-027 #1's "official results with statuses, wind, categories [via the
// unit->round->event chain], record flags, and announcement timestamps
// [carried on UnitDoc]"). RecordFlags is never omitted: it is present as
// an empty array until TASK-022 populates it, so the wire shape is stable
// across that merge (parallel-work note, OQ-058).
type ResultDoc struct {
	ID           string   `json:"id"`
	UnitID       string   `json:"unitId"`
	AthleteID    string   `json:"athleteId"`
	Lane         int      `json:"lane,omitempty"`
	Mark         string   `json:"mark,omitempty"`
	Timing       string   `json:"timing,omitempty"`
	Wind         *float64 `json:"wind,omitempty"`
	Status       string   `json:"status,omitempty"`
	StatusDetail string   `json:"statusDetail,omitempty"`
	Points       *int     `json:"points,omitempty"`
	Placing      *int     `json:"placing,omitempty"`
	RecordFlags  []string `json:"recordFlags"`
	Source       string   `json:"source,omitempty"`
}

// EncodeOMX renders doc as a pretty-printed omx/v1 JSON document, stamping
// SchemaVersion unconditionally so a caller can never accidentally export
// under the wrong version tag.
func EncodeOMX(doc Document) ([]byte, error) {
	doc.SchemaVersion = SchemaVersionOMXV1
	for i := range doc.Results {
		if doc.Results[i].RecordFlags == nil {
			doc.Results[i].RecordFlags = []string{}
		}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("omx/v1: encode: %w", err)
	}
	return data, nil
}

// DecodeOMX parses and validates an omx/v1 document: the schema version
// must match exactly (a v2 document is a distinct format a v1 reader must
// refuse, not guess at — SYS-144's deprecation policy) and every
// cross-reference within the document must resolve (ValidateOMX).
// Unrecognized JSON properties are ignored, not rejected: the schema's own
// versioning policy allows additive optional fields within v1 without a
// version bump, and a v1 reader must tolerate those from a newer v1.x
// writer.
func DecodeOMX(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("omx/v1: decode: %w", err)
	}
	if doc.SchemaVersion != SchemaVersionOMXV1 {
		return Document{}, fmt.Errorf("omx/v1: unsupported schemaVersion %q (want %q)", doc.SchemaVersion, SchemaVersionOMXV1)
	}
	if err := ValidateOMX(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// ValidateOMXSchema checks raw exported bytes against the published
// omx/v1 JSON Schema (UC-027 #1: "validate against the published
// schema").
func ValidateOMXSchema(data []byte) error {
	return ValidateJSONSchema(omxV1Schema, data)
}

// ValidateOMX checks the document's internal referential integrity: every
// foreign key used by an Entry/UnitAssignment/Result/Round/Unit resolves
// to an entity present elsewhere in the same document, so an import can
// never silently drop or misattribute a reference. This is stricter than
// ValidateOMXSchema (which only checks shape/types) and is always run by
// DecodeOMX.
func ValidateOMX(doc Document) error {
	if doc.SchemaVersion != SchemaVersionOMXV1 {
		return fmt.Errorf("omx/v1: schemaVersion must be %q, got %q", SchemaVersionOMXV1, doc.SchemaVersion)
	}
	if doc.Meet.ID == "" || doc.Meet.Name == "" {
		return errors.New("omx/v1: meet id and name are required")
	}

	clubIDs := map[string]bool{}
	for _, c := range doc.Clubs {
		if c.ID == "" || c.Name == "" {
			return errors.New("omx/v1: club id and name are required")
		}
		clubIDs[c.ID] = true
	}

	athleteIDs := map[string]bool{}
	for _, a := range doc.Athletes {
		if a.ID == "" || a.LastName == "" {
			return errors.New("omx/v1: athlete id and last name are required")
		}
		if a.Sex != "M" && a.Sex != "W" {
			return fmt.Errorf("omx/v1: athlete %s: invalid sex %q", a.ID, a.Sex)
		}
		athleteIDs[a.ID] = true
	}

	relayTeamIDs := map[string]bool{}
	for _, t := range doc.RelayTeams {
		if t.ID == "" || t.ClubID == "" || len(t.Composition) == 0 {
			return fmt.Errorf("omx/v1: relay team %s: id, club and composition are required", t.ID)
		}
		if !clubIDs[t.ClubID] {
			return fmt.Errorf("omx/v1: relay team %s: unknown club %s", t.ID, t.ClubID)
		}
		relayTeamIDs[t.ID] = true
	}

	eventIDs := map[string]bool{}
	for _, e := range doc.Events {
		if e.ID == "" || e.DisciplineCode == "" || len(e.CategoryCodes) == 0 {
			return fmt.Errorf("omx/v1: event %s: discipline code and at least one category are required", e.ID)
		}
		eventIDs[e.ID] = true
	}

	roundIDs := map[string]bool{}
	for _, r := range doc.Rounds {
		if r.ID == "" {
			return errors.New("omx/v1: round id is required")
		}
		if !eventIDs[r.EventID] {
			return fmt.Errorf("omx/v1: round %s: unknown event %s", r.ID, r.EventID)
		}
		roundIDs[r.ID] = true
	}

	unitIDs := map[string]bool{}
	for _, u := range doc.Units {
		if u.ID == "" {
			return errors.New("omx/v1: unit id is required")
		}
		if !roundIDs[u.RoundID] {
			return fmt.Errorf("omx/v1: unit %s: unknown round %s", u.ID, u.RoundID)
		}
		unitIDs[u.ID] = true
	}

	entryIDs := map[string]bool{}
	for _, e := range doc.Entries {
		if e.ID == "" {
			return errors.New("omx/v1: entry id is required")
		}
		if !eventIDs[e.EventID] {
			return fmt.Errorf("omx/v1: entry %s: unknown event %s", e.ID, e.EventID)
		}
		if (e.AthleteID == "") == (e.RelayTeamID == "") {
			return fmt.Errorf("omx/v1: entry %s: exactly one of athleteId/relayTeamId is required", e.ID)
		}
		if e.AthleteID != "" && !athleteIDs[e.AthleteID] {
			return fmt.Errorf("omx/v1: entry %s: unknown athlete %s", e.ID, e.AthleteID)
		}
		if e.RelayTeamID != "" && !relayTeamIDs[e.RelayTeamID] {
			return fmt.Errorf("omx/v1: entry %s: unknown relay team %s", e.ID, e.RelayTeamID)
		}
		entryIDs[e.ID] = true
	}

	for _, a := range doc.Assignments {
		if !unitIDs[a.UnitID] {
			return fmt.Errorf("omx/v1: unit assignment %s: unknown unit %s", a.ID, a.UnitID)
		}
		if !entryIDs[a.EntryID] {
			return fmt.Errorf("omx/v1: unit assignment %s: unknown entry %s", a.ID, a.EntryID)
		}
	}

	for _, r := range doc.Results {
		if r.ID == "" {
			return errors.New("omx/v1: result id is required")
		}
		if !unitIDs[r.UnitID] {
			return fmt.Errorf("omx/v1: result %s: unknown unit %s", r.ID, r.UnitID)
		}
		if !athleteIDs[r.AthleteID] {
			return fmt.Errorf("omx/v1: result %s: unknown athlete %s", r.ID, r.AthleteID)
		}
	}
	return nil
}

// omxResultsCSVHeader is the documented column order of the SYS-073 flat
// CSV export: one row per official result, self-contained (event/category/
// round/heat identity and the athlete's name travel with every row, the
// same "GenericRow" philosophy csv.go's FinishLynx-fallback CSV uses) —
// the spreadsheet-consumer half of "CSV and a self-describing structured
// format" (UC-027 #1). It derives from the same Document the JSON export
// uses (ADR-005 §2), so the two artifacts can never disagree about what
// happened.
var omxResultsCSVHeader = []string{
	"event_id", "discipline_code", "category_codes", "round_kind", "unit_id",
	"athlete_id", "last_name", "first_name", "club",
	"lane", "mark", "timing", "wind", "status", "status_detail",
	"points", "placing", "record_flags", "announced_at",
}

// EncodeOMXResultsCSV renders doc's results as the documented flat CSV
// (SYS-073's spreadsheet-consumer artifact), one row per result, denormalized
// with its event/round/unit/athlete/club context so every row is
// self-describing.
func EncodeOMXResultsCSV(doc Document) []byte {
	events := make(map[string]EventDoc, len(doc.Events))
	for _, e := range doc.Events {
		events[e.ID] = e
	}
	rounds := make(map[string]RoundDoc, len(doc.Rounds))
	for _, r := range doc.Rounds {
		rounds[r.ID] = r
	}
	units := make(map[string]UnitDoc, len(doc.Units))
	for _, u := range doc.Units {
		units[u.ID] = u
	}
	athletes := make(map[string]AthleteDoc, len(doc.Athletes))
	for _, a := range doc.Athletes {
		athletes[a.ID] = a
	}
	clubNames := make(map[string]string, len(doc.Clubs))
	for _, c := range doc.Clubs {
		clubNames[c.ID] = c.Name
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(omxResultsCSVHeader)
	for _, r := range doc.Results {
		u := units[r.UnitID]
		rd := rounds[u.RoundID]
		ev := events[rd.EventID]
		ath := athletes[r.AthleteID]
		club := ""
		if len(ath.ClubIDs) > 0 {
			club = clubNames[ath.ClubIDs[0]]
		}
		announced := ""
		if u.AnnouncedAt != nil {
			announced = u.AnnouncedAt.UTC().Format(time.RFC3339)
		}
		wind := ""
		if r.Wind != nil {
			wind = strconv.FormatFloat(*r.Wind, 'f', -1, 64)
		}
		points, placing := "", ""
		if r.Points != nil {
			points = strconv.Itoa(*r.Points)
		}
		if r.Placing != nil {
			placing = strconv.Itoa(*r.Placing)
		}
		flags, _ := json.Marshal(orEmptyStrings(r.RecordFlags))
		_ = w.Write([]string{
			ev.ID, ev.DisciplineCode, joinCodes(ev.CategoryCodes), rd.Kind, u.ID,
			r.AthleteID, ath.LastName, ath.FirstName, club,
			laneOrEmpty(r.Lane), r.Mark, r.Timing, wind, r.Status, r.StatusDetail,
			points, placing, string(flags), announced,
		})
	}
	w.Flush()
	return buf.Bytes()
}

func joinCodes(codes []string) string {
	out := ""
	for i, c := range codes {
		if i > 0 {
			out += "|"
		}
		out += c
	}
	return out
}

func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
