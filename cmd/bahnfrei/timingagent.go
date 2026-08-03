// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// --- "timing-agent" subcommand (TASK-020, ADR-006's hub-first amendment):
// runs on the timing PC, watches a local folder for .lif result files and
// uploads them to the hub over HTTPS, and writes the hub's current
// lynx.ppl/.sch/.evt start-list exports into the same folder whenever they
// change. Same binary, agent mode — no separate program to distribute to
// the timing crew. ---

// timingAgentConfig holds the parsed "timing-agent" subcommand flags.
type timingAgentConfig struct {
	hubURL             string
	meetID             string
	token              string
	watchDir           string
	pollInterval       time.Duration
	once               bool
	insecureSkipVerify bool
}

func parseTimingAgentFlags(args []string, out io.Writer) (timingAgentConfig, error) {
	fs := flag.NewFlagSet("timing-agent", flag.ContinueOnError)
	fs.SetOutput(out)

	hubURL := fs.String("hub-url", "", "hub base URL, e.g. https://hub.local:8443 (required)")
	meetID := fs.String("meet", "", "the meet id to exchange timing data for (required)")
	token := fs.String("token", "", "the timing-agent bearer token issued from the meet's timing-exchange page (required)")
	watchDir := fs.String("watch-dir", ".", "directory the timing system writes .lif files into, and lynx.ppl/.sch/.evt are written to")
	pollInterval := fs.Duration("poll-interval", 5*time.Second, "how often to scan for new/changed .lif files and check for a start-list change")
	once := fs.Bool("once", false, "run a single scan/poll cycle and exit (for scripted verification; the default is to run until interrupted)")
	insecureSkipVerify := fs.Bool("insecure-skip-verify", false, "skip TLS certificate verification — for a venue's self-signed local hub certificate (SYS-093) until trust distribution is designed (OQ-050); never use this against a hub reached over the open internet")

	if err := fs.Parse(args); err != nil {
		return timingAgentConfig{}, err
	}
	if *hubURL == "" {
		return timingAgentConfig{}, fmt.Errorf("--hub-url is required")
	}
	if *meetID == "" {
		return timingAgentConfig{}, fmt.Errorf("--meet is required")
	}
	if *token == "" {
		return timingAgentConfig{}, fmt.Errorf("--token is required")
	}
	if _, err := os.Stat(*watchDir); err != nil {
		return timingAgentConfig{}, fmt.Errorf("--watch-dir %s: %w", *watchDir, err)
	}
	return timingAgentConfig{
		hubURL: strings.TrimRight(*hubURL, "/"), meetID: *meetID, token: *token,
		watchDir: *watchDir, pollInterval: *pollInterval, once: *once, insecureSkipVerify: *insecureSkipVerify,
	}, nil
}

// timingAgentClient is the HTTP client side of ADR-006's hub-first
// amendment: upload .lif/CSV files, poll the export manifest, download
// exports. Deliberately thin — every actual ingest/matching/conflict rule
// lives server-side (internal/app/exchange.go); the agent is a dumb file
// bridge.
type timingAgentClient struct {
	http    *http.Client
	baseURL string
	meetID  string
	token   string
}

func newTimingAgentClient(cfg timingAgentConfig) *timingAgentClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- opt-in --insecure-skip-verify flag, documented for the venue-self-signed-cert case (OQ-050); never the default
	}
	return &timingAgentClient{
		http:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
		baseURL: cfg.hubURL, meetID: cfg.meetID, token: cfg.token,
	}
}

func (c *timingAgentClient) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.http.Do(req) // #nosec G704 -- req targets c.baseURL, an operator-supplied --hub-url CLI flag on the operator's own machine, not attacker-controlled input
}

// uploadResult is the JSON shape the hub's /agent/v1/meets/{id}/lif|csv
// endpoint returns (mirrors app.TimingImportSummary, internal/web/timing.go).
type uploadResult struct {
	BatchID   string `json:"BatchID"`
	Applied   int    `json:"Applied"`
	Conflicts int    `json:"Conflicts"`
}

// upload uploads one .lif or .csv file's bytes.
func (c *timingAgentClient) upload(ctx context.Context, format, filename string, data []byte) (uploadResult, error) {
	url := fmt.Sprintf("%s/agent/v1/meets/%s/%s", c.baseURL, c.meetID, format)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data)) // #nosec G704 -- c.baseURL/c.meetID come from operator-supplied --hub-url/--meet CLI flags on the operator's own machine, not attacker-controlled input
	if err != nil {
		return uploadResult{}, err
	}
	req.Header.Set("X-Filename", filepath.Base(filename))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.do(req)
	if err != nil {
		return uploadResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return uploadResult{}, fmt.Errorf("upload %s: hub returned %s: %s", filename, resp.Status, body)
	}
	var out uploadResult
	if err := json.Unmarshal(body, &out); err != nil {
		return uploadResult{}, fmt.Errorf("upload %s: decode response: %w", filename, err)
	}
	return out, nil
}

type exportManifest struct {
	PPLSHA256 string `json:"pplSha256"`
	SCHSHA256 string `json:"schSha256"`
	EVTSHA256 string `json:"evtSha256"`
}

func (c *timingAgentClient) manifest(ctx context.Context) (exportManifest, error) {
	url := fmt.Sprintf("%s/agent/v1/meets/%s/exports/manifest", c.baseURL, c.meetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) // #nosec G704 -- c.baseURL/c.meetID come from operator-supplied --hub-url/--meet CLI flags on the operator's own machine, not attacker-controlled input
	if err != nil {
		return exportManifest{}, err
	}
	resp, err := c.do(req)
	if err != nil {
		return exportManifest{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return exportManifest{}, fmt.Errorf("manifest: hub returned %s: %s", resp.Status, body)
	}
	var m exportManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return exportManifest{}, fmt.Errorf("manifest: decode response: %w", err)
	}
	return m, nil
}

func (c *timingAgentClient) downloadExport(ctx context.Context, kind string) ([]byte, error) {
	url := fmt.Sprintf("%s/agent/v1/meets/%s/exports/%s", c.baseURL, c.meetID, kind)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) // #nosec G704 -- c.baseURL/c.meetID come from operator-supplied --hub-url/--meet CLI flags on the operator's own machine, not attacker-controlled input
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download %s: hub returned %s: %s", kind, resp.Status, body)
	}
	return io.ReadAll(resp.Body)
}

// lifFileState is what the watcher tracks per .lif file to detect a
// change without a filesystem-event API (fsnotify): FinishLynx re-writes a
// heat's .lif as results are edited/autosaved, so mtime+size (not just
// "have we ever seen this filename") is the change signal.
type lifFileState struct {
	modTime time.Time
	size    int64
}

// timingAgentState is the watcher's in-memory bookkeeping across poll
// cycles: which .lif files have already been uploaded at their last-seen
// mtime/size, and the last export manifest hash written to disk (so an
// unchanged start list is not rewritten every cycle).
type timingAgentState struct {
	lifFiles     map[string]lifFileState
	lastManifest exportManifest
}

func newTimingAgentState() *timingAgentState {
	return &timingAgentState{lifFiles: map[string]lifFileState{}}
}

// scanAndUploadLIFFiles globs watchDir for *.lif/*.LIF files, uploads any
// that are new or whose mtime/size changed since the last cycle, and
// reports what it did (for the CLI's progress output and for tests).
func scanAndUploadLIFFiles(ctx context.Context, client *timingAgentClient, watchDir string, state *timingAgentState, out io.Writer) error {
	var names []string
	for _, pattern := range []string{"*.lif", "*.LIF"} {
		matches, err := filepath.Glob(filepath.Join(watchDir, pattern))
		if err != nil {
			return fmt.Errorf("scan %s: %w", watchDir, err)
		}
		names = append(names, matches...)
	}
	seen := map[string]bool{}
	for _, path := range names {
		if seen[path] {
			continue // *.lif and *.LIF both matched on a case-insensitive filesystem
		}
		seen[path] = true

		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(out, "timing-agent: stat %s: %v\n", path, err)
			continue
		}
		current := lifFileState{modTime: info.ModTime(), size: info.Size()}
		if prev, ok := state.lifFiles[path]; ok && prev == current {
			continue // unchanged since the last cycle
		}

		data, err := os.ReadFile(path) // #nosec G304 -- path is under watchDir, an operator-supplied --watch-dir CLI flag on the operator's own machine, not user input
		if err != nil {
			fmt.Fprintf(out, "timing-agent: read %s: %v\n", path, err)
			continue
		}
		result, err := client.upload(ctx, "lif", filepath.Base(path), data)
		if err != nil {
			fmt.Fprintf(out, "timing-agent: upload %s: %v\n", path, err)
			continue
		}
		state.lifFiles[path] = current
		fmt.Fprintf(out, "timing-agent: uploaded %s (%d applied, %d conflict(s))\n", filepath.Base(path), result.Applied, result.Conflicts)
	}
	return nil
}

// pollAndWriteExports checks the hub's current lynx.ppl/.sch/.evt content
// hashes against what this agent last wrote to watchDir, and re-downloads
// (and rewrites) only the file(s) that changed — ADR-006's "regenerate
// .ppl/.sch/.evt on start-list changes", implemented client-side since the
// hub always serves the live/current export (internal/app/exchange.go's
// ExportTimingFiles doc comment).
func pollAndWriteExports(ctx context.Context, client *timingAgentClient, watchDir string, state *timingAgentState, out io.Writer) error {
	manifest, err := client.manifest(ctx)
	if err != nil {
		return err
	}
	for kind, hash := range map[string]string{"ppl": manifest.PPLSHA256, "sch": manifest.SCHSHA256, "evt": manifest.EVTSHA256} {
		prevHash := ""
		switch kind {
		case "ppl":
			prevHash = state.lastManifest.PPLSHA256
		case "sch":
			prevHash = state.lastManifest.SCHSHA256
		case "evt":
			prevHash = state.lastManifest.EVTSHA256
		}
		if hash == prevHash {
			continue
		}
		data, err := client.downloadExport(ctx, kind)
		if err != nil {
			return err
		}
		if got := sha256Hex(data); got != hash {
			return fmt.Errorf("downloaded %s hash %s does not match manifest hash %s (transfer error?)", kind, got, hash)
		}
		target := filepath.Join(watchDir, "lynx."+kind)
		// 0600: the exported start list carries athlete names (personal data, nFADP/GDPR); owner-only is
		// sufficient since the timing PC's software runs under the same local operator account.
		if err := os.WriteFile(target, data, 0o600); err != nil { // #nosec G703 -- target is under watchDir, an operator-supplied --watch-dir CLI flag, joined with one of the fixed literals "ppl"/"sch"/"evt" above, not user input
			return fmt.Errorf("write %s: %w", target, err)
		}
		fmt.Fprintf(out, "timing-agent: wrote %s (%d bytes)\n", target, len(data))
	}
	state.lastManifest = manifest
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// runTimingAgentCycle runs one scan+poll cycle — the unit the --once flag
// and the tests both drive.
func runTimingAgentCycle(ctx context.Context, client *timingAgentClient, watchDir string, state *timingAgentState, out io.Writer) error {
	if err := scanAndUploadLIFFiles(ctx, client, watchDir, state, out); err != nil {
		return err
	}
	return pollAndWriteExports(ctx, client, watchDir, state, out)
}

// runTimingAgent implements the "timing-agent" subcommand: parses flags,
// then loops scan+poll cycles at --poll-interval until ctx is canceled
// (SIGINT/SIGTERM), or exactly once if --once is set. A single failed
// cycle (a transient network blip, C8's LTE/Wi-Fi tolerance requirement)
// is logged and retried next cycle — it never aborts the agent process,
// since the timing crew has no one watching a terminal during a race.
func runTimingAgent(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := parseTimingAgentFlags(args, out)
	if err != nil {
		return err
	}
	client := newTimingAgentClient(cfg)
	state := newTimingAgentState()

	fmt.Fprintf(out, "timing-agent: watching %s, hub %s, meet %s\n", cfg.watchDir, cfg.hubURL, cfg.meetID)

	if cfg.once {
		return runTimingAgentCycle(ctx, client, cfg.watchDir, state, out)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()
	for {
		if err := runTimingAgentCycle(ctx, client, cfg.watchDir, state, out); err != nil {
			fmt.Fprintf(out, "timing-agent: cycle error (will retry): %v\n", err)
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "timing-agent: shutting down")
			return nil
		case <-ticker.C:
		}
	}
}
