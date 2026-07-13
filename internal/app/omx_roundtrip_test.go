// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/exchange"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// TestUC027_2_OMXRoundTripProperty is TASK-025's fixed-seed, reproducible
// round-trip property test (UC-027 #2, SYS-073): a randomly generated full
// meet — athletes, clubs, events/rounds/units, entries, unit assignments,
// results across the CR 25 status vocabulary with wind, points, placing,
// record flags, in-session corrections and unit announcements — is
// exported to omx/v1 (validated against the published schema), decoded,
// and re-imported into a brand-new store; the official results reconstruct
// equivalently. Equivalence is judged by CONTENT (discipline code,
// category codes, athlete natural key) rather than literal storage id,
// since re-importing into a fresh system always assigns fresh ids
// (exchange.Document's own doc comment; ADR-005 §2).
//
// Each iteration runs as its own subtest named by seed, so a failure's
// exact seed is directly reproducible:
//
//	go test ./internal/app/... -run 'TestUC027_2_OMXRoundTripProperty/seed=<N>'
//
// Iteration count and base seed are overridable (OMX_PROPERTY_ITERATIONS,
// OMX_PROPERTY_SEED) for heavier ad hoc fuzzing; the shipped defaults keep
// CI runtime well under the ~2 minute budget (each iteration opens two
// on-disk SQLite stores, migrations included).
func TestUC027_2_OMXRoundTripProperty(t *testing.T) {
	iterations := 15
	if v := os.Getenv("OMX_PROPERTY_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}
	baseSeed := int64(20260713)
	if v := os.Getenv("OMX_PROPERTY_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			baseSeed = n
		}
	}
	for i := 0; i < iterations; i++ {
		seed := baseSeed + int64(i)
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			runOMXRoundTripCase(t, seed)
		})
	}
}

func runOMXRoundTripCase(t *testing.T, seed int64) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	ctx := context.Background()

	src := openFreshStore(t)
	meetID := generateOMXFixtureMeet(t, src, rng)

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	doc1, err := BuildOMXDocument(ctx, src.DB(), meetID, now)
	if err != nil {
		t.Fatalf("seed %d: BuildOMXDocument: %v", seed, err)
	}

	// UC-027 #1: the JSON export validates against the published schema.
	data, err := exchange.EncodeOMX(doc1)
	if err != nil {
		t.Fatalf("seed %d: EncodeOMX: %v", seed, err)
	}
	if err := exchange.ValidateOMXSchema(data); err != nil {
		t.Fatalf("seed %d: ValidateOMXSchema: %v", seed, err)
	}
	// The CSV derivative must also render without error over the same data
	// (SYS-073's "CSV and a self-describing structured format").
	if csv := exchange.EncodeOMXResultsCSV(doc1); len(csv) == 0 {
		t.Fatalf("seed %d: EncodeOMXResultsCSV produced no output", seed)
	}

	decoded, err := exchange.DecodeOMX(data)
	if err != nil {
		t.Fatalf("seed %d: DecodeOMX: %v", seed, err)
	}

	// UC-027 #2: re-import into a FRESH system.
	dst := openFreshStore(t)
	newMeetID, err := ImportOMXDocument(ctx, dst.DB(), decoded)
	if err != nil {
		t.Fatalf("seed %d: ImportOMXDocument: %v", seed, err)
	}
	doc2, err := BuildOMXDocument(ctx, dst.DB(), newMeetID, now)
	if err != nil {
		t.Fatalf("seed %d: BuildOMXDocument(reimported): %v", seed, err)
	}

	want := flattenOfficialResults(doc1)
	got := flattenOfficialResults(doc2)
	if len(want) == 0 {
		t.Fatalf("seed %d: generator produced zero results — test is not exercising anything", seed)
	}
	if len(want) != len(got) {
		t.Fatalf("seed %d: %d official results before round-trip, %d after — count must be preserved (nothing lost or duplicated)", seed, len(want), len(got))
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok {
			t.Errorf("seed %d: result %q missing after round-trip", seed, key)
			continue
		}
		if w != g {
			t.Errorf("seed %d: result %q = %+v after round-trip, want %+v", seed, key, g, w)
		}
	}
}

func openFreshStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// resultFingerprint is the content-only (no ids) shape a round-trip
// compares — every field UC-027 #1 names ("statuses, wind, categories,
// record flags") plus mark/points/placing/lane/timing.
type resultFingerprint struct {
	Lane         int
	Mark         string
	Timing       string
	Wind         string
	Status       string
	StatusDetail string
	Points       string
	Placing      string
	RecordFlags  string
}

func fingerprintOf(r exchange.ResultDoc) resultFingerprint {
	wind := ""
	if r.Wind != nil {
		wind = strconv.FormatFloat(*r.Wind, 'f', -1, 64)
	}
	points, placing := "", ""
	if r.Points != nil {
		points = strconv.Itoa(*r.Points)
	}
	if r.Placing != nil {
		placing = strconv.Itoa(*r.Placing)
	}
	flags := append([]string(nil), r.RecordFlags...)
	sort.Strings(flags)
	return resultFingerprint{
		Lane: r.Lane, Mark: r.Mark, Timing: r.Timing, Wind: wind,
		Status: r.Status, StatusDetail: r.StatusDetail, Points: points, Placing: placing,
		RecordFlags: strings.Join(flags, ","),
	}
}

func athleteNaturalKey(a exchange.AthleteDoc) string {
	return strings.ToLower(a.LastName) + "|" + strings.ToLower(a.FirstName) + "|" +
		strconv.Itoa(a.BirthYear) + "|" + a.Sex
}

// flattenOfficialResults resolves every result to its (discipline,
// categories, athlete) content key, independent of any storage id — the
// comparable shape a round-trip through fresh ids must preserve.
func flattenOfficialResults(doc exchange.Document) map[string]resultFingerprint {
	events := make(map[string]exchange.EventDoc, len(doc.Events))
	for _, e := range doc.Events {
		events[e.ID] = e
	}
	rounds := make(map[string]exchange.RoundDoc, len(doc.Rounds))
	for _, r := range doc.Rounds {
		rounds[r.ID] = r
	}
	units := make(map[string]exchange.UnitDoc, len(doc.Units))
	for _, u := range doc.Units {
		units[u.ID] = u
	}
	athletes := make(map[string]exchange.AthleteDoc, len(doc.Athletes))
	for _, a := range doc.Athletes {
		athletes[a.ID] = a
	}

	out := make(map[string]resultFingerprint, len(doc.Results))
	for _, r := range doc.Results {
		u := units[r.UnitID]
		rd := rounds[u.RoundID]
		ev := events[rd.EventID]
		ath := athletes[r.AthleteID]
		cats := append([]string(nil), ev.CategoryCodes...)
		sort.Strings(cats)
		key := ev.DisciplineCode + "|" + strings.Join(cats, ",") + "|" + athleteNaturalKey(ath)
		out[key] = fingerprintOf(r)
	}
	return out
}

var (
	omxFixtureTrackDisciplines = []struct {
		Code         string
		WindRelevant bool
	}{
		{"100m", true}, {"400m", false},
	}
	omxFixtureFieldDisciplines = []string{"LJ", "SP"}
	omxFixtureCategoryPool     = []string{"U16 M", "U16 W", "U18 M", "U18 W"}
	omxFixtureStatusPool       = []domain.QualificationStatus{
		domain.StatusNone, domain.StatusNone, domain.StatusNone, domain.StatusNone,
		domain.StatusDNS, domain.StatusDNF, domain.StatusDQ, domain.StatusNM,
	}
	omxFixtureFirstNames = []string{"Anna", "Bea", "Cara", "Dana", "Elin", "Fiona", "Gina", "Hana", "Ida", "Jana", "Kira", "Lena"}
	omxFixtureLastNames  = []string{"Muster", "Meier", "Keller", "Weber", "Huber", "Schmid", "Baumann", "Frei", "Steiner", "Kunz"}
)

// generateOMXFixtureMeet builds a randomized full meet directly against
// the real store API (Create*/Save* — never raw SQL), returning its meet
// id. Every entity BuildOMXDocument reads is exercised: clubs, athletes
// (with participants registered, as capture requires), events/rounds/units,
// entries, unit assignments, settled results across the CR 25 status
// vocabulary (some corrected in-session — only the FINAL state is an
// "official result", UC-027 #1), unit wind and announcements.
func generateOMXFixtureMeet(t *testing.T, s *store.Store, rng *rand.Rand) string {
	t.Helper()
	ctx := context.Background()
	db := s.DB()

	meetStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	meet, err := store.CreateMeet(ctx, db, domain.Meet{
		Name: "Property Meet", Venue: "Test Stadium", StartDate: meetStart, EndDate: meetStart,
		Status: domain.MeetClosed,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}

	numClubs := 2 + rng.Intn(2)
	clubIDs := make([]string, numClubs)
	for i := range clubIDs {
		c, err := store.CreateClub(ctx, db, domain.Club{Name: fmt.Sprintf("Club %d %d", i, rng.Int())})
		if err != nil {
			t.Fatalf("CreateClub: %v", err)
		}
		clubIDs[i] = c.ID
	}

	numAthletes := 10 + rng.Intn(10)
	athleteIDs := make([]string, numAthletes)
	for i := range athleteIDs {
		sex := domain.SexFemale
		if rng.Intn(2) == 0 {
			sex = domain.SexMale
		}
		a, err := store.CreateAthlete(ctx, db, domain.Athlete{
			FirstName: omxFixtureFirstNames[rng.Intn(len(omxFixtureFirstNames))],
			// Index-suffixed last name: keeps every athlete's natural key
			// (lastName/firstName/birthYear/sex) distinct even when the
			// random name pool repeats, since the round-trip equivalence
			// check is keyed on that natural key.
			LastName:  fmt.Sprintf("%s%d", omxFixtureLastNames[rng.Intn(len(omxFixtureLastNames))], i),
			BirthYear: 2005 + rng.Intn(11),
			Sex:       sex,
			ClubIDs:   []string{clubIDs[rng.Intn(numClubs)]},
		})
		if err != nil {
			t.Fatalf("CreateAthlete: %v", err)
		}
		athleteIDs[i] = a.ID
		if _, err := store.RegisterParticipant(ctx, db, meet.ID, a.ID, strconv.Itoa(100+i)); err != nil {
			t.Fatalf("RegisterParticipant: %v", err)
		}
	}

	numEvents := 3 + rng.Intn(3)
	for e := 0; e < numEvents; e++ {
		isTrack := rng.Intn(2) == 0
		var disc string
		var windRel bool
		if isTrack {
			d := omxFixtureTrackDisciplines[rng.Intn(len(omxFixtureTrackDisciplines))]
			disc, windRel = d.Code, d.WindRelevant
		} else {
			disc = omxFixtureFieldDisciplines[rng.Intn(len(omxFixtureFieldDisciplines))]
		}
		cat := omxFixtureCategoryPool[rng.Intn(len(omxFixtureCategoryPool))]
		ev, err := store.CreateEvent(ctx, db, domain.Event{
			MeetID: meet.ID, DisciplineCode: disc, CategoryCodes: []string{cat}, Status: domain.EventClosed,
		})
		if err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		round, err := store.CreateRound(ctx, db, domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
		if err != nil {
			t.Fatalf("CreateRound: %v", err)
		}

		numUnits := 1 + rng.Intn(2)
		for u := 0; u < numUnits; u++ {
			unit, err := store.CreateUnit(ctx, db, domain.Unit{RoundID: round.ID, Location: "Track"})
			if err != nil {
				t.Fatalf("CreateUnit: %v", err)
			}

			var wind *float64
			if windRel && rng.Intn(2) == 0 {
				w := float64(int((-2.0+rng.Float64()*6.0)*10)) / 10 // one decimal, matches capture precision
				wind = &w
				if err := store.SetUnitWind(ctx, db, unit.ID, w); err != nil {
					t.Fatalf("SetUnitWind: %v", err)
				}
			}

			numEntrants := 2 + rng.Intn(3)
			lane := 0
			for k := 0; k < numEntrants; k++ {
				athID := athleteIDs[rng.Intn(len(athleteIDs))]
				if _, err := store.GetEntryByEventAthlete(ctx, db, ev.ID, athID); err == nil {
					continue // this athlete already has an entry for this event
				}
				entry, err := store.CreateEntry(ctx, db, domain.Entry{EventID: ev.ID, AthleteID: athID, Status: domain.EntryConfirmed})
				if err != nil {
					if errors.Is(err, store.ErrDuplicateEntry) {
						continue
					}
					t.Fatalf("CreateEntry: %v", err)
				}
				lane++
				if _, err := store.SaveUnitAssignment(ctx, db, domain.UnitAssignment{
					UnitID: unit.ID, EntryID: entry.ID, SeedRank: lane, Lane: lane,
				}); err != nil {
					t.Fatalf("SaveUnitAssignment: %v", err)
				}

				if rng.Intn(10) == 0 {
					continue // some entrants are never captured (still just an entry)
				}

				status := omxFixtureStatusPool[rng.Intn(len(omxFixtureStatusPool))]
				result := domain.Result{UnitID: unit.ID, AthleteID: athID, Lane: lane, Status: status}
				if status == domain.StatusNone {
					result.Mark = randomMark(rng, isTrack)
					pts := 400 + rng.Intn(600)
					result.Points = &pts
					place := lane
					result.Placing = &place
				}
				if windRel && wind != nil {
					result.Wind = wind
				}
				if rng.Intn(4) == 0 {
					var flags []string
					if rng.Intn(2) == 0 {
						flags = append(flags, "PB")
					}
					if rng.Intn(2) == 0 {
						flags = append(flags, "SB")
					}
					result.RecordFlags = flags
				}
				timing := domain.TimingNone
				if isTrack && status == domain.StatusNone {
					timing = domain.TimingElectronic
				}
				if _, err := store.SaveResultWithSource(ctx, db, result, timing, "manual"); err != nil {
					t.Fatalf("SaveResultWithSource: %v", err)
				}
				// Simulate an in-session correction: only the FINAL saved
				// state is an "official result" (UC-027 #1) — corrections
				// history itself is not part of the omx/v1 contract
				// (OQ-056), it stays in the audit log.
				if status == domain.StatusNone && rng.Intn(3) == 0 {
					corrected := result
					corrected.Mark = randomMark(rng, isTrack)
					if _, err := store.SaveResultWithSource(ctx, db, corrected, timing, "manual"); err != nil {
						t.Fatalf("SaveResultWithSource(correction): %v", err)
					}
				}
			}
			if rng.Intn(2) == 0 {
				if _, _, err := store.AnnounceUnit(ctx, db, unit.ID, "office-test"); err != nil {
					t.Fatalf("AnnounceUnit: %v", err)
				}
			}
		}
	}
	return meet.ID
}

func randomMark(rng *rand.Rand, isTrack bool) string {
	if isTrack {
		return fmt.Sprintf("%d.%02d", 10+rng.Intn(20), rng.Intn(100))
	}
	return fmt.Sprintf("%d.%02d", 3+rng.Intn(15), rng.Intn(100))
}
