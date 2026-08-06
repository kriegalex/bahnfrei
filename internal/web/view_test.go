// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// TestFormatDateAndMarkNotationSYS110UC025_3 covers UC-025 #3 ("dates/
// numbers format per locale while performance-mark notation follows the
// documented project convention consistently in both languages"): dates
// render in the documented day.month.year display convention (SYS-110;
// distinct from the ISO wire format <input type="date"> requires) and a
// captured mark's decimal notation is unaffected by locale — the
// documented convention (see docs/requirements/open-questions-and-
// assumptions.md) keeps marks period-decimal in both DE and FR, matching
// the interchange formats (LIF/TAF3/World Athletics feeds) results flow
// through, deliberately distinct from prose numbers/dates which do vary
// by locale.
func TestFormatDateAndMarkNotationSYS110UC025_3(t *testing.T) {
	// Constructed already in time.Local (rather than UTC then relying on a
	// specific offset) so the expected wall-clock string holds regardless of
	// the machine's configured zone: FormatDateTime renders in the server's
	// local zone (F6/TASK-050 — see view.go's FormatDateTime doc comment),
	// not raw UTC, and no longer carries a "UTC" suffix.
	when := time.Date(2027, time.June, 12, 14, 30, 0, 0, time.Local)

	for _, loc := range []i18n.Locale{i18n.DE, i18n.FR} {
		p := renderPageData(t, loc)

		if got, want := p.FormatDate(when), "12.06.2027"; got != want {
			t.Errorf("locale %q: FormatDate = %q, want %q (SYS-110 documented date convention)", loc, got, want)
		}
		if got, want := p.FormatDateTime(when), "12.06.2027 14:30"; got != want {
			t.Errorf("locale %q: FormatDateTime = %q, want %q (no raw UTC suffix, SYS-110/F6)", loc, got, want)
		}
	}

	// Mark notation (domain.FormatCentiMark) takes no locale parameter at
	// all: it is the same function call regardless of which catalog is
	// active, so "consistent in both languages" holds by construction —
	// the assertion below pins the documented convention (period decimal)
	// so a future change can't silently diverge it from the incumbent's
	// interchange-format-driven rationale.
	if got, want := domain.FormatCentiMark(767, 2), "7.67"; got != want {
		t.Errorf("FormatCentiMark = %q, want %q (period-decimal mark-notation convention, SYS-110)", got, want)
	}
}
