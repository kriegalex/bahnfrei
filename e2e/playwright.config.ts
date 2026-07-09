// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Playwright configuration for the UC-034 browser E2E suite (ADR-003 §5.3:
// Playwright is the ratified browser-E2E tool; chromium only). Each test
// boots its own real Go server (temp SQLite DB) via the fixture in
// helpers/server.ts; globalSetup compiles ./cmd/bahnfrei once. The server
// runs plaintext HTTP on 127.0.0.1 (--tls-mode=off, dev/E2E only): loopback
// is a secure context, so service workers register (UC-034 #3).
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./helpers/global-setup.ts",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: [["list"]],
  timeout: 60_000,
  use: {
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
