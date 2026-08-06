// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-050 defect sweep (usability-audit-volunteer-2026-08.md F11): every
// page previously logged a CSP console error from htmx.min.js injecting an
// inline <style> for .htmx-indicator (blocked by the strict `default-src
// 'self'` CSP, internal/web/middleware.go's securityHeaders) and 404'd on
// favicon.ico. Both are fixed at the source (layout.templ's htmx-config
// meta tag disabling includeIndicatorStyles; static.go/routes.go serving a
// placeholder icon) — this spec drives a real capture page load in a real
// browser and asserts the console/network are actually clean, not just that
// the fix compiles.
import { openUnit } from "../helpers/capture";
import { seedUkcMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test("capture page load has zero console errors and no favicon 404 (F11, TASK-050)", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);

  const consoleErrors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });
  const failedRequests: string[] = [];
  page.on("response", (res) => {
    if (res.status() >= 400) {
      failedRequests.push(`${res.status()} ${res.url()}`);
    }
  });

  await openUnit(page, fx.unitURL);

  expect(consoleErrors, JSON.stringify(consoleErrors, null, 2)).toEqual([]);
  expect(
    failedRequests.filter((r) => r.includes("favicon")),
    JSON.stringify(failedRequests, null, 2),
  ).toEqual([]);
});
