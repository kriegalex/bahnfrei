// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/pdf"
)

// --- fee schedule and per-club fee summary (TASK-016, UC-006 #3, SYS-017) ---

func (s *Server) handleFees(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	sum, err := s.results.FeeSummary(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = detail.Name + " — " + p.T("fees.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("fees.error." + msg)
	}
	_ = feesPage(p, feesViewFrom(detail.ID, detail.Name, detail.Version, sum)).Render(r.Context(), w)
}

func feesViewFrom(meetID, meetName string, version int64, sum app.FeeSummary) feesView {
	v := feesView{
		MeetID: meetID, MeetName: meetName, Version: intToStr(version),
		EntryFeeCHF: centsToCHF(sum.EntryFeeCents), RelayFeeCHF: centsToCHF(sum.RelayFeeCents),
		TotalCHF: centsToCHF(sum.TotalCents), HasFeeSummary: len(sum.Clubs) > 0,
	}
	for _, c := range sum.Clubs {
		v.Clubs = append(v.Clubs, clubFeeRowView{
			ClubName: c.ClubName, IndividualEntries: c.IndividualEntries,
			RelayEntries: c.RelayEntries, TotalCHF: centsToCHF(c.TotalCents),
		})
	}
	return v
}

func feeFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrConflict):
		return "conflict"
	default:
		return "invalid"
	}
}

func (s *Server) handleFeeScheduleSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	entryFee, err1 := chfToCents(r.FormValue("entry_fee"))
	relayFee, err2 := chfToCents(r.FormValue("relay_fee"))
	if err1 != nil || err2 != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/fees?err=invalid", http.StatusSeeOther)
		return
	}
	if err := s.meets.SetFeeSchedule(r.Context(), actor, meetID, version, entryFee, relayFee); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/fees?err="+feeFlashKey(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/fees", http.StatusSeeOther)
}

// handleFeeExport serves the per-club fee summary plus a post-meet
// participation summary as CSV (UC-006 #3: "a post-meet participation
// summary suitable for the Swiss levy report is exportable").
func (s *Server) handleFeeExport(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	sum, err := s.results.FeeSummary(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="fee-summary.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{p.T("fees.export.club"), p.T("fees.export.individual"), p.T("fees.export.relay"), p.T("fees.export.total")})
	for _, c := range sum.Clubs {
		_ = cw.Write([]string{
			c.ClubName, strconv.Itoa(c.IndividualEntries), strconv.Itoa(c.RelayEntries), centsToCHF(c.TotalCents),
		})
	}
	_ = cw.Write([]string{p.T("fees.export.total"), "", "", centsToCHF(sum.TotalCents)})
	cw.Flush()
}

// pdfBuildBibList assembles the printable bib list (UC-006 #1: one section
// per club, plus a trailing section for participants without a club),
// bib-ordered within each section. Takes the same bibRowView rows the bibs
// page itself renders (never a raw store type — web goes through app,
// architecture.md §3).
func pdfBuildBibList(p PageData, meetName string, rows []bibRowView) ([]byte, error) {
	header := pdf.Header{
		DocTitle: p.T("bibs.title"), MeetName: meetName,
		GeneratedAtLabel: p.T("pdf.generated"), GeneratedAt: time.Now(),
	}
	cols := []pdf.Column{
		{Header: p.T("roster.bib"), Weight: 1},
		{Header: p.T("roster.name"), Weight: 3},
	}

	byClub := map[string][][]string{}
	var clubOrder []string
	var unaffiliated [][]string
	for _, row := range rows {
		cells := []string{row.Bib, row.Name}
		if row.Club == "" {
			unaffiliated = append(unaffiliated, cells)
			continue
		}
		if _, ok := byClub[row.Club]; !ok {
			clubOrder = append(clubOrder, row.Club)
		}
		byClub[row.Club] = append(byClub[row.Club], cells)
	}

	var sections []pdf.Section
	for _, club := range clubOrder {
		sections = append(sections, pdf.Section{Heading: club, Columns: cols, Rows: byClub[club]})
	}
	if len(unaffiliated) > 0 {
		sections = append(sections, pdf.Section{Heading: p.T("bibs.no_club"), Columns: cols, Rows: unaffiliated})
	}
	return pdf.Build(pdf.Document{Header: header, Sections: sections})
}
