// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// GenericRow is this system's documented, vendor-neutral interchange row
// (SYS-062): one athlete's start-list placement and/or result on one unit,
// self-contained (event/round/heat identity travels with every row, unlike
// the FinishLynx family's separate .evt/.lif files) so a single CSV can
// round-trip a full start list plus whatever results have already settled
// — the fallback path for ALGE/TimeTronics-class timing sources that speak
// CSV but not the Lynx file family (ADR-006 §3).
type GenericRow struct {
	EventNumber  int
	Round        int
	Heat         int
	Bib          string
	LastName     string
	FirstName    string
	Affiliation  string
	Lane         int
	Mark         string // encoded per discipline unit (SYS-040/041), "" if not yet captured
	Timing       string // "manual" | "electronic" | ""
	Status       string // CR 25 code (domain.QualificationStatus) or ""
	StatusDetail string
	Wind         string // e.g. "+1.1"; "" if not wind-relevant or not recorded
	ReactionTime string
}

// genericCSVHeader is the documented column order (SYS-062: "documented
// generic CSV"). ParseCSV requires an exact header match so a malformed or
// hand-edited file fails loudly instead of silently misreading columns.
var genericCSVHeader = []string{
	"event_number", "round", "heat", "bib", "last_name", "first_name", "affiliation",
	"lane", "mark", "timing", "status", "status_detail", "wind", "reaction_time",
}

// EncodeCSV renders rows as the documented generic interchange file: a
// standard RFC 4180 CSV (comma-separated, quote-on-demand) with a header
// row naming every column.
func EncodeCSV(rows []GenericRow) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(genericCSVHeader)
	for _, r := range rows {
		_ = w.Write([]string{
			strconv.Itoa(r.EventNumber), strconv.Itoa(r.Round), strconv.Itoa(r.Heat),
			r.Bib, r.LastName, r.FirstName, r.Affiliation,
			laneOrEmpty(r.Lane), r.Mark, r.Timing, r.Status, r.StatusDetail, r.Wind, r.ReactionTime,
		})
	}
	w.Flush()
	return buf.Bytes()
}

// ParseCSV reads the documented generic interchange file back into rows
// (SYS-062's round-trip requirement, UC-014 #6). It requires the exact
// documented header (column order and names) so a file that drifted from
// the schema is rejected rather than silently misparsed.
func ParseCSV(data []byte) ([]GenericRow, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = len(genericCSVHeader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("generic csv: read header: %w", err)
	}
	if !equalHeader(header, genericCSVHeader) {
		return nil, fmt.Errorf("generic csv: header %v does not match the documented schema %v", header, genericCSVHeader)
	}

	var out []GenericRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("generic csv: %w", err)
		}
		evt, _ := strconv.Atoi(rec[0])
		round, _ := strconv.Atoi(rec[1])
		heat, _ := strconv.Atoi(rec[2])
		lane, _ := strconv.Atoi(rec[7])
		out = append(out, GenericRow{
			EventNumber: evt, Round: round, Heat: heat, Bib: rec[3],
			LastName: rec[4], FirstName: rec[5], Affiliation: rec[6], Lane: lane,
			Mark: rec[8], Timing: rec[9], Status: rec[10], StatusDetail: rec[11],
			Wind: rec[12], ReactionTime: rec[13],
		})
	}
	return out, nil
}

func laneOrEmpty(lane int) string {
	if lane == 0 {
		return ""
	}
	return strconv.Itoa(lane)
}

func equalHeader(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
