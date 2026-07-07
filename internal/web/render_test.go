// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"html"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// renderPageData builds a PageData for direct template rendering tests.
func renderPageData(t *testing.T, loc i18n.Locale) PageData {
	t.Helper()
	cats, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	return PageData{
		Locale:      loc,
		Locales:     []i18n.Locale{i18n.DE, i18n.FR},
		Cats:        cats,
		Title:       "t",
		Username:    "orga",
		LoggedIn:    true,
		CanOrganize: true,
		CSRFToken:   "tok",
	}
}

// TestRenderMeetPagesStates covers the display states the HTTP flow tests
// don't reach: empty lists, archived meets (forms hidden), incomplete
// versus complete summaries, and both locales (SYS-110: every surface is
// catalog-driven).
func TestRenderMeetPagesStates(t *testing.T) {
	for _, loc := range []i18n.Locale{i18n.DE, i18n.FR} {
		p := renderPageData(t, loc)

		var sb strings.Builder
		if err := meetsListPage(p, nil).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		empty := sb.String()
		if !strings.Contains(empty, html.EscapeString(p.T("meets.empty"))) {
			t.Errorf("[%s] empty meets list missing empty-state text", loc)
		}

		rows := []meetRowView{{ID: "01M", Name: "Meet", Venue: "V", Dates: "2027-06-12", Tier: "C-Meeting", Status: "draft"}}
		sb.Reset()
		if err := meetsListPage(p, rows).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "/meets/01M") {
			t.Errorf("[%s] meets list missing row link", loc)
		}

		// Archived detail: no archive button, no add-event or schedule forms.
		d := meetDetailView{
			ID: "01M", Version: 3, Name: "Meet", Venue: "V", Dates: "2027-06-12",
			Tier: "C-Meeting", Status: "archived", Archived: true,
			Sessions:  []sessionRowView{{Day: "2027-06-12", Label: "S1"}},
			Programme: []programmeRowView{{Discipline: "100 m", Categories: "U16 W", CaptureType: "Lauf", Rounds: "Final", Deadline: "2027-06-01T23:59"}},
			Units:     []unitRowView{{UnitID: "01U", Version: 1, Discipline: "100 m", Categories: "U16 W", Round: "Final", When: "2027-06-12T14:30", Location: "L", Scheduled: true}},
			Versions:  []timetableVersionRowView{{Version: 1, PublishedAt: "2027-06-10 12:00:00 UTC"}},
		}
		sb.Reset()
		if err := meetDetailPage(p, d).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		archived := sb.String()
		if strings.Contains(archived, "/archive") || strings.Contains(archived, "/events") || strings.Contains(archived, "/schedule") {
			t.Errorf("[%s] archived meet page still offers mutation forms", loc)
		}
		if !strings.Contains(archived, "100 m") || !strings.Contains(archived, html.EscapeString(p.T("timetable.versions.title"))) {
			t.Errorf("[%s] archived meet page missing content", loc)
		}

		// Empty detail: placeholder texts for sessions/programme/timetable.
		sb.Reset()
		if err := meetDetailPage(p, meetDetailView{ID: "01M", Name: "Meet", Status: "draft",
			Disciplines: []disciplineOptionView{{Code: "100m", Name: "100 m"}},
			Categories:  []string{"U16 W"},
		}).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		blank := sb.String()
		for _, key := range []string{"meet.sessions.none", "programme.empty", "timetable.empty"} {
			if !strings.Contains(blank, html.EscapeString(p.T(key))) {
				t.Errorf("[%s] blank meet page missing %s text", loc, key)
			}
		}

		// Incomplete sanctioning summary shows the warning; complete doesn't.
		v := sanctioningView{MeetName: "Meet", Venue: "V", HomologationRef: "H",
			Dates: "2027-06-12", Organizer: "orga", Tier: "C-Meeting",
			Sessions:   []sessionRowView{{Day: "2027-06-12", Label: "S1"}},
			Categories: "U16 W", Disciplines: "100 m", GeneratedAt: "2027-06-10", Complete: true}
		sb.Reset()
		if err := sanctioningPage(p, v).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sb.String(), html.EscapeString(p.T("sanctioning.incomplete"))) {
			t.Errorf("[%s] complete summary shows incompleteness warning", loc)
		}
		v.Complete = false
		v.Missing = []string{"homologation reference"}
		sb.Reset()
		if err := sanctioningPage(p, v).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), html.EscapeString(p.T("sanctioning.incomplete"))) {
			t.Errorf("[%s] incomplete summary missing warning", loc)
		}

		// Public timetable renders rows for anonymous visitors.
		pubData := renderPageData(t, loc)
		pubData.LoggedIn = false
		pubData.CanOrganize = false
		pub := publicTimetableView{MeetName: "Meet", Venue: "V", Dates: "2027-06-12",
			Version: 2, PublishedAt: "2027-06-10 12:00 UTC",
			Rows: []unitRowView{{Discipline: "100 m", Categories: "U16 W", Round: "Final", When: "2027-06-12T15:15", Location: "L"}}}
		sb.Reset()
		if err := publicTimetablePage(pubData, pub).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "15:15") {
			t.Errorf("[%s] public timetable missing scheduled time", loc)
		}

		// The meet form in both scheme modes (create: selectable; edit: fixed).
		form := meetFormView{
			Tiers: []string{"C-Meeting"}, Schemes: []string{"swiss-athletics"},
			SchemeID: "swiss-athletics", Sessions: make([]sessionRowView, 2),
		}
		sb.Reset()
		if err := meetFormPage(p, form, "/meets").Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "<select name=\"scheme\"") {
			t.Errorf("[%s] create form missing scheme selector", loc)
		}
		form.SchemeFixed = true
		sb.Reset()
		if err := meetFormPage(p, form, "/meets/01M/edit").Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "type=\"hidden\" name=\"scheme\"") {
			t.Errorf("[%s] edit form must pin the scheme", loc)
		}
	}
}
