// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseTimingAgentFlagsRequiresHubMeetToken(t *testing.T) {
	dir := t.TempDir()
	cases := [][]string{
		{"--meet", "m1", "--token", "t", "--watch-dir", dir},
		{"--hub-url", "https://hub", "--token", "t", "--watch-dir", dir},
		{"--hub-url", "https://hub", "--meet", "m1", "--watch-dir", dir},
	}
	for _, args := range cases {
		if _, err := parseTimingAgentFlags(args, io.Discard); err == nil {
			t.Errorf("parseTimingAgentFlags(%v): want error", args)
		}
	}
	f, err := parseTimingAgentFlags([]string{"--hub-url", "https://hub.local:8443/", "--meet", "m1", "--token", "t", "--watch-dir", dir}, io.Discard)
	if err != nil {
		t.Fatalf("parseTimingAgentFlags: %v", err)
	}
	if f.hubURL != "https://hub.local:8443" { // trailing slash trimmed
		t.Errorf("hubURL = %q", f.hubURL)
	}
}

func TestParseTimingAgentFlagsRejectsMissingWatchDir(t *testing.T) {
	if _, err := parseTimingAgentFlags([]string{
		"--hub-url", "https://hub", "--meet", "m1", "--token", "t", "--watch-dir", "/no/such/dir/at/all",
	}, io.Discard); err == nil {
		t.Error("parseTimingAgentFlags with a nonexistent --watch-dir: want error")
	}
}

// fakeHub is a minimal stand-in for the bahnfrei /agent/v1/... surface,
// used to test the timing-agent client/watcher without a real server
// (internal/web/timing_test.go covers the real handlers end-to-end).
type fakeHub struct {
	t             *testing.T
	wantToken     string
	uploads       []fakeUpload
	ppl, sch, evt []byte
	requests      atomic.Int32
}

type fakeUpload struct {
	format, filename string
	data             []byte
}

func (h *fakeHub) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/meets/m1/lif", h.handleUpload("lif"))
	mux.HandleFunc("/agent/v1/meets/m1/csv", h.handleUpload("csv"))
	mux.HandleFunc("/agent/v1/meets/m1/exports/manifest", h.handleManifest)
	mux.HandleFunc("/agent/v1/meets/m1/exports/ppl", h.handleExport(func() []byte { return h.ppl }))
	mux.HandleFunc("/agent/v1/meets/m1/exports/sch", h.handleExport(func() []byte { return h.sch }))
	mux.HandleFunc("/agent/v1/meets/m1/exports/evt", h.handleExport(func() []byte { return h.evt }))
	return httptest.NewServer(mux)
}

func (h *fakeHub) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	h.requests.Add(1)
	if r.Header.Get("Authorization") != "Bearer "+h.wantToken {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *fakeHub) handleUpload(format string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.checkAuth(w, r) {
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.uploads = append(h.uploads, fakeUpload{format: format, filename: r.Header.Get("X-Filename"), data: data})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(uploadResult{BatchID: fmt.Sprintf("batch-%d", len(h.uploads)), Applied: 1})
	}
}

func (h *fakeHub) handleManifest(w http.ResponseWriter, r *http.Request) {
	if !h.checkAuth(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(exportManifest{
		PPLSHA256: hashOf(h.ppl), SCHSHA256: hashOf(h.sch), EVTSHA256: hashOf(h.evt),
	})
}

func (h *fakeHub) handleExport(get func() []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.checkAuth(w, r) {
			return
		}
		_, _ = w.Write(get())
	}
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestScanAndUploadLIFFilesUploadsOnceThenOnChange covers the watcher's
// core change-detection invariant: a .lif file is uploaded when first
// seen, not re-uploaded on an unchanged rescan, and re-uploaded once its
// content (and therefore mtime/size) changes — FinishLynx rewrites the
// same file as a heat's results are edited/autosaved.
func TestScanAndUploadLIFFilesUploadsOnceThenOnChange(t *testing.T) {
	hub := &fakeHub{t: t, wantToken: "secret"}
	srv := hub.server()
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "secret"})
	state := newTimingAgentState()

	dir := t.TempDir()
	path := filepath.Join(dir, "race.lif")
	if err := os.WriteFile(path, []byte("161,1,1,Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := scanAndUploadLIFFiles(context.Background(), client, dir, state, io.Discard); err != nil {
		t.Fatalf("scan (first): %v", err)
	}
	if len(hub.uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(hub.uploads))
	}

	// Unchanged rescan: no new upload.
	if err := scanAndUploadLIFFiles(context.Background(), client, dir, state, io.Discard); err != nil {
		t.Fatalf("scan (rescan): %v", err)
	}
	if len(hub.uploads) != 1 {
		t.Fatalf("uploads after unchanged rescan = %d, want still 1", len(hub.uploads))
	}

	// Ensure a distinguishable mtime, then rewrite the file (FinishLynx
	// autosave / a corrected result).
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("161,1,1,Test\n1,616,1,Asay,Brycen,SALE,11.02\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scanAndUploadLIFFiles(context.Background(), client, dir, state, io.Discard); err != nil {
		t.Fatalf("scan (after change): %v", err)
	}
	if len(hub.uploads) != 2 {
		t.Fatalf("uploads after content change = %d, want 2", len(hub.uploads))
	}
	if hub.uploads[1].format != "lif" || !strings.Contains(string(hub.uploads[1].data), "Asay") {
		t.Fatalf("second upload = %+v", hub.uploads[1])
	}
}

// TestScanAndUploadLIFFilesRejectsBadToken is the denial-first path: an
// upload the hub rejects (wrong token) is logged, not silently swallowed
// as success, and the file is not marked uploaded (a retry happens next
// cycle).
func TestScanAndUploadLIFFilesRejectsBadToken(t *testing.T) {
	hub := &fakeHub{t: t, wantToken: "correct-token"}
	srv := hub.server()
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "wrong-token"})
	state := newTimingAgentState()

	dir := t.TempDir()
	path := filepath.Join(dir, "race.lif")
	if err := os.WriteFile(path, []byte("161,1,1,Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := scanAndUploadLIFFiles(context.Background(), client, dir, state, &log); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hub.uploads) != 0 {
		t.Fatalf("uploads = %d, want 0 (rejected)", len(hub.uploads))
	}
	if !strings.Contains(log.String(), "race.lif") {
		t.Fatalf("expected the failure to be logged with the filename, got: %s", log.String())
	}
	if _, seen := state.lifFiles[path]; seen {
		t.Fatal("a rejected upload must not be marked as successfully uploaded")
	}
}

// TestPollAndWriteExportsWritesOnlyOnChange covers ADR-006's "regenerate
// on start-list changes" from the agent side: the first poll writes all
// three files, an unchanged second poll rewrites nothing, and changing one
// file's content on the hub rewrites only that file.
func TestPollAndWriteExportsWritesOnlyOnChange(t *testing.T) {
	hub := &fakeHub{t: t, wantToken: "secret", ppl: []byte("ppl-v1"), sch: []byte("sch-v1"), evt: []byte("evt-v1")}
	srv := hub.server()
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "secret"})
	state := newTimingAgentState()
	dir := t.TempDir()

	if err := pollAndWriteExports(context.Background(), client, dir, state, io.Discard); err != nil {
		t.Fatalf("poll (first): %v", err)
	}
	for _, kind := range []string{"ppl", "sch", "evt"} {
		got, err := os.ReadFile(filepath.Join(dir, "lynx."+kind))
		if err != nil {
			t.Fatalf("read lynx.%s: %v", kind, err)
		}
		if string(got) != kind+"-v1" {
			t.Fatalf("lynx.%s = %q, want %q", kind, got, kind+"-v1")
		}
	}

	// Overwrite on disk with a sentinel, then poll again unchanged on the
	// hub side: the sentinel must survive (no rewrite happened).
	sentinelPath := filepath.Join(dir, "lynx.sch")
	if err := os.WriteFile(sentinelPath, []byte("untouched-sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pollAndWriteExports(context.Background(), client, dir, state, io.Discard); err != nil {
		t.Fatalf("poll (unchanged): %v", err)
	}
	got, _ := os.ReadFile(sentinelPath)
	if string(got) != "untouched-sentinel" {
		t.Fatalf("lynx.sch was rewritten despite an unchanged hub manifest: %q", got)
	}

	// Now the hub's evt content actually changes (a lane swap re-export,
	// UC-014 #2): only evt should be rewritten.
	hub.evt = []byte("evt-v2")
	if err := pollAndWriteExports(context.Background(), client, dir, state, io.Discard); err != nil {
		t.Fatalf("poll (evt changed): %v", err)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "lynx.evt"))
	if string(got) != "evt-v2" {
		t.Fatalf("lynx.evt = %q, want evt-v2", got)
	}
	got, _ = os.ReadFile(sentinelPath)
	if string(got) != "untouched-sentinel" {
		t.Fatalf("lynx.sch was rewritten even though only evt changed on the hub: %q", got)
	}
}

// TestNewTimingAgentClientInsecureSkipVerify covers newTimingAgentClient's
// --insecure-skip-verify wiring directly: the opt-in flag (for a venue's
// self-signed local hub certificate, OQ-050) actually reaches the
// transport's TLS config, and is off by default.
func TestNewTimingAgentClientInsecureSkipVerify(t *testing.T) {
	def := newTimingAgentClient(timingAgentConfig{hubURL: "https://hub", meetID: "m1", token: "t"})
	transport, ok := def.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", def.http.Transport)
	}
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecureSkipVerify=false (default) must not skip TLS verification")
	}

	insecure := newTimingAgentClient(timingAgentConfig{hubURL: "https://hub", meetID: "m1", token: "t", insecureSkipVerify: true})
	transport2, ok := insecure.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", insecure.http.Transport)
	}
	if transport2.TLSClientConfig == nil || !transport2.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecureSkipVerify=true must reach the transport's TLS config")
	}
}

// TestUploadRejectsMalformedJSONResponse covers upload's decode-error
// branch: a hub that returns 200 with a non-JSON body is reported as an
// error, not silently treated as zero applied/zero conflicts.
func TestUploadRejectsMalformedJSONResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/meets/m1/lif", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "t"})

	if _, err := client.upload(context.Background(), "lif", "race.lif", []byte("161,1,1,Test\n")); err == nil {
		t.Error("upload with a non-JSON 200 response: want an error, got nil")
	}
}

// TestManifestAndDownloadExportErrorPathsWeb covers manifest's and
// downloadExport's shared non-200 branch: the hub returning an error status
// (rather than being unreachable) is reported with the response body
// included, not silently swallowed.
func TestManifestAndDownloadExportErrorPathsWeb(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/meets/m1/exports/manifest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	mux.HandleFunc("/agent/v1/meets/m1/exports/evt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down for maintenance"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "t"})

	if _, err := client.manifest(context.Background()); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("manifest() with a 500 response: err = %v, want it to include the response body", err)
	}
	if _, err := client.downloadExport(context.Background(), "evt"); err == nil || !strings.Contains(err.Error(), "down for maintenance") {
		t.Errorf("downloadExport() with a 503 response: err = %v, want it to include the response body", err)
	}
}

// TestScanAndUploadLIFFilesLogsReadError covers scanAndUploadLIFFiles' file-
// read-error branch: a *.lif path that exists but cannot be read as a file
// (here, a directory someone dropped in the watched folder) is logged and
// skipped, not uploaded or fatal to the whole scan.
func TestScanAndUploadLIFFilesLogsReadError(t *testing.T) {
	hub := &fakeHub{t: t, wantToken: "secret"}
	srv := hub.server()
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "secret"})
	state := newTimingAgentState()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "not-a-file.lif"), 0o755); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := scanAndUploadLIFFiles(context.Background(), client, dir, state, &log); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hub.uploads) != 0 {
		t.Fatalf("uploads = %d, want 0 (a directory cannot be read as a file)", len(hub.uploads))
	}
	if !strings.Contains(log.String(), "not-a-file.lif") {
		t.Errorf("expected the read failure to be logged with the path, got: %s", log.String())
	}
}

// TestRunTimingAgentCyclePropagatesPollError covers runTimingAgentCycle's
// error-propagation branch: a poll failure (here, the hub's manifest
// endpoint returning an error) surfaces as the cycle's own error rather than
// being swallowed — the caller (runTimingAgent's --once path) needs it to
// report cycle failure.
func TestRunTimingAgentCyclePropagatesPollError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/meets/m1/exports/manifest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := newTimingAgentClient(timingAgentConfig{hubURL: srv.URL, meetID: "m1", token: "secret"})
	state := newTimingAgentState()
	dir := t.TempDir()

	if err := runTimingAgentCycle(context.Background(), client, dir, state, io.Discard); err == nil {
		t.Error("runTimingAgentCycle with a failing manifest poll: want an error, got nil")
	}
}

// TestRunTimingAgentPropagatesFlagParseError covers runTimingAgent's own
// flag-parsing error path (distinct from parseTimingAgentFlags' own tests,
// which call it directly): a missing required flag must fail before any
// client is even constructed.
func TestRunTimingAgentPropagatesFlagParseError(t *testing.T) {
	var out bytes.Buffer
	err := runTimingAgent(context.Background(), []string{"--hub-url", "https://hub"}, &out)
	if err == nil {
		t.Fatal("runTimingAgent with missing --meet/--token: want an error, got nil")
	}
}

// TestRunTimingAgentLoopsUntilContextCanceled covers runTimingAgent's
// non-once branch (the ticker loop real long-running usage takes): with a
// short poll interval and a context that expires shortly after, the agent
// runs at least one full cycle and then shuts down cleanly on ctx.Done(),
// rather than looping forever or erroring.
func TestRunTimingAgentLoopsUntilContextCanceled(t *testing.T) {
	hub := &fakeHub{t: t, wantToken: "secret", ppl: []byte("ppl"), sch: []byte("sch"), evt: []byte("evt")}
	srv := hub.server()
	defer srv.Close()
	dir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	err := runTimingAgent(ctx, []string{
		"--hub-url", srv.URL, "--meet", "m1", "--token", "secret", "--watch-dir", dir,
		"--poll-interval", "15ms",
	}, &out)
	if err != nil {
		t.Fatalf("runTimingAgent (loop mode): %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "shutting down") {
		t.Errorf("expected a shutdown message once the context expired, got:\n%s", out.String())
	}
	if hub.requests.Load() < 2 {
		t.Errorf("expected at least two polls against the fake hub before shutdown, got %d", hub.requests.Load())
	}
}

// TestRunTimingAgentCycleEndToEnd drives one full cycle (upload + poll)
// against the fake hub, the same code path --once exercises.
func TestRunTimingAgentCycleEndToEnd(t *testing.T) {
	hub := &fakeHub{t: t, wantToken: "secret", ppl: []byte("ppl"), sch: []byte("sch"), evt: []byte("evt")}
	srv := hub.server()
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "race.lif"), []byte("161,1,1,Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := runTimingAgent(context.Background(), []string{
		"--hub-url", srv.URL, "--meet", "m1", "--token", "secret", "--watch-dir", dir, "--once",
	}, &out)
	if err != nil {
		t.Fatalf("runTimingAgent --once: %v\n%s", err, out.String())
	}
	if len(hub.uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(hub.uploads))
	}
	if _, err := os.Stat(filepath.Join(dir, "lynx.evt")); err != nil {
		t.Fatalf("lynx.evt was not written: %v", err)
	}
	if !strings.Contains(out.String(), "uploaded race.lif") {
		t.Errorf("expected progress output to mention the upload, got:\n%s", out.String())
	}
}
