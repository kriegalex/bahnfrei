// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"fmt"
	"html"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"

	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// helpScreenFixtures maps every helpRegistry Screen id to a renderer of
// that screen state with fixture view data. The UC-037 #1 coverage test
// walks the registry against these rendered screens (and vice versa) —
// adding a registry entry without a renderable screen, or a screen
// renderer without registry entries, fails the build.
func helpScreenFixtures() map[string]func(p PageData) (string, error) {
	render := func(f func(w *strings.Builder) error) (string, error) {
		var sb strings.Builder
		err := f(&sb)
		return sb.String(), err
	}
	return map[string]func(p PageData) (string, error){
		"meet-form": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return meetFormPage(p, meetFormView{
					Tiers:               []string{"C-Meeting"},
					Schemes:             []string{"swiss-athletics"},
					SchemeID:            "swiss-athletics",
					Sessions:            make([]sessionRowView, 1),
					ResultsPositioning:  "federation_official",
					ResultsPositionings: []string{"federation_official", "primary"},
				}, "/meets").Render(context.Background(), w)
			})
		},
		"meet-detail": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return meetDetailPage(p, meetDetailView{
					ID: "01M", Name: "Meet", Status: "draft",
					Disciplines: []disciplineOptionView{{Code: "100m", Name: "100 m"}},
					Categories:  []string{"U16 W"},
				}).Render(context.Background(), w)
			})
		},
		"entries": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return entriesPage(p, entriesView{
					MeetID: "01M", MeetName: "Meet",
					Events:   []entryEventOptionView{{ID: "E1", Label: "100 m U16 W"}},
					BulkRows: []int{0},
				}).Render(context.Background(), w)
			})
		},
		"roster": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return rosterPage(p, rosterView{MeetID: "01M", MeetName: "Meet"}).Render(context.Background(), w)
			})
		},
		"seeding": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return seedingPage(p, seedingView{
					MeetID: "01M", MeetName: "Meet",
					EventID: "E1", EventLabel: "100 m", RoundID: "R1", RoundLabel: "Final",
				}).Render(context.Background(), w)
			})
		},
		"capture-track": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return capturePage(p, captureView{
					MeetID: "01M", MeetName: "Meet", UnitID: "U1",
					Discipline: "100 m", WindRelevant: true,
					Rows:            []captureRowView{{AthleteID: "A1", Bib: "101", Name: "Test Athletin", Lane: "1"}},
					TrackTimings:    []string{"manual"},
					TrackStatuses:   []string{"DNS"},
					TrackFormAction: "track",
				}).Render(context.Background(), w)
			})
		},
		"vertical-capture-initial": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return verticalCapturePage(p, verticalCaptureView{
					MeetID: "01M", MeetName: "Meet", UnitID: "U1",
					Discipline: "Hochsprung", CanOffice: true,
				}).Render(context.Background(), w)
			})
		},
		"vertical-capture-extend": func(p PageData) (string, error) {
			return render(func(w *strings.Builder) error {
				return verticalCapturePage(p, verticalCaptureView{
					MeetID: "01M", MeetName: "Meet", UnitID: "U1",
					Discipline: "Hochsprung", CanOffice: true,
					Heights: []string{"1.60"},
				}).Render(context.Background(), w)
			})
		},
	}
}

// helpIconPair is one help icon found in a rendered page, paired with the
// name of the nearest preceding non-hidden input/select/textarea in
// document order. That pairing is the mechanical form of UC-037 #1's
// "adjacent to its label": the component contract (help.templ) places
// @helpIcon immediately after the closing </label> of the field it
// explains, so the field's own control is always the closest form control
// before the icon.
type helpIconPair struct {
	Key   string
	Input string
}

// collectHelpPairs parses page and returns every [data-help-key] element
// paired with its preceding form control (see helpIconPair), plus the set
// of non-hidden form-control names present on the page.
func collectHelpPairs(t *testing.T, page string) ([]helpIconPair, map[string]bool) {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse rendered page: %v", err)
	}
	attr := func(n *xhtml.Node, name string) (string, bool) {
		for _, a := range n.Attr {
			if a.Key == name {
				return a.Val, true
			}
		}
		return "", false
	}
	var pairs []helpIconPair
	inputs := map[string]bool{}
	lastInput := ""
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "input", "select", "textarea":
				if typ, _ := attr(n, "type"); typ != "hidden" {
					if name, ok := attr(n, "name"); ok {
						lastInput = name
						inputs[name] = true
					}
				}
			}
			if key, ok := attr(n, "data-help-key"); ok {
				pairs = append(pairs, helpIconPair{Key: key, Input: lastInput})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return pairs, inputs
}

// TestHelpRegistryCoverageSYS115UC037_1 is the mechanical UC-037 #1
// coverage test over the SYS-115 help-content registry (help.go), in both
// directions per screen:
//
//   - every registered (screen, input) renders a help icon whose nearest
//     preceding form control IS that input — no registered input lacks
//     its icon, and no icon sits next to the wrong field;
//   - every help icon rendered on a registered screen has a registry
//     entry — help content cannot bypass the reviewable registry.
//
// It also fails when a registry Screen has no fixture renderer or a
// fixture renderer has no registry entries, so the registry and this
// test's screen list cannot drift apart silently.
func TestHelpRegistryCoverageSYS115UC037_1(t *testing.T) {
	p := renderPageData(t, i18n.DE)
	fixtures := helpScreenFixtures()

	byScreen := map[string][]helpEntry{}
	for _, e := range helpRegistry {
		byScreen[e.Screen] = append(byScreen[e.Screen], e)
	}
	for screen := range byScreen {
		if _, ok := fixtures[screen]; !ok {
			t.Errorf("registry screen %q has no fixture renderer in helpScreenFixtures", screen)
		}
	}
	for screen := range fixtures {
		if len(byScreen[screen]) == 0 {
			t.Errorf("fixture screen %q has no registry entries — remove the fixture or register its inputs", screen)
		}
	}

	for screen, entries := range byScreen {
		renderScreen, ok := fixtures[screen]
		if !ok {
			continue // reported above
		}
		page, err := renderScreen(p)
		if err != nil {
			t.Fatalf("[%s] render: %v", screen, err)
		}
		pairs, inputs := collectHelpPairs(t, page)

		got := map[helpIconPair]bool{}
		for _, pair := range pairs {
			got[pair] = true
		}
		for _, e := range entries {
			if !inputs[e.Input] {
				t.Errorf("[%s] registered input %q does not render on the screen — registry drift (help.go)", screen, e.Input)
				continue
			}
			if !got[helpIconPair{Key: e.Key, Input: e.Input}] {
				t.Errorf("[%s] registered input %q lacks its adjacent help icon (key %q); rendered icons: %v", screen, e.Input, e.Key, pairs)
			}
		}
		if len(pairs) != len(entries) {
			t.Errorf("[%s] %d help icons rendered but %d registry entries — every icon must be registered (SYS-115); rendered: %v", screen, len(pairs), len(entries), pairs)
		}
	}
}

// TestHelpTextsLocalizedDEandFRSYS110UC037_2 pins the DE/FR half of
// UC-037 #2 ("the same short help text appears, localized — asserted in
// DE and FR"): every registry key (and the trigger's accessible name) has
// a non-empty text in both launch catalogs, and a rendered screen embeds
// exactly the active locale's text. The behavioral half (hover/focus/tap
// showing that same text) is e2e/tests/contextual-help-UC037.spec.ts.
func TestHelpTextsLocalizedDEandFRSYS110UC037_2(t *testing.T) {
	cats, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{"help.trigger_label": true, "help.gallery.example": true}
	for _, e := range helpRegistry {
		keys[helpTextKey(e.Key)] = true
	}
	for key := range keys {
		for _, loc := range []i18n.Locale{i18n.DE, i18n.FR} {
			text := cats.Text(loc, key)
			if text == "" || strings.HasPrefix(text, "[[") {
				t.Errorf("[%s] help text %q missing from catalog", loc, key)
			}
		}
	}
	de, fr := cats.Text(i18n.DE, "help.entries.seed"), cats.Text(i18n.FR, "help.entries.seed")
	if de == fr {
		t.Errorf("help.entries.seed identical in DE and FR (%q) — not localized", de)
	}

	for _, loc := range []i18n.Locale{i18n.DE, i18n.FR} {
		p := renderPageData(t, loc)
		page, err := helpScreenFixtures()["entries"](p)
		if err != nil {
			t.Fatal(err)
		}
		want := html.EscapeString(cats.Text(loc, "help.entries.seed"))
		if !strings.Contains(page, want) {
			t.Errorf("[%s] entries screen missing localized help text %q", loc, want)
		}
	}
}

// TestHelpIconATAssociationUC037_4 pins the statically checkable half of
// UC-037 #4: the trigger is a focusable <button> with an accessible name,
// aria-expanded state, and aria-describedby referencing the popup, and
// the popup carries role="tooltip" with that id and starts hidden. The
// axe WCAG 2.2 AA scan of a help-open state is in the e2e spec.
func TestHelpIconATAssociationUC037_4(t *testing.T) {
	p := renderPageData(t, i18n.DE)
	page, err := helpScreenFixtures()["entries"](p)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := xhtml.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	attr := func(n *xhtml.Node, name string) string {
		for _, a := range n.Attr {
			if a.Key == name {
				return a.Val
			}
		}
		return ""
	}
	ids := map[string]*xhtml.Node{}
	var triggers []*xhtml.Node
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			if id := attr(n, "id"); id != "" {
				ids[id] = n
			}
			if n.Data == "button" && strings.Contains(attr(n, "class"), "help-trigger") {
				triggers = append(triggers, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(triggers) == 0 {
		t.Fatal("entries screen renders no help triggers")
	}
	for _, trig := range triggers {
		if attr(trig, "type") != "button" {
			t.Errorf("help trigger must be type=button (never submits its form)")
		}
		if attr(trig, "aria-label") == "" {
			t.Errorf("help trigger lacks an accessible name (aria-label)")
		}
		if attr(trig, "aria-expanded") != "false" {
			t.Errorf("help trigger must start with aria-expanded=false")
		}
		ref := attr(trig, "aria-describedby")
		if ref == "" {
			t.Fatalf("help trigger lacks aria-describedby")
		}
		popup, ok := ids[ref]
		if !ok {
			t.Fatalf("help trigger aria-describedby=%q references no element", ref)
		}
		if attr(popup, "role") != "tooltip" {
			t.Errorf("help popup %q must carry role=tooltip", ref)
		}
		if _, hidden := func() (string, bool) {
			for _, a := range popup.Attr {
				if a.Key == "hidden" {
					return a.Val, true
				}
			}
			return "", false
		}(); !hidden {
			t.Errorf("help popup %q must start hidden", ref)
		}
	}
}

// TestHardConstraintHintsVisibleSYS117UC037_5 covers UC-037 #5 for the
// representative fields the acceptance criterion names ("fixtures for
// representative fields; full sweep via the SYS-117 release audit"): a
// hard input constraint renders as permanently visible `.hint` text
// associated with its input via aria-describedby — present in the static
// HTML without activating any help popup.
func TestHardConstraintHintsVisibleSYS117UC037_5(t *testing.T) {
	for _, loc := range []i18n.Locale{i18n.DE, i18n.FR} {
		p := renderPageData(t, loc)
		cases := []struct {
			screen  string
			hintID  string
			hintKey string
		}{
			{"meet-form", "meet-end-date-hint", "meet.hint.end_date"},
			{"meet-detail", "programme-entry-standard-hint", "programme.hint.entry_standard"},
			{"meet-detail", "programme-entry-limit-hint", "programme.hint.entry_limit"},
			{"entries", "entries-seed-hint", "entries.hint.seed"},
			{"entries", "entries-birth-year-hint", "entries.hint.birth_year"},
			{"seeding", "seeding-track-lanes-hint", "seeding.hint.track_lanes"},
			{"capture-track", "capture-wind-hint", "capture.hint.wind"},
			{"vertical-capture-initial", "vertical-heights-hint", "capture.vertical.hint.heights"},
		}
		fixtures := helpScreenFixtures()
		pages := map[string]string{}
		for _, tc := range cases {
			page, ok := pages[tc.screen]
			if !ok {
				var err error
				page, err = fixtures[tc.screen](p)
				if err != nil {
					t.Fatalf("[%s] render %s: %v", loc, tc.screen, err)
				}
				pages[tc.screen] = page
			}
			if !strings.Contains(page, fmt.Sprintf(`id="%s"`, tc.hintID)) {
				t.Errorf("[%s][%s] missing visible hint element #%s", loc, tc.screen, tc.hintID)
			}
			if !strings.Contains(page, fmt.Sprintf(`aria-describedby="%s"`, tc.hintID)) {
				t.Errorf("[%s][%s] no input associates hint #%s via aria-describedby", loc, tc.screen, tc.hintID)
			}
			if !strings.Contains(page, html.EscapeString(p.T(tc.hintKey))) {
				t.Errorf("[%s][%s] localized hint text %q not rendered", loc, tc.screen, tc.hintKey)
			}
		}
	}

	// The setup form's password hint (a hard minimum-length constraint on
	// an otherwise unregistered, universally familiar field).
	p := renderPageData(t, i18n.DE)
	var sb strings.Builder
	if err := setupPage(p).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `id="setup-password-hint"`) ||
		!strings.Contains(sb.String(), html.EscapeString(p.T("setup.hint.password"))) {
		t.Errorf("setup page missing visible password-length hint")
	}
}
