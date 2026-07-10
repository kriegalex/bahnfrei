// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

func TestParseBackupFlagsRequiresOut(t *testing.T) {
	if _, err := parseBackupFlags(nil, io.Discard); err == nil {
		t.Error("parseBackupFlags with no --out: want error")
	}
	f, err := parseBackupFlags([]string{"--data-dir", "/tmp/x", "--out", "/tmp/x/b.db"}, io.Discard)
	if err != nil {
		t.Fatalf("parseBackupFlags: %v", err)
	}
	if f.dataDir != "/tmp/x" || f.out != "/tmp/x/b.db" {
		t.Errorf("parsed = %+v", f)
	}
}

func TestParseRestoreFlagsRequiresFrom(t *testing.T) {
	if _, err := parseRestoreFlags(nil, io.Discard); err == nil {
		t.Error("parseRestoreFlags with no --from: want error")
	}
}

// TestCLIBackupRestoreRoundTrip drives the "backup" and "restore"
// subcommands directly (no HTTP) — the scripted/cron-friendly path SYS-084
// also requires ("via ... CLI subcommand on the bahnfrei binary"):
// `bahnfrei backup` writes an artifact from a running instance's data
// directory, and `bahnfrei restore` refuses anything but a fresh
// (data-dir-less) target, then reproduces the meet.
func TestCLIBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	sourceDir := t.TempDir()

	deps, err := buildServer(mustParseServe(t, sourceDir))
	if err != nil {
		t.Fatal(err)
	}
	meets := app.NewMeetService(deps.dbase.DB(), mustCatalog(t), mustSchemes(t), mustTables(t), mustTemplates(t))
	organizerSession := app.Session{AccountID: "01ORG", Username: "orga", Role: app.RoleMeetOrganizer}
	if _, err := meets.CreateMeetFromTemplate(ctx, organizerSession, app.TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup, Venue: "Le Mouret",
		Date: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := deps.dbase.Close(); err != nil {
		t.Fatal(err)
	}

	artifact := filepath.Join(t.TempDir(), "backup.db")
	var out strings.Builder
	if err := runBackup(ctx, []string{"--data-dir", sourceDir, "--out", artifact}, &out); err != nil {
		t.Fatalf("runBackup: %v (output: %s)", err, out.String())
	}
	if !strings.Contains(out.String(), "meets:    1") {
		t.Errorf("backup output missing meet count: %s", out.String())
	}

	restoreDir := filepath.Join(t.TempDir(), "restored")
	out.Reset()
	if err := runRestore(ctx, []string{"--data-dir", restoreDir, "--from", artifact}, &out); err != nil {
		t.Fatalf("runRestore: %v (output: %s)", err, out.String())
	}
	if !strings.Contains(out.String(), "1 meet(s)") {
		t.Errorf("restore output missing meet count: %s", out.String())
	}

	// Restore refuses a second time onto the same (now non-fresh) data dir.
	out.Reset()
	err = runRestore(ctx, []string{"--data-dir", restoreDir, "--from", artifact}, &out)
	if err == nil || !strings.Contains(err.Error(), "restore refused") {
		t.Errorf("second restore onto non-fresh dir = %v, want a refusal", err)
	}
}

// httpProbe bundles a client and the base URL its requests target, and
// the shared GET/POST helpers the two live servers in
// TestBackupRestoreFreshInstallSYS084UC020_3 both need.
type httpProbe struct {
	t      *testing.T
	client *http.Client
	base   string
}

func newHTTPProbe(t *testing.T, base string) *httpProbe {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar:           jar,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       10 * time.Second,
	}
	return &httpProbe{t: t, client: client, base: base}
}

func (p *httpProbe) get(path string) (int, string) {
	p.t.Helper()
	resp, err := p.client.Get(p.base + path)
	if err != nil {
		p.t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		p.t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func (p *httpProbe) post(fromPath, toPath string, form url.Values) *http.Response {
	p.t.Helper()
	_, page := p.get(fromPath)
	form.Set("csrf_token", csrfToken(p.t, page))
	resp, err := p.client.PostForm(p.base+toPath, form)
	if err != nil {
		p.t.Fatalf("POST %s: %v", toPath, err)
	}
	_ = resp.Body.Close()
	return resp
}

func (p *httpProbe) login(username, password string) {
	p.t.Helper()
	resp := p.post("/login", "/login", url.Values{"username": {username}, "password": {password}})
	if resp.StatusCode != http.StatusSeeOther {
		p.t.Fatalf("login as %s = %d, want 303", username, resp.StatusCode)
	}
}

// TestBackupRestoreFreshInstallSYS084UC020_3 is the UC-020 #3 end-to-end
// proof: a one-action backup taken from a running instance, restored onto
// a genuinely fresh install (a brand-new data directory the target
// process has never seen), reproduces the full meet state — the same
// admin account still logs in, and the meet's standings and public results
// pages render the same data as before the backup, not just "the file
// opens". Record counts and a whole-database checksum are asserted equal,
// recomputed independently rather than merely re-read.
func TestBackupRestoreFreshInstallSYS084UC020_3(t *testing.T) {
	sourceCtx, cancelSource := context.WithCancel(context.Background())
	defer cancelSource()

	sourceDir := t.TempDir()
	deps, err := buildServer(mustParseServe(t, sourceDir))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- deps.server.Serve(sourceCtx, ln) }()
	probe := newHTTPProbe(t, "https://"+ln.Addr().String())
	waitHealthy(t, probe.client, probe.base, serveErr)

	resp := probe.post("/setup", "/setup", url.Values{
		"username": {"admin"}, "display_name": {"Meet Admin"}, "password": {"korrekt-pferd-batterie"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup = %d", resp.StatusCode)
	}
	probe.login("admin", "korrekt-pferd-batterie")

	// Seed a UKC meet with one participant and one settled, scored result —
	// directly through the app layer against the running server's own
	// store (same *sql.DB, ADR-004 §2 single-writer), so the seeded data is
	// exactly what the running server's handlers see, as a real capture
	// flow would leave it.
	backupCtx := context.Background()
	meets := app.NewMeetService(deps.dbase.DB(), mustCatalog(t), mustSchemes(t), mustTables(t), mustTemplates(t))
	results := app.NewResultsService(deps.dbase.DB(), mustCatalog(t), mustSchemes(t), mustTables(t), mustTemplates(t))
	organizerSession := app.Session{AccountID: "01ORG", Username: "orga", Role: app.RoleMeetOrganizer}
	officeSession := app.Session{AccountID: "01OFF", Username: "office", Role: app.RoleCompetitionOffice}

	rec, err := meets.CreateMeetFromTemplate(backupCtx, organizerSession, app.TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup, Venue: "Le Mouret",
		Date: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateMeetFromTemplate: %v", err)
	}
	participant, err := results.RegisterParticipant(backupCtx, officeSession, rec.ID, app.ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Club: "LC Test", Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}
	if _, err := results.SaveResult(backupCtx, officeSession, rec.ID, app.ResultInput{
		AthleteID: participant.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	// Pre-backup sanity: the standings page already renders the seeded
	// result through the running server.
	code, standingsBefore := probe.get("/meets/" + rec.ID + "/standings")
	if code != http.StatusOK || !strings.Contains(standingsBefore, "Anna Muster") || !strings.Contains(standingsBefore, "710") {
		t.Fatalf("pre-backup standings = %d: %.500s", code, standingsBefore)
	}

	// The one-action backup: the same artifact-production path as the
	// office UI's /admin/backup (proven over HTTP separately by
	// internal/web's TestBackupDownloadServesConsistentArtifactSYS084);
	// called directly here since this test's job is the fresh-install
	// restore proof, not re-proving the download endpoint.
	artifact := filepath.Join(t.TempDir(), "backup.db")
	manifest, err := deps.dbase.Backup(backupCtx, artifact, version)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if manifest.MeetCount != 1 {
		t.Fatalf("manifest.MeetCount = %d, want 1", manifest.MeetCount)
	}

	// Shut the source instance down before standing up the "fresh install"
	// so there is no ambiguity about which process's writes are being read.
	cancelSource()
	if err := <-serveErr; err != nil {
		t.Fatalf("source server shut down with error: %v", err)
	}
	if err := deps.dbase.Close(); err != nil {
		t.Fatal(err)
	}

	// Restore onto a genuinely fresh install directory via the CLI
	// subcommand — UC-020 #3's own wording, "restored onto a fresh
	// installation" — timed against the 15-minute budget.
	restoreDir := t.TempDir()
	var restoreOut strings.Builder
	restoreStart := time.Now()
	if err := runRestore(context.Background(), []string{"--data-dir", restoreDir, "--from", artifact}, &restoreOut); err != nil {
		t.Fatalf("runRestore: %v (%s)", err, restoreOut.String())
	}
	if elapsed := time.Since(restoreStart); elapsed > 15*time.Minute {
		t.Errorf("restore took %v, budget is 15 minutes (UC-020 #3)", elapsed)
	}
	if !strings.Contains(restoreOut.String(), manifest.Checksum) {
		t.Errorf("restore output missing the artifact's checksum: %s", restoreOut.String())
	}

	// Boot a brand-new server against the restored data directory — the
	// "fresh installation" this criterion restores onto.
	restoredCtx, cancelRestored := context.WithCancel(context.Background())
	defer cancelRestored()
	deps2, err := buildServer(mustParseServe(t, restoreDir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = deps2.dbase.Close() }()
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr2 := make(chan error, 1)
	go func() { serveErr2 <- deps2.server.Serve(restoredCtx, ln2) }()
	probe2 := newHTTPProbe(t, "https://"+ln2.Addr().String())
	waitHealthy(t, probe2.client, probe2.base, serveErr2)

	// The restored instance's data independently recomputes to the same
	// manifest checksum and counts (UC-020 #3: "record counts, checksums
	// equal") — recomputed fresh, not just re-reading the stored claim.
	recomputed, err := deps2.dbase.Backup(context.Background(), filepath.Join(t.TempDir(), "verify.db"), version)
	if err != nil {
		t.Fatal(err)
	}
	if recomputed.Checksum != manifest.Checksum {
		t.Errorf("restored instance checksum %q != original %q", recomputed.Checksum, manifest.Checksum)
	}
	if recomputed.MeetCount != manifest.MeetCount || recomputed.ResultCount != manifest.ResultCount {
		t.Errorf("restored counts = %+v, want %+v", recomputed, manifest)
	}

	// The same admin account still authenticates on the restored instance
	// (accounts are meet data too — SYS-084 "all meet data") — a fresh
	// login against the new instance, not a reused session cookie.
	probe2.login("admin", "korrekt-pferd-batterie")

	// Data intact + pages render: standings and the public (unauthenticated)
	// results page both show the restored result.
	code, standingsAfter := probe2.get("/meets/" + rec.ID + "/standings")
	if code != http.StatusOK || !strings.Contains(standingsAfter, "Anna Muster") || !strings.Contains(standingsAfter, "710") {
		t.Fatalf("restored standings = %d: %.500s", code, standingsAfter)
	}
	code, publicAfter := probe2.get("/m/" + rec.ID + "/results")
	if code != http.StatusOK || !strings.Contains(publicAfter, "Anna Muster") || !strings.Contains(publicAfter, "8.42") {
		t.Fatalf("restored public results = %d: %.500s", code, publicAfter)
	}

	cancelRestored()
	if err := <-serveErr2; err != nil {
		t.Fatalf("restored server shut down with error: %v", err)
	}
}

func mustParseServe(t *testing.T, dataDir string) serveConfig {
	t.Helper()
	cfg, err := parseServeFlags([]string{"--data-dir", dataDir}, io.Discard)
	if err != nil {
		t.Fatalf("parseServeFlags: %v", err)
	}
	return cfg
}

func mustCatalog(t *testing.T) *domain.DisciplineCatalog {
	t.Helper()
	c, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustSchemes(t *testing.T) map[string]*domain.CategoryScheme {
	t.Helper()
	s, err := domain.BuiltinCategorySchemes()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustTables(t *testing.T) map[string]*domain.ScoringTable {
	t.Helper()
	tb, err := domain.BuiltinScoringTables()
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func mustTemplates(t *testing.T) map[string]*domain.MeetTemplate {
	t.Helper()
	tp, err := domain.BuiltinMeetTemplates()
	if err != nil {
		t.Fatal(err)
	}
	return tp
}
