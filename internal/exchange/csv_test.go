// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import "testing"

// TestGenericCSVRoundTripLosslessSYS062UC014_6 checks the documented
// generic CSV fallback (SYS-062) round-trips a start list plus results
// losslessly (UC-014 #6), including status/wind/edge-case rows.
func TestGenericCSVRoundTripLosslessSYS062UC014_6(t *testing.T) {
	rows := []GenericRow{
		{EventNumber: 1, Round: 1, Heat: 1, Bib: "23", LastName: "Duck", FirstName: "Don",
			Affiliation: "MIT", Lane: 1, Mark: "11.4", Timing: "manual"},
		{EventNumber: 1, Round: 1, Heat: 1, Bib: "54", LastName: "Bear, Jr.", FirstName: "Smokey",
			Lane: 2, Status: "DNS"},
		{EventNumber: 1, Round: 1, Heat: 1, Bib: "94", LastName: "Rabbit", FirstName: "Jackie",
			Lane: 3, Status: "DQ", StatusDetail: "TR16.8", Wind: "+2.4", ReactionTime: "0.148"},
		{EventNumber: 2, Round: 1, Heat: 1, Bib: "", LastName: "", FirstName: "", Lane: 0}, // no lane assigned yet
	}
	data := EncodeCSV(rows)
	got, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v\n%s", err, data)
	}
	if len(got) != len(rows) {
		t.Fatalf("got %d rows, want %d", len(got), len(rows))
	}
	for i := range rows {
		if got[i] != rows[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], rows[i])
		}
	}
}

// TestParseCSVRejectsWrongSchemaSYS062UC014_6 is the denial/conflict-first
// path: a file that has drifted from the documented schema must be
// rejected, not silently misread into the wrong columns.
func TestParseCSVRejectsWrongSchemaSYS062UC014_6(t *testing.T) {
	bad := []byte("bib,name\n23,Don Duck\n")
	if _, err := ParseCSV(bad); err == nil {
		t.Fatal("expected an error for a CSV file with a non-matching header")
	}
}

// TestParseCSVEmptyBody confirms a header-only file parses to zero rows,
// not an error (an empty start list export is valid).
func TestParseCSVEmptyBody(t *testing.T) {
	got, err := ParseCSV(EncodeCSV(nil))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d rows, want 0", len(got))
	}
}
