// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package pdf

import (
	"bytes"
	"fmt"
	"time"

	gofpdf "codeberg.org/go-pdf/fpdf"
)

// Header is the meet/session/unit context and generation timestamp every
// printable document carries — repeated on every page so a multi-page
// document never loses it (SYS-072, UC-018 #1: "carry meet/session/unit
// headers and generation timestamps").
type Header struct {
	// DocTitle names the kind of document (already translated), e.g.
	// "Capture sheet" / "Result list".
	DocTitle string
	// MeetName is the meet's display name.
	MeetName string
	// Subtitle is the session/unit or division context, e.g. a discipline
	// name or a division code. Empty renders no subtitle line.
	Subtitle string
	// GeneratedAtLabel is the translated "Generated" prefix.
	GeneratedAtLabel string
	// GeneratedAt is stamped in the document (SYS-072).
	GeneratedAt time.Time
}

// Column is one table column: an already-translated header and a relative
// width weight. Weights are normalized against the page's usable width;
// a zero or unset weight for every column in a section spreads columns
// evenly.
type Column struct {
	Header string
	Weight float64
}

// Section is one table block: an optional already-translated sub-heading
// (e.g. a division code) plus its columns and fully-formatted row cells.
// len(row) may be less than len(Columns); missing trailing cells render
// blank (a printed capture sheet's not-yet-filled cells, UC-018 #1).
type Section struct {
	Heading string
	Columns []Column
	Rows    [][]string
}

// Document is everything Build renders: the shared header plus the
// section tables that follow it.
type Document struct {
	Header   Header
	Sections []Section
}

// Page geometry (A4 portrait, millimetres) — the PoC's one fixed layout;
// a later landscape variant would add an Orientation field to Document.
const (
	marginMM     = 12.0
	pageWidthMM  = 210.0
	headerRuleMM = 3.0
)

// Build renders doc to PDF bytes.
func Build(doc Document) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginMM, marginMM, marginMM)
	pdf.SetHeaderFuncMode(func() { renderHeader(pdf, doc.Header) }, true)
	pdf.SetAutoPageBreak(true, marginMM+8)
	pdf.AliasNbPages("")
	pdf.SetTitle(doc.Header.DocTitle+" — "+doc.Header.MeetName, true)

	pdf.AddPage()
	for i, sec := range doc.Sections {
		if i > 0 {
			pdf.Ln(4)
		}
		renderSection(pdf, sec)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf: render: %w", err)
	}
	if pdf.Err() {
		return nil, fmt.Errorf("pdf: %w", pdf.Error())
	}
	return buf.Bytes(), nil
}

// renderHeader draws the repeating meet/session/unit + timestamp block
// (SYS-072). It runs once per page via SetHeaderFuncMode, immediately
// after AddPage repositions to the top margin.
func renderHeader(pdf *gofpdf.Fpdf, h Header) {
	tr := pdf.UnicodeTranslatorFromDescriptor("") // cp1252 covers DE/FR diacritics
	pdf.SetFont("Helvetica", "B", 14)
	pdf.CellFormat(0, 7, tr(h.MeetName), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 6, tr(h.DocTitle), "", 1, "L", false, 0, "")
	if h.Subtitle != "" {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 6, tr(h.Subtitle), "", 1, "L", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 8)
	stamp := h.GeneratedAtLabel
	if stamp != "" {
		stamp += ": "
	}
	stamp += h.GeneratedAt.Format("2006-01-02 15:04")
	pdf.CellFormat(0, 5, tr(stamp), "", 1, "L", false, 0, "")
	pdf.Ln(1)
	y := pdf.GetY()
	pdf.SetDrawColor(160, 160, 160)
	pdf.Line(marginMM, y, pageWidthMM-marginMM, y)
	pdf.Ln(headerRuleMM)
}

// renderSection draws one heading + bordered table.
func renderSection(pdf *gofpdf.Fpdf, sec Section) {
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	if sec.Heading != "" {
		pdf.SetFont("Helvetica", "B", 12)
		pdf.CellFormat(0, 7, tr(sec.Heading), "", 1, "L", false, 0, "")
	}
	if len(sec.Columns) == 0 {
		return
	}
	widths := columnWidths(pdf, sec.Columns)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(225, 225, 225)
	for i, c := range sec.Columns {
		pdf.CellFormat(widths[i], 6.5, tr(c.Header), "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 9)
	for _, row := range sec.Rows {
		for i := range sec.Columns {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			pdf.CellFormat(widths[i], 6.5, tr(cell), "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
}

// columnWidths normalizes column weights against the page's usable width.
func columnWidths(pdf *gofpdf.Fpdf, cols []Column) []float64 {
	usable := pageWidthMM - 2*marginMM
	var total float64
	for _, c := range cols {
		total += c.Weight
	}
	widths := make([]float64, len(cols))
	if total <= 0 {
		// No weights given: spread columns evenly rather than divide by
		// zero.
		even := usable / float64(len(cols))
		for i := range widths {
			widths[i] = even
		}
		return widths
	}
	for i, c := range cols {
		widths[i] = usable * c.Weight / total
	}
	return widths
}
