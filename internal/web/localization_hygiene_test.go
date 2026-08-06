// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// discLocalized pins, per locale, the DE/FR catalog strings the UKC template
// fixture's three disciplines localize to (discipline.60m, discipline.ZoneLJ,
// discipline.BallThrow200g) alongside the English catalog names they must
// never render as (F6/SYS-111 defect sweep, TASK-050).
var discLocalized = map[string]struct{ sixtyM, zoneLJ, ballThrow string }{
	"de": {"60 m", "Zonen-Weitsprung (UKC)", "Ballwurf 200 g (UKC)"},
	"fr": {"60 m", "Saut en longueur par zones (UKC)", "Lancer de balle 200 g (UKC)"},
}

const (
	englishSixtyM  = "60 metres"
	englishZoneLJ  = "Zone Long Jump (UKC)"
	englishBallThr = "200 g Ball Throw (UKC)"
)

// switchLocale applies the DE/FR locale switch (routes.go's GET /locale) on
// an already-authenticated client, mirroring how a real operator toggles
// the footer language selector (SYS-110).
func switchLocale(t *testing.T, client *http.Client, base, loc string) {
	t.Helper()
	resp := mustGet(t, client, base+"/locale?lang="+loc)
	_ = bodyString(t, resp)
}

// TestCaptureIndexLocalizedDisciplineNameSYS111 covers F6: the capture index
// page (handleCaptureIndex) previously listed every unit by the catalog's
// canonical English discipline name even under the FR/DE catalogs; it now
// reuses localizedDisciplineName like the standings surface, in both launch
// languages.
func TestCaptureIndexLocalizedDisciplineNameSYS111(t *testing.T) {
	for _, loc := range []string{"de", "fr"} {
		t.Run(loc, func(t *testing.T) {
			deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
			client, base := newTestClient(t, deps)
			setupAndLogin(t, client, base)
			switchLocale(t, client, base, loc)

			meetID, _ := ukcCaptureFixture(t, client, base)
			body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/capture"))

			want := discLocalized[loc]
			for _, s := range []string{want.sixtyM, want.zoneLJ, want.ballThrow} {
				if !strings.Contains(body, s) {
					t.Errorf("[%s] capture index missing localized discipline %q: %s", loc, s, body)
				}
			}
			for _, s := range []string{englishSixtyM, englishZoneLJ, englishBallThr} {
				if strings.Contains(body, s) {
					t.Errorf("[%s] capture index still renders the English catalog name %q", loc, s)
				}
			}
		})
	}
}

// TestCaptureUnitTitleLocalizedSYS111 covers F6: a capture unit page's
// <title>/<h1> ("… — Zone Long Jump (UKC)") previously used the catalog's
// English name regardless of locale; captureView now localizes it like
// the vertical-jump capture page already did.
func TestCaptureUnitTitleLocalizedSYS111(t *testing.T) {
	for _, loc := range []string{"de", "fr"} {
		t.Run(loc, func(t *testing.T) {
			deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
			client, base := newTestClient(t, deps)
			setupAndLogin(t, client, base)
			switchLocale(t, client, base, loc)

			meetID, units := ukcCaptureFixture(t, client, base)
			want := discLocalized[loc]
			unitID := units[want.zoneLJ]
			if unitID == "" {
				t.Fatalf("[%s] capture index did not expose a %q unit: %v", loc, want.zoneLJ, units)
			}
			body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/capture/"+unitID))

			if !strings.Contains(body, `<h1 class="capture-title">`) || !strings.Contains(body, want.zoneLJ) {
				t.Errorf("[%s] capture unit page missing localized h1 %q: %s", loc, want.zoneLJ, body)
			}
			if !strings.Contains(body, "<title>"+"UBS Kids Cup Le Mouret 2026 — "+want.zoneLJ) {
				t.Errorf("[%s] capture unit page <title> is not %q: %s", loc, "UBS Kids Cup Le Mouret 2026 — "+want.zoneLJ, body)
			}
			if strings.Contains(body, englishZoneLJ) {
				t.Errorf("[%s] capture unit page still renders the English catalog name %q", loc, englishZoneLJ)
			}
		})
	}
}

// TestMeetHubProgrammeAndTimetableLocalizedDisciplineSYS111 covers F6: the
// meet hub's Programme table (the operator's event list) and Units/
// timetable table previously showed the catalog's canonical English
// discipline name in their "Disziplin" column while standings already
// localized — unitRows and meetDetailView's Programme mapping now both
// reuse localizedDisciplineName.
func TestMeetHubProgrammeAndTimetableLocalizedDisciplineSYS111(t *testing.T) {
	for _, loc := range []string{"de", "fr"} {
		t.Run(loc, func(t *testing.T) {
			deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
			client, base := newTestClient(t, deps)
			setupAndLogin(t, client, base)
			switchLocale(t, client, base, loc)

			meetID, _ := ukcCaptureFixture(t, client, base)
			body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))

			// Scoped to the Programme/Units table cells (meets.templ renders
			// each as a bare `<td>…</td>`, no surrounding whitespace) rather
			// than the whole page body: the same page's "add programme
			// event" discipline `<select>` deliberately still lists every
			// catalog discipline by its canonical English name (a distinct,
			// out-of-scope control — TASK-050 owns the Programme/Units
			// table display, not that picker) and would otherwise produce a
			// false failure below.
			want := discLocalized[loc]
			for _, s := range []string{want.sixtyM, want.zoneLJ, want.ballThrow} {
				if !strings.Contains(body, "<td>"+s+"</td>") {
					t.Errorf("[%s] meet hub table missing localized discipline cell %q: %s", loc, s, body)
				}
			}
			for _, s := range []string{englishSixtyM, englishZoneLJ, englishBallThr} {
				if strings.Contains(body, "<td>"+s+"</td>") {
					t.Errorf("[%s] meet hub table still renders the English catalog name %q", loc, s)
				}
			}
		})
	}
}

// TestPublishedTimetableTimestampLocalNoRawUTCSuffixSYS110 covers F6's
// second finding: "Publizierte Versionen — Version 1 — publiziert am
// 06.08.2026 05:29 UTC" read as raw UTC on an operator surface. The meet
// hub's published-versions line (and the public timetable's mirror of the
// same PublishedAt field) now render PageData.FormatDateTime's local-zone,
// no-suffix format instead.
func TestPublishedTimetableTimestampLocalNoRawUTCSuffixSYS110(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	meetID, _ := ukcCaptureFixture(t, client, base)
	meetPage := base + "/meets/" + meetID

	resp := postForm(t, client, meetPage, meetPage+"/timetable/publish", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish timetable = %d, want 303", resp.StatusCode)
	}

	hubBody := bodyString(t, mustGet(t, client, meetPage))
	if !strings.Contains(hubBody, "Version 1") {
		t.Fatalf("meet hub missing the published version row: %s", hubBody)
	}
	if strings.Contains(hubBody, "UTC") {
		t.Errorf("meet hub's published-version timestamp still carries a raw UTC suffix (SYS-110/F6): %s", hubBody)
	}

	anon, anonBase := newTestClient(t, deps)
	_ = anonBase
	pubBody := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/timetable"))
	if strings.Contains(pubBody, "UTC") {
		t.Errorf("public timetable's published-version timestamp still carries a raw UTC suffix (SYS-110/F6): %s", pubBody)
	}
}

// TestFaviconServedSYS116TASK050 covers F11: every page previously 404'd on
// favicon.ico (static.go's go:embed list is hand-enumerated, so an
// unlisted file silently 404s). The route now serves a neutral, brand-free
// placeholder icon both at the conventional root path browsers probe
// unconditionally and under /static/, matching the <link rel="icon"> the
// base layout now carries.
func TestFaviconServedSYS116TASK050(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/favicon.ico", "/static/favicon.ico"} {
		resp := mustGet(t, client, base+path)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "image/") && !strings.Contains(ct, "icon") {
			t.Errorf("GET %s Content-Type = %q, want an image/icon type", path, ct)
		}
		if len(body) == 0 {
			t.Errorf("GET %s returned an empty body", path)
		}
	}
}
