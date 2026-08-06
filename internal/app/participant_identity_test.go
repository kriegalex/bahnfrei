// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// TestUpdateParticipantIdentitySYS150UC043_1 covers UC-043 #1's happy path:
// an office-issued correction takes effect on the roster (Participants),
// the capture listing (UnitCapture) and standings — including the name and
// club change and the birth-year-driven category re-derivation (W12 →
// W13), all off one commit with no separate "recompute" step.
func TestUpdateParticipantIdentitySYS150UC043_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Ana", LastName: "Musterr", BirthYear: 2014,
		Sex: domain.SexFemale, Club: "LC Typo", Bib: "101",
	})

	st, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings before correction: %v", err)
	}
	if row := findRow(t, st, "W12", "101"); row.LastName != "Musterr" {
		t.Fatalf("pre-correction W12 row = %+v", row)
	}

	updated, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2013, Sex: domain.SexFemale,
		Club: "LC Test", Bib: "101",
	})
	if err != nil {
		t.Fatalf("UpdateParticipantIdentity: %v", err)
	}
	if updated.Athlete.FirstName != "Anna" || updated.Athlete.LastName != "Muster" || updated.Athlete.BirthYear != 2013 {
		t.Errorf("returned row = %+v", updated)
	}

	// Roster (Participants) reflects the corrected name/club immediately.
	participants, err := results.Participants(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	if len(participants) != 1 || participants[0].Athlete.FirstName != "Anna" || participants[0].Athlete.LastName != "Muster" {
		t.Fatalf("roster after correction = %+v", participants)
	}
	clubs, err := results.ClubNamesFor(ctx, participants)
	if err != nil {
		t.Fatalf("ClubNamesFor: %v", err)
	}
	if got := clubs[participants[0].Athlete.ClubIDs[0]]; got != "LC Test" {
		t.Errorf("roster club after correction = %q, want LC Test", got)
	}

	// Capture listing (UnitCapture) resolves the athlete row fresh too.
	unitID, err := results.disciplineUnit(ctx, rec.ID, "60m")
	if err != nil {
		t.Fatalf("disciplineUnit: %v", err)
	}
	view, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	if len(view.Rows) != 1 || view.Rows[0].LastName != "Muster" || view.Rows[0].BirthYear != 2013 {
		t.Errorf("capture row after correction = %+v", view.Rows)
	}

	// UC-043 #1: category re-derivation. Anna moved from W12 (birth year
	// 2014) to W13 (birth year 2013) — the standings row must have moved
	// divisions, not just carried the new birth year inside the old one.
	st, err = results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings after correction: %v", err)
	}
	for _, div := range st.Divisions {
		if div.CategoryCode == "W12" {
			for _, row := range div.Rows {
				if row.Bib == "101" {
					t.Errorf("bib 101 still present in W12 after the birth-year correction: %+v", row)
				}
			}
		}
	}
	w13 := findRow(t, st, "W13", "101")
	if w13.LastName != "Muster" || w13.BirthYear != 2013 {
		t.Errorf("W13 row after correction = %+v", w13)
	}
}

// TestUpdateParticipantIdentityVersionConflictSYS150UC043_1: a stale
// expectedVersion (a concurrent edit already landed) is rejected, input
// otherwise unchanged.
func TestUpdateParticipantIdentityVersionConflictSYS150UC043_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	// A first correction lands and bumps the participant's version.
	if _, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster-Corrected", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	}); err != nil {
		t.Fatalf("first correction: %v", err)
	}

	// A second correction submitted against the stale (pre-first-edit)
	// version must be refused as a conflict, not silently applied.
	_, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Someone", LastName: "Else", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale-version correction = %v, want ErrConflict", err)
	}

	participants, err := results.Participants(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	if participants[0].Athlete.LastName != "Muster-Corrected" {
		t.Errorf("name after rejected conflict = %q, want the first correction to have won", participants[0].Athlete.LastName)
	}
}

// TestUpdateParticipantIdentityBibCollisionSYS150UC043_1: correcting a
// participant's bib to one already held by someone else in the meet is
// rejected (reuses AssignBib's store-level uniqueness check).
func TestUpdateParticipantIdentityBibCollisionSYS150UC043_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})

	_, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})
	if !errors.Is(err, ErrDuplicateParticipant) {
		t.Fatalf("bib collision = %v, want ErrDuplicateParticipant", err)
	}
}

// TestUpdateParticipantIdentityValidationSYS150 exercises the structural
// validation UpdateParticipantIdentity applies as defense-in-depth behind
// the web layer's own field-level checks (SYS-150: sex M/W, birth-year
// bounds, bib format, a non-empty last name).
func TestUpdateParticipantIdentityValidationSYS150(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})

	base := ParticipantIdentityInput{FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101"}
	cases := []struct {
		name string
		mut  func(in ParticipantIdentityInput) ParticipantIdentityInput
	}{
		{"empty last name", func(in ParticipantIdentityInput) ParticipantIdentityInput { in.LastName = "  "; return in }},
		{"birth year too early", func(in ParticipantIdentityInput) ParticipantIdentityInput { in.BirthYear = 1899; return in }},
		{"birth year in the future", func(in ParticipantIdentityInput) ParticipantIdentityInput { in.BirthYear = 3000; return in }},
		{"invalid sex", func(in ParticipantIdentityInput) ParticipantIdentityInput { in.Sex = "X"; return in }},
		{"non-numeric bib", func(in ParticipantIdentityInput) ParticipantIdentityInput { in.Bib = "12a"; return in }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, c.mut(base)); !errors.Is(err, ErrParticipantIdentityInvalid) {
				t.Errorf("%s: err = %v, want ErrParticipantIdentityInvalid", c.name, err)
			}
		})
	}
}

// TestUpdateParticipantIdentityMarksUnchangedSYS150UC043_3 covers UC-043
// #3: an identity correction — including one that moves the athlete's
// category via a sex change — never alters or re-scores an already
// captured mark. The scoring tables key columns by (discipline, timing,
// sex), never by age/category (internal/domain/scoring.go), so a
// hypothetical re-score would only ever be triggered by a stored Result
// being rewritten; this test pins that no such rewrite happens.
func TestUpdateParticipantIdentityMarksUnchangedSYS150UC043_3(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	saved := save(t, results, rec.ID, ResultInput{
		AthleteID: anna.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic,
	})
	if saved.Points == nil || *saved.Points != 710 {
		t.Fatalf("fixture points = %v, want 710", saved.Points)
	}
	unitID, err := results.disciplineUnit(ctx, rec.ID, "60m")
	if err != nil {
		t.Fatalf("disciplineUnit: %v", err)
	}
	before, err := store.GetResult(ctx, st.DB(), unitID, anna.AthleteID)
	if err != nil {
		t.Fatalf("GetResult before: %v", err)
	}

	// A correction that also flips sex (M12 uses a different points
	// column than W12) must still leave the stored mark/points untouched.
	if _, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexMale, Bib: "101",
	}); err != nil {
		t.Fatalf("UpdateParticipantIdentity: %v", err)
	}

	after, err := store.GetResult(ctx, st.DB(), unitID, anna.AthleteID)
	if err != nil {
		t.Fatalf("GetResult after: %v", err)
	}
	if after.Mark != before.Mark || after.Points == nil || before.Points == nil || *after.Points != *before.Points {
		t.Fatalf("stored result changed by identity correction: before %+v, after %+v", before, after)
	}
	if after.Mark != "8.42" || *after.Points != 710 {
		t.Errorf("stored result = %+v, want the original 8.42/710 unchanged", after)
	}
}

// TestUpdateParticipantIdentityAuditSYS150UC043_2 covers UC-043 #2: the
// audit row carries the actor, before/after snapshots and timestamp via
// the SYS-046 mechanism (store.AppendAudit), mirroring CorrectResult's
// convention.
func TestUpdateParticipantIdentityAuditSYS150UC043_2(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Ana", LastName: "Musterr", BirthYear: 2014, Sex: domain.SexFemale, Club: "LC Typo", Bib: "101",
	})
	if _, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Club: "LC Test", Bib: "101",
		Reason: "typo + wrong club at registration",
	}); err != nil {
		t.Fatalf("UpdateParticipantIdentity: %v", err)
	}

	entries, err := store.ListAudit(ctx, st.DB(), 50)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Action != "participant.identity_correct" || e.EntityID != anna.ID {
			continue
		}
		found = true
		if e.Actor != office.AccountID {
			t.Errorf("audit actor = %q, want %q", e.Actor, office.AccountID)
		}
		if e.Before == "" || e.After == "" {
			t.Error("identity-correction audit entry must carry before/after snapshots")
		}
		if !strings.Contains(e.Before, "Ana Musterr") || !strings.Contains(e.After, "Anna Muster") {
			t.Errorf("audit before/after = %q / %q, want the old/new names", e.Before, e.After)
		}
		if e.Reason != "typo + wrong club at registration" {
			t.Errorf("audit reason = %q, want the optional reason carried through", e.Reason)
		}
		if e.TS.IsZero() {
			t.Error("audit timestamp is zero")
		}
	}
	if !found {
		t.Fatal("no participant.identity_correct audit entry found")
	}
}

// TestUpdateParticipantIdentityErasedRejectedSYS150 covers the privacy
// gate: an already-erased (anonymized) participant is never editable
// through this path.
func TestUpdateParticipantIdentityErasedRejectedSYS150(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	privacy := NewPrivacyService(st.DB())
	if err := privacy.EraseAthlete(ctx, office, anna.AthleteID, "subject request"); err != nil {
		t.Fatalf("EraseAthlete: %v", err)
	}

	_, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if !errors.Is(err, ErrAthleteAnonymized) {
		t.Fatalf("correcting an erased participant = %v, want ErrAthleteAnonymized", err)
	}
}

// TestUpdateParticipantIdentityPropagatesToSeriesUploadExportSYS150 covers
// the last named UC-043 #1 surface: the SYS-077 series-upload workbook
// (app.SeriesUploadExport), which walks CurrentStandings just like the
// standings page — the corrected name and re-derived category appear
// there too with no export-specific handling.
func TestUpdateParticipantIdentityPropagatesToSeriesUploadExportSYS150(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Ana", LastName: "Musterr", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if _, err := results.UpdateParticipantIdentity(ctx, office, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2013, Sex: domain.SexFemale, Bib: "101",
	}); err != nil {
		t.Fatalf("UpdateParticipantIdentity: %v", err)
	}

	data, _, err := results.SeriesUploadExport(ctx, rec.ID)
	if err != nil {
		t.Fatalf("SeriesUploadExport: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("excelize.OpenReader: %v", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (header + Anna)", len(rows))
	}
	row := rows[1]
	if row[0] != "W13" {
		t.Errorf("series-upload category = %q, want W13 (birth-year correction re-derived)", row[0])
	}
	if row[3] != "Muster" || row[4] != "Anna" {
		t.Errorf("series-upload name = %s %s, want Muster Anna", row[4], row[3])
	}
}

// TestUpdateParticipantIdentityAuthorizationSYS150UC043 mirrors the
// established office-capability gate (SYS-090) every other participant
// mutation in this file already enforces (RegisterParticipant,
// SetOutOfCompetition): field officials cannot correct identity data.
func TestUpdateParticipantIdentityAuthorizationSYS150UC043(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	var forbidden ErrForbidden
	if _, err := results.UpdateParticipantIdentity(ctx, fieldOfficial, rec.ID, anna.ID, anna.Version, ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	}); !errors.As(err, &forbidden) {
		t.Errorf("as field official = %v, want ErrForbidden (SYS-090)", err)
	}
}
