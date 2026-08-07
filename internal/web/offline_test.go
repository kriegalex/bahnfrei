// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestServiceWorkerServedAtRootPath: the capture service worker must be
// served from a root-path URL with the Service-Worker-Allowed header so it
// can claim the /meets/…/capture/ scope (UC-034 #3) — a worker served under
// /static/ could only control /static/.
func TestServiceWorkerServedAtRootPath(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/capture-sw.js")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /capture-sw.js = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
	if got := resp.Header.Get("Service-Worker-Allowed"); got != "/meets/" {
		t.Errorf("Service-Worker-Allowed = %q, want /meets/", got)
	}
	if !strings.Contains(body, "bahnfrei-capture") {
		t.Error("service worker body should carry its cache name")
	}
}

// TestOfflineIslandAssetsServed: the compiled islands are embedded and served
// under /static/ (ADR-003: no CDN, binary self-contained).
func TestOfflineIslandAssetsServed(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/static/capture-offline.js", "/static/office-banner.js"} {
		resp := mustGet(t, client, base+path)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
		if !strings.Contains(body, "SPDX-License-Identifier: AGPL-3.0-only") {
			t.Errorf("%s must carry the SPDX header (ADR-001 §3)", path)
		}
	}
}

// TestCapturePageWiresOfflineIsland: a field unit's capture page renders the
// island configuration (endpoints, SW scope, translated strings via data-*
// attributes — the CSP forbids inline scripts, SYS-092) and the offline
// status indicator (SYS-087).
func TestCapturePageWiresOfflineIsland(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitID := units["Zonen-Weitsprung (UKC)"] // localized discipline name (SYS-111/F6, TASK-050)
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/capture/"+unitID))

	for _, want := range []string{
		`id="capture-offline-status"`,
		`id="capture-offline-config"`,
		`data-sync-url="/meets/` + meetID + `/capture/` + unitID + `/sync"`,
		`data-checkout-url="/meets/` + meetID + `/capture/` + unitID + `/checkout"`,
		`data-sw-url="/capture-sw.js"`,
		`data-sw-scope="/meets/` + meetID + `/capture/"`,
		`data-i18n-offline=`,
		`data-i18n-pending=`,
		`data-athlete=`, // cell forms carry the op coordinates for the queue
		`data-seq=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("field capture page missing %s", want)
		}
	}
	// The script src carries a content-fingerprint query parameter
	// (DEC-039/TASK-059, assetURL); assert the prefix rather than the
	// bare literal so this test survives asset-content changes.
	if !strings.Contains(body, `src="/static/capture-offline.js?v=`) {
		t.Error("field capture page missing the fingerprinted capture-offline.js script src")
	}

	// The track surface is not the offline island's scope (ADR-004 §8 covers
	// horizontal field capture): no island config there.
	trackBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/capture/"+units["60 m (UKC)"]))
	if strings.Contains(trackBody, `id="capture-offline-config"`) {
		t.Error("track capture page must not wire the field offline island")
	}
}

// TestLayoutWiresOfficeBanner: authenticated operator surfaces render the
// degraded-state banner + island (UC-034 #7, SYS-087); anonymous public
// surfaces do not.
func TestLayoutWiresOfficeBanner(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	setupAndLogin(t, client, base)
	body := bodyString(t, mustGet(t, client, base+"/meets"))
	if !strings.Contains(body, `id="offline-banner"`) || !strings.Contains(body, "/static/office-banner.js") {
		t.Error("logged-in operator surface must wire the offline banner island")
	}

	anonClient, _ := newTestClient(t, deps)
	anonBody := bodyString(t, mustGet(t, anonClient, base+"/login"))
	if strings.Contains(anonBody, `id="offline-banner"`) {
		t.Error("anonymous surfaces must not render the operator banner")
	}
}

// TestTLSModeOffServesPlaintext: the dev/E2E-only "off" TLS mode serves
// plaintext HTTP on the bare listener (loopback is a secure context, so the
// browser E2E suite can register service workers against http://localhost —
// UC-034 #3). Production modes keep wrapping the listener in TLS (SYS-093).
func TestTLSModeOffServesPlaintext(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeOff})
	srv := deps.withConfig(Config{Addr: "127.0.0.1:0", TLS: TLSConfig{Mode: TLSModeOff}})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("plain HTTP GET against tls-mode=off server: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz over plaintext = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned %v after shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not shut down within 5s")
	}
}
