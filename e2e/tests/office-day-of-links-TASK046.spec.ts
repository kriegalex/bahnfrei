// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-046 (SYS-151, UC-041 #1, finding F5/OQ-113): the office dashboard
// panel used to link only roster and standings per meet — check-in and
// capture/reconciliation were reachable only through a typed URL or an
// organizer handing one over (OQ-111/OQ-113). This suite proves the fix in
// a real browser: from "/" an office session reaches check-in within two
// link activations (the office panel's meet-hub link, then the hub's
// per-event check-in link, both rendered — never a typed URL), and the
// capture index / reconciliation links are one activation away. The Go
// handler tests (internal/web/dashboard_test.go,
// TestOfficeHomeDayOfLinksSYS151UC041_1 and
// TestHomeLinkWalkReachabilityUC041_1And3) cover the same ground over HTTP;
// this spec is the browser-rendered, click-driven proof.
import { seedTrackMeet, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test.describe("SYS-151 UC-041 #1: office day-of links reachable within two clicks from /", () => {
  test("check-in reached in two clicks; capture/reconciliation in one", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);
    const fx = await seedTrackMeet(context.request, app.baseURL, ["Alice"]);

    // Provision and switch to a competition-office session — the role this
    // task's F5 fix targets.
    const adminPage = await context.request.get(app.baseURL + "/admin");
    const csrfMatch = (await adminPage.text()).match(/name="csrf_token" value="([^"]+)"/);
    if (!csrfMatch) {
      throw new Error("admin page missing csrf_token");
    }
    const createRes = await context.request.post(app.baseURL + "/admin/accounts", {
      form: {
        username: "office-e2e",
        display_name: "Office E2E",
        password: "s3cret-passphrase",
        role: "competition_office",
        csrf_token: csrfMatch[1],
      },
      maxRedirects: 0,
    });
    expect(createRes.status()).toBe(303);

    await context.request.post(app.baseURL + "/logout", { maxRedirects: 0 });
    const loginPage = await context.request.get(app.baseURL + "/login");
    const loginCsrf = (await loginPage.text()).match(/name="csrf_token" value="([^"]+)"/);
    if (!loginCsrf) {
      throw new Error("login page missing csrf_token");
    }
    const loginRes = await context.request.post(app.baseURL + "/login", {
      form: { username: "office-e2e", password: "s3cret-passphrase", csrf_token: loginCsrf[1] },
      maxRedirects: 0,
    });
    expect(loginRes.status()).toBe(303);

    // --- Activation budget: "/" -> meet hub -> check-in (2 clicks). ---
    await page.goto(app.baseURL + "/");
    await page.getByRole("link", { name: "Check-in (Programm)" }).click();
    await expect(page).toHaveURL(`${app.baseURL}/meets/${fx.meetID}`);
    await page.getByRole("link", { name: "Check-in", exact: true }).click();
    await expect(page).toHaveURL(fx.checkinURL);
    await expect(page.getByText("Alice Test")).toBeVisible();

    // --- Capture and reconciliation: one click each from "/". ---
    await page.goto(app.baseURL + "/");
    await page.getByRole("link", { name: "Resultaterfassung" }).click();
    await expect(page).toHaveURL(`${app.baseURL}/meets/${fx.meetID}/capture`);

    await page.goto(app.baseURL + "/");
    await page.getByRole("link", { name: "Abgleich (Offline-Erfassung)" }).click();
    await expect(page).toHaveURL(`${app.baseURL}/meets/${fx.meetID}/reconciliation`);
  });
});
