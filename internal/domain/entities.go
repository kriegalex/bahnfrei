// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"errors"
	"fmt"
	"regexp"
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

// licenceNoPattern is a deliberately permissive well-formedness check for a
// federation licence number entered by hand (DEC-023, OQ-033/OQ-104,
// TASK-039): Unicode letters/digits, spaces, dots, hyphens and slashes,
// 1–40 characters. The CSV/Alabus import path (internal/app/import.go's
// resolveImportAthlete) imposes NO format at all — there, a licence number
// is just an opaque external-ID join key (ADR-005 §6) trusted as given by
// the import file, because the real Swiss Athletics/Alabus licence-number
// format is unverified (OQ-030). ValidLicenceNo exists only to catch
// obviously mistyped/pasted input on the online-entry form (stray
// newlines/control characters, absurd length) before it becomes a stored
// join key that could never match a real licence — it does not invent or
// enforce a specific national licence-number scheme.
var licenceNoPattern = regexp.MustCompile(`^[\p{L}\p{N} ./-]{1,40}$`)

// ValidLicenceNo reports whether s (already trimmed) is well-formed enough
// to accept as a licence-number join key. An empty string is NOT valid by
// this function's contract — the field is optional, so callers check
// emptiness themselves before calling this (mirrors domain.EvaluateEntryStandard's
// division of labour with ErrSeedPerformanceRequired).
func ValidLicenceNo(s string) bool {
	return licenceNoPattern.MatchString(s)
}

// Athlete is the SyRS §2 Athlete entity: person data, birth date/year, sex,
// nationality, club affiliation(s), licence number(s) (as ExternalIDs), para
// sport class(es), and publication-consent flags (SYS-010).
// Anonymized/AnonymizedAt record a completed SYS-101 erasure request
// (internal/domain/privacy.go AnonymizePersonalData) — see that file for
// which fields an erasure clears and which it deliberately preserves to
// keep official results valid.
type Athlete struct {
	ID           string
	FirstName    string
	LastName     string
	BirthDate    *time.Time // full date, if known (SYS-010)
	BirthYear    int        // always required; derived from BirthDate when present
	Sex          Sex
	Nationality  string
	ClubIDs      []string
	ExternalIDs  ExternalIDs
	ParaClasses  []string // e.g. "T38" (Later, DEC-007)
	Consent      PublicationConsent
	Anonymized   bool
	AnonymizedAt *time.Time
}

// PublicationConsent records an athlete's SYS-103 publication-consent
// flags. See internal/domain/privacy.go for the rationale behind the
// opt-out (ResultsPublicationWithdrawn) vs. opt-in (PhotoConsentGiven,
// ExtendedDataConsentGiven) shape and how these flags are enforced.
type PublicationConsent struct {
	// ResultsPublicationWithdrawn suppresses this athlete's name/club from
	// public surfaces and publication-intended exports when true (SYS-103,
	// UC-023 #2) — an opt-out flag: zero value (false) means "not
	// withdrawn", i.e. publicly listed, matching this system's pre-consent-
	// tracking behaviour and standard federation practice of publishing
	// competition results as the sporting record. Applies uniformly to any
	// athlete once recorded, not only minors — see OQ-041.
	ResultsPublicationWithdrawn bool
	// PhotoConsentGiven is an opt-in flag (zero value = not given) required
	// before any photo of this athlete could be published. No photo
	// publication surface exists yet in this system; the flag is recorded
	// now so consent can be captured at entry time (UC-023) ahead of that
	// feature — see OQ-042.
	PhotoConsentGiven bool
	// ExtendedDataConsentGiven is an opt-in flag (zero value = not given)
	// reserved for any future public field beyond the SYS-100 minimal set.
	// Not currently enforced anywhere: SYS-100 already caps what public
	// surfaces show regardless of this flag — see OQ-042.
	ExtendedDataConsentGiven bool
	// RecordedAt/RecordedBy are the audit context of the last consent
	// change (who, when) — RecordedBy is purged by the SYS-102 retention
	// job once it is no longer operationally needed (internal/app/privacy.go
	// PurgeExpired); RecordedAt and the flags themselves are retained since
	// resetting them could silently re-publish a withdrawn athlete.
	RecordedAt time.Time
	RecordedBy string
}

// MinorAgeThreshold is the age (in full years) below which an athlete is
// treated as a minor for SYS-103 consent purposes (Swiss age of majority,
// ZGB Art. 14) — see OQ-040 for confirmation that this is the correct
// threshold for publication-consent purposes specifically (as opposed to
// general legal capacity).
const MinorAgeThreshold = 18

// IsMinor reports whether the athlete is under MinorAgeThreshold as of the
// given date, using the full birth date when known and otherwise the
// conservative approximation of "born on 31 December of BirthYear" (the
// latest possible birthday for that year, so an athlete is never
// misclassified as an adult when only the birth year is on file).
func (a Athlete) IsMinor(asOf time.Time) bool {
	birth := a.BirthDate
	if birth == nil {
		d := time.Date(a.BirthYear, time.December, 31, 0, 0, 0, 0, time.UTC)
		birth = &d
	}
	age := asOf.Year() - birth.Year()
	// Compare month/day, not YearDay: YearDay is not comparable across a
	// leap/non-leap year pair (the same calendar month/day can fall on a
	// different day-of-year), which would misjudge a birthday by one day
	// around 29 February in some year pairs.
	if asOf.Month() < birth.Month() || (asOf.Month() == birth.Month() && asOf.Day() < birth.Day()) {
		age--
	}
	return age < MinorAgeThreshold
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

// ResultsPositioning is the organizer's per-meet official-results
// positioning choice (SYS-076): whether a federation channel is the
// official source (this system's public pages must then carry an
// "unofficial results" label with a reference to that source) or whether
// this system itself is the primary/official publication (no label).
type ResultsPositioning string

const (
	// ResultsPositioningFederationOfficial marks a federation channel (or
	// other external system) as the official results source — the default
	// for sanctioned meets (SYS-076). Every public results page/export
	// then carries the "unofficial results" label plus a reference to
	// OfficialSourceName/OfficialSourceURL.
	ResultsPositioningFederationOfficial ResultsPositioning = "federation_official"
	// ResultsPositioningPrimary marks this system as the primary/official
	// publication (e.g. unsanctioned meets) — no unofficial-results label
	// renders.
	ResultsPositioningPrimary ResultsPositioning = "primary"
)

// Valid reports whether p is one of the enumerated positioning values.
func (p ResultsPositioning) Valid() bool {
	return p == ResultsPositioningFederationOfficial || p == ResultsPositioningPrimary
}

// Meet is the SyRS §2 Meet entity: name, venue, date range, sessions,
// organizer, tier, status (SYS-001/004/006). HomologationRef is the venue's
// homologation reference recorded for the sanctioning summary (SYS-006).
// CategorySchemeID names the category scheme this meet resolves categories
// against (SYS-005, UC-002 #1: "the organizer selects the built-in Swiss
// Athletics scheme").
// TemplateID/ScoringTableID record which competition template created the
// meet and which points table scores it (SYS-053) — empty for meets
// composed by hand.
// ResultsPositioning/OfficialSourceName/OfficialSourceURL record the
// organizer's SYS-076 official-results positioning choice for this meet's
// public pages and exports.
type Meet struct {
	ID                 string
	Name               string
	Venue              string
	HomologationRef    string
	StartDate          time.Time
	EndDate            time.Time
	Organizer          string
	Tier               MeetTier
	Status             MeetStatus
	CategorySchemeID   string
	TemplateID         string
	ScoringTableID     string
	ResultsPositioning ResultsPositioning
	OfficialSourceName string
	OfficialSourceURL  string
	ExternalIDs        ExternalIDs
	// EntryFeeCents/RelayFeeCents are the SYS-017 configurable fee schedule
	// (Rappen/cents, CHF): a flat per-individual-entry fee and a flat
	// per-relay-team-entry fee, summed per club/athlete for the fee summary
	// (UC-006 #3). Zero means "no fee configured" — never an error, since
	// many club meets are entry-free.
	EntryFeeCents int64
	RelayFeeCents int64
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
	if m.ResultsPositioning != "" && !m.ResultsPositioning.Valid() {
		return fmt.Errorf("meet: invalid results positioning %q (SYS-076)", m.ResultsPositioning)
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
	// EntryLimit caps the number of active (non-scratched) entries this
	// event accepts (SYS-015 "entry limits"); zero means unlimited.
	EntryLimit int
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
	// SubmittedBy is the account ID that created this entry (empty for
	// entries with no acting account, e.g. a future bulk import): the
	// submitter's "my entries" view (UC-003 #1/#2) and the audit trail both
	// key off this.
	SubmittedBy string
}

// Validate checks the minimal Entry invariants (SYS-011/012): identity, the
// event it targets, and exactly one of athlete/relay-team composition.
func (e *Entry) Validate() error {
	if e.ID == "" {
		return errors.New("entry: id is required")
	}
	if e.EventID == "" {
		return errors.New("entry: event id is required")
	}
	if e.AthleteID == "" && e.RelayTeamID == "" {
		return errors.New("entry: athlete id or relay team id is required")
	}
	if e.AthleteID != "" && e.RelayTeamID != "" {
		return errors.New("entry: athlete id and relay team id are mutually exclusive")
	}
	return nil
}

// RelayTeam is the SyRS §2 relay-team composition (SYS-012): a club's
// ordered leg assignment for one relay entry, plus reserves. Composition may
// be revised (UC-003 #4: "permits changes until the configured deadline")
// by replacing Composition/Reserves under optimistic concurrency.
type RelayTeam struct {
	ID     string
	ClubID string
	// Composition is the ordered athlete IDs running each leg, in order.
	Composition []string
	// Reserves lists reserve athlete IDs, not assigned to a leg.
	Reserves []string
}

// Validate checks the minimal RelayTeam invariants (SYS-012): identity, club,
// and a non-empty, duplicate-free leg composition.
func (t *RelayTeam) Validate() error {
	if t.ID == "" {
		return errors.New("relay team: id is required")
	}
	if t.ClubID == "" {
		return errors.New("relay team: club id is required")
	}
	if len(t.Composition) == 0 {
		return errors.New("relay team: at least one leg is required")
	}
	seen := make(map[string]bool, len(t.Composition)+len(t.Reserves))
	for _, id := range t.Composition {
		if id == "" {
			return errors.New("relay team: leg athlete id is required")
		}
		if seen[id] {
			return fmt.Errorf("relay team: athlete %q assigned to more than one leg", id)
		}
		seen[id] = true
	}
	for _, id := range t.Reserves {
		if id == "" {
			return errors.New("relay team: reserve athlete id is required")
		}
		if seen[id] {
			return fmt.Errorf("relay team: athlete %q is both a leg and a reserve", id)
		}
		seen[id] = true
	}
	return nil
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
	// StatusQR/StatusQJ/StatusQD are the manual-advancement codes (D2.4,
	// D5.2, SYS-029): advanced by Referee, Jury of Appeal, or draw
	// respectively — set by round progression (TASK-018), never by the
	// operator-settable capture-status vocabulary (SYS-045,
	// internal/domain/track.go's captureStatuses).
	StatusQR QualificationStatus = "qR"
	StatusQJ QualificationStatus = "qJ"
	StatusQD QualificationStatus = "qD"
)

// UnitAssignment is one entry's placement within a Round's Unit (heat,
// flight or lane group): its seed rank within the unit, drawn lane (0 = no
// lane assigned — a by-lot or non-laned event), and qualification code once
// round progression runs (SyRS §2; SYS-026–030, D2.2–D2.4). ManualOverride
// marks a heat/lane the operator hand-edited (SYS-028): a later regeneration
// SHALL leave it untouched unless explicitly released.
type UnitAssignment struct {
	ID             string
	UnitID         string
	EntryID        string
	SeedRank       int
	Lane           int
	Qualification  QualificationStatus
	ManualOverride bool
}

// Validate checks the minimal UnitAssignment invariants (SYS-026): identity,
// the unit and entry it links, and — when set — a qualification code drawn
// from the D2.4 advancement vocabulary.
func (a *UnitAssignment) Validate() error {
	if a.ID == "" {
		return errors.New("unit assignment: id is required")
	}
	if a.UnitID == "" {
		return errors.New("unit assignment: unit id is required")
	}
	if a.EntryID == "" {
		return errors.New("unit assignment: entry id is required")
	}
	switch a.Qualification {
	case StatusNone, StatusQ, StatusQt, StatusQR, StatusQJ, StatusQD:
	default:
		return fmt.Errorf("unit assignment: invalid qualification code %q (SYS-029/D5.2)", a.Qualification)
	}
	return nil
}

// Result is the SyRS §2 Participation/Result entity: per-unit lane/position,
// mark, wind, status, points, placing, record flags. Attempt-sequence detail
// (per-trial X/O/– for field events) is owned by the capture-UI tasks
// (TASK-008/021); this is the settled-result shape.
type Result struct {
	ID        string
	UnitID    string
	AthleteID string
	Lane      int
	Mark      string // encoded per discipline unit (time/distance/height/points)
	Wind      *float64
	Status    QualificationStatus
	// StatusDetail qualifies Status where CR 25 demands it — a DQ's rule
	// reference ("TR16.8"), rendered as "DQ (TR16.8)" (SYS-045).
	StatusDetail string
	Points       *int
	Placing      *int
	RecordFlags  []string // e.g. "PB", "SB", "NR" (D6.1)
}

// RecordType is the WA/Swiss record-abbreviation vocabulary (D6.1): the
// kind of reference-list row a RecordReference is. PB/SB are not reference
// rows (no file loads them) — they are computed per athlete from in-system
// history (internal/domain/record.go) and carry their own flag codes
// directly (FlagPersonalBest/FlagSeasonBest), never a RecordType value.
type RecordType string

const (
	RecordWorld    RecordType = "WR"
	RecordArea     RecordType = "AR"
	RecordNational RecordType = "NR"
	RecordMeeting  RecordType = "meeting"
)

// FlagCode returns the D6.1 result-list abbreviation this reference type
// flags a bettering performance with — "MR" for a loaded meeting-record row
// (RecordMeeting's own value, "meeting", is the file vocabulary, not the
// printed flag), the type's own value otherwise (WR/AR/NR already are their
// flag codes).
func (t RecordType) FlagCode() string {
	if t == RecordMeeting {
		return "MR"
	}
	return string(t)
}

// RecordReference is the SyRS §2 Record/Best reference entity: type, scope,
// category, discipline, mark, holder, date (D6.1/D6.2). Loaded from a
// RecordList data file (internal/domain/record.go, ADR-005 §4 "rule-shaped
// data is data, not code") — never hand-built in application code.
type RecordReference struct {
	ID   string     `json:"id"`
	Type RecordType `json:"type"`
	// Scope is the reference's jurisdiction label for display (e.g. "CH",
	// "Europe", "World") — free text, not matched against anything; a
	// RecordMeeting entry's actual meet scoping is MeetID below, not Scope.
	Scope          string `json:"scope"`
	CategoryCode   string `json:"categoryCode"`
	DisciplineCode string `json:"disciplineCode"`
	Mark           string `json:"mark"`
	// MeetID scopes a RecordMeeting entry to the one meet it is a record
	// for (SYS-049 "meeting records"): only that meet's captured results are
	// evaluated against it. Empty for WR/AR/NR entries, which apply across
	// every meet that loads this list (D6.1's federation-wide reference
	// lists).
	MeetID          string    `json:"meetId,omitempty"`
	HolderAthleteID string    `json:"holderAthleteId,omitempty"`
	Date            time.Time `json:"date,omitempty"`
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
