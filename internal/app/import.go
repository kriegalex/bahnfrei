// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Entry import (TASK-017, UC-004, SYS-013): CSV import against a
// configurable mapping profile, preview-before-commit, per-row error
// reporting, and idempotent re-import. Builds on TASK-016's entry/athlete/
// club helpers (internal/app/entry.go) — a new individual entry created here
// is the same domain.Entry an online submission creates, so it flows
// through the same eligibility (SYS-014) and downstream (bib/fee) machinery
// with no special-casing. ---

// ErrUnknownImportProfile means profileID does not name a wired mapping
// profile.
var ErrUnknownImportProfile = errors.New("unknown import mapping profile")

// ImportRowStatus is one CSV row's outcome (SYS-013: "report per-row
// acceptance/rejection with reasons").
type ImportRowStatus string

const (
	// ImportRowAccepted means the row created a new entry (and, possibly, a
	// new athlete/club).
	ImportRowAccepted ImportRowStatus = "accepted"
	// ImportRowUpdated means the row matched an athlete/entry a prior import
	// (or this same import file, re-run) already created — idempotent
	// re-import (UC-004 #2): no duplicate is created.
	ImportRowUpdated ImportRowStatus = "updated"
	// ImportRowRejected means the row failed structural validation (missing
	// required field, unknown event code, unresolvable category) and was
	// not imported.
	ImportRowRejected ImportRowStatus = "rejected"
)

// ImportRowResult is one CSV row's parse/import outcome for the preview and
// commit report (SYS-013, UC-004 #1/#3).
type ImportRowResult struct {
	// RowNumber is 1-based, counting only data rows (the header is not
	// row 1) — matches how a spreadsheet user reads "row N".
	RowNumber int
	Status    ImportRowStatus
	// Reason explains a Rejected row; empty for Accepted/Updated.
	Reason string

	FirstName, LastName, Club, EventCode, CategoryCode, SeedPerformance string
	BirthYear                                                           int
	Sex                                                                 domain.Sex

	// EntryID/AthleteID are set for Accepted/Updated rows (empty for
	// Rejected, and also empty in preview mode — nothing is persisted yet).
	EntryID, AthleteID string
	// Eligibility is the SYS-014 evaluation for this row's entry — computed
	// in both preview and commit (preview: read-only, nothing persisted;
	// commit: also stored, see entry_eligibility). Zero value for Rejected
	// rows.
	Eligibility domain.EligibilityResult
}

// ImportReport summarizes one CSV import run (preview or commit) against one
// mapping profile (SYS-013).
type ImportReport struct {
	ProfileID string
	Accepted  int
	Updated   int
	Rejected  int
	Rows      []ImportRowResult
	// Committed is false for PreviewCSVImport (nothing persisted) and true
	// for CommitCSVImport.
	Committed bool
}

// PreviewCSVImport parses and validates srcCSV against the named mapping
// profile without persisting anything (UC-004: "preview-before-commit").
// Every row's structural validity and (where resolvable) eligibility
// outcome is reported exactly as CommitCSVImport would apply it.
func (s *ResultsService) PreviewCSVImport(ctx context.Context, actor Session, meetID, profileID string, srcCSV io.Reader) (ImportReport, error) {
	return s.runCSVImport(ctx, actor, meetID, profileID, srcCSV, false)
}

// CommitCSVImport parses, validates and persists srcCSV against the named
// mapping profile (SYS-013): accepted/updated rows create or reuse athletes,
// clubs and entries, each new individual entry is evaluated for eligibility
// (SYS-014) exactly as an online submission is, and rejected rows are
// reported with their row number and reason. Re-running the same file is
// idempotent (UC-004 #2): matched athletes/entries are reused, never
// duplicated.
func (s *ResultsService) CommitCSVImport(ctx context.Context, actor Session, meetID, profileID string, srcCSV io.Reader) (ImportReport, error) {
	return s.runCSVImport(ctx, actor, meetID, profileID, srcCSV, true)
}

func (s *ResultsService) runCSVImport(ctx context.Context, actor Session, meetID, profileID string, srcCSV io.Reader, commit bool) (ImportReport, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return ImportReport{}, err
	}
	profile, ok := s.importProfiles[profileID]
	if !ok {
		return ImportReport{}, ErrUnknownImportProfile
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return ImportReport{}, err
	}
	scheme := s.schemes[meet.CategorySchemeID]
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return ImportReport{}, err
	}

	rows, header, err := readCSVRows(srcCSV)
	if err != nil {
		return ImportReport{}, fmt.Errorf("read CSV: %w", err)
	}
	colFor, missing := columnIndex(header, profile)
	if len(missing) > 0 {
		return ImportReport{}, fmt.Errorf("CSV header is missing column(s) required by profile %q: %s", profileID, strings.Join(missing, ", "))
	}

	report := ImportReport{ProfileID: profileID, Committed: commit}

	var tx store.DBTX = s.db
	var sqlTx *sql.Tx
	if commit {
		t, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return ImportReport{}, fmt.Errorf("commit csv import: %w", err)
		}
		defer func() { _ = t.Rollback() }()
		tx = t
		sqlTx = t
	}

	for i, raw := range rows {
		row := ImportRowResult{RowNumber: i + 1}
		field := func(f domain.ImportField) string {
			idx, ok := colFor[f]
			if !ok || idx >= len(raw) {
				return ""
			}
			return strings.TrimSpace(raw[idx])
		}
		row.FirstName, row.LastName = field(domain.ImportFieldFirstName), field(domain.ImportFieldLastName)
		row.Club = field(domain.ImportFieldClub)
		row.EventCode = field(domain.ImportFieldEventCode)
		row.CategoryCode = field(domain.ImportFieldCategoryCode)
		row.SeedPerformance = field(domain.ImportFieldSeedPerformance)
		licenceNo := field(domain.ImportFieldLicenceNo)

		if row.FirstName == "" || row.LastName == "" {
			row.Status, row.Reason = ImportRowRejected, "first name and last name are required"
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		birthYear, err := strconv.Atoi(field(domain.ImportFieldBirthYear))
		if err != nil || birthYear <= 0 {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("invalid birth year %q", field(domain.ImportFieldBirthYear))
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		row.BirthYear = birthYear
		row.Sex = domain.Sex(strings.ToUpper(field(domain.ImportFieldSex)))
		if row.Sex != domain.SexMale && row.Sex != domain.SexFemale {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("invalid sex %q (want M or W)", field(domain.ImportFieldSex))
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		discCode, embeddedCat, ok := splitEventCode(row.EventCode)
		if !ok {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("malformed event code %q (want \"<discipline>/<category>\")", row.EventCode)
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		// The event row itself is always identified by the eventCode
		// column's own (disciplineCode, category) pair — an "unknown event
		// code" rejection (UC-004 #3). The separate categoryCode column, if
		// given and different, is the athlete's actual target category
		// within that event (SYS-002 combined events) — validated against
		// the resolved event's offered categories as a distinct "bad
		// category" rejection, matching UC-004 #3's third malformed-row
		// case.
		event, ok := findEventByDisciplineCategory(events, discCode, embeddedCat)
		if !ok {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("unknown event code %q for this meet's programme", row.EventCode)
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		targetCategory := embeddedCat
		if row.CategoryCode != "" && row.CategoryCode != embeddedCat {
			if !containsString(event.CategoryCodes, row.CategoryCode) {
				row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("category %q is not offered by event %q", row.CategoryCode, row.EventCode)
				report.Rows = append(report.Rows, row)
				report.Rejected++
				continue
			}
			targetCategory = row.CategoryCode
		}
		row.CategoryCode = targetCategory

		athlete, _, err := resolveImportAthlete(ctx, tx, licenceNo, row.FirstName, row.LastName, birthYear, row.Sex, commit)
		if err != nil {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("athlete lookup/create failed: %v", err)
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		if _, err := resolveImportClub(ctx, tx, row.Club, commit); err != nil {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("club lookup/create failed: %v", err)
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}

		var existingEntry store.EntryRecord
		hasExisting := false
		if athlete.ID != "" {
			if e, err := store.GetEntryByEventAthlete(ctx, tx, event.ID, athlete.ID); err == nil {
				existingEntry, hasExisting = e, true
			} else if !errors.Is(err, store.ErrNotFound) {
				row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("entry lookup failed: %v", err)
				report.Rows = append(report.Rows, row)
				report.Rejected++
				continue
			}
		}

		// SYS-015 entry-standard evaluation (best-effort for import rows):
		// a malformed seed/standard is a structural rejection, matching the
		// online-entry path; a *missing* seed is not — unlike UC-003's
		// online form, an imported club roster legitimately may carry no
		// seed time for some athletes, and import's job (SYS-013) is to
		// take over the data as given, not enforce SYS-015's stricter
		// online-submission requirement.
		fails := false
		if f, err := s.evaluateStandard(event, row.SeedPerformance); err == nil {
			fails = f
		} else if !errors.Is(err, ErrSeedPerformanceRequired) {
			row.Status, row.Reason = ImportRowRejected, err.Error()
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}

		switch {
		case hasExisting:
			row.Status = ImportRowUpdated
			row.EntryID = existingEntry.ID
		default:
			row.Status = ImportRowAccepted
		}

		if !commit {
			row.AthleteID = athlete.ID
			if athlete.ID != "" {
				others, _ := otherActiveRaceDistances(ctx, tx, meetID, athlete.ID, row.EntryID)
				_, hasLicence := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence)
				if licenceNo != "" {
					hasLicence = true
				}
				if scheme != nil {
					row.Eligibility = domain.EvaluateEligibility(scheme, s.catalog, domain.EligibilityInput{
						BirthYear: birthYear, Sex: row.Sex, TargetCategoryCode: targetCategory,
						DisciplineCode: event.DisciplineCode, HasLicence: hasLicence, MeetTier: meet.Tier,
						AsOf: meet.StartDate, OtherRaceDistancesM: others,
					})
				}
			}
			report.Rows = append(report.Rows, row)
			if row.Status == ImportRowUpdated {
				report.Updated++
			} else {
				report.Accepted++
			}
			continue
		}

		// commit == true from here on: persist.
		if _, err := store.EnsureParticipant(ctx, tx, meetID, athlete.ID); err != nil {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("participant registration failed: %v", err)
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}

		entryID := row.EntryID
		if !hasExisting {
			rec, err := store.CreateEntry(ctx, tx, domain.Entry{
				EventID: event.ID, AthleteID: athlete.ID, SeedPerformance: row.SeedPerformance,
				Source: domain.EntrySourceImport, FailsStandard: fails, SubmittedBy: actor.AccountID,
			})
			if err != nil {
				row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("entry create failed: %v", err)
				report.Rows = append(report.Rows, row)
				report.Rejected++
				continue
			}
			entryID = rec.ID
		}
		row.EntryID, row.AthleteID = entryID, athlete.ID

		result, err := s.evaluateAndStoreEligibility(ctx, tx, meet, event, athlete, entryID)
		if err != nil {
			row.Status, row.Reason = ImportRowRejected, fmt.Sprintf("eligibility evaluation failed: %v", err)
			report.Rows = append(report.Rows, row)
			report.Rejected++
			continue
		}
		row.Eligibility = result

		report.Rows = append(report.Rows, row)
		if row.Status == ImportRowUpdated {
			report.Updated++
		} else {
			report.Accepted++
		}
	}

	if commit {
		after, _ := json.Marshal(map[string]any{
			"profile": profileID, "accepted": report.Accepted, "updated": report.Updated, "rejected": report.Rejected,
		})
		if _, err := store.AppendAudit(ctx, sqlTx, store.AuditEntry{
			Actor: actor.AccountID, Action: "entry.import", EntityType: "meet", EntityID: meetID, After: string(after),
		}); err != nil {
			return ImportReport{}, fmt.Errorf("audit entry.import: %w", err)
		}
		if err := sqlTx.Commit(); err != nil {
			return ImportReport{}, fmt.Errorf("commit csv import: %w", err)
		}
	}
	return report, nil
}

// readCSVRows parses srcCSV, returning the header row and every data row
// (ragged rows are tolerated — a short row simply misses its trailing
// optional columns, reported as empty values rather than a parse error).
func readCSVRows(srcCSV io.Reader) (rows [][]string, header []string, err error) {
	r := csv.NewReader(srcCSV)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, errors.New("empty CSV file (no header row)")
	}
	return all[1:], all[0], nil
}

// columnIndex maps each of profile's mapped fields to its column index in
// header (exact match against the profile's documented header text, after
// trimming surrounding whitespace/BOM artefacts a spreadsheet export can
// leave on the first header cell). missing lists the header text of every
// required field the CSV's header row does not carry.
func columnIndex(header []string, profile *domain.ImportMappingProfile) (colFor map[domain.ImportField]int, missing []string) {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))] = i
	}
	colFor = map[domain.ImportField]int{}
	for _, c := range profile.Columns {
		if i, ok := idx[c.Header]; ok {
			colFor[c.Field] = i
		}
	}
	for _, f := range domain.RequiredImportFields {
		if _, ok := colFor[f]; !ok {
			h, _ := profile.HeaderFor(f)
			missing = append(missing, h)
		}
	}
	return colFor, missing
}

// splitEventCode splits an eventCode column value on its last "/" into
// "<disciplineCode>/<categoryCode>" (the mapping profiles' documented
// eventCodeFormat). Reports ok=false for anything else, including an empty
// string or a code with no category segment.
func splitEventCode(code string) (disciplineCode, categoryCode string, ok bool) {
	i := strings.LastIndex(code, "/")
	if i <= 0 || i == len(code)-1 {
		return "", "", false
	}
	return code[:i], code[i+1:], true
}

// containsString reports whether list contains s.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// findEventByDisciplineCategory finds the meet event whose discipline code
// matches disciplineCode and whose category codes include categoryCode —
// the join key an import row's (eventCode, categoryCode) pair resolves to
// (SYS-013).
func findEventByDisciplineCategory(events []store.EventRecord, disciplineCode, categoryCode string) (store.EventRecord, bool) {
	for _, e := range events {
		if e.DisciplineCode != disciplineCode {
			continue
		}
		for _, c := range e.CategoryCodes {
			if c == categoryCode {
				return e, true
			}
		}
	}
	return store.EventRecord{}, false
}

// resolveImportAthlete resolves the athlete an import row targets (SYS-013,
// UC-004 #2 idempotency): a licence number matches first, then the natural
// key (first/last name, birth year, sex); if neither matches, commit=true
// creates a new athlete (carrying the licence number if given) while
// commit=false (preview) returns a non-persisted placeholder record (ID
// empty). The second return value reports whether an existing athlete was
// found (never true for a preview placeholder).
func resolveImportAthlete(ctx context.Context, db store.DBTX, licenceNo, firstName, lastName string, birthYear int, sex domain.Sex, commit bool) (store.AthleteRecord, bool, error) {
	if licenceNo != "" {
		a, err := store.FindAthleteByExternalID(ctx, db, domain.NamespaceSwissAthleticsLicence, licenceNo)
		switch {
		case err == nil:
			return a, true, nil
		case !errors.Is(err, store.ErrNotFound):
			return store.AthleteRecord{}, false, err
		}
	}
	if a, err := store.FindAthleteByNaturalKey(ctx, db, firstName, lastName, birthYear, sex); err == nil {
		if commit && licenceNo != "" {
			if _, ok := a.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok {
				if _, err := store.SetAthleteExternalID(ctx, db, a.ID, a.Version, domain.NamespaceSwissAthleticsLicence, licenceNo); err == nil {
					a.ExternalIDs.Set(domain.NamespaceSwissAthleticsLicence, licenceNo)
				}
			}
		}
		return a, true, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.AthleteRecord{}, false, err
	}

	if !commit {
		return store.AthleteRecord{Athlete: domain.Athlete{
			FirstName: firstName, LastName: lastName, BirthYear: birthYear, Sex: sex,
		}}, false, nil
	}
	// SYS-103 consent on import (TASK-023): an import file carries no
	// consent artifact, so a newly created athlete gets the zero-value
	// PublicationConsent — "not withdrawn" per the documented baseline
	// (domain.PublicationConsent), with no RecordedAt/RecordedBy claiming
	// a consent interaction that never happened. A matched EXISTING
	// athlete's consent state is deliberately never touched by import
	// (this function only ever adds a licence external ID to a match) —
	// re-importing a withdrawn athlete must not silently re-publish them.
	a := domain.Athlete{FirstName: firstName, LastName: lastName, BirthYear: birthYear, Sex: sex}
	if licenceNo != "" {
		a.ExternalIDs = domain.ExternalIDs{domain.NamespaceSwissAthleticsLicence: licenceNo}
	}
	rec, err := store.CreateAthlete(ctx, db, a)
	return rec, false, err
}

// resolveImportClub resolves the club named by an import row, if any (an
// empty name is not an error — an unaffiliated athlete): matches an
// existing club by name, else creates one (commit) or returns a
// non-persisted placeholder (preview).
func resolveImportClub(ctx context.Context, db store.DBTX, name string, commit bool) (store.ClubRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return store.ClubRecord{}, nil
	}
	c, err := store.GetClubByName(ctx, db, name)
	switch {
	case err == nil:
		return c, nil
	case !errors.Is(err, store.ErrNotFound):
		return store.ClubRecord{}, err
	}
	if !commit {
		return store.ClubRecord{Club: domain.Club{Name: name}}, nil
	}
	return store.CreateClub(ctx, db, domain.Club{Name: name})
}
