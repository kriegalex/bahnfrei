// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// UC-040 "Truthful sync-failure handling & recovery" — browser E2E suite
// (SYS-149; TASK-044). Regression coverage for usability-audit finding F1:
// saving an invalid mark used to fail the WHOLE /sync batch with HTTP 400,
// which the client (islands/src/capture-offline.ts) could not distinguish
// from a real connectivity failure — it showed the "Verbindung
// unterbrochen" offline banner, retried forever, and the poisoned op
// survived reload and blocked every later capture (head-of-line
// blocking). This suite drives the real Go server and the real wire
// protocol (internal/sync/doc.go), same convention as uc-034.spec.ts.
import {
  captureAttempt,
  cell,
  expectPending,
  expectSynced,
  offlineStatus,
  openUnit,
  standings,
} from "../helpers/capture";
import { ADMIN, seedUkcMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test("UC-040 #1/#2/#3: a rejected op renders at its cell, never blocks the queue, and can be corrected or discarded", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  // Save an invalid mark while genuinely online (the F1 repro): the server
  // now answers 200 with a per-op "rejected" outcome instead of a
  // batch-wide 400.
  const rejectedCell = cell(page, fx.athletes["101"], 1);
  await captureAttempt(page, fx.athletes["101"], 1, "abc");
  await expect(rejectedCell).toHaveAttribute("data-rejected", "invalid_mark");
  await expect(rejectedCell.locator(".field-error")).toBeVisible();

  // Never presented as a connectivity problem, and not auto-retried: the
  // rejected op is immediately removed from the pending count.
  await expect(page.locator("#offline-banner")).toBeHidden();
  await expect(offlineStatus(page)).not.toHaveAttribute("data-state", "offline");
  await expectPending(page, 0);

  // A later, valid save on a DIFFERENT athlete still applies — the
  // rejection must not head-of-line-block the queue (UC-040 #2, the exact
  // defect: "later valid saves never apply").
  await captureAttempt(page, fx.athletes["102"], 1, "3.80");
  await expectSynced(page);
  await expect(standings(page)).toContainText("3.80", { timeout: 10_000 });

  // Discard clears the rejected marker and the stale value (UC-040 #3).
  await rejectedCell.locator(".cell-reject-discard").click();
  await expect(rejectedCell).not.toHaveAttribute("data-rejected", /.+/);
  await expect(rejectedCell.locator('input[name="value"]')).toHaveValue("");

  // Correct-and-save also works: typing a new, valid value over the
  // rejected cell and saving is the natural correction path.
  await captureAttempt(page, fx.athletes["101"], 1, "3.60");
  await expectSynced(page);
  await expect(standings(page)).toContainText("3.60", { timeout: 10_000 });
});

test("UC-040 #4: an expired session prompts re-authentication; the queue survives re-login", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);
  await expect(offlineStatus(page)).toHaveAttribute("data-state", "online");

  // Simulate the session expiring mid-queue (route interception, per the
  // task brief): the real 403 shape a lapsed 12h TTL produces upstream of
  // the handler is pinned server-side by
  // internal/web/sync_test.go:TestSyncSYS149UC040_4ExpiredSessionIs403;
  // this exercises the other half of the client's 401||403 branch.
  await context.route("**/sync", (route) => {
    if (route.request().method() !== "POST") {
      return route.continue();
    }
    return route.fulfill({ status: 401, contentType: "application/json", body: "{}" });
  });

  await captureAttempt(page, fx.athletes["101"], 1, "3.44");
  const reauth = page.locator("#capture-reauth-banner");
  await expect(reauth).toBeVisible();
  await expect(reauth.locator("a")).toHaveAttribute("href", "/login");

  // Not a connectivity message, and not silently retried forever: the op
  // stays queued (never dropped) rather than being deleted or retried.
  await expect(page.locator("#offline-banner")).toBeHidden();
  await expect(offlineStatus(page)).not.toHaveAttribute("data-state", "offline");
  await expectPending(page, 1);

  // Recovery: a plain navigation to /login and back re-authenticates (SYS-149
  // says a plain navigation is fine — the durable IndexedDB queue is
  // untouched by the whole episode). The queue then applies with no
  // re-entry.
  await context.unroute("**/sync");
  await page.goto(`${app.baseURL}/login`);
  await page.locator('input[name="username"]').fill(ADMIN.username);
  await page.locator('input[name="password"]').fill(ADMIN.password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await page.goto(fx.unitURL);

  await expectSynced(page);
  await expect(standings(page)).toContainText("3.44", { timeout: 10_000 });
});

test("UC-040 #5: the status region never reports 'all transferred' while an op is pending", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  await openUnit(page, fx.unitURL);

  // Record every attribute mutation of the indicator; flag the exact
  // contradiction F1 named — "online" (all-transferred) text with a
  // non-zero pending count.
  await page.evaluate(() => {
    const el = document.getElementById("capture-offline-status")!;
    (window as unknown as { __violations: unknown[] }).__violations = [];
    const record = () => {
      if (el.dataset.state === "online" && el.dataset.pending !== "0") {
        (window as unknown as { __violations: unknown[] }).__violations.push({
          state: el.dataset.state,
          pending: el.dataset.pending,
          text: el.textContent,
        });
      }
    };
    new MutationObserver(record).observe(el, {
      attributes: true,
      childList: true,
      characterData: true,
      subtree: true,
    });
    record();
  });

  // A mix of a rejected op and valid ops, back to back — the scenario most
  // likely to race the indicator between states.
  await captureAttempt(page, fx.athletes["101"], 1, "abc");
  await captureAttempt(page, fx.athletes["101"], 2, "3.20");
  await captureAttempt(page, fx.athletes["102"], 1, "3.50");
  await expectSynced(page);

  const violations = await page.evaluate(
    () => (window as unknown as { __violations: unknown[] }).__violations,
  );
  expect(violations, JSON.stringify(violations)).toEqual([]);
});
