// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"bytes"
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// ErrNoSeriesUploadTemplate reports that the meet's template names no
// SYS-077 series-upload template (or names one this service was not wired
// with) — the export route surfaces this as "not available for this meet".
var ErrNoSeriesUploadTemplate = fmt.Errorf("series upload template not configured for this meet")

// SetSeriesUploadTemplates wires the SYS-077 series-upload templates map,
// keyed by SeriesUploadTemplate.ID, at service construction time (mirrors
// the OnResultsChanged hook-setter pattern: additive, so existing callers
// of NewResultsService keep compiling unchanged). Normally the built-in
// data (domain.BuiltinSeriesUploadTemplates).
func (s *ResultsService) SetSeriesUploadTemplates(templates map[string]*domain.SeriesUploadTemplate) {
	s.seriesUploads = templates
}

// SeriesUploadAvailable reports whether meetID's template names a
// SYS-077 series-upload template this service was wired with — the
// standings page uses it to show the download link only where an export
// actually exists, without building the workbook just to check.
func (s *ResultsService) SeriesUploadAvailable(ctx context.Context, meetID string) bool {
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return false
	}
	meetTpl, ok := s.templates[meet.TemplateID]
	if !ok || meetTpl.SeriesUploadTemplateID == "" {
		return false
	}
	_, ok = s.seriesUploads[meetTpl.SeriesUploadTemplateID]
	return ok
}

// SeriesUploadExport builds the SYS-077 series results-upload workbook for
// a meet (UC-035 #1–#3): one row per participant per division, in the
// meet's template order, with every discipline's mark and points aligned to
// the template-driven column layout — never hardcoded in this function,
// which only walks the SeriesUploadTemplate shape generically. Returns the
// XLSX bytes and a suggested download filename.
func (s *ResultsService) SeriesUploadExport(ctx context.Context, meetID string) ([]byte, string, error) {
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, "", err
	}
	meetTpl, ok := s.templates[meet.TemplateID]
	if !ok || meetTpl.SeriesUploadTemplateID == "" {
		return nil, "", ErrNoSeriesUploadTemplate
	}
	tpl, ok := s.seriesUploads[meetTpl.SeriesUploadTemplateID]
	if !ok {
		return nil, "", ErrNoSeriesUploadTemplate
	}

	standings, err := s.Standings(ctx, meetID)
	if err != nil {
		return nil, "", err
	}

	discNames := make([]string, len(standings.Disciplines))
	for i, code := range standings.Disciplines {
		name := code
		if d, ok := s.catalog.ByCode(code); ok {
			name = d.Name
		}
		discNames[i] = name
	}

	buf, err := buildSeriesUploadWorkbook(tpl, standings, discNames)
	if err != nil {
		return nil, "", err
	}
	filename := fmt.Sprintf("%s-series-upload.xlsx", sanitizeFilenamePart(meet.Name))
	return buf, filename, nil
}

// buildSeriesUploadWorkbook renders the workbook: header row from the
// template's identity + per-discipline + trailing columns, then one data
// row per standing row across every division, in division/rank order. It
// never special-cases a series or discipline by name — every column comes
// from tpl and standings.Disciplines.
func buildSeriesUploadWorkbook(tpl *domain.SeriesUploadTemplate, standings MeetStandings, discNames []string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, tpl.SheetName); err != nil {
		return nil, fmt.Errorf("series upload export: set sheet name: %w", err)
	}

	headers := seriesUploadHeaders(tpl, discNames)
	headerValues := make([]any, len(headers))
	for i, h := range headers {
		headerValues[i] = h
	}
	if err := writeSeriesUploadRow(f, tpl.SheetName, 1, headerValues); err != nil {
		return nil, err
	}

	row := 2
	for _, div := range standings.Divisions {
		for _, r := range div.Rows {
			values := seriesUploadRowValues(tpl, div.CategoryCode, r)
			if err := writeSeriesUploadRow(f, tpl.SheetName, row, values); err != nil {
				return nil, err
			}
			row++
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("series upload export: write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

// seriesUploadHeaders expands the template into the sheet's flat header
// row: identity columns, then one mark+points column pair per meet
// discipline (template event order), then trailing columns.
func seriesUploadHeaders(tpl *domain.SeriesUploadTemplate, discNames []string) []string {
	out := make([]string, 0, len(tpl.IdentityColumns)+2*len(discNames)+len(tpl.TrailingColumns))
	for _, c := range tpl.IdentityColumns {
		out = append(out, c.Header)
	}
	for _, name := range discNames {
		out = append(out, fmt.Sprintf(tpl.DisciplineMarkHeader, name))
		out = append(out, fmt.Sprintf(tpl.DisciplinePointsHeader, name))
	}
	for _, c := range tpl.TrailingColumns {
		out = append(out, c.Header)
	}
	return out
}

// seriesUploadRowValues renders one participant's row aligned to
// seriesUploadHeaders' column order. A discipline the athlete has no
// scoring result for renders the template's MissingMarkPlaceholder rather
// than a blank cell or a dropped row (UC-035 #2); a non-scoring status
// (DNS, NM, DQ, …) renders in place of the placeholder.
func seriesUploadRowValues(tpl *domain.SeriesUploadTemplate, divisionCode string, r StandingRow) []any {
	out := make([]any, 0, len(tpl.IdentityColumns)+2*len(r.Marks)+len(tpl.TrailingColumns))
	for _, c := range tpl.IdentityColumns {
		out = append(out, seriesUploadFieldValue(c.Field, divisionCode, r))
	}
	for _, m := range r.Marks {
		out = append(out, seriesUploadMarkCell(tpl, m))
		out = append(out, seriesUploadPointsCell(m))
	}
	for _, c := range tpl.TrailingColumns {
		out = append(out, seriesUploadFieldValue(c.Field, divisionCode, r))
	}
	return out
}

func seriesUploadFieldValue(field domain.SeriesUploadColumnField, divisionCode string, r StandingRow) any {
	switch field {
	case domain.SeriesUploadFieldRank:
		return r.Rank
	case domain.SeriesUploadFieldBib:
		return r.Bib
	case domain.SeriesUploadFieldLastName:
		return r.LastName
	case domain.SeriesUploadFieldFirstName:
		return r.FirstName
	case domain.SeriesUploadFieldBirthYear:
		return r.BirthYear
	case domain.SeriesUploadFieldSex:
		return string(r.Sex)
	case domain.SeriesUploadFieldClub:
		return r.ClubName
	case domain.SeriesUploadFieldDivision:
		return divisionCode
	case domain.SeriesUploadFieldTotal:
		return r.Total
	default:
		return "" // unreachable: SeriesUploadTemplate.Validate rejects unknown fields
	}
}

func seriesUploadMarkCell(tpl *domain.SeriesUploadTemplate, m domain.CombinedPerformance) any {
	switch {
	case m.Status != domain.StatusNone:
		return string(m.Status)
	case m.Mark != "":
		return m.Mark
	default:
		return tpl.MissingMarkPlaceholder
	}
}

func seriesUploadPointsCell(m domain.CombinedPerformance) any {
	if m.Points == nil {
		return ""
	}
	return *m.Points
}

func writeSeriesUploadRow(f *excelize.File, sheet string, row int, values []any) error {
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			return fmt.Errorf("series upload export: cell coordinates: %w", err)
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return fmt.Errorf("series upload export: set cell %s: %w", cell, err)
		}
	}
	return nil
}

// sanitizeFilenamePart lowercases and replaces anything but ASCII
// letters/digits/hyphens with a hyphen, so a meet name becomes a safe
// download filename fragment.
func sanitizeFilenamePart(name string) string {
	out := make([]byte, 0, len(name))
	lastHyphen := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, byte(r)) // #nosec G115 -- r is bounded to 'a'-'z'/'0'-'9' by the case guard above, well within byte range
			lastHyphen = false
		case r >= 'A' && r <= 'Z':
			out = append(out, byte(r-'A'+'a'))
			lastHyphen = false
		default:
			if !lastHyphen && len(out) > 0 {
				out = append(out, '-')
				lastHyphen = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return "meet"
	}
	return string(out)
}
