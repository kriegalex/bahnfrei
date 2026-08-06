// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-047 (SYS-152, UC-041 #4-5, usability-audit F7/F8): check-in used to
// show a bare "Keine Meldungen für dieses Rennen" alongside a live,
// unconditional "Check-in schliessen" destructive action even when there
// was nothing to close. This suite proves the fix in a real browser: an
// event with zero online entries states why (OQ-117's honest,
// both-readings copy) and offers no destructive control at all; once
// entries exist, the close action goes through a TASK-034-style confirm
// sub-page that states the affected count before the confirm button. The
// Go handler tests (internal/web/seeding_test.go,
// TestCheckinEmptyStateSYS152UC041_4,
// TestCheckInCloseInapplicableWhenAllResolvedSYS152UC041_4,
// TestCheckInCloseConfirmShowsCountAndClosesSYS152UC041_5) cover the same
// ground over HTTP; this spec is the browser-rendered, click-driven proof.
import { csrfFrom, seedTrackMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test.describe("SYS-152 UC-041 #4-5: honest check-in empty state and gated close action", () => {
  test("empty check-in explains why and hides the close action; entries make it live with a count", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedTrackMeet(context.request, app.baseURL, []);

    // --- Zero entries: the reason renders, no destructive control at all. ---
    await page.goto(fx.checkinURL);
    await expect(page.getByText("Roster verwaltet")).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Teilnehmende dieses Bewerbs auf der Ergebniserfassung ansehen" }),
    ).toHaveAttribute("href", `/meets/${fx.meetID}/capture`);
    await expect(page.getByRole("link", { name: "Check-in schliessen" })).toHaveCount(0);
    await expect(page.locator('form[action$="/checkin/close"]')).toHaveCount(0);

    // --- Submit two online entries directly (mirrors seedTrackMeet's own
    // per-name loop) so the list is non-empty but still fully "entered". ---
    const entriesPage = `${app.baseURL}/meets/${fx.meetID}/entries`;
    for (const lastName of ["Alpha", "Beta"]) {
      const csrf = csrfFrom(await (await context.request.get(entriesPage)).text());
      const res = await context.request.post(`${app.baseURL}/meets/${fx.meetID}/entries/individual`, {
        form: {
          event: fx.eventID,
          first_name: "Runner",
          last_name: lastName,
          birth_year: "2005",
          sex: "W",
          seed: "13.50",
          csrf_token: csrf,
        },
        maxRedirects: 0,
      });
      expect(res.status()).toBe(303);
    }

    // --- Now the close action is live, links to its confirm sub-page. ---
    await page.goto(fx.checkinURL);
    const closeLink = page.getByRole("link", { name: "Check-in schliessen" });
    await expect(closeLink).toBeVisible();
    await closeLink.click();
    await expect(page).toHaveURL(`${fx.checkinURL}/close/confirm`);

    // The confirm sub-page states the affected count (UC-041 #5) before its
    // own confirm button, and posts to the close route.
    await expect(page.getByText("2 nicht bestätigte")).toBeVisible();
    await expect(page.locator(`form[action="/meets/${fx.meetID}/events/${fx.eventID}/checkin/close"]`)).toHaveCount(
      1,
    );

    await page.getByRole("button", { name: "Check-in schliessen" }).click();
    await expect(page).toHaveURL(fx.checkinURL);

    // Nothing is left to close now: the action disappears again.
    await expect(page.getByRole("link", { name: "Check-in schliessen" })).toHaveCount(0);
    await expect(page.getByText("Nichts zu schliessen")).toBeVisible();
  });
});
