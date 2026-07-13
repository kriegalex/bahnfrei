// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Keyboard-only navigation helpers for the TASK-030 suite (SYS-114): the
// acceptance bar is that an operator can drive check-in/result-entry/status
// flows using nothing but Tab/Shift-Tab/Enter/Space/arrow keys, so tests
// must actually walk focus with real key events rather than jump to a
// locator with .focus() (which proves nothing about reachability or
// tab-order traps).
import { expect, type Locator, type Page } from "@playwright/test";

/**
 * Presses Tab (or Shift+Tab) until the given locator's element owns
 * document.activeElement, asserting it is reached within maxSteps. A stuck
 * focus loop that never reaches the target (a keyboard trap, or the control
 * being genuinely unreachable) fails loudly here rather than silently
 * falling through to a .focus() workaround.
 */
export async function tabUntilFocused(
  page: Page,
  target: Locator,
  opts: { maxSteps?: number; shift?: boolean } = {},
): Promise<void> {
  const maxSteps = opts.maxSteps ?? 80;
  const key = opts.shift ? "Shift+Tab" : "Tab";
  for (let i = 0; i < maxSteps; i++) {
    if (await isFocused(target)) {
      return;
    }
    await page.keyboard.press(key);
  }
  expect(
    await isFocused(target),
    `keyboard focus never reached the target within ${maxSteps} ${key} presses (unreachable control or a trap)`,
  ).toBe(true);
}

// A short, bounded timeout: locator.evaluate() otherwise waits (up to the
// test's default timeout) for the element to appear, which would make every
// Tab step in the loop below pay that wait whenever the target hasn't
// rendered yet (or never will, on a genuinely unreachable/missing control) —
// turning a real keyboard-trap failure into a multi-minute timeout instead
// of a fast, clear assertion failure.
async function isFocused(locator: Locator): Promise<boolean> {
  return locator
    .evaluate((el) => el === document.activeElement, undefined, { timeout: 250 })
    .catch(() => false);
}

/**
 * Asserts that pressing Tab actually moves focus off the current element at
 * least once — a direct keyboard-trap check (SC 2.1.2) distinct from
 * tabUntilFocused's reachability check.
 */
export async function assertTabAdvancesFocus(page: Page): Promise<void> {
  const before = await page.evaluate(() => document.activeElement?.outerHTML ?? null);
  await page.keyboard.press("Tab");
  const after = await page.evaluate(() => document.activeElement?.outerHTML ?? null);
  expect(after, "Tab must move focus (keyboard trap)").not.toBe(before);
}

/** Moves a focused <select>'s value by n options via real ArrowDown/ArrowUp
 *  key presses (never .selectOption(), which does not exercise keyboard
 *  interaction at all). */
export async function arrowSelect(page: Page, steps: number): Promise<void> {
  const key = steps >= 0 ? "ArrowDown" : "ArrowUp";
  for (let i = 0; i < Math.abs(steps); i++) {
    await page.keyboard.press(key);
  }
}
