// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "time"

// --- privacy: public data minimization and consent enforcement (TASK-023,
// SYS-100/SYS-103, UC-023). These are the single, central, tested
// functions every public/publication-intended renderer must call before it
// puts an athlete's identity on a page — never per-page ad-hoc logic
// (UC-023 #1). See internal/app/privacy.go for the data-subject-rights
// (export/erasure) and retention-purge services (SYS-101/SYS-102,
// UC-024), which operate on stored data rather than render-time views. ---

// PublicSuppressedMarker is the neutral placeholder every public/
// publication-intended surface renders in place of an athlete's name or
// club when SYS-103 consent enforcement suppresses their identity
// (UC-023 #2: "suppressed/neutralized"). This system implements the
// "neutralize" legal mode only (see OQ-043): the row itself — bib, rank,
// marks, total — is kept so a division's ranking stays intact and its row
// count never leaks who was suppressed by elimination (a fully omitted row
// would itself be an inference side-channel). It intentionally matches the
// existing missing-discipline gap marker (web.markOrGap's "–") so the two
// privacy-preserving blanks a viewer encounters read consistently, and is
// deliberately not an i18n key: a translated placeholder risks looking like
// a plausible real name in some locale, which a fixed non-lingual symbol
// cannot.
const PublicSuppressedMarker = "—"

// PublicDisplayNameFor returns the name a public/publication-intended
// surface may render for a person with the given consent flags: the real
// first/last name normally, or PublicSuppressedMarker/"" when
// ResultsPublicationWithdrawn is set (SYS-103, UC-023 #2). It takes the
// flags and name as plain values (not an Athlete) so it works uniformly
// whether the caller holds a domain.Athlete directly (public start lists)
// or a flattened projection like app.StandingRow (public results, printed
// result lists) that copies PublicationConsent alongside the name fields
// it needs it for.
func PublicDisplayNameFor(consent PublicationConsent, firstName, lastName string) (string, string) {
	if consent.ResultsPublicationWithdrawn {
		return PublicSuppressedMarker, ""
	}
	return firstName, lastName
}

// PublicDisplayClubFor mirrors PublicDisplayNameFor for the club/team
// field: a withdrawn athlete's club is also neutralized, since club
// affiliation combined with a division/category can narrow identity almost
// as much as a name at a small club.
func PublicDisplayClubFor(consent PublicationConsent, club string) string {
	if consent.ResultsPublicationWithdrawn {
		return PublicSuppressedMarker
	}
	return club
}

// anonymizedFirstName is the fixed, non-reversible pseudonym first name
// AnonymizePersonalData assigns. The last name it pairs with incorporates a
// suffix of the athlete's own opaque ID (never derived from the erased
// name) purely to keep otherwise-identical rows visually distinguishable
// in office tooling — it carries no information about the erased identity.
const anonymizedFirstName = "Anonymized"

// AnonymizePersonalData implements the SYS-101 erasure/pseudonymization
// request (UC-024 #2): it clears the fields that identify the person while
// preserving what the sporting record needs to stay valid and correctly
// categorized:
//   - FirstName/LastName become a stable, non-reversible pseudonym.
//   - BirthDate (the full date) is cleared; BirthYear is KEPT — category
//     and eligibility resolution (domain.CategoryScheme.ResolveDefaultCategory)
//     re-derives a participant's division from BirthYear at render time on
//     every Standings() call (internal/app/results.go), so clearing it
//     would silently corrupt historical standings, not just hide a date.
//   - ExternalIDs (licence numbers, federation athlete IDs) are cleared:
//     SYS-100 already forbids ever showing these publicly, and internally
//     they exist only to link this person to federation systems — exactly
//     the linkage erasure ends.
//   - Sex, Nationality and ClubIDs are KEPT: SYS-100 lists nationality
//     among the fields a public result may legitimately show, sex drives
//     category/scoring, and club-level team results depend on ClubIDs —
//     none of the three are, on their own, an identifying token the way a
//     name or licence number is (see OQ-043 for the residual small-sample
//     re-identification risk this accepts).
//
// The caller (internal/app/privacy.go PrivacyService.EraseAthlete) is
// responsible for auditing the action WITHOUT embedding the pre-erasure
// name in the audit row's before/after JSON — see that file's comment for
// why (the audit log is append-only; embedding the erased name there would
// permanently defeat the erasure it documents).
func (a *Athlete) AnonymizePersonalData(now time.Time) {
	a.FirstName = anonymizedFirstName
	a.LastName = "Athlete-" + shortIDSuffix(a.ID)
	a.BirthDate = nil
	a.ExternalIDs = ExternalIDs{}
	a.Anonymized = true
	t := now
	a.AnonymizedAt = &t
}

// shortIDSuffix returns the trailing characters of an opaque ULID-style ID
// (falling back to the whole string if shorter) — just enough to keep
// otherwise-identical anonymized rows visually distinguishable; it is a
// substring of the athlete's own existing ID, never a hash or transform of
// the erased name, so it carries no information the ID itself didn't
// already carry.
func shortIDSuffix(id string) string {
	const n = 6
	if len(id) <= n {
		return id
	}
	return id[len(id)-n:]
}
