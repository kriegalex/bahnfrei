// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// UC-042 "Public find-your-athlete" browser E2E suite (SYS-153, TASK-048):
// the client-side filter island (public-filter.ts) against the real public
// results page — phone viewport, live-update persistence (UC-042 #3), and
// the no-JS GET fallback (UC-042 #1's "functional without client-side
// scripting"). Handler-level coverage (jump nav, ?q= server-side filtering,
// cache safety, localization) lives in internal/web/public_test.go; this
// suite only covers what needs a real browser.
import { captureAttempt, expectSynced, openUnit } from "../helpers/capture";
import { seedUkcMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test("UC-042 #1: filtering by name hides non-matching rows on a phone viewport", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  await page.setViewportSize({ width: 360, height: 740 });
  await page.goto(`${app.baseURL}/m/${fx.meetID}/results`);

  await expect(page.getByText("Anna Muster")).toBeVisible();
  await expect(page.getByText("Bea Beispiel")).toBeVisible();

  const filterInput = page.locator('[data-public-filter] input[name="q"]');
  await filterInput.fill("Anna");

  await expect(page.getByText("Anna Muster")).toBeVisible();
  await expect(page.getByText("Bea Beispiel")).toBeHidden();
  await expect(page.locator("[data-filter-count]")).toContainText("1");

  // Clearing the filter restores every row (progressive enhancement is a
  // live, reversible client-side transform — nothing was re-fetched).
  await filterInput.fill("");
  await expect(page.getByText("Bea Beispiel")).toBeVisible();
});

test("UC-042 #3: a filter active on the results page survives the SSE live-refresh swap", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  await page.goto(`${app.baseURL}/m/${fx.meetID}/results`);
  const filterInput = page.locator('[data-public-filter] input[name="q"]');
  await filterInput.fill("Anna");
  await expect(page.getByText("Bea Beispiel")).toBeHidden();

  // A real result save from the office capture surface, on a second page
  // sharing the same authenticated context — this is what fires the
  // meet's SSE "results" event the public page's live-refresh island
  // (public-live.js) is listening for.
  const officePage = await context.newPage();
  await openUnit(officePage, fx.unitURL);
  await captureAttempt(officePage, fx.athletes["101"], 1, "3.10");
  await expectSynced(officePage); // wait for the offline queue to flush before closing the page
  await officePage.close();

  // The live-refresh island replaces #public-results' entire content on
  // the SSE event; waiting for the new mark to appear proves that swap
  // happened (not just that the filter never changed anything).
  await expect(page.locator("#public-results")).toContainText("3.10", { timeout: 15_000 });

  // public-filter.ts must have restored the query into the (freshly
  // swapped-in) input and re-applied it to the fresh rows.
  await expect(filterInput).toHaveValue("Anna");
  await expect(page.getByText("Anna Muster")).toBeVisible();
  await expect(page.getByText("Bea Beispiel")).toBeHidden();
});

test.describe("no-JS fallback", () => {
  test.use({ javaScriptEnabled: false });

  test("UC-042 #1: the plain GET ?q= form submit filters rows with no client-side scripting", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedUkcMeet(context.request, app.baseURL);

    await page.goto(`${app.baseURL}/m/${fx.meetID}/results`);
    await expect(page.getByText("Anna Muster")).toBeVisible();
    await expect(page.getByText("Bea Beispiel")).toBeVisible();

    await page.locator('[data-public-filter] input[name="q"]').fill("Anna");
    await page.locator('[data-public-filter] form[role="search"] button[type="submit"]').click();

    await expect(page).toHaveURL(/[?&]q=Anna/);
    await expect(page.getByText("Anna Muster")).toBeVisible();
    // No JS ran at all, so a non-matching row is not merely hidden by the
    // island — the server never rendered it in the first place.
    await expect(page.getByText("Bea Beispiel")).toBeHidden();
    await expect(page.locator("[data-filter-count]")).toContainText("1");
  });
});
