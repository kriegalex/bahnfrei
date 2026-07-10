// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // read the downloaded artifact back in the test
)

// loginAsAdmin bootstraps the very first account (an instance admin) and
// logs the test client in, returning the admin's username.
func loginAsAdmin(t *testing.T, deps *testServerDeps, client *http.Client, base string) string {
	t.Helper()
	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	loginForm := bodyString(t, mustGet(t, client, base+"/login"))
	token := csrfTokenFrom(t, loginForm)
	resp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return "admin"
}

// TestAdminPageOffersBackupDownloadLinkSYS084 covers the office-UI half of
// SYS-084: the admin workspace offers a one-action backup link, gated at
// instance-admin level like the rest of /admin (UC-020's actor).
func TestAdminPageOffersBackupDownloadLinkSYS084(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	loginAsAdmin(t, deps, client, base)

	body := bodyString(t, mustGet(t, client, base+"/admin"))
	if !strings.Contains(body, "/admin/backup") {
		t.Errorf("admin page missing the backup download link: %s", body)
	}
}

// TestBackupDownloadRequiresInstanceAdminSYS084 mirrors
// TestAdminRouteRequiresInstanceAdminRole for the backup endpoint
// specifically: anonymous access is refused.
func TestBackupDownloadRequiresInstanceAdminSYS084(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/admin/backup")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("anonymous GET /admin/backup = %d, want 403", resp.StatusCode)
	}
}

// TestBackupDownloadServesConsistentArtifactSYS084 drives the one-action
// backup end to end over HTTP (UC-020 #3): an instance admin clicks the
// link, gets a downloadable SQLite artifact back with the meet's data and
// metadata already in it, no separate export/prepare step.
func TestBackupDownloadServesConsistentArtifactSYS084(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	loginAsAdmin(t, deps, client, base)

	ctx := context.Background()

	// Seed one meet via the same HTTP flow TestUKCTemplateRosterStandingsFlow
	// uses: the built-in template quick-create form.
	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /meets/from-template = %d, want 303 (body: %s)", resp.StatusCode, bodyString(t, resp))
	}
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()
	meetID := strings.TrimPrefix(loc, "/meets/")

	resp = mustGet(t, client, base+"/admin/backup")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/backup = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.sqlite3" {
		t.Errorf("Content-Type = %q", ct)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, ".db") {
		t.Errorf("Content-Disposition = %q, want an attachment filename", disposition)
	}

	tmp, err := os.CreateTemp(t.TempDir(), "downloaded-*.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		t.Fatal(err)
	}
	path := tmp.Name()
	_ = tmp.Close()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open downloaded artifact: %v", err)
	}
	defer func() { _ = db.Close() }()

	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM meets WHERE id = ?`, meetID).Scan(&name); err != nil {
		t.Fatalf("read meet from downloaded artifact: %v", err)
	}
	if name != "UBS Kids Cup Le Mouret 2026" {
		t.Errorf("artifact meet name = %q", name)
	}
}
