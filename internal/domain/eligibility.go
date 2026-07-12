// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"fmt"
	"time"
)

// EligibilityOutcome is the severity of an entry's eligibility evaluation
// (SYS-014, UC-005): eligible entries need no action; warning entries are
// informational (e.g. an unlicensed C-Meeting entry, whose results are
// simply excluded from ranking lists); blocked entries must be flagged
// before start-list generation and require an authorized override to
// proceed (UC-005 #4).
type EligibilityOutcome string

const (
	EligibilityEligible EligibilityOutcome = "eligible"
	EligibilityWarning  EligibilityOutcome = "warning"
	EligibilityBlocked  EligibilityOutcome = "blocked"
)

// worse returns the more severe of two outcomes (blocked > warning >
// eligible).
func (o EligibilityOutcome) worse(other EligibilityOutcome) EligibilityOutcome {
	rank := map[EligibilityOutcome]int{EligibilityEligible: 0, EligibilityWarning: 1, EligibilityBlocked: 2}
	if rank[other] > rank[o] {
		return other
	}
	return o
}

// Eligibility flag codes (SYS-014, UC-005 #1–#3): stable identifiers a UI or
// export can key display/i18n off, distinct from the free-text Reason.
const (
	FlagCategoryUnresolved = "category_unresolved"
	FlagUnknownCategory    = "unknown_category"
	FlagCategoryMismatch   = "category_mismatch"
	FlagBarredDiscipline   = "barred_discipline"
	FlagYouthMaxDistance   = "youth_max_distance"
	FlagYouthOneRacePerDay = "youth_one_race_per_day"
	FlagLicenceMissing     = "licence_missing"
	FlagLicenceMissingWarn = "licence_missing_unranked"
)

// EligibilityFlag is one violation or warning surfaced by EvaluateEligibility
// (UC-005 #1–#3), carrying a stable Code, its severity and a human-readable
// Reason with rule citation where one applies.
type EligibilityFlag struct {
	Code     string
	Outcome  EligibilityOutcome
	Reason   string
	Citation string
}

// EligibilityInput is everything EvaluateEligibility needs to decide one
// entry's eligibility (SYS-014): the athlete's identity, the event's target
// category and discipline, the meet's licence-requiring tier, and (since the
// "no race ≥600m/day" check, UC-005 #3, needs cross-entry context the domain
// layer has no store access to) the distances of the athlete's other active
// entries the app layer has already gathered.
type EligibilityInput struct {
	BirthYear          int
	Sex                Sex
	TargetCategoryCode string
	DisciplineCode     string
	HasLicence         bool
	MeetTier           MeetTier
	AsOf               time.Time
	// OtherRaceDistancesM are the track-race distances (metres) of the
	// athlete's other active entries considered for the one-race-per-day cap
	// (UC-005 #3) — see RequiresLicence/EvaluateEligibility doc for the
	// "same meet, not same day" approximation this ships with (OQ-031).
	OtherRaceDistancesM []int
}

// EligibilityResult is the outcome of EvaluateEligibility: the worst flag's
// outcome, every flag raised, and the category start-up/down decision
// (mirrors CategoryScheme.EvaluateEntry's EntryDecision for callers that
// also want that detail).
type EligibilityResult struct {
	Outcome     EligibilityOutcome
	Flags       []EligibilityFlag
	StartedUp   bool
	StartedDown bool
}

// addFlag appends f and widens the result's overall Outcome to at least
// f.Outcome.
func (r *EligibilityResult) addFlag(f EligibilityFlag) {
	r.Flags = append(r.Flags, f)
	r.Outcome = r.Outcome.worse(f.Outcome)
}

// RequiresLicence reports whether tier is one of the Swiss Athletics tiers
// that mandate a licence for club athletes (D1.2/D3.1): A-Meeting and
// B-Meeting. C-Meeting permits unlicensed athletes to start — their results
// are simply excluded from ranking lists (a warning, not a block). Any other
// (custom/unsanctioned) tier string is treated like C-Meeting: never
// blocking, since it opts out of the Swiss sanctioning ladder entirely.
func RequiresLicence(tier MeetTier) bool {
	return tier == "A-Meeting" || tier == "B-Meeting"
}

// EvaluateEligibility decides one entry's eligibility outcome against
// scheme/catalog (SYS-014, UC-005 #1–#3): category resolution and
// start-up/down rules, licence requirement for the meet tier, youth-
// protection max stadium distance, and the max-one-race-at/above-600m-per-
// day cap for U10–U14 (D4.3). It is a pure function — every fact it needs is
// in EligibilityInput — so the app layer owns all storage/cross-entry
// lookups and this stays independently unit-testable against the D4.2/D4.3
// reference fixtures (SYS-142).
func EvaluateEligibility(scheme *CategoryScheme, catalog *DisciplineCatalog, in EligibilityInput) EligibilityResult {
	var res EligibilityResult
	res.Outcome = EligibilityEligible

	athleteDefault, err := scheme.ResolveDefaultCategory(in.BirthYear, in.Sex, in.AsOf)
	if err != nil {
		res.addFlag(EligibilityFlag{
			Code: FlagCategoryUnresolved, Outcome: EligibilityBlocked,
			Reason: fmt.Sprintf("no default category resolves for this athlete: %v", err),
		})
		return res
	}

	target, ok := scheme.CategoryByCode(in.TargetCategoryCode)
	if !ok {
		res.addFlag(EligibilityFlag{
			Code: FlagUnknownCategory, Outcome: EligibilityBlocked,
			Reason: fmt.Sprintf("event category %q is not defined by scheme %q", in.TargetCategoryCode, scheme.ID),
		})
		return res
	}
	if catalog != nil {
		if _, ok := catalog.ByCode(in.DisciplineCode); !ok {
			res.addFlag(EligibilityFlag{
				Code: FlagUnknownCategory, Outcome: EligibilityBlocked,
				Reason: fmt.Sprintf("discipline %q is not defined by the catalog", in.DisciplineCode),
			})
			return res
		}
	}

	decision := scheme.EvaluateEntry(athleteDefault, target)
	if !decision.Allowed {
		res.addFlag(EligibilityFlag{Code: FlagCategoryMismatch, Outcome: EligibilityBlocked, Reason: decision.Reason})
	}
	res.StartedUp = decision.StartedUp
	res.StartedDown = decision.StartedDown

	// Discipline restrictions and youth-protection bands apply per the
	// athlete's own (default) category — CheckDisciplineEligibility's doc
	// comment: "applies regardless of which event category the athlete is
	// entered under."
	if ok, citation := scheme.CheckDisciplineEligibility(in.DisciplineCode, athleteDefault.Code); !ok {
		res.addFlag(EligibilityFlag{
			Code: FlagBarredDiscipline, Outcome: EligibilityBlocked,
			Reason: fmt.Sprintf("%s is barred for category %s", in.DisciplineCode, athleteDefault.Code), Citation: citation,
		})
	}

	if rule, ok := scheme.YouthProtectionFor(athleteDefault.Code); ok {
		if dist, distOK := TrackDistanceMeters(in.DisciplineCode); distOK {
			if rule.MaxStadiumDistanceM > 0 && dist > rule.MaxStadiumDistanceM {
				res.addFlag(EligibilityFlag{
					Code: FlagYouthMaxDistance, Outcome: EligibilityBlocked,
					Reason:   fmt.Sprintf("%s (%dm) exceeds the %dm youth-protection max for category %s", in.DisciplineCode, dist, rule.MaxStadiumDistanceM, athleteDefault.Code),
					Citation: rule.Citation,
				})
			}
			if rule.MaxOneRaceAtOrAboveM > 0 && dist >= rule.MaxOneRaceAtOrAboveM {
				for _, other := range in.OtherRaceDistancesM {
					if other >= rule.MaxOneRaceAtOrAboveM {
						res.addFlag(EligibilityFlag{
							Code: FlagYouthOneRacePerDay, Outcome: EligibilityBlocked,
							Reason:   fmt.Sprintf("category %s may run at most one race of %dm or longer per competition day", athleteDefault.Code, rule.MaxOneRaceAtOrAboveM),
							Citation: rule.Citation,
						})
						break
					}
				}
			}
		}
	}

	if !in.HasLicence {
		if RequiresLicence(in.MeetTier) {
			res.addFlag(EligibilityFlag{
				Code: FlagLicenceMissing, Outcome: EligibilityBlocked,
				Reason: fmt.Sprintf("a Swiss Athletics licence is required to start at a %s (D3.1)", in.MeetTier),
			})
		} else {
			res.addFlag(EligibilityFlag{
				Code: FlagLicenceMissingWarn, Outcome: EligibilityWarning,
				Reason: "unlicensed: results are excluded from Swiss Athletics ranking lists (D1.2)",
			})
		}
	}

	return res
}
