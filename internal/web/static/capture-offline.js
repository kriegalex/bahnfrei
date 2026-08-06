"use strict";
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
    function req(name) {
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
        // Non-retryable outcomes (SYS-149, UC-040): a rejected op's reason
        // renders at its cell, never as a connectivity message.
        error: req("i18nError"),
        discard: req("i18nDiscard"),
        rejectGeneric: req("i18nRejectGeneric"),
        // Per-cell save-state badge text (SYS-148, UC-039 #4/#5): distinguishable
        // by text/icon, not color alone (base.css styles data-pending/data-state
        // on the same .cell-form these badges live in).
        cellPending: req("i18nCellPending"),
        cellConfirmed: req("i18nCellConfirmed"),
    };
    // Per-reason rejection text, keyed by the wire's RejectReason vocabulary
    // (internal/domain/sync.go). An unrecognized/future reason code falls
    // back to TXT.rejectGeneric rather than rendering nothing.
    const REJECT_REASON_TEXT = {
        invalid_mark: req("i18nRejectInvalidMark"),
        unknown_athlete: req("i18nRejectUnknownAthlete"),
        announced: req("i18nRejectAnnounced"),
    };
    const statusEl = document.getElementById("capture-offline-status");
    const reauthBanner = document.getElementById("capture-reauth-banner");
    const standingsEl = document.querySelector("[data-sse-refresh]");
    const REFRESH_URL = standingsEl ? standingsEl.dataset.sseRefresh || "" : "";
    // A stable per-device label so re-opening the unit is an idempotent
    // re-checkout (server-side CheckoutUnit treats same account + same device
    // label as a no-op rather than bumping the generation). Persisted so the
    // device keeps its identity across reloads/restarts.
    function deviceLabel() {
        try {
            let l = localStorage.getItem("bf-device-label");
            if (!l) {
                l = "web-" + Math.random().toString(36).slice(2, 10);
                localStorage.setItem("bf-device-label", l);
            }
            return l;
        }
        catch {
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
    const lastRand = new Array(16).fill(0);
    function randomChars() {
        const bytes = new Uint8Array(16);
        crypto.getRandomValues(bytes);
        for (let i = 0; i < 16; i++) {
            lastRand[i] = bytes[i] % 32;
        }
    }
    function incrementRand() {
        for (let i = 15; i >= 0; i--) {
            if (lastRand[i] < 31) {
                lastRand[i]++;
                return;
            }
            lastRand[i] = 0;
        }
    }
    function encodeTime(now) {
        let out = "";
        let t = now;
        for (let i = 0; i < 10; i++) {
            out = ENC[t % 32] + out;
            t = Math.floor(t / 32);
        }
        return out;
    }
    function ulid() {
        const now = Date.now();
        if (now <= lastTime) {
            incrementRand();
        }
        else {
            lastTime = now;
            randomChars();
        }
        let rand = "";
        for (let i = 0; i < 16; i++) {
            rand += ENC[lastRand[i]];
        }
        return encodeTime(lastTime) + rand;
    }
    const DB_NAME = "bahnfrei-capture";
    const DB_VERSION = 1;
    const STORE_QUEUE = "queue";
    const STORE_STAMPS = "stamps";
    function openDB() {
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
    let dbPromise = null;
    function db() {
        if (!dbPromise) {
            dbPromise = openDB();
        }
        return dbPromise;
    }
    function tx(store, mode, body) {
        return db().then((d) => new Promise((resolve, reject) => {
            const t = d.transaction(store, mode);
            const request = body(t.objectStore(store));
            t.oncomplete = () => resolve(request.result);
            t.onerror = () => reject(t.error);
            t.onabort = () => reject(t.error);
        }));
    }
    function putStamp(s) {
        return tx(STORE_STAMPS, "readwrite", (store) => store.put(s));
    }
    function getStamp() {
        return tx(STORE_STAMPS, "readonly", (store) => store.get(UNIT_ID));
    }
    function putOp(op) {
        return tx(STORE_QUEUE, "readwrite", (store) => store.put(op));
    }
    function deleteOp(opId) {
        return tx(STORE_QUEUE, "readwrite", (store) => store.delete(opId));
    }
    function allOps() {
        return tx(STORE_QUEUE, "readonly", (store) => store.getAll()).then((ops) => ops
            .filter((o) => o.unitId === UNIT_ID)
            .sort((a, b) => (a.opId < b.opId ? -1 : a.opId > b.opId ? 1 : 0)));
    }
    function render(state, pending) {
        if (!statusEl) {
            return;
        }
        // "online" specifically claims every capture has been transferred
        // (TXT.online literally reads "…alle Erfassungen übertragen") — that is
        // only truthful once the queue is empty. A caller reporting "online"
        // while ops remain queued (e.g. enqueue()'s indicator refresh fires
        // right after putOp, before flush() has had a chance to mark
        // "syncing") is downgraded to "syncing" here rather than trusted
        // verbatim: this is the literal contradiction usability-audit finding
        // F1 named ("all transferred" + a non-zero pending count shown at
        // once) and UC-040 #5 forbids it unconditionally, at every call site,
        // not just the ones this file happens to get right today.
        const effective = state === "online" && pending > 0 ? "syncing" : state;
        let label = TXT.online;
        if (effective === "offline") {
            label = TXT.offline;
        }
        else if (effective === "syncing") {
            label = TXT.syncing;
        }
        else if (effective === "error") {
            label = TXT.error;
        }
        if (pending > 0) {
            label += " · " + TXT.pending.replace("{n}", String(pending));
        }
        statusEl.textContent = label;
        statusEl.dataset.state = effective;
        statusEl.dataset.pending = String(pending);
    }
    async function refreshIndicator(state) {
        const pending = (await allOps()).length;
        const s = state || (navigator.onLine ? "online" : "offline");
        render(s, pending);
    }
    let noticeShown = false;
    function showReconcileNotice() {
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
    // ---- Re-authentication prompt (SYS-149, UC-040 #4) ----------------------
    // A 401/403 from /sync mid-queue is not connectivity and not solved by
    // retrying: show a persistent banner with a link to /login and stop. The
    // durable IndexedDB queue is untouched — a plain navigation to /login and
    // back re-runs this island's startup flow (bottom of file), which resumes
    // the same queue against the same cached checkout stamp, applying every
    // pending op with no re-entry (cf. SYS-087).
    function showReauthBanner() {
        if (reauthBanner) {
            reauthBanner.hidden = false;
        }
    }
    function hideReauthBanner() {
        if (reauthBanner) {
            reauthBanner.hidden = true;
        }
    }
    // ---- Optimistic grid update --------------------------------------------
    // Update the cell the op came from without waiting for the network: show the
    // captured value and, once the server confirms (applied/duplicate), set the
    // cell's stored version to the server's authoritative value so a later
    // correction carries the right expected version. Offline, this keeps the grid
    // live entirely from local state; when online the standings section stays the
    // server's source of truth (it refreshes over SSE / after each flush).
    function cellFor(athleteId, seq) {
        return document.querySelector('.cell-form[data-athlete="' +
            cssEscape(athleteId) +
            '"][data-seq="' +
            seq +
            '"]');
    }
    function cssEscape(v) {
        return v.replace(/["\\]/g, "\\$&");
    }
    // Set the cell's optimistic version from an applied/duplicate ack. The server
    // reports the authoritative stored version, so we SET rather than blindly
    // increment: a blind +1 double-counts when the same op is acknowledged again
    // after a page render (reload/SSE) already reflected the write — e.g. a flaky
    // reconnect whose ack was lost, replayed after a reload, would push the cell
    // past the real version and make the next correction submit a stale expected
    // version (SYS-085). A missing/zero version falls back to an increment so the
    // grid still advances if the server omitted it.
    function setCellVersion(athleteId, seq, version) {
        const form = cellFor(athleteId, seq);
        if (!form) {
            return;
        }
        const vInput = form.querySelector('input[name="version"]');
        if (vInput) {
            vInput.value =
                typeof version === "number" && version > 0
                    ? String(version)
                    : String((parseInt(vInput.value, 10) || 0) + 1);
        }
    }
    // ---- Per-cell save-state badge (SYS-148, UC-039 #4/#5) ------------------
    // Styles data-pending/data-state on the SAME .cell-form base.css already
    // has rules for — never a parallel attribute — plus a small text/icon
    // badge so the state is distinguishable by more than color (WCAG 2.2).
    // "confirmed" is the state added by this task: previously data-pending
    // was set on enqueue and never cleared, so a saved cell looked identical
    // to an unsaved one forever (usability-audit finding F3).
    function cellBadge(form) {
        let badge = form.querySelector(".cell-save-badge");
        if (!badge) {
            badge = document.createElement("span");
            badge.className = "cell-save-badge";
            badge.setAttribute("aria-hidden", "true");
            form.appendChild(badge);
        }
        return badge;
    }
    function markPending(form) {
        delete form.dataset.state;
        form.dataset.pending = "1";
        cellBadge(form).textContent = TXT.cellPending;
    }
    function markConfirmed(form) {
        delete form.dataset.pending;
        form.dataset.state = "confirmed";
        cellBadge(form).textContent = TXT.cellConfirmed;
    }
    function clearCellBadge(form) {
        delete form.dataset.state;
        const badge = form.querySelector(".cell-save-badge");
        if (badge) {
            badge.textContent = "";
        }
    }
    // ---- Row-derived cells (SYS-148, UC-039 #4) ------------------------------
    // Updates the grid's own Result/Points cells for one athlete straight from
    // the sync ack's authoritative values (never a client-side recompute — it
    // cannot see corrections or the meet's scoring-table lookup), so they
    // reflect a confirmed save without a manual reload (usability-audit F3:
    // previously only the standings fragment below the fold updated).
    function updateRowDerived(athleteId, result, points) {
        const resultCell = document.querySelector('[data-role="result"][data-athlete-row="' + cssEscape(athleteId) + '"]');
        if (resultCell) {
            resultCell.textContent = result;
        }
        const pointsCell = document.querySelector('[data-role="points"][data-athlete-row="' + cssEscape(athleteId) + '"]');
        if (pointsCell) {
            pointsCell.textContent = points;
        }
    }
    // ---- Per-op rejection (SYS-149, UC-040 #1/#3) ---------------------------
    // A rejected op is terminal and non-retryable: it never re-enters the
    // queue. What renders here is purely a UI affordance at the offending
    // cell — correct-or-discard — not durable state; a reload before the
    // operator acts simply drops the notice (the underlying grid still shows
    // whatever was last confirmed by the server).
    function clearRejection(form) {
        delete form.dataset.rejected;
        form.querySelectorAll(".cell-reject").forEach((el) => el.remove());
        clearCellBadge(form);
    }
    function renderRejection(athleteId, seq, reason) {
        const form = cellFor(athleteId, seq);
        if (!form) {
            return;
        }
        clearRejection(form);
        form.dataset.rejected = reason;
        delete form.dataset.pending;
        const wrap = document.createElement("span");
        wrap.className = "cell-reject";
        const msg = document.createElement("p");
        msg.className = "field-error";
        msg.setAttribute("role", "alert");
        msg.textContent = REJECT_REASON_TEXT[reason] || TXT.rejectGeneric;
        const discard = document.createElement("button");
        discard.type = "button";
        discard.className = "cell-reject-discard";
        discard.textContent = TXT.discard;
        discard.addEventListener("click", () => {
            const valueInput = form.querySelector('input[name="value"]');
            if (valueInput) {
                valueInput.value = "";
            }
            const windInput = form.querySelector('input[name="wind"]');
            if (windInput) {
                windInput.value = "";
            }
            clearRejection(form);
        });
        wrap.appendChild(msg);
        wrap.appendChild(discard);
        // A child of the form, not a sibling: a <form> may hold arbitrary flow
        // content, and keeping the notice inside it means one query — the cell
        // form — finds the whole cell's state for tests and future styling.
        form.appendChild(wrap);
    }
    // ---- Flush (auto, in-order, idempotent) ---------------------------------
    let flushing = false;
    let flushAgain = false;
    let backoff = 0;
    let retryTimer;
    const BACKOFF_MIN = 1000;
    const BACKOFF_MAX = 30000;
    function scheduleRetry() {
        backoff = backoff === 0 ? BACKOFF_MIN : Math.min(backoff * 2, BACKOFF_MAX);
        window.clearTimeout(retryTimer);
        retryTimer = window.setTimeout(() => {
            void flush();
        }, backoff);
    }
    async function flush() {
        if (flushing) {
            flushAgain = true;
            return;
        }
        // The reentrancy lock MUST be set here, before any await: flush() reads
        // IndexedDB and can call ensureCheckout() before it ever touches the
        // network, all of which yield to the event loop. Two overlapping
        // triggers (e.g. a queued backoff-retry timer firing at the same moment
        // the 'online' event re-invokes flush() after a rapid reconnect) would
        // otherwise both observe flushing === false, both read the same pending
        // op(s), and both POST them in separate requests — a chaotic-flapping
        // bug found by the connectivity-chaos suite (e2e/tests/chaos-m1.spec.ts)
        // that produced a duplicate reconciliation item for one captured trial
        // despite the server's per-opId dedupe ledger being correct: the ledger
        // only de-duplicates a single opId across requests it actually sees, it
        // cannot undo the client having minted two requests instead of one from
        // a single queued op. Setting the lock synchronously, before the first
        // await, closes that window (SYS-085 exactly-once).
        flushing = true;
        try {
            if (!navigator.onLine) {
                await refreshIndicator("offline");
                return;
            }
            const ops = await allOps();
            if (ops.length === 0) {
                await refreshIndicator("online");
                return;
            }
            let stamp = await getStamp();
            if (!stamp) {
                // No stamp yet (never checked out online): try to obtain one, else wait.
                const ok = await ensureCheckout();
                if (!ok) {
                    scheduleRetry();
                    return;
                }
                stamp = await getStamp();
            }
            render("syncing", ops.length);
            let appliedAny = false;
            try {
                const s = stamp;
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
                if (res.status === 401 || res.status === 403) {
                    // Expired session, or (403) access to the unit no longer holds —
                    // neither is connectivity and neither is fixed by retrying.
                    // Every queued op is left untouched (SYS-149, UC-040 #4).
                    showReauthBanner();
                    await refreshIndicator();
                    return;
                }
                if (!res.ok && res.status < 500) {
                    // A non-retryable batch-level failure that is not a per-op
                    // rejection (validation/business-rule failures already come back
                    // as "rejected" inside a 200 — see below). Surface an error
                    // state, not connectivity, and stop: no scheduleRetry loop. The
                    // ops stay queued for inspection; the next capture (or a reload)
                    // triggers a fresh attempt without an automatic backoff chain.
                    await refreshIndicator("error");
                    return;
                }
                if (!res.ok) {
                    throw new Error("sync HTTP " + res.status);
                }
                hideReauthBanner();
                const out = (await res.json());
                const byId = new Map(ops.map((o) => [o.opId, o]));
                for (const r of out.results) {
                    const op = byId.get(r.opId);
                    if (r.status === "applied") {
                        appliedAny = true;
                        if (op) {
                            setCellVersion(op.athleteId, op.seq, r.version);
                            const form = cellFor(op.athleteId, op.seq);
                            if (form) {
                                markConfirmed(form);
                            }
                            updateRowDerived(op.athleteId, r.result || "", r.points || "");
                        }
                        await deleteOp(r.opId);
                    }
                    else if (r.status === "duplicate") {
                        if (op) {
                            setCellVersion(op.athleteId, op.seq, r.version);
                            const form = cellFor(op.athleteId, op.seq);
                            if (form) {
                                markConfirmed(form);
                            }
                            updateRowDerived(op.athleteId, r.result || "", r.points || "");
                        }
                        await deleteOp(r.opId);
                    }
                    else if (r.status === "reconciliation") {
                        showReconcileNotice();
                        await deleteOp(r.opId);
                    }
                    else if (r.status === "rejected") {
                        // Non-retryable (SYS-149, UC-040 #1): render at the cell, drop
                        // from the queue — never re-attempted, never a connectivity
                        // message. A rejection never blocks later ops in this same
                        // batch (UC-040 #2): every other result in `out.results` is
                        // still processed on its own branch above/below.
                        if (op) {
                            renderRejection(op.athleteId, op.seq, r.reason || "");
                        }
                        await deleteOp(r.opId);
                    }
                }
                backoff = 0; // success resets backoff
            }
            catch (err) {
                scheduleRetry();
                // fetch() rejects with a TypeError when the server is unreachable — a
                // state navigator.onLine cannot see (it is link-layer only: the venue
                // AP can be up while the meet server is down or unroutable). Show
                // "offline" so the indicator never claims Online while queued captures
                // cannot leave the device (SYS-087). HTTP-level failures keep the
                // onLine-derived state: the server answered, it is just unhappy.
                await refreshIndicator(err instanceof TypeError ? "offline" : undefined);
                return;
            }
            if (appliedAny && REFRESH_URL) {
                // Pull the authoritative standings fragment straight away rather
                // than waiting for the SSE round-trip, which may still be
                // reconnecting after a blip (UC-034 #2: standings update on
                // reconnect).
                void refreshStandings();
            }
            await refreshIndicator();
        }
        finally {
            flushing = false;
        }
        if (flushAgain) {
            flushAgain = false;
            void flush();
        }
    }
    async function refreshStandings() {
        if (!REFRESH_URL || !standingsEl) {
            return;
        }
        try {
            const res = await fetch(REFRESH_URL, { headers: { Accept: "text/html" } });
            if (res.ok) {
                standingsEl.innerHTML = await res.text();
            }
        }
        catch {
            /* keep last good standings */
        }
    }
    async function ensureCheckout() {
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
            const co = (await res.json());
            await putStamp({
                unitId: UNIT_ID,
                token: co.token,
                generation: co.generation,
                startListVersion: co.startListVersion,
            });
            return true;
        }
        catch {
            return false;
        }
    }
    // ---- Capture entry (intercept the per-cell form submit) -----------------
    async function enqueue(form) {
        const athleteId = form.dataset.athlete || "";
        const seq = parseInt(form.dataset.seq || "0", 10);
        const valueInput = form.querySelector('input[name="value"]');
        const windInput = form.querySelector('input[name="wind"]');
        const versionInput = form.querySelector('input[name="version"]');
        const op = {
            opId: ulid(),
            unitId: UNIT_ID,
            athleteId,
            seq,
            value: valueInput ? valueInput.value.trim() : "",
            wind: windInput ? windInput.value.trim() : "",
            version: versionInput ? parseInt(versionInput.value, 10) || 0 : 0,
        };
        // A fresh submit on a previously rejected cell is the "correct" path
        // (UC-040 #1/#3): clear the stale rejection notice before queuing the
        // new attempt.
        clearRejection(form);
        // Durable-first: the op is in IndexedDB before any network I/O, so a crash
        // or offline reload never loses the capture (SYS-085).
        await putOp(op);
        markPending(form);
        await refreshIndicator();
        void flush();
    }
    document.querySelectorAll(".cell-form").forEach((form) => {
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
    async function applyQueueToGrid() {
        for (const op of await allOps()) {
            const form = cellFor(op.athleteId, op.seq);
            if (!form) {
                continue;
            }
            const valueInput = form.querySelector('input[name="value"]');
            if (valueInput) {
                valueInput.value = op.value;
            }
            const windInput = form.querySelector('input[name="wind"]');
            if (windInput && op.wind) {
                windInput.value = op.wind;
            }
            markPending(form);
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
