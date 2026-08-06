// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-015 (M1 demo assembly) connectivity-chaos run — UC-034 #1/#2/#7. The
// uc-034.spec.ts suite proves each acceptance criterion in isolation; this
// file simulates a realistically chaotic meet-day network that exercises
// several criteria AT ONCE and CONCURRENTLY, the way M1's club-visit demo
// actually behaves: two field officials capturing different event units
// while their connections flap repeatedly, and the office doing start-list
// work through its own blips at the same time — including a start-list
// revision landing mid-storm, which must route the affected device's
// in-flight captures to reconciliation without losing or duplicating a
// single one.
//
// Timings are compressed (tens/hundreds of ms, not the 5-30s outages the
// acceptance criteria describe) so the whole run stays CI-tolerable
// (~3 min budget) while still being genuinely non-deterministic in length
// and interleaving — a seeded PRNG only randomizes outage/window duration,
// never the logical outcome, which is pinned by explicit hand-off signals
// between the three actors (see `deferred` below).
import {
  captureAttempt,
  cell,
  expectPending,
  expectSynced,
  offlineStatus,
  openUnit,
} from "../helpers/capture";
import { csrfFrom, seedUkcMeet, setupAndLogin, type UkcFixture } from "../helpers/seed";
import { expect, test } from "../helpers/server";
import type { APIRequestContext, BrowserContext, Page } from "@playwright/test";

// ---- small test-local utilities -------------------------------------------

/** Deterministic PRNG (mulberry32) so "random" flap timings are reproducible
 *  across CI runs while still varying per device/run — the chaos is real,
 *  the test's pass/fail is not left to chance. */
function mulberry32(seed: number): () => number {
  let s = seed >>> 0;
  return function random(): number {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** A resolvable promise used to hand off strict ordering between the three
 *  concurrently-running chaos actors (e.g. "office revised the start list")
 *  without serializing the whole scenario. */
function deferredSignal(): { promise: Promise<void>; resolve: () => void } {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

/** One connectivity flap: an outage of randomized (compressed) length, then
 *  a randomized window back online. Represents the 5-30s real-world outages
 *  UC-034 describes, compressed to keep the suite CI-tolerable. */
async function flapOnce(context: BrowserContext, rand: () => number): Promise<void> {
  await context.setOffline(true);
  await new Promise((r) => setTimeout(r, 150 + rand() * 300));
  await context.setOffline(false);
  await new Promise((r) => setTimeout(r, 120 + rand() * 250));
}

/** Arms a one-shot "ack never arrives" flaky reconnect on the next /sync
 *  call: the server receives and decides the batch, but the response is
 *  dropped, forcing the client to retry with the same opId(s) (SYS-085
 *  exactly-once must hold whatever the eventual per-op outcome is). Mirrors
 *  the technique in uc-034.spec.ts "UC-034 #2". Returns a getter for whether
 *  the drop actually fired, so the caller can assert the test exercised it. */
async function armOneShotAckDrop(
  context: BrowserContext,
): Promise<{ fired(): boolean; disarm(): Promise<void> }> {
  let dropped = false;
  await context.route("**/sync", async (route) => {
    if (dropped) {
      await route.continue();
      return;
    }
    dropped = true;
    await route.fetch(); // the server processes and decides the batch...
    await route.abort("connectionreset"); // ...but the ack never arrives
  });
  return {
    fired: () => dropped,
    disarm: () => context.unroute("**/sync"),
  };
}

// ---- the three chaos actors -------------------------------------------

/** Field official A: opens the Zone Long Jump unit, captures Anna's first
 *  trial (still under the pre-revision start list), then — once the office
 *  has revised the start list mid-storm — keeps capturing through more
 *  flaps. Every capture after the revision must land in reconciliation
 *  (UC-034 #4), never silently applied and never silently dropped, and the
 *  last batch is put through a flaky-ack retry to prove the reconciliation
 *  decision is exactly-once too. */
async function deviceAStorm(
  page: Page,
  context: BrowserContext,
  unitURL: string,
  anna: string,
  bea: string,
  rand: () => number,
  firstApplied: { resolve: () => void },
  revisionDone: Promise<void>,
): Promise<void> {
  await openUnit(page, unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  // Anna's first trial, captured through a couple of flaps before the office
  // has touched anything — this one must apply normally.
  await context.setOffline(true);
  await captureAttempt(page, anna, 1, "3.05");
  await expectPending(page, 1);
  await flapOnce(context, rand);
  await context.setOffline(false);
  await expectSynced(page);
  firstApplied.resolve();

  // Wait for the office's start-list revision to land (UC-034 #4's ordering,
  // now inside a busier storm rather than in isolation).
  await revisionDone;

  // Every further capture on this device now targets a start list the
  // device's cached stamp no longer matches: each must be surfaced for
  // reconciliation, one item per op, nothing merged and nothing dropped.
  const rest: readonly (readonly [string, number, string])[] = [
    [anna, 2, "3.16"],
    [anna, 3, "3.27"],
    [bea, 1, "2.95"],
    [bea, 2, "3.02"],
  ];
  for (const [athleteID, seq, value] of rest) {
    await context.setOffline(true);
    await captureAttempt(page, athleteID, seq, value);
    await context.setOffline(false);
    await expectSynced(page);
  }
  await flapOnce(context, rand); // one more flap before the last, flaky one

  // The last capture: queue it offline, arm a one-shot ack-drop, then
  // reconnect — the retry must resolve to the SAME single reconciliation
  // item, not a duplicate one.
  await context.setOffline(true);
  await captureAttempt(page, bea, 3, "3.19");
  await expectPending(page, 1);
  const drop = await armOneShotAckDrop(context);
  await context.setOffline(false);
  await expectSynced(page);
  await drop.disarm();
  expect(drop.fired(), "the ack-drop route must have intercepted a /sync call").toBe(true);
}

/** Field official B: an independent device on the 200g Ball Throw unit,
 *  entirely unaffected by the office's start-list revision (different
 *  unit). Captures both athletes' full trial series through continuous
 *  flapping, then submits one correction under a flaky-ack retry — proving
 *  exactly-once holds for a normal apply path too, not just the
 *  reconciliation path device A exercises. */
async function deviceBStorm(
  page: Page,
  context: BrowserContext,
  unitURL: string,
  anna: string,
  bea: string,
  rand: () => number,
): Promise<void> {
  await openUnit(page, unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  const captures: readonly (readonly [string, number, string])[] = [
    [anna, 1, "12.40"],
    [bea, 1, "11.80"],
    [anna, 2, "12.55"],
    [bea, 2, "11.95"],
    [anna, 3, "12.61"],
    [bea, 3, "12.10"],
  ];
  for (const [athleteID, seq, value] of captures) {
    await flapOnce(context, rand); // repeated flaps around every trial
    await captureAttempt(page, athleteID, seq, value);
    await expectSynced(page);
  }

  // A correction (a legitimate re-save with the bumped version the island
  // already tracks) under a flaky reconnect: the server applies it but the
  // ack never arrives, so the client retries with the same opId.
  const drop = await armOneShotAckDrop(context);
  await context.setOffline(true);
  await captureAttempt(page, anna, 1, "12.45");
  await context.setOffline(false);
  await expectSynced(page);
  await drop.disarm();
  expect(drop.fired(), "the ack-drop route must have intercepted a /sync call").toBe(true);
}

/** The office: works the roster through its own blips (UC-034 #7 — typed
 *  input must survive a dropped connection, no data loss, no re-login) and,
 *  once field device A has landed its pre-revision capture, revises unit A's
 *  start list mid-storm — the exact trigger UC-034 #4 names, now happening
 *  concurrently with two field devices still capturing. */
async function officeStorm(
  page: Page,
  context: BrowserContext,
  request: APIRequestContext,
  baseURL: string,
  fx: UkcFixture,
  unitAID: string,
  rand: () => number,
  firstApplied: Promise<void>,
  revisionDone: { resolve: () => void },
): Promise<void> {
  await page.goto(`${baseURL}/meets/${fx.meetID}/roster`);
  await page.locator('input[name="first_name"]').fill("Theo");
  await page.locator('input[name="last_name"]').fill("Sturm");
  await page.locator('input[name="birth_year"]').fill("2014");
  await page.locator('select[name="sex"]').selectOption("M");
  await page.locator('input[name="bib"]').fill("103");

  // Blip mid-form: connection drops while the office is still typing. The
  // degraded banner appears and a submit attempt must not navigate away and
  // lose the typed input (SYS-087).
  await context.setOffline(true);
  await expect(page.locator("#offline-banner")).toBeVisible();
  await page.locator('form button[type="submit"]').last().click();
  await expect(page.locator('input[name="first_name"]')).toHaveValue("Theo");
  await expect(page.locator('input[name="bib"]')).toHaveValue("103");
  await expect(page.locator("#offline-banner")).toBeVisible();

  // Reconnect: banner clears, the same form now submits — no restart, no
  // re-login, no re-entry.
  await context.setOffline(false);
  await expect(page.locator("#offline-banner")).toBeHidden();
  await page.locator('form button[type="submit"]').last().click();
  await expect(page.locator("body")).toContainText("Theo");
  await expect(page.locator("form[action='/logout'] button")).toBeVisible();

  // Now the start-list work: revise unit A once device A's pre-revision
  // capture has landed (the ordering UC-034 #4 names), still mid-storm.
  await firstApplied;
  const meetsPage = await request.get(`${baseURL}/meets`);
  const csrf = csrfFrom(await meetsPage.text());
  const revise = await request.post(
    `${baseURL}/meets/${fx.meetID}/capture/${unitAID}/revise-startlist`,
    { form: { csrf_token: csrf }, maxRedirects: 0 },
  );
  expect(revise.status()).toBe(303);
  revisionDone.resolve();

  // A second office blip while the field storm keeps going — blips are not
  // a one-off; SYS-087 must hold across repeats within the same session.
  await flapOnce(context, rand);
}

// ---- the chaos run -------------------------------------------

test("M1 connectivity chaos: two field officials + office survive a flapping meet-day network", async ({
  app,
  browser,
  context,
  page,
}) => {
  test.setTimeout(90_000);

  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  // SYS-111/F6 (TASK-050): the capture index now localizes discipline
  // names, so fx.units is keyed by the DE catalog string, not the English
  // canonical name.
  const unitAID = fx.units["Zonen-Weitsprung (UKC)"];
  const unitBID = fx.units["Ballwurf 200 g (UKC)"];
  const unitAURL = `${app.baseURL}/meets/${fx.meetID}/capture/${unitAID}`;
  const unitBURL = `${app.baseURL}/meets/${fx.meetID}/capture/${unitBID}`;
  const anna = fx.athletes["101"];
  const bea = fx.athletes["102"];

  const contextB = await browser.newContext();
  const pageB = await contextB.newPage();
  await setupAndLogin(contextB.request, app.baseURL);

  const officeContext = await browser.newContext();
  const officePage = await officeContext.newPage();
  await setupAndLogin(officeContext.request, app.baseURL);

  const firstApplied = deferredSignal();
  const revisionDone = deferredSignal();

  try {
    await Promise.all([
      deviceAStorm(page, context, unitAURL, anna, bea, mulberry32(0xc0ffee), firstApplied, revisionDone.promise),
      deviceBStorm(pageB, contextB, unitBURL, anna, bea, mulberry32(0xfacade)),
      officeStorm(
        officePage,
        officeContext,
        officeContext.request,
        app.baseURL,
        fx,
        unitAID,
        mulberry32(0xbeef01),
        firstApplied.promise,
        revisionDone,
      ),
    ]);

    // ---- Post-storm: zero lost acknowledged captures, zero duplicates ----

    // Device A: Anna #1 applied (version 1); everything captured after the
    // revision was never written to the unit — reconciliation holds it, the
    // grid shows no attempt for it (never silently applied).
    await page.reload();
    await expect(cell(page, anna, 1).locator('input[name="value"]')).toHaveValue("3.05");
    await expect(cell(page, anna, 1).locator('input[name="version"]')).toHaveValue("1");
    const reconciledOnly: readonly (readonly [string, number, string])[] = [
      [anna, 2, "3.16"],
      [anna, 3, "3.27"],
      [bea, 1, "2.95"],
      [bea, 2, "3.02"],
      [bea, 3, "3.19"],
    ];
    for (const [athleteID, seq, capturedValue] of reconciledOnly) {
      const c = cell(page, athleteID, seq);
      const v = await c.locator('input[name="value"]').inputValue();
      expect(
        v,
        `athlete ${athleteID} trial ${seq} must not have been silently applied`,
      ).not.toBe(capturedValue);
      await expect(c.locator('input[name="version"]')).toHaveValue("0");
    }

    // Device B: unaffected by the revision — every trial applied exactly
    // once (version 1), and the flaky-ack-retried correction landed exactly
    // once too (version 2, not 3 — a double-apply would have bumped it
    // further).
    await pageB.reload();
    const wantB: readonly (readonly [string, number, string, string])[] = [
      [anna, 1, "12.45", "2"],
      [anna, 2, "12.55", "1"],
      [anna, 3, "12.61", "1"],
      [bea, 1, "11.80", "1"],
      [bea, 2, "11.95", "1"],
      [bea, 3, "12.10", "1"],
    ];
    for (const [athleteID, seq, value, version] of wantB) {
      const c = cell(pageB, athleteID, seq);
      await expect(c.locator('input[name="value"]')).toHaveValue(value);
      await expect(c.locator('input[name="version"]')).toHaveValue(version);
    }

    // ---- Reconciliation queue: exactly the expected conflicted items ----

    const reconBody = await (
      await officeContext.request.get(`${app.baseURL}/meets/${fx.meetID}/reconciliation`)
    ).text();
    // Exactly 5 items (Anna #2/#3, Bea #1/#2/#3), all reason "start-list
    // changed", all against the Zone Long Jump unit — device B's unit never
    // appears because it never produced a reconciliation item.
    const startListCount = (reconBody.match(/Startliste/g) ?? []).length;
    expect(startListCount).toBe(5);
    expect(reconBody).not.toContain("Veraltete");
    expect(reconBody).not.toContain("Konflikt");
    expect(reconBody).toContain("Zone Long Jump (UKC)");
    expect(reconBody).not.toContain("200 g Ball Throw (UKC)");
    for (const value of ["3.16", "3.27", "2.95", "3.02", "3.19"]) {
      expect(reconBody).toContain(value);
    }
    // Nothing silently discarded and nothing silently applied: Anna #1's
    // applied mark never shows up as a pending reconciliation item.
    expect(reconBody).not.toContain("3.05");

    // ---- Live public results reflect every settled result ----

    const resultsBody = await (
      await officeContext.request.get(`${app.baseURL}/m/${fx.meetID}/results`)
    ).text();
    expect(resultsBody).toContain("3.05"); // Anna's settled long-jump mark
    expect(resultsBody).toContain("12.61"); // Anna's best settled throw
    expect(resultsBody).toContain("12.10"); // Bea's best settled throw
    for (const value of ["3.16", "3.27", "2.95", "3.02", "3.19"]) {
      expect(resultsBody, `unsettled mark ${value} must not reach public results`).not.toContain(value);
    }
  } finally {
    await contextB.close();
    await officeContext.close();
  }
});
