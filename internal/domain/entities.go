// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"errors"
	"fmt"
	"time"
)

// Sex is the athlete/category sex marker used throughout the domain (D4.2
// vocabulary: Men/Women, "M"/"W"). It intentionally mirrors the federation
// vocabulary rather than introducing a separate biological/administrative
// distinction — out of scope for SyRS §2.
type Sex string

const (
	SexMale   Sex = "M"
	SexFemale Sex = "W"
)

// ExternalIDs is a namespaced set of foreign identifiers (ADR-005 §6):
// federation identifiers are typed, additive attributes, never overloaded
// onto an entity's own (ULID) identity. New federations add a namespace
// key; no schema change is required.
type ExternalIDs map[string]string

// Get returns the identifier stored under namespace ns, if any.
func (e ExternalIDs) Get(ns string) (string, bool) {
	v, ok := e[ns]
	return v, ok
}

// Set stores id under namespace ns, initializing the map if needed.
func (e *ExternalIDs) Set(ns, id string) {
	if *e == nil {
		*e = ExternalIDs{}
	}
	(*e)[ns] = id
}

// Well-known external-ID namespaces (ADR-005 §6). The set is additive: a new
// federation is a new namespace string, never a change to this type or to
// omx/v1's serialization shape.
const (
	// NamespaceSwissAthleticsLicence is an athlete's Swiss Athletics licence
	// number — WO 2026 §5.3b: the only join key into the Bestenliste.
	NamespaceSwissAthleticsLicence = "swiss-athletics:licence"
	// NamespaceWorldAthleticsID is an athlete's World Athletics ID.
	NamespaceWorldAthleticsID = "world-athletics:athlete-id"
	// NamespaceSwissAthleticsMeetID is a meet's Swiss Athletics
	// Wettkampfverwaltung identifier.
	NamespaceSwissAthleticsMeetID = "swiss-athletics:wettkampfverwaltung-id"
	// NamespaceWAGlobalCalendarID is a meet's World Athletics Global
	// Calendar identifier (TAF3 v7010+).
	NamespaceWAGlobalCalendarID = "world-athletics:global-calendar-id"
	// NamespaceSwissAthleticsClubCode is a club's Swiss Athletics club code.
	NamespaceSwissAthleticsClubCode = "swiss-athletics:club-code"
)

// Athlete is the SyRS §2 Athlete entity: person data, birth date/year, sex,
// nationality, club affiliation(s), licence number(s) (as ExternalIDs), para
// sport class(es), and publication-consent flags (SYS-010).
type Athlete struct {
	ID          string
	FirstName   string
	LastName    string
	BirthDate   *time.Time // full date, if known (SYS-010)
	BirthYear   int        // always required; derived from BirthDate when present
	Sex         Sex
	Nationality string
	ClubIDs     []string
	ExternalIDs ExternalIDs
	ParaClasses []string // e.g. "T38" (Later, DEC-007)
	Consent     PublicationConsent
}

// PublicationConsent records whether an athlete's results/name may appear on
// public surfaces (SYS-100/103).
type PublicationConsent struct {
	PublicResultsAllowed bool
	RecordedAt           time.Time
}

// Validate checks the invariants SYS-010 requires: birth year is always
// present (full date optional), and sex is one of the two domain values.
func (a *Athlete) Validate() error {
	if a.ID == "" {
		return errors.New("athlete: id is required")
	}
	if a.LastName == "" {
		return errors.New("athlete: last name is required")
	}
	if a.BirthDate != nil {
		a.BirthYear = a.BirthDate.Year()
	}
	if a.BirthYear == 0 {
		return errors.New("athlete: birth year is required (SYS-010)")
	}
	if a.Sex != SexMale && a.Sex != SexFemale {
		return fmt.Errorf("athlete: invalid sex %q", a.Sex)
	}
	return nil
}

// Club is the SyRS §2 Club/Team entity: name, federation code (as
// ExternalIDs). Relay teams are compositions of athletes, not a Club field.
type Club struct {
	ID          string
	Name        string
	ExternalIDs ExternalIDs
}

// MeetStatus is the Meet lifecycle (SyRS §2, SYS-001).
type MeetStatus string

const (
	MeetDraft     MeetStatus = "draft"
	MeetPublished MeetStatus = "published"
	MeetLive      MeetStatus = "live"
	MeetClosed    MeetStatus = "closed"
	MeetArchived  MeetStatus = "archived"
)

// MeetTier is the Swiss Athletics sanctioning tier (A/B/C-Meeting, D1.2) or a
// custom tier for unsanctioned meets.
type MeetTier string

// Meet is the SyRS §2 Meet entity: name, venue, date range, sessions,
// organizer, tier, status (SYS-001/004/006). HomologationRef is the venue's
// homologation reference recorded for the sanctioning summary (SYS-006).
// CategorySchemeID names the category scheme this meet resolves categories
// against (SYS-005, UC-002 #1: "the organizer selects the built-in Swiss
// Athletics scheme").
type Meet struct {
	ID               string
	Name             string
	Venue            string
	HomologationRef  string
	StartDate        time.Time
	EndDate          time.Time
	Organizer        string
	Tier             MeetTier
	Status           MeetStatus
	CategorySchemeID string
	ExternalIDs      ExternalIDs
}

// Validate checks the minimal Meet invariants (SYS-001).
func (m *Meet) Validate() error {
	if m.ID == "" {
		return errors.New("meet: id is required")
	}
	if m.Name == "" {
		return errors.New("meet: name is required")
	}
	if m.EndDate.Before(m.StartDate) {
		return errors.New("meet: end date before start date")
	}
	return nil
}

// Session is one competition session within a Meet's timetable: a labeled
// block on one competition day (SYS-001: "one or more competition days,
// sessions per day").
type Session struct {
	ID     string
	MeetID string
	Day    time.Time // the competition day this session belongs to (date only)
	Label  string    // e.g. "Vormittag", "Session 1"
}

// Validate checks the minimal Session invariants (SYS-001).
func (s *Session) Validate() error {
	if s.ID == "" {
		return errors.New("session: id is required")
	}
	if s.MeetID == "" {
		return errors.New("session: meet id is required")
	}
	if s.Day.IsZero() {
		return errors.New("session: day is required")
	}
	return nil
}

// EventStatus is the Event lifecycle within a Meet (SyRS §2).
type EventStatus string

const (
	EventDraft     EventStatus = "draft"
	EventPublished EventStatus = "published"
	EventClosed    EventStatus = "closed"
)

// Event is the SyRS §2 Event entity: meet × discipline × category (or
// combined categories), entry conditions, status (SYS-002/003).
type Event struct {
	ID             string
	MeetID         string
	DisciplineCode string
	CategoryCodes  []string   // >1 for combined-category events (SYS-002)
	EntryStandard  string     // seed-performance threshold, if configured (SYS-015)
	EntryDeadline  *time.Time // entry condition per event (SYS-002, UC-001 #3)
	Status         EventStatus
}

// RoundKind is the round position within an Event's progression (D2.1).
type RoundKind string

const (
	RoundQualification RoundKind = "qualification"
	RoundSemifinal     RoundKind = "semifinal"
	RoundFinal         RoundKind = "final"
)

// Round is the SyRS §2 Round entity: qualification/semi/final, grouping Units
// (heats/flights/groups). Seeding/progression logic is out of scope for this
// package (TASK-018); Round/Unit here are data shapes only.
type Round struct {
	ID      string
	EventID string
	Kind    RoundKind
}

// Unit is a heat/flight/group within a Round: scheduled time and venue
// location (SyRS §2).
type Unit struct {
	ID          string
	RoundID     string
	ScheduledAt time.Time
	Location    string
}

// EntryStatus is the Entry lifecycle (SyRS §2, SYS-011/016).
type EntryStatus string

const (
	EntryEntered   EntryStatus = "entered"
	EntryConfirmed EntryStatus = "confirmed"
	EntryScratched EntryStatus = "scratched"
	EntryDNS       EntryStatus = "dns"
)

// EntrySource records how an Entry was created (SYS-011/013).
type EntrySource string

const (
	EntrySourceOnline EntrySource = "online"
	EntrySourceImport EntrySource = "import"
	EntrySourceManual EntrySource = "manual"
)

// Entry is the SyRS §2 Entry entity: athlete/relay team × event, seed
// performance, status, source, fees. StartedUp/StartedDown record the
// category-scheme decision that admitted this entry (UC-002 #3).
type Entry struct {
	ID              string
	EventID         string
	AthleteID       string // empty for relay-team entries
	RelayTeamID     string // empty for individual entries
	SeedPerformance string
	Status          EntryStatus
	Source          EntrySource
	StartedUp       bool
	StartedDown     bool
	FailsStandard   bool // seed does not meet the event's entry standard (SYS-015)
}

// QualificationStatus is the standard result/qualification code vocabulary
// (Competition Rule 25, D5.2): DNS, DNF, NM, DQ, O, X, etc.
type QualificationStatus string

const (
	StatusNone QualificationStatus = ""
	StatusDNS  QualificationStatus = "DNS"
	StatusDNF  QualificationStatus = "DNF"
	StatusNM   QualificationStatus = "NM"
	StatusNH   QualificationStatus = "NH"
	StatusDQ   QualificationStatus = "DQ"
	StatusO    QualificationStatus = "O"
	StatusX    QualificationStatus = "X"
	StatusPass QualificationStatus = "-"
	StatusR    QualificationStatus = "r"
	StatusQ    QualificationStatus = "Q"
	StatusQt   QualificationStatus = "q"
)

// Result is the SyRS §2 Participation/Result entity: per-unit lane/position,
// mark, wind, status, points, placing, record flags. Attempt-sequence detail
// (per-trial X/O/– for field events) is owned by the capture-UI tasks
// (TASK-008/021); this is the settled-result shape.
type Result struct {
	ID          string
	UnitID      string
	AthleteID   string
	Lane        int
	Mark        string // encoded per discipline unit (time/distance/height/points)
	Wind        *float64
	Status      QualificationStatus
	Points      *int
	Placing     *int
	RecordFlags []string // e.g. "PB", "SB", "NR" (D6.1)
}

// RecordType is the WA/Swiss record-abbreviation vocabulary (D6.1).
type RecordType string

const (
	RecordWorld    RecordType = "WR"
	RecordArea     RecordType = "AR"
	RecordNational RecordType = "NR"
	RecordMeeting  RecordType = "meeting"
)

// RecordReference is the SyRS §2 Record/Best reference entity: type, scope,
// category, discipline, mark, holder, date (D6.1/D6.2).
type RecordReference struct {
	ID              string
	Type            RecordType
	Scope           string
	CategoryCode    string
	DisciplineCode  string
	Mark            string
	HolderAthleteID string
	Date            time.Time
}

// DocumentKind enumerates official-document kinds (SyRS §2).
type DocumentKind string

const (
	DocumentStartList  DocumentKind = "start-list"
	DocumentResultList DocumentKind = "result-list"
	DocumentRecordProt DocumentKind = "record-protocol"
)

// OfficialDocument is the SyRS §2 Official document entity: start lists,
// result lists (versioned, with announcement timestamp), record protocols
// (SYS-004/047).
type OfficialDocument struct {
	ID          string
	MeetID      string
	Kind        DocumentKind
	Version     int
	AnnouncedAt time.Time
}

// Role is a per-meet role assignment (SYS-090).
type Role string

const (
	RoleOrganizer     Role = "organizer"
	RoleOffice        Role = "office"
	RoleFieldOfficial Role = "field-official"
)

// UserRoleAssignment is the SyRS §2 User/Role entity: account, role
// assignment per meet.
type UserRoleAssignment struct {
	ID     string
	UserID string
	MeetID string
	Role   Role
}

// AuditEvent is the SyRS §2 Audit event entity as seen by the domain/app
// layer: actor, timestamp, entity, before/after, reason (SYS-046). The
// durable, trigger-enforced append-only log itself lives in internal/store
// (TASK-003); this is the shape app-layer services populate before writing.
type AuditEvent struct {
	ID         string
	Actor      string
	Timestamp  time.Time
	EntityType string
	EntityID   string
	Before     string
	After      string
	Reason     string
}
