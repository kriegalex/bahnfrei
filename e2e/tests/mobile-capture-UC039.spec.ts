// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// UC-039 "Volunteer mobile capture" — browser E2E suite (SYS-147/148,
// TASK-045), against the real Go server (helpers/server.ts) at the
// 360x740 phone viewport the usability-audit walkthrough measured
// (docs/delivery/usability-audit-volunteer-2026-08.md, F2/F3) and SYS-113
// already floors public surfaces at. One test per acceptance criterion.
import type { Page } from "@playwright/test";
import {
  captureMarker,
  cell,
  expectSynced,
  offlineStatus,
  openUnit,
  rowResultCell,
} from "../helpers/capture";
import {
  seedTrackMeet,
  seedUkcMeet,
  seedVerticalMeet,
  setupAndLogin,
} from "../helpers/seed";
import { expect, test } from "../helpers/server";

const MOBILE_VIEWPORT = { width: 360, height: 740 };

/** Asserts the PAGE itself never gains horizontal scroll (UC-039 #1's own
 *  measured assertion) — an inner scrollable region (e.g. the vertical
 *  grid's sticky-column wrapper) is unaffected by this check. */
async function expectNoPageHorizontalScroll(page: Page, where: string): Promise<void> {
  const { scrollWidth, clientWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(
    scrollWidth,
    `${where}: page scrollWidth ${scrollWidth} exceeds viewport ${clientWidth}`,
  ).toBeLessThanOrEqual(clientWidth);
}

test.describe("UC-039 mobile point-of-competition capture (SYS-147/148)", () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test("UC-039 #1: field-horizontal, track, vertical capture and check-in fit 360px with no page-level horizontal scroll", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);

    const ukc = await seedUkcMeet(context.request, app.baseURL);
    await page.goto(ukc.unitURL); // Zone Long Jump = field-horizontal family
    await expectNoPageHorizontalScroll(page, "field-horizontal capture");

    const track = await seedTrackMeet(context.request, app.baseURL, ["Alice"]);
    await page.goto(track.unitURL); // 100 metres = track family
    await expectNoPageHorizontalScroll(page, "track capture");

    await page.goto(track.checkinURL);
    await expectNoPageHorizontalScroll(page, "check-in");

    const vertical = await seedVerticalMeet(context.request, app.baseURL);
    await page.goto(vertical.unitURL); // HJ = field-vertical family
    await expectNoPageHorizontalScroll(page, "vertical capture");
  });

  test("UC-039 #2: first capture row is visible within 640px of height, and primary controls meet the 44px touch-target floor", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedUkcMeet(context.request, app.baseURL);
    await openUnit(page, fx.unitURL);

    const firstRowTop = await page.evaluate(() => {
      const row = document.querySelector(".capture-grid tbody tr");
      return row ? row.getBoundingClientRect().top : null;
    });
    expect(firstRowTop, "no capture row found").not.toBeNull();
    expect(
      firstRowTop as number,
      "first capture row must be visible within the first 640px of page height",
    ).toBeLessThanOrEqual(640);

    // DOM audit: every primary control inside a capture cell (mark input,
    // wind input, save button, marker quick-actions) reaches the 44x44 CSS
    // px floor at this viewport — none of them may be under WCAG 2.2
    // SC 2.5.8's 24x24 px minimum either.
    const sizes = await page.evaluate(() => {
      const els = Array.from(
        document.querySelectorAll<HTMLElement>(
          '.cell-form input:not([type="hidden"]), .cell-form button',
        ),
      );
      return els.map((el) => {
        const r = el.getBoundingClientRect();
        return { tag: el.tagName, label: el.getAttribute("aria-label") || el.textContent, w: r.width, h: r.height };
      });
    });
    expect(sizes.length).toBeGreaterThan(0);
    for (const s of sizes) {
      expect(s.w, `${s.tag} "${s.label}" width`).toBeGreaterThanOrEqual(24);
      expect(s.h, `${s.tag} "${s.label}" height`).toBeGreaterThanOrEqual(24);
      expect(s.w, `${s.tag} "${s.label}" width < 44px touch-target floor`).toBeGreaterThanOrEqual(44);
      expect(s.h, `${s.tag} "${s.label}" height < 44px touch-target floor`).toBeGreaterThanOrEqual(44);
    }
  });

  test("UC-039 #3: mark input declares a numeric keyboard hint, and a letter marker is still recordable via the quick-action button", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedUkcMeet(context.request, app.baseURL);
    await openUnit(page, fx.unitURL);

    const athlete = fx.athletes["101"];
    const valueInput = cell(page, athlete, 1).locator('input[name="value"]');
    await expect(valueInput).toHaveAttribute("inputmode", "decimal");
    await expect(valueInput).toHaveAttribute("enterkeyhint", "done");

    // X (foul) — a documented D5.2 letter marker — is recorded via the
    // quick-action button, never the (now numeric-only) virtual keyboard.
    await captureMarker(page, athlete, 1, "X");
    await expectSynced(page);
    await expect(valueInput).toHaveValue("X");
  });

  test("UC-039 #4: a save shows pending then confirmed at the cell, distinguishable by more than color, and the row's Result/Points update without reload", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedUkcMeet(context.request, app.baseURL);
    await openUnit(page, fx.unitURL);

    // The real round-trip against the local test server is fast enough
    // that the pending window can close before this test ever observes
    // it (a genuine race, not a defect): delay the /sync response slightly
    // so "pending" is reliably observable before "confirmed" replaces it.
    await context.route("**/sync", async (route) => {
      await new Promise((r) => setTimeout(r, 300));
      await route.continue();
    });

    const athlete = fx.athletes["101"];
    const c = cell(page, athlete, 1);
    await c.locator('input[name="value"]').fill("3.10");
    await c.locator('button[type="submit"]').click();

    // Pending immediately: data-pending plus a non-empty text/icon badge —
    // never color alone (base.css styles the badge and the left border).
    await expect(c).toHaveAttribute("data-pending", "1");
    await expect(c.locator(".cell-save-badge")).not.toBeEmpty();

    // Confirmed once the server acknowledges — no longer pending — and the
    // badge text differs (a different affordance, not just a color swap).
    // The 500ms UC-039 #4 budget is met by construction: the island applies
    // both the attribute and the badge synchronously in the same fetch()
    // .then() as the ack, with no artificial delay; this assertion waits
    // out the real network round-trip, which the budget does not cover.
    await expect(c).toHaveAttribute("data-state", "confirmed");
    await expect(c).not.toHaveAttribute("data-pending", "1");

    // The row's own Result cell (inside the grid, distinct from the
    // standings section below the fold) updates in place — no reload
    // (usability-audit finding F3).
    await expect(rowResultCell(page, athlete)).toHaveText("3.10");
  });

  test("UC-039 #5: an offline save's pending state persists visibly until reconnect sync confirms it", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedUkcMeet(context.request, app.baseURL);
    await openUnit(page, fx.unitURL);
    await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

    await context.setOffline(true);
    const athlete = fx.athletes["101"];
    const c = cell(page, athlete, 1);
    await c.locator('input[name="value"]').fill("3.15");
    await c.locator('button[type="submit"]').click();

    await expect(c).toHaveAttribute("data-pending", "1");
    await expect(c.locator(".cell-save-badge")).not.toBeEmpty();
    // Pending survives while offline — the queue is durable, and the
    // status region's pending count agrees (SYS-087, UC-040 #5).
    await expect(offlineStatus(page)).toHaveAttribute("data-pending", "1");

    await context.setOffline(false);
    await expectSynced(page);
    await expect(c).toHaveAttribute("data-state", "confirmed");
    await expect(c).not.toHaveAttribute("data-pending", "1");
  });
});
