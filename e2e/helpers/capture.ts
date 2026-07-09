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

/** Types a mark into a grid cell and saves it (island-intercepted submit). */
export async function captureAttempt(
  page: Page,
  athleteID: string,
  seq: number,
  value: string,
): Promise<void> {
  const c = cell(page, athleteID, seq);
  await c.locator('input[name="value"]').fill(value);
  await c.locator("button").click();
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
