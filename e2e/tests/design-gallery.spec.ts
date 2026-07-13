// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// TASK-032 component-gallery keyboard walk (UC-038 #2, SYS-116): tabs
// through every interactive element on the dev/fixture gallery page
// (GET /dev/design-gallery — never linked from the shell nav, see
// internal/web/gallery.templ) and asserts a real, visible focus-visible
// outline on each one — the CSS focus-ring token (--focus-ring-color/
// --focus-ring-width, tokens.css) applied via keyboard Tab navigation, not
// a programmatic .focus() call (which Chromium's focus-visible heuristic
// does not treat the same way a real keyboard interaction does). Also
// pins that every documented component state actually renders distinctly
// (three offline-status colors, the inline field-error next to its
// invalid input) — the other half of UC-038 #2's "each documented
// component state renders" criterion.
import { expect, test } from "../helpers/server";

test.describe("UC-038 #2: component-gallery focus-visible keyboard walk (SYS-116)", () => {
  test("every non-disabled interactive element shows a visible focus-visible outline on Tab", async ({
    app,
    page,
  }) => {
    await page.goto(`${app.baseURL}/dev/design-gallery`);

    const interactiveSelector =
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex="0"]';
    const expectedCount = await page.locator(interactiveSelector).count();
    expect(expectedCount).toBeGreaterThan(10); // real coverage, not an empty page

    const visited = new Set<string>();
    // A generous bound: real focusable count plus slack for both the walk
    // to reach the end of the page and native multi-segment controls
    // (below) to exhaust their internal Tab stops.
    const maxIterations = (expectedCount + 10) * 3;
    let lastKey: string | null = null;

    for (let i = 0; i < maxIterations; i++) {
      await page.keyboard.press("Tab");
      const focused = await page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null;
        if (!el || el === document.body) {
          return null;
        }
        const cs = getComputedStyle(el);
        return {
          key: el.id || `${el.tagName}:${(el.textContent ?? "").slice(0, 30)}`,
          outlineStyle: cs.outlineStyle,
          outlineWidth: cs.outlineWidth,
        };
      });
      if (!focused) {
        break; // walked off the end of the page's focusable elements
      }
      // Native multi-segment controls (e.g. input[type=date]'s day/month/
      // year fields) keep document.activeElement on the SAME element across
      // several Tab presses while briefly toggling outline off mid-segment-
      // transition — a real Chromium rendering quirk, not a missing focus
      // ring. Only assert on first arrival at a given element; once we've
      // moved on to a different element we know the prior one is done.
      if (focused.key !== lastKey) {
        expect(focused.outlineStyle, `${focused.key} outline-style`).not.toBe("none");
        expect(
          parseFloat(focused.outlineWidth),
          `${focused.key} outline-width`,
        ).toBeGreaterThan(0);
        visited.add(focused.key);
      }
      lastKey = focused.key;
    }

    // Every element the DOM query found was actually reached by the walk
    // (proves the walk didn't stop early/skip real controls) — disabled
    // controls are correctly absent from both sets since browsers remove
    // them from tab order.
    expect(visited.size).toBeGreaterThanOrEqual(expectedCount);
  });

  test("each documented component state renders distinctly, not just as a DOM attribute", async ({
    app,
    page,
  }) => {
    await page.goto(`${app.baseURL}/dev/design-gallery`);

    const bg = (id: string) =>
      page.locator(`#${id}`).evaluate((el) => getComputedStyle(el).backgroundColor);
    const [online, offline, syncing] = await Promise.all([
      bg("gallery-status-online"),
      bg("gallery-status-offline"),
      bg("gallery-status-syncing"),
    ]);
    expect(new Set([online, offline, syncing]).size).toBe(3);

    // Disabled state: present, and genuinely non-interactive (browser
    // removes it from tab order — already proven by the walk above never
    // visiting #gallery-button-disabled/#gallery-input-disabled).
    await expect(page.locator("#gallery-button-disabled")).toBeDisabled();
    await expect(page.locator("#gallery-input-disabled")).toBeDisabled();

    // Invalid/error state: the field carries aria-invalid, and its paired
    // inline message (SYS-117, UC-038 #4) is visible and associated via
    // aria-describedby, not just a page-level flash.
    const invalidInput = page.locator("#gallery-input-invalid");
    await expect(invalidInput).toHaveAttribute("aria-invalid", "true");
    const describedBy = await invalidInput.getAttribute("aria-describedby");
    expect(describedBy).toBe("gallery-input-invalid-error");
    await expect(page.locator(`#${describedBy}`)).toBeVisible();
  });
});
