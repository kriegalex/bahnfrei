// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newTestClient starts a plain (non-TLS) httptest server over the
// Server's route tree and returns an http.Client with a cookie jar, so
// tests exercise session/CSRF/locale cookies the way a browser would
// across multiple requests.
func newTestClient(t *testing.T, deps *testServerDeps) (*http.Client, string) {
	t.Helper()
	srv := httptest.NewServer(deps.server.routes())
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't auto-follow; assertions check the redirect itself
		},
	}, srv.URL
}

func mustGet(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func bodyString(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func csrfTokenFrom(t *testing.T, body string) string {
	t.Helper()
	const marker = `name="csrf_token" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("csrf_token hidden field not found in body: %s", body)
	}
	rest := body[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

func TestHandleHome(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	// A fresh instance sends visitors to the first-run setup (UC-001 #1).
	resp := mustGet(t, client, base+"/")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/setup" {
		t.Fatalf("GET / on fresh instance = %d -> %q, want 303 -> /setup", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Once an account exists, the home page renders normally.
	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Admin", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	resp = mustGet(t, client, base+"/")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200; body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "Bahnfrei") {
		t.Errorf("home page missing app title: %s", body)
	}
}

func TestHandleHealthz(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", resp.StatusCode)
	}
	if body := bodyString(t, resp); strings.TrimSpace(body) != "ok" {
		t.Errorf("GET /healthz body = %q, want \"ok\"", body)
	}
}

func TestHandleNotFound(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/nowhere")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nowhere = %d, want 404", resp.StatusCode)
	}
}

func TestStaticAssetServed(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/static/htmx.min.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/htmx.min.js = %d, want 200", resp.StatusCode)
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, "htmx") {
		t.Errorf("static asset does not look like htmx.min.js")
	}
}

func TestEventsRouteStreamsSSE(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	srv := httptest.NewServer(deps.server.routes())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events/demo-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events/demo-topic: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events/demo-topic = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestLoginFlowSuccessAndFailure(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}

	loginForm := bodyString(t, mustGet(t, client, base+"/login"))
	token := csrfTokenFrom(t, loginForm)

	// Wrong password: 401, no session cookie set.
	resp := mustGet(t, client, base+"/") // re-fetch to reset any state; not strictly needed
	_ = resp.Body.Close()

	badResp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"wrong"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	if badResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad login = %d, want 401", badResp.StatusCode)
	}
	badBody := bodyString(t, badResp)
	if !strings.Contains(badBody, "ungültig") && !strings.Contains(badBody, "invalide") {
		t.Errorf("bad login body missing localized error: %s", badBody)
	}

	// Correct password: redirect + session cookie.
	goodResp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = goodResp.Body.Close() }()
	if goodResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("good login = %d, want 303", goodResp.StatusCode)
	}
	found := false
	for _, c := range goodResp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("successful login did not set the session cookie")
	}
}

func TestLoginRejectsWithoutCSRFToken(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	// Prime the CSRF cookie via a GET, but submit the form without the
	// matching hidden field.
	_ = mustGet(t, client, base+"/login").Body.Close()

	resp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("login without csrf_token = %d, want 403", resp.StatusCode)
	}
}

func TestAdminRouteRequiresInstanceAdminRole(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	// Anonymous: forbidden.
	resp := mustGet(t, client, base+"/admin")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("anonymous GET /admin = %d, want 403", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Bootstrap gives an instance-admin account; log in and retry.
	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	loginForm := bodyString(t, mustGet(t, client, base+"/login"))
	token := csrfTokenFrom(t, loginForm)
	loginResp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResp.Body.Close()

	adminResp := mustGet(t, client, base+"/admin")
	defer func() { _ = adminResp.Body.Close() }()
	if adminResp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated admin GET /admin = %d, want 200", adminResp.StatusCode)
	}
}

func TestLocaleSwitchPersistsAcrossRequests(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	// Default locale (DE): the login title renders "Anmelden".
	de := bodyString(t, mustGet(t, client, base+"/login"))
	if !strings.Contains(de, "Anmelden") {
		t.Errorf("default-locale login page missing German title: %s", de)
	}

	switchResp := mustGet(t, client, base+"/locale?lang=fr")
	_ = switchResp.Body.Close()
	if switchResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /locale?lang=fr = %d, want 303", switchResp.StatusCode)
	}

	fr := bodyString(t, mustGet(t, client, base+"/login"))
	if !strings.Contains(fr, "Connexion") {
		t.Errorf("after switching to fr, login page missing French title: %s", fr)
	}
}

func TestLocaleSwitchIgnoresUnknownLocale(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	_ = mustGet(t, client, base+"/locale?lang=xx-not-a-locale").Body.Close()
	body := bodyString(t, mustGet(t, client, base+"/login"))
	if !strings.Contains(body, "Anmelden") {
		t.Errorf("unknown locale should leave the default (DE) in effect: %s", body)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	loginForm := bodyString(t, mustGet(t, client, base+"/login"))
	token := csrfTokenFrom(t, loginForm)
	loginResp, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResp.Body.Close()

	if resp := mustGet(t, client, base+"/admin"); resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("expected authenticated access before logout, got %d", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}

	home := bodyString(t, mustGet(t, client, base+"/"))
	logoutToken := csrfTokenFrom(t, home)
	logoutResp, err := client.PostForm(base+"/logout", url.Values{"csrf_token": {logoutToken}})
	if err != nil {
		t.Fatal(err)
	}
	_ = logoutResp.Body.Close()

	afterResp := mustGet(t, client, base+"/admin")
	defer func() { _ = afterResp.Body.Close() }()
	if afterResp.StatusCode != http.StatusForbidden {
		t.Errorf("GET /admin after logout = %d, want 403", afterResp.StatusCode)
	}
}
