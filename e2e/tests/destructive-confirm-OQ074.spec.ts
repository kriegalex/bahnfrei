// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-034 destructive-action confirmation flow (OQ-074, usability-audit
// H3/F5, UC-038 #3): the shell's CSP forbids inline JS, so every
// destructive/irreversible action (athlete erasure, account disable, meet
// archive, retention purge) now sits behind a real GET confirm sub-page
// (internal/web/confirm.go/.templ) instead of a JS confirm() dialog. This
// suite proves it in a real browser for two of the four actions: meet
// archive (plain confirm/cancel) and athlete erasure (the stronger,
// typed-confirmation friction the ASVS review recommended as defense-in-
// depth on the two irreversible-and-unrecoverable actions). The Go handler
// tests (internal/web/confirm_test.go) cover all four over HTTP, including
// account disable and retention purge.
import { csrfFrom, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

/** Creates a bare draft meet (no events/entries needed for archive). */
async function createMeet(
  request: import("@playwright/test").APIRequestContext,
  baseURL: string,
): Promise<string> {
  const formPage = await request.get(`${baseURL}/meets/new`);
  const csrf = csrfFrom(await formPage.text());
  const res = await request.post(`${baseURL}/meets`, {
    form: {
      name: "Abendmeeting Uster",
      venue: "Stadion Buchholz",
      start_date: "2027-06-12",
      end_date: "2027-06-13",
      tier: "C-Meeting",
      scheme: "swiss-athletics",
      csrf_token: csrf,
    },
    maxRedirects: 0,
  });
  expect(res.status()).toBe(303);
  return (res.headers()["location"] ?? "").replace("/meets/", "");
}

/** Registers one roster participant with bib "1" (for the erasure test). */
async function registerParticipant(
  request: import("@playwright/test").APIRequestContext,
  baseURL: string,
  meetID: string,
): Promise<void> {
  const rosterURL = `${baseURL}/meets/${meetID}/roster`;
  const rosterPage = await request.get(rosterURL);
  const csrf = csrfFrom(await rosterPage.text());
  const res = await request.post(rosterURL, {
    form: {
      first_name: "Anna",
      last_name: "Muster",
      birth_year: "2011",
      sex: "W",
      bib: "1",
      csrf_token: csrf,
    },
    maxRedirects: 0,
  });
  expect(res.status()).toBe(303);
}

test.describe("OQ-074: destructive-action confirmation flow", () => {
  test("meet archive requires the confirm step, and Cancel really cancels", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const meetID = await createMeet(context.request, app.baseURL);
    const meetURL = `${app.baseURL}/meets/${meetID}`;

    await page.goto(meetURL);
    // The single-click submit button is gone: archiving is a navigation to
    // a confirm sub-page, not an inline POST form.
    await expect(page.locator('form[action$="/archive"]')).toHaveCount(0);
    await page.getByRole("link", { name: "Archivieren" }).click();
    await expect(page).toHaveURL(`${meetURL}/archive/confirm`);
    await expect(page.locator("h1")).toHaveText("Wettkampf archivieren?");
    await expect(page.getByText("Abendmeeting Uster")).toBeVisible();

    // Cancel is a plain navigation back to the meet page — nothing posted.
    await page.getByRole("link", { name: "Abbrechen" }).click();
    await expect(page).toHaveURL(meetURL);
    await expect(page.getByText("Archiviert")).toHaveCount(0);

    // Confirming (the confirm page's own form) does archive it.
    await page.goto(`${meetURL}/archive/confirm`);
    await page.getByRole("button", { name: "Archivieren" }).click();
    await expect(page).toHaveURL(meetURL);
    await expect(page.getByText("Archiviert")).toBeVisible();
  });

  test("athlete erasure requires the confirm step and a matching typed bib", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const meetID = await createMeet(context.request, app.baseURL);
    await registerParticipant(context.request, app.baseURL, meetID);

    const privacyURL = `${app.baseURL}/meets/${meetID}/privacy`;
    await page.goto(privacyURL);
    await expect(page.locator('form[action$="/erase"]')).toHaveCount(0);
    await page.getByRole("link", { name: "Löschen" }).click();
    await expect(page).toHaveURL(/\/erase\/confirm$/);
    await expect(page.getByText("Anna Muster")).toBeVisible();

    // A wrong typed confirmation is rejected inline, and nothing is erased.
    await page.getByLabel(/eintippen/).fill("wrong");
    await page.getByRole("button", { name: "Löschen" }).click();
    await expect(page.locator(".field-error")).toBeVisible();
    await page.goto(privacyURL);
    await expect(page.getByText("Anna Muster")).toBeVisible();

    // The correct token (the bib, "1") erases.
    await page.goto(`${app.baseURL}/meets/${meetID}/privacy`);
    await page.getByRole("link", { name: "Löschen" }).click();
    await page.getByLabel(/eintippen/).fill("1");
    await page.getByRole("button", { name: "Löschen" }).click();
    await expect(page).toHaveURL(privacyURL);
    await expect(page.getByText("Anna Muster")).toHaveCount(0);
  });
});
