// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func fixtureDocument() Document {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	wind := 1.4
	points := 710
	placing := 1
	return Document{
		SchemaVersion: SchemaVersionOMXV1,
		ExportedAt:    now,
		Meet:          MeetDoc{ID: "meet1", Name: "UKC Test", Venue: "Zurich", StartDate: now, EndDate: now, Status: "closed"},
		Clubs:         []ClubDoc{{ID: "club1", Name: "LC Test"}},
		Athletes: []AthleteDoc{
			{ID: "ath1", FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: "W", ClubIDs: []string{"club1"}},
		},
		Events: []EventDoc{
			{ID: "ev1", DisciplineCode: "60m", CategoryCodes: []string{"W12"}, Status: "closed"},
		},
		Rounds: []RoundDoc{{ID: "rd1", EventID: "ev1", Kind: "final", Seq: 0}},
		Units:  []UnitDoc{{ID: "u1", RoundID: "rd1"}},
		Results: []ResultDoc{
			{ID: "res1", UnitID: "u1", AthleteID: "ath1", Mark: "8.42", Timing: "electronic",
				Wind: &wind, Status: "", Points: &points, Placing: &placing, RecordFlags: []string{}},
		},
	}
}

func TestEncodeDecodeOMXRoundTripSYS073UC027_1(t *testing.T) {
	doc := fixtureDocument()
	data, err := EncodeOMX(doc)
	if err != nil {
		t.Fatalf("EncodeOMX: %v", err)
	}
	if !strings.Contains(string(data), `"schemaVersion": "omx/v1"`) {
		t.Errorf("encoded document does not stamp schemaVersion: %s", data)
	}

	// UC-027 #1: "validate against the published schema".
	if err := ValidateOMXSchema(data); err != nil {
		t.Fatalf("ValidateOMXSchema: %v", err)
	}

	got, err := DecodeOMX(data)
	if err != nil {
		t.Fatalf("DecodeOMX: %v", err)
	}
	if got.Meet.ID != doc.Meet.ID || len(got.Athletes) != 1 || len(got.Results) != 1 {
		t.Errorf("decoded document = %+v, want equivalent to fixture", got)
	}
	if got.Results[0].Mark != "8.42" || *got.Results[0].Points != 710 {
		t.Errorf("decoded result = %+v", got.Results[0])
	}
}

func TestDecodeOMXRejectsWrongSchemaVersion(t *testing.T) {
	doc := fixtureDocument()
	doc.SchemaVersion = "omx/v2"
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOMX(data); err == nil {
		t.Error("DecodeOMX with schemaVersion omx/v2: want error (SYS-144 deprecation policy: a v1 reader refuses a v2 document rather than guessing)")
	}
}

func TestDecodeOMXToleratesAdditiveUnknownFields(t *testing.T) {
	doc := fixtureDocument()
	data, err := EncodeOMX(doc)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a newer v1.x writer that added an optional field this
	// reader does not know about — must still decode (forward-compatible
	// within v1, per the schema's own versioning policy).
	withExtra := strings.Replace(string(data), `"schemaVersion": "omx/v1",`,
		`"schemaVersion": "omx/v1", "futureOptionalField": "ignored",`, 1)
	if _, err := DecodeOMX([]byte(withExtra)); err != nil {
		t.Errorf("DecodeOMX must tolerate an unknown additive field, got: %v", err)
	}
}

func TestOMXV1SchemaIsPublishedAndSelfValidating(t *testing.T) {
	raw := OMXV1Schema()
	if len(raw) == 0 {
		t.Fatal("OMXV1Schema() returned no bytes")
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("published schema is not valid JSON: %v", err)
	}
	if parsed["$id"] == nil {
		t.Error("published schema has no $id")
	}
	// A conforming document must validate against the exact bytes this
	// function returns (SYS-144: the same artifact this system documents
	// is the one it checks exports against — no drift between the two).
	data, err := EncodeOMX(fixtureDocument())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSONSchema(raw, data); err != nil {
		t.Errorf("a conforming document must validate against OMXV1Schema(): %v", err)
	}
}

func TestValidateOMXRejectsDanglingReference(t *testing.T) {
	doc := fixtureDocument()
	doc.Results[0].UnitID = "no-such-unit"
	if err := ValidateOMX(doc); err == nil {
		t.Error("ValidateOMX with a result referencing an unknown unit: want error")
	}
}

func TestDecodeOMXRejectsMalformedJSON(t *testing.T) {
	if _, err := DecodeOMX([]byte(`{not json`)); err == nil {
		t.Error("DecodeOMX with malformed JSON: want error")
	}
}

// TestValidateOMXRejectsEachDanglingReferenceKind sweeps every referential
// check ValidateOMX performs (SYS-073's "complete... reconstruct official
// results" depends on every cross-reference resolving, never silently
// dropped) — one broken document per reference kind.
func TestValidateOMXRejectsEachDanglingReferenceKind(t *testing.T) {
	base := func() Document {
		d := fixtureDocument()
		d.RelayTeams = []RelayTeamDoc{{ID: "rt1", ClubID: "club1", Composition: []string{"ath1"}}}
		d.Entries = []EntryDoc{{ID: "en1", EventID: "ev1", AthleteID: "ath1", Status: "confirmed"}}
		d.Assignments = []AssignmentDoc{{ID: "as1", UnitID: "u1", EntryID: "en1"}}
		return d
	}
	cases := map[string]func(*Document){
		"missing meet id":           func(d *Document) { d.Meet.ID = "" },
		"missing meet name":         func(d *Document) { d.Meet.Name = "" },
		"club missing name":         func(d *Document) { d.Clubs[0].Name = "" },
		"athlete missing last name": func(d *Document) { d.Athletes[0].LastName = "" },
		"athlete bad sex":           func(d *Document) { d.Athletes[0].Sex = "X" },
		"relay team unknown club":   func(d *Document) { d.RelayTeams[0].ClubID = "no-such-club" },
		"relay team empty composition": func(d *Document) {
			d.RelayTeams[0].Composition = nil
		},
		"event missing discipline": func(d *Document) { d.Events[0].DisciplineCode = "" },
		"event missing categories": func(d *Document) { d.Events[0].CategoryCodes = nil },
		"round unknown event":      func(d *Document) { d.Rounds[0].EventID = "no-such-event" },
		"unit unknown round":       func(d *Document) { d.Units[0].RoundID = "no-such-round" },
		"entry unknown event":      func(d *Document) { d.Entries[0].EventID = "no-such-event" },
		"entry neither athlete nor relay team": func(d *Document) {
			d.Entries[0].AthleteID = ""
		},
		"entry both athlete and relay team": func(d *Document) {
			d.Entries[0].RelayTeamID = "rt1"
		},
		"entry unknown athlete": func(d *Document) { d.Entries[0].AthleteID = "no-such-athlete" },
		"entry unknown relay team": func(d *Document) {
			d.Entries[0].AthleteID = ""
			d.Entries[0].RelayTeamID = "no-such-relay"
		},
		"assignment unknown unit":  func(d *Document) { d.Assignments[0].UnitID = "no-such-unit" },
		"assignment unknown entry": func(d *Document) { d.Assignments[0].EntryID = "no-such-entry" },
		"result unknown athlete":   func(d *Document) { d.Results[0].AthleteID = "no-such-athlete" },
		"wrong schema version":     func(d *Document) { d.SchemaVersion = "omx/v2" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := base()
			mutate(&d)
			if err := ValidateOMX(d); err == nil {
				t.Errorf("ValidateOMX with %s: want error", name)
			}
		})
	}
}

func TestEncodeOMXResultsCSVSYS073(t *testing.T) {
	doc := fixtureDocument()
	csvBytes := EncodeOMXResultsCSV(doc)
	s := string(csvBytes)
	if !strings.Contains(s, "event_id,discipline_code") {
		t.Fatalf("csv missing documented header: %s", s)
	}
	if !strings.Contains(s, "Muster") || !strings.Contains(s, "8.42") || !strings.Contains(s, "60m") {
		t.Errorf("csv row missing expected fields: %s", s)
	}
}
