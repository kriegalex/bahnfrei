// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Compiles the real server binary once for the whole suite; every test then
// spawns its own instance against a fresh temp SQLite DB (helpers/server.ts).
import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import * as path from "node:path";

const repoRoot = path.resolve(__dirname, "..", "..");
export const binDir = path.join(__dirname, "..", ".bin");
export const binPath = path.join(
  binDir,
  process.platform === "win32" ? "bahnfrei.exe" : "bahnfrei",
);

export default function globalSetup(): void {
  mkdirSync(binDir, { recursive: true });
  execFileSync("go", ["build", "-o", binPath, "./cmd/bahnfrei"], {
    cwd: repoRoot,
    stdio: "inherit",
  });
}
