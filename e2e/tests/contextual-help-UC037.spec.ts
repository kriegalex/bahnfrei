// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-031 contextual-help behavior suite (UC-037 #2–#4, SYS-115): drives
// the help-icon component's gallery instance (help.templ + /static/help.js;
// GET /dev/design-gallery renders it in the real placement convention)
// through the three activation modes SYS-115 mandates (pointer hover,
// keyboard focus, click/tap — never hover-only) and the WCAG 2.2 SC 1.4.13
// behaviors: dismissible via Escape without moving focus, hoverable (the
// pointer can travel onto the popup), persistent until dismissed or
// de-hovered/blurred. Localization is asserted against the DE and FR
// catalogs read straight from the repo (the same files the binary embeds),
// and a help-open state passes the same axe WCAG 2.2 AA rule set the
// UC-026 suite uses (UC-037 #4).
//
// The registry-coverage half of UC-037 (#1) and the visible-hint fixtures
// (#5) are Go rendering tests: internal/web/help_test.go.
import AxeBuilder from "@axe-core/playwright";
import { readFileSync } from "node:fs";
import * as path from "node:path";
import { expect, test } from "../helpers/server";

const TRIGGER = '[data-help-key="gallery.example"] .help-trigger';
const POPUP = "#help-gallery-example";

function catalogText(locale: "de" | "fr", key: string): string {
  const file = path.resolve(__dirname, `../../internal/web/i18n/locales/${locale}.json`);
  const value = (JSON.parse(readFileSync(file, "utf-8")) as Record<string, string>)[key];
  if (!value) {
    throw new Error(`${locale}.json has no ${key}`);
  }
  return value;
}

test.describe("UC-037: contextual input help (SYS-115, WCAG 2.2 SC 1.4.13)", () => {
  test("UC-037 #2: hover, keyboard focus and click each reveal the same localized help text (DE, then FR)", async ({
    app,
    page,
  }) => {
    await page.goto(`${app.baseURL}/dev/design-gallery`);
    const popup = page.locator(POPUP);
    const trigger = page.locator(TRIGGER);

    // (a) pointer hover
    await trigger.hover();
    await expect(popup).toBeVisible();
    await expect(popup).toHaveText(catalogText("de", "help.gallery.example"));
    await page.mouse.move(0, 0);
    await expect(popup).toBeHidden();

    // (b) keyboard focus
    await trigger.focus();
    await expect(popup).toBeVisible();
    await expect(popup).toHaveText(catalogText("de", "help.gallery.example"));
    await page.locator("#gallery-input-text").focus();
    await expect(popup).toBeHidden();

    // (c) click/tap — the touchscreen path (NN/g popup tip)
    await trigger.click();
    await expect(popup).toBeVisible();
    await expect(popup).toHaveText(catalogText("de", "help.gallery.example"));
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Escape");
    await expect(popup).toBeHidden();

    // Same content, localized: switch the session to FR (SYS-110) and the
    // popup carries the FR catalog text instead.
    await page.goto(`${app.baseURL}/locale?lang=fr`);
    await page.goto(`${app.baseURL}/dev/design-gallery`);
    await trigger.hover();
    await expect(popup).toBeVisible();
    await expect(popup).toHaveText(catalogText("fr", "help.gallery.example"));
  });

  test("UC-037 #3: open help is hoverable, persistent, and Escape-dismissible without moving focus (SC 1.4.13)", async ({
    app,
    page,
  }) => {
    await page.goto(`${app.baseURL}/dev/design-gallery`);
    const popup = page.locator(POPUP);
    const trigger = page.locator(TRIGGER);

    // Hoverable: pointer moves from the trigger ONTO the popup content —
    // it must not disappear while hovered.
    await trigger.hover();
    await expect(popup).toBeVisible();
    await popup.hover();
    await expect(popup).toBeVisible();
    // De-hover ends a hover-opened popup (the "persistent … until
    // de-hovered" arm of SC 1.4.13).
    await page.mouse.move(0, 0);
    await expect(popup).toBeHidden();

    // Persistent: a click/tap-opened popup stays open with the pointer
    // elsewhere until explicitly dismissed…
    await trigger.click();
    await expect(popup).toBeVisible();
    await page.mouse.move(0, 0);
    await expect(popup).toBeVisible();

    // …and Escape dismisses it WITHOUT moving keyboard focus.
    const focusedBefore = await page.evaluate(() => document.activeElement?.className ?? "");
    expect(focusedBefore).toContain("help-trigger");
    await page.keyboard.press("Escape");
    await expect(popup).toBeHidden();
    const focusedAfter = await page.evaluate(() => document.activeElement?.className ?? "");
    expect(focusedAfter).toContain("help-trigger");
    await expect(trigger).toHaveAttribute("aria-expanded", "false");

    // A second activation re-opens; clicking outside also dismisses (the
    // toggletip's touch-reachable dismissal, complementing Escape).
    await trigger.click();
    await expect(popup).toBeVisible();
    await page.locator("h1").click();
    await expect(popup).toBeHidden();
  });

  test("UC-037 #4: help trigger is AT-associated and a help-open state has zero automatable WCAG 2.2 AA violations", async ({
    app,
    page,
  }) => {
    await page.goto(`${app.baseURL}/dev/design-gallery`);
    const trigger = page.locator(TRIGGER);

    // Programmatic association: trigger describes itself with the popup.
    await expect(trigger).toHaveAttribute("aria-describedby", "help-gallery-example");
    await expect(page.locator(POPUP)).toHaveAttribute("role", "tooltip");

    // Scan the OPEN state — the popup rendered, expanded, on top of the
    // page — with the same rule set as the UC-026 accessibility suite.
    await trigger.click();
    await expect(page.locator(POPUP)).toBeVisible();
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze();
    expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([]);
  });
});
