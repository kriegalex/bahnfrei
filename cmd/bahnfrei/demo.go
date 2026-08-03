// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// Demo credentials, printed on seed so the founder can hand phones around
// at the club visit (DEC-011). Demo data only — never reuse for a real meet.
const (
	demoOrganizerUser = "organizer"
	demoOrganizerPass = "demo-organizer-pw"
	demoOfficeUser    = "office"
	demoOfficePass    = "demo-office-pw" // #nosec G101 -- printed demo-mode credential (DEC-011), not a real secret; documented above as never for real meets
	demoOfficialUser  = "official"
	demoOfficialPass  = "demo-official-pw"
)

// demoFlags holds the parsed "demo" subcommand flags.
type demoFlags struct {
	dataDir string
}

func parseDemoFlags(args []string, out io.Writer) (demoFlags, error) {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(out)
	dataDir := fs.String("data-dir", ".", "directory for the demo instance's database (must not already contain one)")
	if err := fs.Parse(args); err != nil {
		return demoFlags{}, err
	}
	return demoFlags{dataDir: *dataDir}, nil
}

// runDemo implements the "demo" subcommand (TASK-015, DEC-011): on a fresh
// data directory it seeds a ready-to-demo UBS Kids Cup meet — template
// meet with a published timetable, a 30-athlete Swiss youth roster across
// six divisions, pre-captured results so standings and the public results
// page are non-empty, and one account per demo role with the field
// official already assigned to two units. It refuses to touch a directory
// that already holds a database.
func runDemo(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := parseDemoFlags(args, out)
	if err != nil {
		return err
	}

	dbPath := filepath.Join(cfg.dataDir, "bahnfrei.db")
	if _, err := os.Stat(dbPath); err == nil {
		return fmt.Errorf("demo: %s already holds a database; refusing to seed a non-fresh data dir", cfg.dataDir)
	}
	if err := os.MkdirAll(cfg.dataDir, 0o750); err != nil {
		return fmt.Errorf("demo: create data dir: %w", err)
	}

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("demo: open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		return fmt.Errorf("demo: load discipline catalog: %w", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		return fmt.Errorf("demo: load category schemes: %w", err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		return fmt.Errorf("demo: load scoring tables: %w", err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		return fmt.Errorf("demo: load meet templates: %w", err)
	}

	sessions := app.NewSessionManager(time.Hour) // seeding only; "serve" wires its own
	auth := app.NewAuthService(st.DB(), sessions, app.DefaultPasswordParams)
	meets := app.NewMeetService(st.DB(), catalog, schemes, tables, templates)
	results := app.NewResultsService(st.DB(), catalog, schemes, tables, templates)

	meetID, captured, err := seedDemo(ctx, auth, meets, results)
	if err != nil {
		return fmt.Errorf("demo: %w", err)
	}

	fmt.Fprintf(out, "bahnfrei: demo meet seeded in %s\n", cfg.dataDir)
	fmt.Fprintf(out, "  meet id:   %s\n", meetID)
	fmt.Fprintf(out, "  roster:    %d athletes, 6 divisions (M/W 10-12), 4 clubs\n", len(demoRoster))
	fmt.Fprintf(out, "  results:   %d pre-captured marks (60 m, zone long jump, ball throw)\n", captured)
	fmt.Fprintln(out, "  accounts (demo credentials — never reuse for a real meet):")
	fmt.Fprintf(out, "    %-10s %-20s instance admin (runs meet setup in the demo)\n", demoOrganizerUser, demoOrganizerPass)
	fmt.Fprintf(out, "    %-10s %-20s competition office\n", demoOfficeUser, demoOfficePass)
	fmt.Fprintf(out, "    %-10s %-20s field official, assigned: 60 m + zone long jump\n", demoOfficialUser, demoOfficialPass)
	fmt.Fprintf(out, "  start:     bahnfrei serve --data-dir %s\n", cfg.dataDir)
	fmt.Fprintf(out, "  operator:  https://<host>/meets/%s\n", meetID)
	fmt.Fprintf(out, "  public:    https://<host>/m/%s/results\n", meetID)
	return nil
}

// demoAthlete is one seeded roster line with its pre-captured marks; an
// empty mark means the discipline is not yet captured — the standings then
// show the gap explicitly (UC-033 #3), which is a demo talking point.
type demoAthlete struct {
	first, last string
	sex         domain.Sex
	age         int // meet-year age; birth year = meet year - age (UKC scheme)
	club        string
	bib         string
	sprint      string // 60m, electronic, s
	longJump    string // ZoneLJ, m
	ball        string // BallThrow200g, m
}

// demoRoster is the seeded Swiss youth roster: 30 athletes with common
// German- and French-Swiss names, spread over M/W 10–12 and four clubs.
// Every athlete has a 60 m mark, most a long jump, two thirds a ball throw.
// Marks stay inside the ranges the UKC scoring-table fixtures prove
// (60 m 8.42–10.00, ZoneLJ 3.00–4.20, ball 20.00–38.50).
var demoRoster = []demoAthlete{
	// M10
	{"Luca", "Brunner", domain.SexMale, 10, "TV Uster", "101", "9.61", "3.42", "26.50"},
	{"Jonas", "Keller", domain.SexMale, 10, "LC Zürich", "102", "9.48", "3.51", "28.00"},
	{"Théo", "Rochat", domain.SexMale, 10, "CA Fribourg", "103", "9.74", "3.35", ""},
	{"Nino", "Steiner", domain.SexMale, 10, "TV Uster", "104", "9.87", "3.28", "24.50"},
	{"Loïc", "Berger", domain.SexMale, 10, "US Yverdon", "105", "9.55", "3.47", ""},
	// M11
	{"Elias", "Meier", domain.SexMale, 11, "LC Zürich", "106", "9.22", "3.68", "30.50"},
	{"Julien", "Favre", domain.SexMale, 11, "CA Fribourg", "107", "9.35", "3.61", "29.00"},
	{"Ben", "Huber", domain.SexMale, 11, "TV Uster", "108", "9.41", "3.55", ""},
	{"Matteo", "Weber", domain.SexMale, 11, "LC Zürich", "109", "9.29", "", "31.50"},
	{"Nathan", "Bonvin", domain.SexMale, 11, "US Yverdon", "110", "9.50", "3.49", "27.50"},
	// M12
	{"Noah", "Bachmann", domain.SexMale, 12, "TV Uster", "111", "8.86", "4.05", "35.00"},
	{"Samuel", "Roth", domain.SexMale, 12, "LC Zürich", "112", "8.95", "3.92", "33.50"},
	{"Quentin", "Maillard", domain.SexMale, 12, "CA Fribourg", "113", "9.04", "3.88", ""},
	{"Tim", "Frei", domain.SexMale, 12, "TV Uster", "114", "9.12", "3.79", "32.00"},
	{"Yannick", "Perret", domain.SexMale, 12, "US Yverdon", "115", "8.90", "4.11", "36.50"},
	// W10
	{"Mia", "Baumann", domain.SexFemale, 10, "TV Uster", "116", "9.78", "3.22", "22.00"},
	{"Elena", "Suter", domain.SexFemale, 10, "LC Zürich", "117", "9.85", "3.18", ""},
	{"Chloé", "Ducret", domain.SexFemale, 10, "CA Fribourg", "118", "9.70", "3.30", "23.50"},
	{"Lina", "Graf", domain.SexFemale, 10, "TV Uster", "119", "9.94", "3.10", ""},
	{"Camille", "Rossier", domain.SexFemale, 10, "US Yverdon", "120", "9.66", "3.26", "21.50"},
	// W11
	{"Anna", "Widmer", domain.SexFemale, 11, "LC Zürich", "121", "9.44", "3.52", "25.00"},
	{"Léa", "Morand", domain.SexFemale, 11, "CA Fribourg", "122", "9.52", "3.45", "24.00"},
	{"Sara", "Kunz", domain.SexFemale, 11, "TV Uster", "123", "9.60", "3.40", ""},
	{"Nora", "Zbinden", domain.SexFemale, 11, "LC Zürich", "124", "9.47", "", "26.00"},
	{"Manon", "Chevalley", domain.SexFemale, 11, "US Yverdon", "125", "9.58", "3.38", ""},
	// W12
	{"Julia", "Moser", domain.SexFemale, 12, "TV Uster", "126", "8.42", "4.12", "38.50"},
	{"Emma", "Gerber", domain.SexFemale, 12, "LC Zürich", "127", "9.08", "3.85", "30.00"},
	{"Zoé", "Pittet", domain.SexFemale, 12, "CA Fribourg", "128", "9.15", "3.80", ""},
	{"Livia", "Hofer", domain.SexFemale, 12, "TV Uster", "129", "9.20", "3.72", "28.50"},
	{"Alice", "Rey", domain.SexFemale, 12, "US Yverdon", "130", "9.02", "3.90", "31.00"},
}

// demoUnitSchedule is the demo timetable: one unit per UKC discipline.
var demoUnitSchedule = map[string]struct {
	hour, minute int
	location     string
}{
	"60m":           {9, 30, "Bahn A"},
	"ZoneLJ":        {10, 30, "Anlage 1"},
	"BallThrow200g": {11, 30, "Wurfplatz"},
}

// seedDemo provisions accounts, the template meet, the published
// timetable, the roster, the pre-captured results and the field-official
// unit assignments — all through the same app-service paths the web UI
// calls, so the seeded state is exactly what operating the UI produces.
// It returns the meet ID and the number of captured marks.
func seedDemo(ctx context.Context, auth *app.AuthService, meets *app.MeetService, results *app.ResultsService) (string, int, error) {
	// Accounts. Bootstrap provisions the first (instance-admin) account —
	// the same first-run path as the /setup flow; it doubles as the
	// demo's "organizer" login. The two other roles are created by it.
	if _, err := auth.Bootstrap(ctx, demoOrganizerUser, "Demo Organizer", demoOrganizerPass); err != nil {
		return "", 0, fmt.Errorf("bootstrap organizer: %w", err)
	}
	organizer, err := auth.Login(ctx, demoOrganizerUser, demoOrganizerPass)
	if err != nil {
		return "", 0, fmt.Errorf("login organizer: %w", err)
	}
	if _, err := auth.CreateAccount(ctx, organizer, app.CreateAccountRequest{
		Username: demoOfficeUser, DisplayName: "Demo Competition Office",
		Password: demoOfficePass, Role: app.RoleCompetitionOffice,
	}); err != nil {
		return "", 0, fmt.Errorf("create office account: %w", err)
	}
	officialAcct, err := auth.CreateAccount(ctx, organizer, app.CreateAccountRequest{
		Username: demoOfficialUser, DisplayName: "Demo Field Official",
		Password: demoOfficialPass, Role: app.RoleFieldOfficial,
	})
	if err != nil {
		return "", 0, fmt.Errorf("create field-official account: %w", err)
	}
	office, err := auth.Login(ctx, demoOfficeUser, demoOfficePass)
	if err != nil {
		return "", 0, fmt.Errorf("login office: %w", err)
	}

	// The meet: the built-in UBS Kids Cup template (UC-033 #1), dated
	// today so the demo reads as a live meet day.
	now := time.Now()
	meetDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	meet, err := meets.CreateMeetFromTemplate(ctx, organizer, app.TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup,
		Name:       "UBS Kids Cup Uster (Demo)",
		Venue:      "Stadion Buchholz, Uster",
		Date:       meetDay,
	})
	if err != nil {
		return "", 0, fmt.Errorf("create meet from template: %w", err)
	}

	// Timetable: schedule each discipline's unit, then publish (SYS-004)
	// so the public timetable page is live.
	detail, err := meets.Meet(ctx, meet.ID)
	if err != nil {
		return "", 0, fmt.Errorf("load meet: %w", err)
	}
	unitByDiscipline := map[string]string{}
	for _, u := range detail.Units {
		unitByDiscipline[u.DisciplineCode] = u.UnitID
		slot, ok := demoUnitSchedule[u.DisciplineCode]
		if !ok {
			continue
		}
		at := meetDay.Add(time.Duration(slot.hour)*time.Hour + time.Duration(slot.minute)*time.Minute)
		if err := meets.ScheduleUnit(ctx, organizer, u.UnitID, u.UnitVersion, at, slot.location); err != nil {
			return "", 0, fmt.Errorf("schedule %s unit: %w", u.DisciplineCode, err)
		}
	}
	if _, err := meets.PublishTimetable(ctx, organizer, meet.ID); err != nil {
		return "", 0, fmt.Errorf("publish timetable: %w", err)
	}

	// Roster and pre-captured results, via the same office flows the
	// roster and capture UIs call (RegisterParticipant / SaveResult).
	captured := 0
	for _, a := range demoRoster {
		p, err := results.RegisterParticipant(ctx, office, meet.ID, app.ParticipantInput{
			FirstName: a.first, LastName: a.last,
			BirthYear: meetDay.Year() - a.age,
			Sex:       a.sex, Club: a.club, Bib: a.bib,
		})
		if err != nil {
			return "", 0, fmt.Errorf("register %s %s: %w", a.first, a.last, err)
		}
		var marks []app.ResultInput
		if a.sprint != "" {
			marks = append(marks, app.ResultInput{
				AthleteID: p.AthleteID, DisciplineCode: "60m",
				Mark: a.sprint, Timing: domain.TimingElectronic,
			})
		}
		if a.longJump != "" {
			marks = append(marks, app.ResultInput{AthleteID: p.AthleteID, DisciplineCode: "ZoneLJ", Mark: a.longJump})
		}
		if a.ball != "" {
			marks = append(marks, app.ResultInput{AthleteID: p.AthleteID, DisciplineCode: "BallThrow200g", Mark: a.ball})
		}
		for _, in := range marks {
			if _, err := results.SaveResult(ctx, office, meet.ID, in); err != nil {
				return "", 0, fmt.Errorf("save %s %s for %s %s: %w", in.DisciplineCode, in.Mark, a.first, a.last, err)
			}
			captured++
		}
	}

	// Field-official scoping (SYS-090, TASK-013): the demo official is
	// assigned the 60 m and zone long jump units, so the phone-capture
	// part of the demo runs under real per-event scoping.
	for _, disc := range []string{"60m", "ZoneLJ"} {
		unitID, ok := unitByDiscipline[disc]
		if !ok {
			return "", 0, fmt.Errorf("template meet has no %s unit", disc)
		}
		if err := results.AssignFieldOfficialUnit(ctx, office, meet.ID, unitID, officialAcct.ID); err != nil {
			return "", 0, fmt.Errorf("assign official to %s: %w", disc, err)
		}
	}

	return meet.ID, captured, nil
}
