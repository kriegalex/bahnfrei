// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// newImportFixture is entryFixture with a C-Meeting/Women/100m event, the
// baseline programme the import tests target.
func newImportFixture(t *testing.T) entryFixture {
	return newTieredEntryFixture(t, "C-Meeting", "100m", "Women")
}

// systemNativeCSVHeader is the built-in system-native profile's column
// order (internal/domain/data/import-profiles/system-native.json).
const systemNativeCSVHeader = "first_name,last_name,birth_year,sex,club,licence_no,event_code,category_code,seed_performance"

func systemNativeCSVRow(first, last, birthYear, sex, club, licence, eventCode, categoryCode, seed string) string {
	return strings.Join([]string{first, last, birthYear, sex, club, licence, eventCode, categoryCode, seed}, ",")
}

// TestCSVImportAcceptsValidRowsSYS013UC004_1 covers UC-004 #1 verbatim: a
// documented system-native CSV of 200 entries imports 200 entries/athletes
// with the import report showing 200 accepted / 0 rejected.
func TestCSVImportAcceptsValidRowsSYS013UC004_1(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	var lines []string
	lines = append(lines, systemNativeCSVHeader)
	for i := 0; i < 200; i++ {
		lines = append(lines, systemNativeCSVRow(
			"Runner", fmt.Sprintf("Number%03d", i), "1995", "W", "LC Test", "",
			"100m/Women", "", "12.90"))
	}
	src := strings.NewReader(strings.Join(lines, "\n"))

	report, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, src)
	if err != nil {
		t.Fatalf("CommitCSVImport: %v", err)
	}
	if report.Accepted != 200 || report.Rejected != 0 || report.Updated != 0 {
		t.Fatalf("report = %+v, want 200 accepted / 0 rejected / 0 updated", report)
	}
	if len(report.Rows) != 200 {
		t.Fatalf("len(Rows) = %d, want 200", len(report.Rows))
	}

	flagged, err := f.results.EligibilityExceptions(ctx, office, f.meetID)
	if err != nil {
		t.Fatalf("EligibilityExceptions: %v", err)
	}
	// C-Meeting + no licence column populated -> every imported entry is a
	// licence warning (D1.2), so all 200 show up as (non-blocking) exceptions.
	if len(flagged) != 200 {
		t.Fatalf("EligibilityExceptions = %d entries, want 200 (unlicensed warning each)", len(flagged))
	}
}

// TestCSVImportIdempotentSYS013UC004_2 covers UC-004 #2: importing the same
// file twice creates no duplicates, and the second run's report shows
// updates, not inserts.
func TestCSVImportIdempotentSYS013UC004_2(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	csvBody := strings.Join([]string{
		systemNativeCSVHeader,
		systemNativeCSVRow("Anna", "Muster", "1995", "W", "LC Test", "SA-1001", "100m/Women", "", "12.85"),
		systemNativeCSVRow("Bea", "Keller", "1996", "W", "LC Test", "", "100m/Women", "", "13.10"),
	}, "\n")

	first, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("first CommitCSVImport: %v", err)
	}
	if first.Accepted != 2 || first.Updated != 0 || first.Rejected != 0 {
		t.Fatalf("first report = %+v, want 2 accepted / 0 updated / 0 rejected", first)
	}

	second, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("second CommitCSVImport: %v", err)
	}
	if second.Accepted != 0 || second.Updated != 2 || second.Rejected != 0 {
		t.Fatalf("second report = %+v, want 0 accepted / 2 updated / 0 rejected (idempotent re-import)", second)
	}
	// Entry IDs must be stable across the two runs (no duplicate rows).
	for i := range first.Rows {
		if first.Rows[i].EntryID != second.Rows[i].EntryID {
			t.Errorf("row %d: entry id changed across re-import (%q -> %q), want stable id (no duplicate)", i+1, first.Rows[i].EntryID, second.Rows[i].EntryID)
		}
	}

	// Count every entry at the meet directly (not just flagged ones — Anna
	// carries a licence number and is fully eligible with no flags) to
	// prove the second import created no duplicate rows.
	all, err := store.ListEntriesByMeet(ctx, f.st.DB(), f.meetID)
	if err != nil {
		t.Fatalf("ListEntriesByMeet: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("meet has %d entries after two imports of the same 2-row file, want exactly 2 (no duplicates)", len(all))
	}
}

// TestCSVImportMalformedRowsRejectedSYS013UC004_3 covers UC-004 #3 verbatim:
// a file with 3 malformed rows (missing birth year, unknown event code, bad
// category) — the valid rows import, the 3 rows are rejected, and each
// rejection carries a row number and reason.
func TestCSVImportMalformedRowsRejectedSYS013UC004_3(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	csvBody := strings.Join([]string{
		systemNativeCSVHeader,
		systemNativeCSVRow("Anna", "Muster", "1995", "W", "LC Test", "", "100m/Women", "", "12.85"),                  // row 1: valid
		systemNativeCSVRow("Bea", "NoBirthYear", "", "W", "LC Test", "", "100m/Women", "", "13.00"),                  // row 2: missing birth year
		systemNativeCSVRow("Cara", "BadEvent", "1997", "W", "LC Test", "", "9999m/Women", "", "5.00"),                // row 3: unknown event code
		systemNativeCSVRow("Dora", "BadCategory", "1998", "W", "LC Test", "", "100m/Women", "NotACategory", "12.50"), // row 4: bad category
		systemNativeCSVRow("Ella", "Valid", "1999", "W", "LC Test", "", "100m/Women", "", "12.60"),                   // row 5: valid
	}, "\n")

	report, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("CommitCSVImport: %v", err)
	}
	if report.Accepted != 2 || report.Rejected != 3 {
		t.Fatalf("report = %+v, want 2 accepted / 3 rejected", report)
	}
	if len(report.Rows) != 5 {
		t.Fatalf("len(Rows) = %d, want 5", len(report.Rows))
	}

	wantRejectedRows := map[int]string{2: "birth year", 3: "event code", 4: "category"}
	for rowNum, substr := range wantRejectedRows {
		row := report.Rows[rowNum-1]
		if row.RowNumber != rowNum {
			t.Errorf("Rows[%d].RowNumber = %d, want %d", rowNum-1, row.RowNumber, rowNum)
		}
		if row.Status != ImportRowRejected {
			t.Errorf("row %d: status = %q, want rejected", rowNum, row.Status)
		}
		if row.Reason == "" || !strings.Contains(row.Reason, substr) {
			t.Errorf("row %d: reason = %q, want it to mention %q", rowNum, row.Reason, substr)
		}
	}
	for _, rowNum := range []int{1, 5} {
		row := report.Rows[rowNum-1]
		if row.Status != ImportRowAccepted || row.EntryID == "" {
			t.Errorf("row %d: expected accepted with an entry id, got %+v", rowNum, row)
		}
	}
}

// TestCSVImportAlabusProfileSYS013UC004_4 covers UC-004 #4: importing
// against the Alabus mapping profile (German headers, licence-number
// column) maps athletes with licence numbers and category/discipline
// combinations correctly (OQ-030: the profile itself is an assumption,
// pending a real Alabus export sample — this proves the mapping-profile
// *mechanism* works generically, independent of the exact column layout).
func TestCSVImportAlabusProfileSYS013UC004_4(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	header := "Vorname,Name,Jahrgang,Geschlecht,Verein,Lizenznummer,Disziplin,Kategorie,Meldeleistung"
	row := "Anna,Muster,1995,W,LC Bern,SA-4711,100m/Women,,12.85"
	src := strings.NewReader(header + "\n" + row)

	report, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileAlabus, src)
	if err != nil {
		t.Fatalf("CommitCSVImport: %v", err)
	}
	if report.Accepted != 1 || report.Rejected != 0 {
		t.Fatalf("report = %+v, want 1 accepted / 0 rejected", report)
	}
	got := report.Rows[0]
	if got.FirstName != "Anna" || got.LastName != "Muster" || got.BirthYear != 1995 || got.Sex != domain.SexFemale {
		t.Fatalf("parsed athlete = %+v, want Anna Muster 1995 W", got)
	}
	if got.AthleteID == "" {
		t.Fatal("expected an athlete id for the accepted row")
	}

	athlete, err := store.GetAthlete(ctx, f.st.DB(), got.AthleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if id, ok := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-4711" {
		t.Fatalf("licence external id = (%q, %v), want (\"SA-4711\", true) — UC-004 #4 \"athletes map with licence numbers\"", id, ok)
	}
}

// TestCSVImportPreviewDoesNotPersistSYS013UC004 covers the
// "preview-before-commit" requirement (UC-004): PreviewCSVImport reports
// the same accept/reject outcome as a commit would, but creates nothing.
func TestCSVImportPreviewDoesNotPersistSYS013UC004(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	csvBody := strings.Join([]string{
		systemNativeCSVHeader,
		systemNativeCSVRow("Anna", "Muster", "1995", "W", "LC Test", "", "100m/Women", "", "12.85"),
	}, "\n")

	preview, err := f.results.PreviewCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("PreviewCSVImport: %v", err)
	}
	if preview.Committed {
		t.Fatal("PreviewCSVImport report must have Committed = false")
	}
	if preview.Accepted != 1 || preview.Rejected != 0 {
		t.Fatalf("preview report = %+v, want 1 accepted / 0 rejected", preview)
	}
	if preview.Rows[0].EntryID != "" {
		t.Errorf("preview row must not carry a persisted entry id, got %q", preview.Rows[0].EntryID)
	}

	exceptions, err := f.results.EligibilityExceptions(ctx, office, f.meetID)
	if err != nil {
		t.Fatalf("EligibilityExceptions: %v", err)
	}
	if len(exceptions) != 0 {
		t.Fatalf("preview must not persist any entry, found %d flagged entries", len(exceptions))
	}

	// Committing the identical file afterwards still succeeds as a fresh
	// accept (nothing from the preview lingered).
	commit, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("CommitCSVImport after preview: %v", err)
	}
	if commit.Accepted != 1 {
		t.Fatalf("commit after preview = %+v, want 1 accepted", commit)
	}
}

// TestCSVImportRequiresOfficeRoleSYS090 covers the "authorized operator"
// gate: an entry submitter cannot run an import (this is a competition
// office action, not a self-service one).
func TestCSVImportRequiresOfficeRoleSYS090(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	src := strings.NewReader(systemNativeCSVHeader)
	var forbidden ErrForbidden
	if _, err := f.results.CommitCSVImport(ctx, entrySubmitter, f.meetID, domain.ImportProfileSystemNative, src); !errors.As(err, &forbidden) {
		t.Errorf("entry-submitter CommitCSVImport = %v, want ErrForbidden", err)
	}
}

// TestCSVImportUnknownProfileRejected covers the profile-selection guard.
func TestCSVImportUnknownProfileRejected(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	src := strings.NewReader(systemNativeCSVHeader)
	if _, err := f.results.CommitCSVImport(ctx, office, f.meetID, "not-a-real-profile", src); !errors.Is(err, ErrUnknownImportProfile) {
		t.Errorf("CommitCSVImport with unknown profile = %v, want ErrUnknownImportProfile", err)
	}
}

// TestCSVImportMissingRequiredColumnRejected covers the header-validation
// guard: a CSV missing a column the profile requires is rejected up front,
// not row by row.
func TestCSVImportMissingRequiredColumnRejected(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	src := strings.NewReader("first_name,last_name\nAnna,Muster")
	if _, err := f.results.CommitCSVImport(ctx, office, f.meetID, domain.ImportProfileSystemNative, src); err == nil {
		t.Fatal("expected an error for a CSV missing required columns")
	}
}
