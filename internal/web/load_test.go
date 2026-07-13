// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

//go:build perf

package web

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/apptest"
	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// --- TASK-027 SYS-122 / UC-017 #4 load harness: "Given the reference
// hosting size, when a load test simulates 2,000 concurrent viewers, then
// p95 page render <=3s and no errors >0.1% (SYS-122)", plus SYS-071's
// live-update latency re-measured under load (the traceability matrix
// earmarks exactly this: "A (load-bearing p95 latency under concurrent
// load): TASK-027").
//
// Client choice: goroutine-based Go clients against a real net/http
// listener (httptest.NewServer — genuine TCP over loopback), rather than
// k6/vegeta/similar. Justification: (1) no new external tool dependency;
// (2) it drives the actual production web.Server, SSE bus and
// ResultsService.Standings computation, so real bottlenecks surface
// exactly as under an external load generator; (3) goroutine clients are
// cheap — the expensive side is the server (see below), which an external
// tool would not change.
//
// SAFETY (OOM incident, 2026-07-13): the first unconfined 2,000-viewer run
// of this test reached ~27 GB RSS and was OOM-killed by the kernel, taking
// the host session with it — twice. Root cause is a genuine server-side
// finding, not harness overhead: ResultsService.Standings allocates an
// athletes×disciplines performance matrix per call (reference scale:
// 1,500×250 CombinedPerformance per request, before the multi-MB rendered
// HTML), there is no caching, and store.Open's SetMaxOpenConns(1) convoys
// all requests so thousands are in flight simultaneously, each holding
// that allocation. NEVER run these tests unconfined: use
// scripts/run-perf-tests.sh, which wraps every invocation in a systemd
// user scope (MemoryMax=12G, MemorySwapMax=0) with GOMEMLIMIT=10GiB so a
// runaway dies inside the scope, never the host. Full numbers and the
// OQ-066 write-up: docs/delivery/perf-and-recovery-task-027.md.

// loadMetrics is one load run's outcome.
type loadMetrics struct {
	viewers       int
	succeeded     int
	errCount      int
	p95           time.Duration
	sseObserved   int
	sseErrCount   int
	sseP95        time.Duration
	peakHeapInUse uint64 // process-wide peak (server + harness share this binary)
	peakSys       uint64
}

// memSampler polls runtime.MemStats until stop is closed, tracking peaks.
// The reading is process-wide — this test binary hosts both the server and
// the client goroutines — but the harness side is intentionally lean
// (io.Discard body sinks, one small bufio reader per SSE conn), so peak
// heap is dominated by server-side per-request state; the report
// quantifies the split by comparing scales.
func memSampler(stop <-chan struct{}) func() (heap, sys uint64) {
	var peakHeap, peakSys uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		var ms runtime.MemStats
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				runtime.ReadMemStats(&ms)
				if ms.HeapInuse > peakHeap {
					peakHeap = ms.HeapInuse
				}
				if ms.Sys > peakSys {
					peakSys = ms.Sys
				}
			}
		}
	}()
	return func() (uint64, uint64) {
		<-done
		return peakHeap, peakSys
	}
}

// loadFixture stands up the server once (seeding the SYS-120 reference
// corpus is comparatively expensive) for every scale a test runs.
type loadFixture struct {
	ts         *httptest.Server
	fix        apptest.Fixture
	lf         apptest.LargeMeetFixture
	resultsURL string
	sseURL     string
	trackUnit  string
}

func newLoadFixture(t *testing.T) *loadFixture {
	t.Helper()
	fix := apptest.New(t, time.Hour)
	lf := apptest.SeedLargeMeet(t, fix, apptest.SYS120Scale)

	cats, err := i18n.Load()
	if err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	bus := NewBus()
	cfg := Config{Addr: "127.0.0.1:0", TLS: TLSConfig{Mode: TLSModeOff}, AppVersion: "load-test"}
	srv := New(cfg, fix.Auth, fix.Sessions, fix.Meets, fix.Results, fix.Backup, cats, bus).SetPrivacy(fix.Privacy)
	ts := httptest.NewServer(srv.routes())
	t.Cleanup(ts.Close)

	// A track unit for the SSE-trigger save (SeedLargeMeet reuses each
	// discipline code across ~11 units to reach 250 event-units, so the
	// DisciplineCode-keyed SaveResult overload does not apply — use the
	// unit-keyed SaveTrackResult, same as the real capture UI and the
	// SYS-120 result-save benchmark).
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("BuiltinDisciplineCatalog: %v", err)
	}
	trackUnit := ""
	for i, code := range lf.DisciplineCodes {
		if disc, ok := catalog.ByCode(code); ok && disc.Family == domain.FamilyTrack {
			trackUnit = lf.UnitIDs[i]
			break
		}
	}
	if trackUnit == "" || len(lf.AthleteIDs) == 0 {
		t.Fatal("fixture has no track unit/athletes")
	}
	return &loadFixture{
		ts: ts, fix: fix, lf: lf,
		resultsURL: ts.URL + "/m/" + lf.MeetID + "/results",
		sseURL:     ts.URL + "/events/meet-" + lf.MeetID,
		trackUnit:  trackUnit,
	}
}

// runViewerLoad launches `viewers` concurrent public-results GETs plus
// `sseViewers` held-open SSE subscriptions, publishes one real result save
// mid-run, and returns the measured metrics.
func runViewerLoad(t *testing.T, f *loadFixture, viewers, sseViewers int, timeout time.Duration) loadMetrics {
	t.Helper()
	// Settle the previous scale's garbage so peak-memory deltas between
	// scales measure THIS scale's in-flight cost, not leftovers.
	runtime.GC()
	debug.FreeOSMemory()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stopSampler := make(chan struct{})
	samplerResult := memSampler(stopSampler)

	var (
		mu        sync.Mutex
		latencies = make([]time.Duration, 0, viewers)
		errCount  int
	)
	var wg sync.WaitGroup
	wg.Add(viewers)
	for i := 0; i < viewers; i++ {
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.resultsURL, nil)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				return
			}
			start := time.Now()
			resp, err := http.DefaultClient.Do(req)
			elapsed := time.Since(start)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errCount++
				return
			}
			_, _ = readAndDiscard(resp)
			latencies = append(latencies, elapsed)
			if resp.StatusCode != http.StatusOK {
				errCount++
			}
		}()
	}

	sseLatencies := make([]time.Duration, sseViewers)
	sseErrors := make([]bool, sseViewers)
	published := make(chan time.Time, 1)
	var sseWG sync.WaitGroup
	sseWG.Add(sseViewers)
	for i := 0; i < sseViewers; i++ {
		go func(idx int) {
			defer sseWG.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.sseURL, nil)
			if err != nil {
				sseErrors[idx] = true
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				sseErrors[idx] = true
				return
			}
			defer func() { _ = resp.Body.Close() }()
			reader := bufio.NewReader(resp.Body)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					sseErrors[idx] = true
					return
				}
				if strings.HasPrefix(line, "event: results") {
					select {
					case at := <-published:
						sseLatencies[idx] = time.Since(at)
						published <- at // let other subscribers read it too
					default:
						// This subscriber raced the publisher's timestamp
						// hand-off — delivery was effectively immediate.
						sseLatencies[idx] = 0
					}
					return
				}
			}
		}(i)
	}

	// Let subscribers connect, then publish one confirmed result save —
	// what the office's browser POST does in production (SaveTrackResult
	// fires ResultsService.notifyChanged onto the SSE bus).
	time.Sleep(500 * time.Millisecond)
	publishAt := time.Now()
	if _, err := f.fix.Results.SaveTrackResult(context.Background(), webOffice, f.lf.MeetID, f.trackUnit, app.TrackResultInput{
		AthleteID: f.lf.AthleteIDs[0], Time: "10.10", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (SSE trigger): %v", err)
	}
	published <- publishAt

	wg.Wait()
	sseWG.Wait()
	close(stopSampler)
	peakHeap, peakSys := samplerResult()

	m := loadMetrics{viewers: viewers, errCount: errCount, succeeded: len(latencies),
		peakHeapInUse: peakHeap, peakSys: peakSys}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	m.p95 = percentileDur(latencies, 0.95)

	var sseSamples []time.Duration
	for i, lat := range sseLatencies {
		if sseErrors[i] {
			m.sseErrCount++
			continue
		}
		sseSamples = append(sseSamples, lat)
	}
	m.sseObserved = len(sseSamples)
	sort.Slice(sseSamples, func(i, j int) bool { return sseSamples[i] < sseSamples[j] })
	m.sseP95 = percentileDur(sseSamples, 0.95)
	return m
}

// percentileDur returns the p-quantile (0..1) of an ASCENDING-sorted slice
// by nearest rank; zero for an empty slice.
func percentileDur(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted))*p + 0.5)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func logMetrics(t *testing.T, label string, m loadMetrics) {
	t.Helper()
	perViewer := uint64(0)
	if m.viewers > 0 {
		perViewer = m.peakHeapInUse / uint64(m.viewers)
	}
	t.Logf("%s: viewers=%d ok=%d err=%d p95=%v | SSE observed=%d err=%d p95=%v | peakHeapInuse=%.1f MiB (%.2f MiB/viewer) peakSys=%.1f MiB",
		label, m.viewers, m.succeeded, m.errCount, m.p95,
		m.sseObserved, m.sseErrCount, m.sseP95,
		float64(m.peakHeapInUse)/(1<<20), float64(perViewer)/(1<<20), float64(m.peakSys)/(1<<20))
}

// TestSYS122ViewerScalingProfile measures the public-results surface at
// ascending viewer counts that fit comfortably in the confined memory
// scope (see the SAFETY note above), recording p95 render latency, SSE
// delivery latency, and peak heap — the per-viewer memory and latency
// slopes that (a) explain the unconfined 2,000-viewer OOM and (b) allow
// extrapolation to SYS-122's full 2,000. No SYS-122 budget assertion here
// (that is TestSYS122TwoThousandConcurrentViewers' job): this profile
// asserts only that requests succeed, and reports the scaling data the
// OQ-066 architecture decision needs.
func TestSYS122ViewerScalingProfile(t *testing.T) {
	f := newLoadFixture(t)
	for _, scale := range []struct{ viewers, sse int }{
		{100, 5}, {250, 13}, {500, 25},
	} {
		m := runViewerLoad(t, f, scale.viewers, scale.sse, 8*time.Minute)
		logMetrics(t, "SYS-122 scaling profile", m)
		if m.errCount > 0 {
			t.Errorf("scale %d: %d requests errored (all should succeed well below the memory cap)", scale.viewers, m.errCount)
		}
	}
}

// TestSYS122TwoThousandConcurrentViewers asserts the literal UC-017 #4 /
// SYS-122 budgets at the full 2,000-viewer scale. KNOWN TO FAIL on the
// current architecture (see the SAFETY note and OQ-066): store.Open's
// single connection convoys all reads and Standings recomputes uncached
// per request, so p95 is queueing delay far over the 3 s budget — and the
// aggregate in-flight memory exceeds any reasonable cap. The assertion is
// deliberately NOT weakened (the brief forbids silently relaxing an
// unmeetable budget); it flips green when the OQ-066 decision (read pool +
// results caching) lands. ONLY run inside the memory-confined scope
// (scripts/run-perf-tests.sh full).
func TestSYS122TwoThousandConcurrentViewers(t *testing.T) {
	const (
		viewers      = 2000
		sseViewers   = 100
		renderBudget = 3 * time.Second
		errorBudget  = 0.001 // 0.1%
		sseBudget    = 10 * time.Second
	)
	f := newLoadFixture(t)
	m := runViewerLoad(t, f, viewers, sseViewers, 12*time.Minute)
	logMetrics(t, "SYS-122 full-scale", m)

	errRate := float64(m.errCount) / float64(viewers)
	if errRate > errorBudget {
		t.Errorf("SYS-122: error rate %.4f%% exceeds budget %.4f%% (OQ-066)", errRate*100, errorBudget*100)
	}
	if m.p95 > renderBudget {
		t.Errorf("SYS-122: p95 render %v exceeds the 3s budget (OQ-066)", m.p95)
	}
	if m.sseP95 > sseBudget {
		t.Errorf("SYS-071 under load: SSE delivery p95 %v exceeds the 10s budget", m.sseP95)
	}
}

func readAndDiscard(resp *http.Response) (int64, error) {
	defer func() { _ = resp.Body.Close() }()
	return io.Copy(io.Discard, resp.Body)
}
