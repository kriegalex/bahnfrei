// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Offline-tolerant field-capture island (TASK-009, UC-034; SYS-085/086/087).
// The repo's first TypeScript island (ADR-003: TS, minimal, per-widget; no
// bundler, no CDN — CSP forbids inline scripts, so this compiles to a static
// /static/capture-offline.js the Go binary embeds).
//
// Responsibilities, all against the wire contract in internal/sync/doc.go:
//   - cache the checkout stamp {token, generation, startListVersion} durably
//     (IndexedDB) so a queued op can be replayed verbatim after a restart;
//   - append every attempt to a durable, ordered local queue BEFORE any
//     network I/O and update the grid optimistically, so capture never blocks
//     on connectivity (SYS-085);
//   - auto-flush the queue in order whenever connectivity is available — no
//     operator "sync" action — reusing each op's ULID as the idempotency key
//     across retries, with exponential backoff on failure;
//   - register a tightly-scoped service worker so a reload/browser restart
//     while offline still serves the capture page (UC-034 #3);
//   - show a clear, i18n'd offline/syncing/pending indicator (SYS-087).
//
// The island wraps everything in an IIFE so it declares no globals (matching
// capture.js); the ULID generator and a tiny IndexedDB layer are inlined
// because there is no bundler and the CSP blocks external modules.
(function () {
  "use strict";

  const cfg = document.getElementById("capture-offline-config");
  if (!cfg) {
    return; // not the capture page
  }
  const data = cfg.dataset;
  function req(name: string): string {
    const v = data[name];
    if (v === undefined) {
      throw new Error("capture-offline: missing data-" + name);
    }
    return v;
  }

  const SYNC_URL = req("syncUrl");
  const CHECKOUT_URL = req("checkoutUrl");
  const UNIT_ID = req("unitId");
  const CSRF = req("csrf");
  const SW_URL = req("swUrl");
  const SW_SCOPE = req("swScope");
  const TXT = {
    online: req("i18nOnline"),
    offline: req("i18nOffline"),
    syncing: req("i18nSyncing"),
    pending: req("i18nPending"), // contains "{n}"
    reconcile: req("i18nReconcile"),
  };

  const statusEl = document.getElementById("capture-offline-status");
  const standingsEl = document.querySelector<HTMLElement>("[data-sse-refresh]");
  const REFRESH_URL = standingsEl ? standingsEl.dataset.sseRefresh || "" : "";

  // A stable per-device label so re-opening the unit is an idempotent
  // re-checkout (server-side CheckoutUnit treats same account + same device
  // label as a no-op rather than bumping the generation). Persisted so the
  // device keeps its identity across reloads/restarts.
  function deviceLabel(): string {
    try {
      let l = localStorage.getItem("bf-device-label");
      if (!l) {
        l = "web-" + Math.random().toString(36).slice(2, 10);
        localStorage.setItem("bf-device-label", l);
      }
      return l;
    } catch {
      return "web";
    }
  }
  const DEVICE = deviceLabel();

  // ---- ULID (monotonic within the device; idempotency key, SYS-085) -------
  // Crockford base32, 48-bit millisecond timestamp + 80-bit randomness. When
  // two ids are minted in the same millisecond the random component is
  // incremented, so ids are strictly increasing and therefore sort in
  // capture order — the queue replays in submission order by sorting on key.
  const ENC = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
  let lastTime = 0;
  const lastRand = new Array<number>(16).fill(0);

  function randomChars(): void {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    for (let i = 0; i < 16; i++) {
      lastRand[i] = bytes[i] % 32;
    }
  }
  function incrementRand(): void {
    for (let i = 15; i >= 0; i--) {
      if (lastRand[i] < 31) {
        lastRand[i]++;
        return;
      }
      lastRand[i] = 0;
    }
  }
  function encodeTime(now: number): string {
    let out = "";
    let t = now;
    for (let i = 0; i < 10; i++) {
      out = ENC[t % 32] + out;
      t = Math.floor(t / 32);
    }
    return out;
  }
  function ulid(): string {
    const now = Date.now();
    if (now <= lastTime) {
      incrementRand();
    } else {
      lastTime = now;
      randomChars();
    }
    let rand = "";
    for (let i = 0; i < 16; i++) {
      rand += ENC[lastRand[i]];
    }
    return encodeTime(lastTime) + rand;
  }

  // ---- Durable storage (IndexedDB) ----------------------------------------
  interface QueuedOp {
    opId: string;
    unitId: string;
    athleteId: string;
    seq: number;
    value: string;
    wind: string;
    version: number;
  }
  interface Stamp {
    unitId: string;
    token: string;
    generation: number;
    startListVersion: number;
  }

  const DB_NAME = "bahnfrei-capture";
  const DB_VERSION = 1;
  const STORE_QUEUE = "queue";
  const STORE_STAMPS = "stamps";

  function openDB(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
      const open = indexedDB.open(DB_NAME, DB_VERSION);
      open.onupgradeneeded = () => {
        const db = open.result;
        if (!db.objectStoreNames.contains(STORE_QUEUE)) {
          db.createObjectStore(STORE_QUEUE, { keyPath: "opId" });
        }
        if (!db.objectStoreNames.contains(STORE_STAMPS)) {
          db.createObjectStore(STORE_STAMPS, { keyPath: "unitId" });
        }
      };
      open.onsuccess = () => resolve(open.result);
      open.onerror = () => reject(open.error);
    });
  }

  let dbPromise: Promise<IDBDatabase> | null = null;
  function db(): Promise<IDBDatabase> {
    if (!dbPromise) {
      dbPromise = openDB();
    }
    return dbPromise;
  }

  function tx<T>(
    store: string,
    mode: IDBTransactionMode,
    body: (s: IDBObjectStore) => IDBRequest<T>,
  ): Promise<T> {
    return db().then(
      (d) =>
        new Promise<T>((resolve, reject) => {
          const t = d.transaction(store, mode);
          const request = body(t.objectStore(store));
          t.oncomplete = () => resolve(request.result);
          t.onerror = () => reject(t.error);
          t.onabort = () => reject(t.error);
        }),
    );
  }

  function putStamp(s: Stamp): Promise<IDBValidKey> {
    return tx(STORE_STAMPS, "readwrite", (store) => store.put(s));
  }
  function getStamp(): Promise<Stamp | undefined> {
    return tx<Stamp | undefined>(STORE_STAMPS, "readonly", (store) =>
      store.get(UNIT_ID),
    );
  }
  function putOp(op: QueuedOp): Promise<IDBValidKey> {
    return tx(STORE_QUEUE, "readwrite", (store) => store.put(op));
  }
  function deleteOp(opId: string): Promise<undefined> {
    return tx<undefined>(STORE_QUEUE, "readwrite", (store) => store.delete(opId));
  }
  function allOps(): Promise<QueuedOp[]> {
    return tx<QueuedOp[]>(STORE_QUEUE, "readonly", (store) => store.getAll()).then(
      (ops) =>
        ops
          .filter((o) => o.unitId === UNIT_ID)
          .sort((a, b) => (a.opId < b.opId ? -1 : a.opId > b.opId ? 1 : 0)),
    );
  }

  // ---- Indicator (SYS-087) ------------------------------------------------
  function render(state: "online" | "offline" | "syncing", pending: number): void {
    if (!statusEl) {
      return;
    }
    let label = TXT.online;
    if (state === "offline") {
      label = TXT.offline;
    } else if (state === "syncing") {
      label = TXT.syncing;
    }
    if (pending > 0) {
      label += " · " + TXT.pending.replace("{n}", String(pending));
    }
    statusEl.textContent = label;
    statusEl.dataset.state = state;
    statusEl.dataset.pending = String(pending);
  }

  async function refreshIndicator(state?: "online" | "offline" | "syncing"): Promise<void> {
    const pending = (await allOps()).length;
    const s = state || (navigator.onLine ? "online" : "offline");
    render(s, pending);
  }

  let noticeShown = false;
  function showReconcileNotice(): void {
    if (noticeShown || !statusEl) {
      return;
    }
    noticeShown = true;
    const note = document.createElement("p");
    note.className = "offline-notice";
    note.setAttribute("role", "status");
    note.textContent = TXT.reconcile;
    statusEl.insertAdjacentElement("afterend", note);
  }

  // ---- Optimistic grid update --------------------------------------------
  // Update the cell the op came from without waiting for the network: show the
  // captured value and, once the server confirms (applied/duplicate), advance
  // the cell's stored version so a later correction carries the right expected
  // version. Offline, this keeps the grid live entirely from local state; when
  // online the standings section stays the server's source of truth (it
  // refreshes over SSE / after each flush).
  function cellFor(athleteId: string, seq: number): HTMLFormElement | null {
    return document.querySelector<HTMLFormElement>(
      '.cell-form[data-athlete="' +
        cssEscape(athleteId) +
        '"][data-seq="' +
        seq +
        '"]',
    );
  }
  function cssEscape(v: string): string {
    return v.replace(/["\\]/g, "\\$&");
  }
  function bumpCellVersion(athleteId: string, seq: number): void {
    const form = cellFor(athleteId, seq);
    if (!form) {
      return;
    }
    const vInput = form.querySelector<HTMLInputElement>('input[name="version"]');
    if (vInput) {
      vInput.value = String((parseInt(vInput.value, 10) || 0) + 1);
    }
  }

  // ---- Flush (auto, in-order, idempotent) ---------------------------------
  let flushing = false;
  let flushAgain = false;
  let backoff = 0;
  let retryTimer: number | undefined;
  const BACKOFF_MIN = 1000;
  const BACKOFF_MAX = 30000;

  function scheduleRetry(): void {
    backoff = backoff === 0 ? BACKOFF_MIN : Math.min(backoff * 2, BACKOFF_MAX);
    window.clearTimeout(retryTimer);
    retryTimer = window.setTimeout(() => {
      void flush();
    }, backoff);
  }

  async function flush(): Promise<void> {
    if (flushing) {
      flushAgain = true;
      return;
    }
    if (!navigator.onLine) {
      await refreshIndicator("offline");
      return;
    }
    const ops = await allOps();
    if (ops.length === 0) {
      await refreshIndicator("online");
      return;
    }
    const stamp = await getStamp();
    if (!stamp) {
      // No stamp yet (never checked out online): try to obtain one, else wait.
      const ok = await ensureCheckout();
      if (!ok) {
        scheduleRetry();
        return;
      }
    }

    flushing = true;
    render("syncing", ops.length);
    let appliedAny = false;
    try {
      const s = (await getStamp())!;
      const body = {
        token: s.token,
        deviceLabel: DEVICE,
        generation: s.generation,
        startListVersion: s.startListVersion,
        ops: ops.map((o) => ({
          opId: o.opId,
          athleteId: o.athleteId,
          seq: o.seq,
          value: o.value,
          wind: o.wind || undefined,
          version: o.version,
        })),
      };
      const res = await fetch(SYNC_URL, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": CSRF },
        body: JSON.stringify(body),
        credentials: "same-origin",
      });
      if (!res.ok) {
        throw new Error("sync HTTP " + res.status);
      }
      const out = (await res.json()) as {
        results: { opId: string; status: string; reason?: string }[];
      };
      const byId = new Map(ops.map((o) => [o.opId, o]));
      for (const r of out.results) {
        const op = byId.get(r.opId);
        if (r.status === "applied") {
          appliedAny = true;
          if (op) {
            bumpCellVersion(op.athleteId, op.seq);
          }
          await deleteOp(r.opId);
        } else if (r.status === "duplicate") {
          if (op) {
            bumpCellVersion(op.athleteId, op.seq);
          }
          await deleteOp(r.opId);
        } else if (r.status === "reconciliation") {
          showReconcileNotice();
          await deleteOp(r.opId);
        }
      }
      backoff = 0; // success resets backoff
    } catch {
      scheduleRetry();
      flushing = false;
      await refreshIndicator();
      return;
    } finally {
      flushing = false;
    }

    if (appliedAny && REFRESH_URL) {
      // Pull the authoritative standings fragment straight away rather than
      // waiting for the SSE round-trip, which may still be reconnecting after
      // a blip (UC-034 #2: standings update on reconnect).
      void refreshStandings();
    }
    await refreshIndicator();
    if (flushAgain) {
      flushAgain = false;
      void flush();
    }
  }

  async function refreshStandings(): Promise<void> {
    if (!REFRESH_URL || !standingsEl) {
      return;
    }
    try {
      const res = await fetch(REFRESH_URL, { headers: { Accept: "text/html" } });
      if (res.ok) {
        standingsEl.innerHTML = await res.text();
      }
    } catch {
      /* keep last good standings */
    }
  }

  async function ensureCheckout(): Promise<boolean> {
    if (!navigator.onLine) {
      return false;
    }
    // Never re-stamp while captures are queued: a queued op must replay under
    // the stamp it was captured with (internal/sync/doc.go: the device resends
    // the whole stamp verbatim). Re-checking-out here after an office override
    // or start-list change would mint a fresh token/generation and make stale
    // ops apply silently instead of routing to reconciliation (UC-034 #4/#5).
    const existing = await getStamp();
    if (existing && (await allOps()).length > 0) {
      return true;
    }
    try {
      const res = await fetch(CHECKOUT_URL, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": CSRF },
        body: JSON.stringify({ deviceLabel: DEVICE }),
        credentials: "same-origin",
      });
      if (!res.ok) {
        return false;
      }
      const co = (await res.json()) as {
        token: string;
        generation: number;
        startListVersion: number;
      };
      await putStamp({
        unitId: UNIT_ID,
        token: co.token,
        generation: co.generation,
        startListVersion: co.startListVersion,
      });
      return true;
    } catch {
      return false;
    }
  }

  // ---- Capture entry (intercept the per-cell form submit) -----------------
  async function enqueue(form: HTMLFormElement): Promise<void> {
    const athleteId = form.dataset.athlete || "";
    const seq = parseInt(form.dataset.seq || "0", 10);
    const valueInput = form.querySelector<HTMLInputElement>('input[name="value"]');
    const windInput = form.querySelector<HTMLInputElement>('input[name="wind"]');
    const versionInput = form.querySelector<HTMLInputElement>('input[name="version"]');
    const op: QueuedOp = {
      opId: ulid(),
      unitId: UNIT_ID,
      athleteId,
      seq,
      value: valueInput ? valueInput.value.trim() : "",
      wind: windInput ? windInput.value.trim() : "",
      version: versionInput ? parseInt(versionInput.value, 10) || 0 : 0,
    };
    // Durable-first: the op is in IndexedDB before any network I/O, so a crash
    // or offline reload never loses the capture (SYS-085).
    await putOp(op);
    form.dataset.pending = "1";
    await refreshIndicator();
    void flush();
  }

  document.querySelectorAll<HTMLFormElement>(".cell-form").forEach((form) => {
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      void enqueue(form);
    });
  });

  // ---- Service worker (UC-034 #3: survive reload/restart offline) ---------
  // After registration, ask the worker to cache THIS page: the first
  // navigation happened before the worker controlled the page, so without
  // this an offline reload right after the first visit would miss the cache.
  if ("serviceWorker" in navigator) {
    navigator.serviceWorker
      .register(SW_URL, { scope: SW_SCOPE })
      .then(() => navigator.serviceWorker.ready)
      .then((reg) => {
        reg.active?.postMessage({ type: "cache-page", url: location.href });
      })
      .catch(() => {
        /* SW is an enhancement; capture still works from IndexedDB without it */
      });
  }

  // ---- Local grid state after an offline reload (UC-034 #3) ---------------
  // A page served from the SW cache shows the server state at cache time; the
  // queued (not yet acknowledged) captures live in IndexedDB. Re-apply them to
  // the grid so entry continues from what the official actually captured.
  async function applyQueueToGrid(): Promise<void> {
    for (const op of await allOps()) {
      const form = cellFor(op.athleteId, op.seq);
      if (!form) {
        continue;
      }
      const valueInput = form.querySelector<HTMLInputElement>('input[name="value"]');
      if (valueInput) {
        valueInput.value = op.value;
      }
      const windInput = form.querySelector<HTMLInputElement>('input[name="wind"]');
      if (windInput && op.wind) {
        windInput.value = op.wind;
      }
      form.dataset.pending = "1";
    }
  }

  // ---- Wiring -------------------------------------------------------------
  window.addEventListener("online", () => {
    backoff = 0;
    void flush();
  });
  window.addEventListener("offline", () => {
    void refreshIndicator("offline");
  });

  // On load: restore local grid state, refresh the stamp (online, and only if
  // the queue is empty — see ensureCheckout), draw the indicator, and drain
  // any queue left over from a previous offline session (UC-034 #3).
  void (async () => {
    await applyQueueToGrid();
    await ensureCheckout();
    await refreshIndicator();
    void flush();
  })();
})();
