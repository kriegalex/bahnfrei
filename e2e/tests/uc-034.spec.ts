// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// UC-034 "Offline-tolerant field capture & walk-by sync" — browser E2E suite
// (SYS-085/086/087; ADR-004 §8). One test per acceptance criterion, against
// the real Go server (helpers/server.ts) and the real wire protocol
// (internal/sync/doc.go). Connectivity chaos via context.setOffline.
import {
  captureAttempt,
  cell,
  expectPending,
  expectSynced,
  offlineStatus,
  openUnit,
  standings,
  waitForOfflineReady,
} from "../helpers/capture";
import { seedUkcMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test("UC-034 #1: capture continues uninterrupted offline, with a clear indicator", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  // Connectivity cut mid-event.
  await context.setOffline(true);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "offline");

  // Attempt entry continues uninterrupted: three trials land in the local
  // queue and the grid keeps showing them.
  await captureAttempt(page, fx.athletes["101"], 1, "3.10");
  await captureAttempt(page, fx.athletes["101"], 2, "3.20");
  await captureAttempt(page, fx.athletes["101"], 3, "3.30");
  await expectPending(page, 3);
  for (const [seq, v] of [
    [1, "3.10"],
    [2, "3.20"],
    [3, "3.30"],
  ] as const) {
    await expect(
      cell(page, fx.athletes["101"], seq).locator('input[name="value"]'),
    ).toHaveValue(v);
  }
  // The indicator is clearly offline and i18n'd (visible text is non-empty).
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "offline");
  await expect(offlineStatus(page)).not.toBeEmpty();
});

test("UC-034 #2: reconnect auto-syncs in order; flaky reconnects produce no duplicates", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  // Record every sync batch the client sends: ops must be in capture order.
  const batches: { opId: string; seq: number }[][] = [];
  page.on("request", (req) => {
    if (req.url().endsWith("/sync") && req.method() === "POST") {
      const body = JSON.parse(req.postData() || "{}") as {
        ops?: { opId: string; seq: number }[];
      };
      if (body.ops) {
        batches.push(body.ops.map((o) => ({ opId: o.opId, seq: o.seq })));
      }
    }
  });

  await context.setOffline(true);
  await captureAttempt(page, fx.athletes["101"], 1, "3.10");
  await captureAttempt(page, fx.athletes["101"], 2, "3.20");
  await captureAttempt(page, fx.athletes["101"], 3, "3.30");
  await expectPending(page, 3);

  // Walk back into range: the queue flushes automatically (no sync button),
  // and the standings update.
  await context.setOffline(false);
  await expectSynced(page);
  await expect(standings(page)).toContainText("3.30", { timeout: 10_000 });

  // Ordered: within every batch, ops are in capture order (ULIDs monotonic).
  for (const batch of batches) {
    const seqs = batch.map((o) => o.seq);
    expect(seqs).toEqual([...seqs].sort((a, b) => a - b));
    const ids = batch.map((o) => o.opId);
    expect(ids).toEqual([...ids].sort());
  }

  // The nastiest flaky reconnect: the server RECEIVES and APPLIES the batch
  // but the device never sees the acknowledgement (connection dies on the
  // response). The client must retry with the SAME opId and the server must
  // answer "duplicate" — no second write (SYS-085 exactly-once).
  let dropped = false;
  await context.route("**/sync", async (route) => {
    if (dropped) {
      return route.continue();
    }
    dropped = true;
    await route.fetch(); // server processes the batch…
    await route.abort("connectionreset"); // …but the ack never arrives
  });
  await context.setOffline(true);
  await captureAttempt(page, fx.athletes["102"], 1, "3.55");
  await expectPending(page, 1);
  await context.setOffline(false);
  await expectSynced(page); // retry with the same opId acked as duplicate
  await context.unroute("**/sync");
  expect(dropped).toBe(true);

  // No duplicates: reload and check the server's stored state — each trial
  // has version 1 (a double-apply would have bumped it to 2).
  await page.reload();
  for (const [athlete, seq, v] of [
    [fx.athletes["101"], 1, "3.10"],
    [fx.athletes["101"], 2, "3.20"],
    [fx.athletes["101"], 3, "3.30"],
    [fx.athletes["102"], 1, "3.55"],
  ] as const) {
    const c = cell(page, athlete, seq);
    await expect(c.locator('input[name="value"]')).toHaveValue(v);
    await expect(c.locator('input[name="version"]')).toHaveValue("1");
  }
  // And a flaky replay never lands in reconciliation either.
  const recon = await context.request.get(
    `${app.baseURL}/meets/${fx.meetID}/reconciliation`,
  );
  expect(await recon.text()).toContain("Keine offenen"); // empty state (DE)
});

test("UC-034 #3: offline page reload survives; queued attempts persist and sync on reconnect", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);
  // Wait until the service worker holds the page in its cache, so an offline
  // reload can be served without the network (UC-034 #3).
  await waitForOfflineReady(page);

  await context.setOffline(true);
  await captureAttempt(page, fx.athletes["101"], 1, "3.11");
  await captureAttempt(page, fx.athletes["102"], 1, "3.22");
  await expectPending(page, 2);

  // Reload while STILL offline: the service worker serves the page and the
  // IndexedDB queue survives independently — the captured attempts are
  // restored into the grid and still pending.
  await page.reload();
  // Faithfulness guard: the reloaded page is SW-controlled, and the network
  // is genuinely unreachable THROUGH the service worker too (this probe is
  // SW-mediated and never cached) — so the page can only have come from the
  // SW cache, not from a network fetch that slipped past the emulation.
  expect(
    await page.evaluate(() => navigator.serviceWorker.controller !== null),
  ).toBe(true);
  expect(
    await page.evaluate(() =>
      fetch("/healthz").then(
        () => "reachable",
        () => "unreachable",
      ),
    ),
  ).toBe("unreachable");
  await expectPending(page, 2);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "offline");
  await expect(
    cell(page, fx.athletes["101"], 1).locator('input[name="value"]'),
  ).toHaveValue("3.11");
  await expect(
    cell(page, fx.athletes["102"], 1).locator('input[name="value"]'),
  ).toHaveValue("3.22");

  // Reconnect: they sync with no operator action.
  await context.setOffline(false);
  await expectSynced(page);
  await expect(standings(page)).toContainText("3.11", { timeout: 10_000 });
  await expect(standings(page)).toContainText("3.22");
});

test("UC-034 #4: office start-list revision while offline routes captures to reconciliation", async ({
  app,
  context,
  page,
  playwright,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  // Device goes offline with a queued capture…
  await context.setOffline(true);
  await captureAttempt(page, fx.athletes["101"], 1, "3.33");
  await expectPending(page, 1);

  // …meanwhile the office revises the unit's start list (slice-1 route).
  const office = await playwright.request.newContext();
  await setupAndLogin(office, app.baseURL);
  const meetsPage = await office.get(`${app.baseURL}/meets`);
  const csrf = (await meetsPage.text()).match(
    /name="csrf_token" value="([^"]+)"/,
  )![1];
  const revise = await office.post(`${fx.unitURL}/revise-startlist`, {
    form: { csrf_token: csrf },
    maxRedirects: 0,
  });
  expect(revise.status()).toBe(303);

  // Device walks back into range: the op lands in the office reconciliation
  // view — never silently discarded (the Web.TEC 2 loss mode C2.1 is the
  // named anti-pattern) — and the device shows a non-blocking notice.
  await context.setOffline(false);
  await expectSynced(page);
  await expect(page.locator(".offline-notice")).toBeVisible();

  const recon = await office.get(
    `${app.baseURL}/meets/${fx.meetID}/reconciliation`,
  );
  const reconBody = await recon.text();
  expect(reconBody).toContain("Zone Long Jump (UKC)");
  expect(reconBody).toContain("3.33");
  expect(reconBody).toContain("Startliste"); // reason: start-list change (DE)

  // Not applied to the result state (the office decides).
  const st = await office.get(`${fx.unitURL}/standings`);
  expect(await st.text()).not.toContain("3.33");
  await office.dispose();
});

test("UC-034 #5: office checkout override — capture resumes elsewhere; stale device reconciles", async ({
  app,
  browser,
  context,
  page,
  playwright,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  // Device A holds the checkout and goes offline with a queued capture.
  await openUnit(page, fx.unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");
  await context.setOffline(true);
  await captureAttempt(page, fx.athletes["101"], 1, "3.11");
  await expectPending(page, 1);

  // Device A is lost: the office overrides the checkout (audited, SYS-046).
  const office = await playwright.request.newContext();
  await setupAndLogin(office, app.baseURL);
  const meetsPage = await office.get(`${app.baseURL}/meets`);
  const csrf = (await meetsPage.text()).match(
    /name="csrf_token" value="([^"]+)"/,
  )![1];
  const override = await office.post(`${fx.unitURL}/override`, {
    form: { csrf_token: csrf, device: "tablet-B", reason: "device lost" },
    maxRedirects: 0,
  });
  expect(override.status()).toBe(303);

  // Capture resumes on a second device (its own browser context/profile).
  const contextB = await browser.newContext();
  const pageB = await contextB.newPage();
  await setupAndLogin(contextB.request, app.baseURL);
  await openUnit(pageB, fx.unitURL);
  await captureAttempt(pageB, fx.athletes["101"], 1, "3.99");
  await expectSynced(pageB);
  await expect(standings(pageB)).toContainText("3.99", { timeout: 10_000 });

  // The original device reconnects: its stale capture lands in
  // reconciliation, NOT silently applied over device B's value.
  await context.setOffline(false);
  await expectSynced(page);
  await expect(page.locator(".offline-notice")).toBeVisible();

  const recon = await office.get(
    `${app.baseURL}/meets/${fx.meetID}/reconciliation`,
  );
  const reconBody = await recon.text();
  expect(reconBody).toContain("3.11");
  expect(reconBody).toContain("Veraltete"); // reason: stale checkout (DE)

  const st = await office.get(`${fx.unitURL}/standings`);
  const stBody = await st.text();
  expect(stBody).toContain("3.99"); // device B's capture stands
  expect(stBody).not.toContain("3.11"); // device A's did not overwrite it

  await contextB.close();
  await office.dispose();
});

test("UC-034 #6: the sync protocol is transport-agnostic (same-origin HTTP only)", async ({
  app,
  context,
  page,
}) => {
  // Wi-Fi vs the device's own mobile data is indistinguishable at this
  // layer by construction: the island speaks plain same-origin HTTP(S) to
  // relative URLs — no Wi-Fi-specific discovery, no fixed LAN addresses, no
  // second transport stack. Criteria #1–#3 above only manipulate
  // connectivity (offline/online), never a transport type, so they hold
  // identically over any IP path. This test pins the construction: every
  // request the capture page makes goes to the page's own origin, and the
  // island's endpoints are origin-relative.
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  const offOrigin: string[] = [];
  page.on("request", (req) => {
    if (!req.url().startsWith(app.baseURL)) {
      offOrigin.push(req.url());
    }
  });

  await openUnit(page, fx.unitURL);
  await captureAttempt(page, fx.athletes["101"], 1, "3.40");
  await expectSynced(page);

  const config = page.locator("#capture-offline-config");
  for (const attr of ["data-sync-url", "data-checkout-url", "data-sw-url"]) {
    const v = await config.getAttribute(attr);
    expect(v, `${attr} must be origin-relative`).toMatch(/^\//);
  }
  expect(offOrigin, "no request may leave the server origin").toEqual([]);
});

test("UC-034 #7: office surface tolerates a ≤5-min blip — banner, no data loss, no re-login", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  // The session cookie must outlive a 5-minute blip (SYS-087: reconnection
  // without re-login) — default TTL is hours, not minutes.
  const session = (await context.cookies(app.baseURL)).find(
    (c) => c.name === "bf_session",
  );
  expect(session).toBeDefined();
  expect(session!.expires * 1000).toBeGreaterThan(Date.now() + 5 * 60 * 1000);

  // Mid-form on an office surface (roster/check-in class of page)…
  await page.goto(`${app.baseURL}/meets/${fx.meetID}/roster`);
  await page.locator('input[name="first_name"]').fill("Cara");
  await page.locator('input[name="last_name"]').fill("Chaos");
  await page.locator('input[name="birth_year"]').fill("2014");
  await page.locator('select[name="sex"]').selectOption("W");
  await page.locator('input[name="bib"]').fill("103");

  // …the connection drops: degraded banner appears, and a submit attempt
  // must NOT navigate away and clear the operator's typed input.
  await context.setOffline(true);
  await expect(page.locator("#offline-banner")).toBeVisible();
  await page.locator('form button[type="submit"]').last().click();
  await expect(page.locator('input[name="first_name"]')).toHaveValue("Cara");
  await expect(page.locator('input[name="bib"]')).toHaveValue("103");
  await expect(page.locator("#offline-banner")).toBeVisible();

  // Reconnect (a short blip, well under 5 minutes): banner clears by itself,
  // the submit now succeeds — same session, no restart, no re-entry.
  await context.setOffline(false);
  await expect(page.locator("#offline-banner")).toBeHidden();
  await page.locator('form button[type="submit"]').last().click();
  await expect(page.locator("body")).toContainText("Cara");
  await expect(page.locator("body")).toContainText("103");
  // Still logged in: the operator navigation shows the session, not /login.
  await expect(page.locator("form[action='/logout'] button")).toBeVisible();
  expect(page.url()).not.toContain("/login");
});
