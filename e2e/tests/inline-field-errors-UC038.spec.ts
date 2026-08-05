// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-034 inline field-level validation errors (OQ-075, usability-audit
// H9/F3, UC-038 #4): a submitted validation error must render inline at the
// offending field (`.field-error` + `aria-invalid`, not just a page-level
// flash), name what to fix, and preserve every other submitted value. This
// suite proves it in a real browser on the meet-setup form — the audit's
// own literal repro case (end date before start date) — plus the
// online-entry form. The Go handler tests
// (internal/web/{meets,entries,capture}_test.go) cover all three UC-038 #4
// representative forms (meet setup, online entry, result correction) over
// HTTP.
import { csrfFrom, setupAndLogin } from "../helpers/seed";
import { expect, test } from "../helpers/server";

test.describe("UC-038 #4: inline field-level validation errors (OQ-075)", () => {
  test("meet setup: end date before start date names the field and preserves input", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);

    await page.goto(`${app.baseURL}/meets/new`);
    await page.getByLabel("Name", { exact: true }).fill("Abendmeeting Uster");
    await page.getByLabel("Anlage").fill("Stadion Buchholz");
    await page.locator('input[name="start_date"]').fill("2027-06-13");
    await page.locator('input[name="end_date"]').fill("2027-06-12"); // end before start
    await page.getByRole("button", { name: "Speichern" }).click();

    // Re-rendered on the SAME form (never a redirect away) with the error
    // inline at end_date, not just a page-level flash.
    await expect(page).toHaveURL(`${app.baseURL}/meets`);
    const endDateInput = page.locator('input[name="end_date"]');
    await expect(endDateInput).toHaveAttribute("aria-invalid", "true");
    const describedBy = await endDateInput.getAttribute("aria-describedby");
    expect(describedBy).toContain("end_date-error");
    await expect(page.locator("#end_date-error")).toBeVisible();
    await expect(page.locator("#end_date-error")).toHaveText(
      "Das letzte Wettkampfdatum darf nicht vor dem ersten liegen.",
    );

    // The valid fields survive the re-render — no input lost.
    await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Abendmeeting Uster");
    await expect(page.getByLabel("Anlage")).toHaveValue("Stadion Buchholz");
    await expect(page.locator('input[name="start_date"]')).toHaveValue("2027-06-13");
    await expect(page.locator('input[name="end_date"]')).toHaveValue("2027-06-12");

    // The name field has no error of its own — no shared/generic marker.
    await expect(page.locator("#name-error")).toHaveCount(0);
  });

  test("online entry: an implausible birth year names the field and preserves the rest", async ({
    app,
    context,
    page,
  }) => {
    await setupAndLogin(context.request, app.baseURL);

    // A minimal published meet with one open event, over HTTP (mirrors
    // helpers/seed.ts's seedTrackMeet up through publish, without entries).
    const formPage = await context.request.get(`${app.baseURL}/meets/new`);
    const meetRes = await context.request.post(`${app.baseURL}/meets`, {
      form: {
        name: "Abendmeeting Uster",
        venue: "Stadion Buchholz",
        start_date: "2027-06-12",
        end_date: "2027-06-13",
        tier: "C-Meeting",
        scheme: "swiss-athletics",
        csrf_token: csrfFrom(await formPage.text()),
      },
      maxRedirects: 0,
    });
    const meetID = (meetRes.headers()["location"] ?? "").replace("/meets/", "");
    const meetURL = `${app.baseURL}/meets/${meetID}`;

    const meetPage = await context.request.get(meetURL);
    await context.request.post(`${meetURL}/events`, {
      form: {
        discipline: "100m",
        categories: "U18 W",
        round_final: "1",
        csrf_token: csrfFrom(await meetPage.text()),
      },
      maxRedirects: 0,
    });
    const meetPage2 = await context.request.get(meetURL);
    await context.request.post(`${meetURL}/publish`, {
      form: { version: "1", csrf_token: csrfFrom(await meetPage2.text()) },
      maxRedirects: 0,
    });

    const entriesURL = `${meetURL}/entries`;
    await page.goto(entriesURL);
    // Scoped to the individual-entry form specifically: the same page also
    // renders a bulk-entry form whose club field shares the literal `name`
    // attribute "club" (the bulk/relay per-athlete fields use suffixed
    // names like `bulk_first_name_N`/`leg_first_name_N`, but "club" is
    // shared verbatim between the individual and bulk forms).
    const individualForm = page.locator('form[action$="/entries/individual"]');
    await individualForm.locator('input[name="first_name"]').fill("Anna");
    await individualForm.locator('input[name="last_name"]').fill("Muster");
    // A birth year in the future: the input's own `min="1900"` HTML5
    // constraint is satisfied (so the browser lets the submit through —
    // this must be a SERVER-side rejection, not a native-validation block),
    // but it is not a real birth year, which the handler's own validation
    // (individualEntryBirthYearBounds) catches.
    await individualForm.locator('input[name="birth_year"]').fill("9999");
    await individualForm.locator('input[name="club"]').fill("LC Test");
    await individualForm.locator('input[name="seed"]').fill("13.50");
    // DEC-023/TASK-039: the optional licence field must survive the
    // re-render exactly like every other valid field on this form.
    await individualForm.locator('input[name="licence"]').fill("SA-2026-01");
    await individualForm.getByRole("button", { name: "Melden" }).click();

    // No redirect on a field-error re-render: the browser stays on the
    // form's own POST target (unlike a successful submit, which redirects
    // back to entriesURL).
    await expect(page).toHaveURL(`${entriesURL}/individual`);
    const birthYearInput = page.locator('form[action$="/entries/individual"] input[name="birth_year"]');
    await expect(birthYearInput).toHaveAttribute("aria-invalid", "true");
    await expect(page.locator("#birth_year-error")).toBeVisible();

    // The rest of the submission survives the re-render.
    const reRenderedForm = page.locator('form[action$="/entries/individual"]');
    await expect(reRenderedForm.locator('input[name="first_name"]')).toHaveValue("Anna");
    await expect(reRenderedForm.locator('input[name="last_name"]')).toHaveValue("Muster");
    await expect(reRenderedForm.locator('input[name="club"]')).toHaveValue("LC Test");
    await expect(reRenderedForm.locator('input[name="seed"]')).toHaveValue("13.50");
    await expect(reRenderedForm.locator('input[name="licence"]')).toHaveValue("SA-2026-01");
    await expect(page.getByText("Anna Muster")).toHaveCount(0); // not submitted
  });
});
