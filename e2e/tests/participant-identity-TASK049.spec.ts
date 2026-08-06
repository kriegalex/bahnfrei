// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-049 (SYS-150/UC-043) participant identity correction — one browser
// E2E spec proving the office-role edit path end to end: a typo'd name
// registered on the roster, corrected through the dedicated edit form
// (internal/web/standings.templ's participantFormPage), and visible on the
// public results page — the surface a founder/volunteer actually checks
// after a correction (UC-043 #1's "propagate to ... exports" made
// concrete). The Go test suites already cover validation, conflict,
// authorization and audit content (internal/app/participant_identity_test.go,
// internal/web/participant_identity_test.go); this spec is the one
// real-browser confirmation the form itself works end to end.
import { seedUkcMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test("TASK-049: correcting a participant's name from the roster reaches the public results page", async ({
  app,
  context,
  page,
}) => {
  await setupAndLogin(context.request, app.baseURL);
  const fx = await seedUkcMeet(context.request, app.baseURL);
  const rosterURL = `${app.baseURL}/meets/${fx.meetID}/roster`;
  const publicResultsURL = `${app.baseURL}/m/${fx.meetID}/results`;

  // seedUkcMeet registers Anna Muster (bib 101) with no club; correct her
  // last name and add the club she forgot to give at registration — a
  // routine day-of fix (usability-audit F4's motivating scenario).
  await page.goto(rosterURL);
  await expect(page.getByText("Anna Muster")).toBeVisible();
  await page.getByRole("link", { name: "Bearbeiten" }).first().click();
  await expect(page).toHaveURL(new RegExp(`/meets/${fx.meetID}/roster/.+/edit$`));

  // The form shows current values (UC-043: "the form must show current
  // values").
  await expect(page.locator('input[name="first_name"]')).toHaveValue("Anna");
  await expect(page.locator('input[name="last_name"]')).toHaveValue("Muster");

  await page.locator('input[name="last_name"]').fill("Musterfrau");
  await page.locator('input[name="club"]').fill("LC Fribourg");
  await page.getByRole("button", { name: "Korrektur speichern" }).click();

  // Back on the roster, corrected.
  await expect(page).toHaveURL(rosterURL);
  await expect(page.getByText("Anna Musterfrau")).toBeVisible();
  await expect(page.getByText("Anna Muster", { exact: true })).toHaveCount(0);
  await expect(page.getByText("LC Fribourg")).toBeVisible();

  // Public results page reflects the correction too (no separate
  // recompute step, per this task's category-re-derivation design note).
  await page.goto(publicResultsURL);
  await expect(page.getByText("Anna Musterfrau")).toBeVisible();
});
