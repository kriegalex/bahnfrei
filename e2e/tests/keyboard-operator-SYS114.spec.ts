// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-030 "Operator keyboard-only efficiency" — browser E2E suite closing
// SYS-114 (STR-035): high-frequency competition-office actions (check-in,
// result entry, status setting) must be operable keyboard-only, with bulk
// operations for multi-athlete actions. Covers UC-007 (check-in and DNS
// handling) and UC-010 (track result capture), against the real Go server
// (helpers/server.ts) with real HTTP-seeded fixtures (helpers/seed.ts's
// seedTrackMeet). No .click() on any task-critical element: every
// interaction is Tab/Shift-Tab/Enter/Space/ArrowDown/ArrowUp, and every
// target control's reachability is proven by walking real Tab key presses
// (helpers/keyboard.ts's tabUntilFocused) rather than jumping to it with
// .focus(), which would prove nothing about tab order or keyboard traps.
import { assertTabAdvancesFocus, arrowSelect, tabUntilFocused } from "../helpers/keyboard";
import { seedTrackMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test.describe("SYS-114 UC-007: check-in is keyboard-only, incl. bulk DNS-close and reinstate", () => {
  test("confirm one entry, keyboard bulk-close DNSes the rest, keyboard reinstate", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedTrackMeet(context.request, app.baseURL, [
      "Alice",
      "Bella",
      "Clara",
    ]);

    await page.goto(fx.checkinURL);
    // Sanity keyboard-trap check right after load: Tab must move focus at
    // all (SC 2.1.2) before we rely on it for the rest of the flow.
    await assertTabAdvancesFocus(page);

    // All three entries start "entered" (awaiting check-in): each row has
    // its own confirm form (structural selector — locale-independent).
    const aliceConfirm = page.locator(
      'tr:has-text("Alice Test") form[action*="/confirm"] button[type="submit"]',
    );
    const bellaConfirm = page.locator(
      'tr:has-text("Bella Test") form[action*="/confirm"] button[type="submit"]',
    );
    const claraConfirm = page.locator(
      'tr:has-text("Clara Test") form[action*="/confirm"] button[type="submit"]',
    );
    await expect(aliceConfirm).toBeVisible();
    await expect(bellaConfirm).toBeVisible();
    await expect(claraConfirm).toBeVisible();

    // Keyboard-confirm Alice only: Tab from the top of the page all the way
    // to her row's confirm button (through the skip link, nav, locale
    // switch and the "close check-in" bulk form first), then Enter.
    await tabUntilFocused(page, aliceConfirm);
    await page.keyboard.press("Enter");
    await expect(aliceConfirm).toHaveCount(0); // she is no longer "entered"

    // Bulk operation (UC-007 #1): one keyboard action on the "close
    // check-in" button marks every STILL-unconfirmed entry (Bella, Clara —
    // two athletes at once) DNS, without touching Alice's confirmation.
    const closeButton = page.locator(
      'form[action*="/checkin/close"] button[type="submit"]',
    );
    await tabUntilFocused(page, closeButton);
    await page.keyboard.press("Enter");

    const bellaReinstate = page.locator(
      'tr:has-text("Bella Test") form[action*="/reinstate"] button[type="submit"]',
    );
    const claraReinstate = page.locator(
      'tr:has-text("Clara Test") form[action*="/reinstate"] button[type="submit"]',
    );
    await expect(bellaReinstate).toBeVisible();
    await expect(claraReinstate).toBeVisible();
    // Alice was already confirmed before the close: neither a confirm nor a
    // reinstate form remains on her row (the bulk action left her alone).
    await expect(
      page.locator('tr:has-text("Alice Test") form[action*="/confirm"]'),
    ).toHaveCount(0);
    await expect(
      page.locator('tr:has-text("Alice Test") form[action*="/reinstate"]'),
    ).toHaveCount(0);

    // Referee override (UC-007 #2): reinstate Bella's DNS via keyboard
    // (Space this time, to prove both activation keys work on a button).
    await tabUntilFocused(page, bellaReinstate);
    await page.keyboard.press(" ");
    await expect(bellaReinstate).toHaveCount(0);
    // Clara stays DNS — the reinstate was per-athlete, not another bulk op.
    await expect(claraReinstate).toBeVisible();
  });
});

test.describe("SYS-114 UC-010: track result entry is keyboard-only (times, statuses, corrections)", () => {
  test("manual time, DNF/DQ status via arrow keys, and a reasoned correction", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedTrackMeet(context.request, app.baseURL, [
      "Alice",
      "Bella",
      "Clara",
    ]);

    await page.goto(fx.unitURL);
    await assertTabAdvancesFocus(page);

    // --- Alice: manual time entry, keyboard-typed, keyboard-saved. ---
    const aliceTime = page.locator('tr:has-text("Alice Test") input[name="time"]');
    const aliceSave = page.locator('tr:has-text("Alice Test") button[type="submit"]');
    await tabUntilFocused(page, aliceTime);
    await page.keyboard.type("11.32"); // D5.1 hand-time round-up -> 11.4 h
    await tabUntilFocused(page, aliceSave); // Tab past timing/status/detail
    await page.keyboard.press("Enter");
    await expect(page.locator("#capture-standings")).toContainText("11.4 h");

    // --- Bella: DNF status set purely via ArrowDown, no time typed (UC-010
    // #3's own example pairs one DNF with one DQ). ---
    const bellaStatus = page.locator('tr:has-text("Bella Test") select[name="status"]');
    const bellaSave = page.locator('tr:has-text("Bella Test") button[type="submit"]');
    await tabUntilFocused(page, bellaStatus);
    await arrowSelect(page, 2); // "" -> DNS -> DNF
    await expect(bellaStatus).toHaveValue("DNF");
    await tabUntilFocused(page, bellaSave);
    await page.keyboard.press(" "); // Space activates the button too
    await expect(page.locator("#capture-standings")).toContainText("DNF");

    // --- Clara: DQ status (ArrowDown x3) plus its required rule reference,
    // both keyboard-entered — a DQ without one is rejected server-side
    // (SYS-045), so this also proves the detail field is reachable. ---
    const claraStatus = page.locator('tr:has-text("Clara Test") select[name="status"]');
    const claraDetail = page.locator(
      'tr:has-text("Clara Test") input[name="status_detail"]',
    );
    const claraSave = page.locator('tr:has-text("Clara Test") button[type="submit"]');
    await tabUntilFocused(page, claraStatus);
    await arrowSelect(page, 3); // "" -> DNS -> DNF -> DQ
    await expect(claraStatus).toHaveValue("DQ");
    await tabUntilFocused(page, claraDetail);
    await page.keyboard.type("TR16.8");
    await tabUntilFocused(page, claraSave);
    await page.keyboard.press("Enter");
    await expect(page.locator("#capture-standings")).toContainText("DQ (TR16.8)");

    // --- Correction (UC-015 crossover UC-010 needs): announce the unit via
    // keyboard, then correct Alice's time with a reason — the row's own
    // form switches from /track to /correct and grows reason/escalation
    // inputs once the unit is announced (capture.templ's CorrectionMode),
    // all still reachable by Tab. ---
    const announce = page.locator('form[action*="/announce"] button[type="submit"]');
    await tabUntilFocused(page, announce);
    await page.keyboard.press("Enter");
    await expect(page.locator(".capture-correction-notice")).toBeVisible();

    const aliceReason = page.locator('tr:has-text("Alice Test") input[name="reason"]');
    await tabUntilFocused(page, aliceTime);
    // The time input never re-renders a stored value (capture.templ), so
    // it is blank again after the reload — just type the corrected time.
    await page.keyboard.type("11.20");
    await tabUntilFocused(page, aliceReason);
    await page.keyboard.type("re-timed from video");
    await tabUntilFocused(page, aliceSave);
    await page.keyboard.press("Enter");
    await expect(page.locator("#capture-standings")).toContainText("11.2 h");
  });
});
