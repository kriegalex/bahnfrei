// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-024 accessibility suite (UC-026 "Accessibility of public surfaces",
// SYS-112/SYS-113): an automated WCAG 2.2 AA scan (axe-core, the same
// engine the file name signals as "SYS113" — one file covers both SYS-112
// and SYS-113 since they're audited together per the traceability matrix)
// against every public page type SYS-070 names — overview, timetable,
// start lists, results — plus the 360px-width usability check SYS-113
// requires explicitly. Seeds a real UBS Kids Cup meet (the same fixture
// uc-034.spec.ts uses) through the real HTTP surface, publishes its
// timetable, and captures one real result so the results page renders its
// populated division-standings table, not just the empty state.
import AxeBuilder from "@axe-core/playwright";
import { captureAttempt, expectSynced, openUnit } from "../helpers/capture";
import { publishTimetable, seedUkcMeet, setupAndLogin, type UkcFixture } from "../helpers/seed";
import { expect, test } from "../helpers/server";
import type { Page } from "@playwright/test";

/** Runs the WCAG 2.2 AA automated rule set against the current page (UC-026
 *  #1) and asserts zero violations, with the violation list attached to the
 *  failure so a real regression is diagnosable from CI output alone. */
async function expectNoWCAG22AAViolations(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([]);
}

/** Seeds a UKC meet, publishes its timetable, and captures one Zone Long
 *  Jump result — the shared fixture every scan in this file starts from,
 *  so all four public page types have real, non-empty content. */
async function seedPopulatedMeet(
  request: import("@playwright/test").APIRequestContext,
  baseURL: string,
  page: Page,
): Promise<UkcFixture> {
  await setupAndLogin(request, baseURL);
  const fx = await seedUkcMeet(request, baseURL);
  await publishTimetable(request, baseURL, fx.meetID, fx.units["Zonen-Weitsprung (UKC)"]);
  await openUnit(page, fx.unitURL);
  await captureAttempt(page, fx.athletes["101"], 1, "3.10");
  await expectSynced(page); // wait for the offline queue to flush before reading it back publicly
  return fx;
}

test.describe("UC-026 automated WCAG 2.2 AA scan (SYS-112, SYS-113)", () => {
  test("UC-026 #1: meet overview (/m/{id}) has zero automatable WCAG 2.2 AA violations", async ({
    app,
    context,
    page,
  }) => {
    const fx = await seedPopulatedMeet(context.request, app.baseURL, page);
    await page.goto(`${app.baseURL}/m/${fx.meetID}`);
    await expectNoWCAG22AAViolations(page);
  });

  test("UC-026 #1: public timetable (/m/{id}/timetable) has zero automatable WCAG 2.2 AA violations", async ({
    app,
    context,
    page,
  }) => {
    const fx = await seedPopulatedMeet(context.request, app.baseURL, page);
    await page.goto(`${app.baseURL}/m/${fx.meetID}/timetable`);
    await expectNoWCAG22AAViolations(page);
  });

  test("UC-026 #1: public start lists (/m/{id}/startlists) has zero automatable WCAG 2.2 AA violations", async ({
    app,
    context,
    page,
  }) => {
    const fx = await seedPopulatedMeet(context.request, app.baseURL, page);
    await page.goto(`${app.baseURL}/m/${fx.meetID}/startlists`);
    await expectNoWCAG22AAViolations(page);
  });

  test("UC-026 #1: public results (/m/{id}/results), populated with a captured result, has zero automatable WCAG 2.2 AA violations", async ({
    app,
    context,
    page,
  }) => {
    const fx = await seedPopulatedMeet(context.request, app.baseURL, page);
    await page.goto(`${app.baseURL}/m/${fx.meetID}/results`);
    // Confirm the scan actually exercises the populated table, not the
    // empty-standings state (a scan of an accidentally-empty page would
    // pass trivially and hide real findings in the table markup).
    await expect(page.locator("table")).toContainText("3.10");
    await expectNoWCAG22AAViolations(page);
  });

  test("SYS-113: public pages are usable at 360px width — no horizontal page scroll, wide tables scroll in their own box", async ({
    app,
    context,
    page,
  }) => {
    const fx = await seedPopulatedMeet(context.request, app.baseURL, page);
    await page.setViewportSize({ width: 360, height: 740 });
    for (const path of [
      `/m/${fx.meetID}`,
      `/m/${fx.meetID}/timetable`,
      `/m/${fx.meetID}/startlists`,
      `/m/${fx.meetID}/results`,
    ]) {
      await page.goto(app.baseURL + path);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      );
      expect(overflow, `${path} scrolls horizontally at 360px`).toBe(false);
      await expectNoWCAG22AAViolations(page);
    }
  });
});
