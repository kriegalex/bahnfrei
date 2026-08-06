// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Page-object helpers for the field-capture surface (UC-034): opening a unit
// (which takes the checkout), capturing an attempt through the grid, and the
// offline indicator's machine-readable state (data-state / data-pending —
// locale-independent, the visible text is i18n'd).
import { expect, type Locator, type Page } from "@playwright/test";

/** Opens the capture unit page and waits for the island's checkout to land. */
export async function openUnit(page: Page, unitURL: string): Promise<void> {
  const checkout = page.waitForResponse(
    (r) => r.url().endsWith("/checkout") && r.status() === 200,
  );
  await page.goto(unitURL);
  await checkout;
}

export function cell(page: Page, athleteID: string, seq: number): Locator {
  return page.locator(
    `form.cell-form[data-athlete="${athleteID}"][data-seq="${seq}"]`,
  );
}

/** Types a mark into a grid cell and saves it (island-intercepted submit).
 *  Targets the submit button explicitly (TASK-045): the cell now also
 *  carries letter-marker quick-action buttons (`.marker-btn`, type=button),
 *  so a bare `button` locator would be ambiguous. */
export async function captureAttempt(
  page: Page,
  athleteID: string,
  seq: number,
  value: string,
): Promise<void> {
  const c = cell(page, athleteID, seq);
  await c.locator('input[name="value"]').fill(value);
  await c.locator('button[type="submit"]').click();
}

/** Clicks a letter-marker quick-action button (TASK-045, SYS-147, UC-039
 *  #3) instead of typing — the mark input declares inputmode="decimal" (or
 *  "none" for the vertical grid), so this is the documented alternate path
 *  for the non-numeric D5.2 markers (X foul, – pass, r retirement, o
 *  clear). Saves immediately: the button sets the value and re-submits the
 *  form itself (capture-markers.js). */
export async function captureMarker(
  page: Page,
  athleteID: string,
  seq: number,
  marker: string,
): Promise<void> {
  const c = cell(page, athleteID, seq);
  await c.locator(`button.marker-btn[data-marker="${marker}"]`).click();
}

/** The cell's save-state at a glance (SYS-148, UC-039 #4/#5): "pending"
 *  while a save is in flight, "confirmed" once acknowledged, or "" once
 *  neither attribute is present (e.g. before any save, or after a
 *  rejection clears both). */
export async function cellSaveState(
  page: Page,
  athleteID: string,
  seq: number,
): Promise<string> {
  const c = cell(page, athleteID, seq);
  const pending = await c.getAttribute("data-pending");
  if (pending === "1") {
    return "pending";
  }
  return (await c.getAttribute("data-state")) ?? "";
}

/** The row's own Result/Points cells (field-horizontal grid), keyed by
 *  athlete — the ones capture-offline.ts updates in place from the sync
 *  ack (SYS-148, UC-039 #4), distinct from the standings section below. */
export function rowResultCell(page: Page, athleteID: string): Locator {
  return page.locator(`[data-role="result"][data-athlete-row="${athleteID}"]`);
}
export function rowPointsCell(page: Page, athleteID: string): Locator {
  return page.locator(`[data-role="points"][data-athlete-row="${athleteID}"]`);
}

export function offlineStatus(page: Page): Locator {
  return page.locator("#capture-offline-status");
}

export async function expectPending(page: Page, n: number): Promise<void> {
  await expect(offlineStatus(page)).toHaveAttribute("data-pending", String(n));
}

/** Waits until the queue is fully drained and the device reports online. */
export async function expectSynced(page: Page): Promise<void> {
  await expect(offlineStatus(page)).toHaveAttribute("data-pending", "0", {
    timeout: 15_000,
  });
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");
}

export function standings(page: Page): Locator {
  return page.locator("#capture-standings");
}

/** Waits until the service worker has the current page in its cache, so an
 *  offline reload can be served (UC-034 #3). */
export async function waitForOfflineReady(page: Page): Promise<void> {
  await page.waitForFunction(
    async () => (await caches.match(location.href)) !== undefined,
    undefined,
    { timeout: 15_000 },
  );
}
