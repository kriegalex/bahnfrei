// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Per-test real-server fixture: spawns the compiled bahnfrei binary with a
// fresh temp SQLite data dir on a free loopback port, plaintext HTTP
// (--tls-mode=off, the dev/E2E-only mode; loopback is a secure context so
// service workers register). Exposes baseURL plus stop/start for tests that
// need a server-side outage.
import { test as base } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { binPath } from "./global-setup";

export interface AppServer {
  baseURL: string;
  /** Stop the server process (simulates a server-side outage). */
  stop(): Promise<void>;
  /** Restart on the same port and data dir. */
  start(): Promise<void>;
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.listen(0, "127.0.0.1", () => {
      const address = srv.address();
      if (address && typeof address === "object") {
        const port = address.port;
        srv.close(() => resolve(port));
      } else {
        srv.close(() => reject(new Error("no port")));
      }
    });
    srv.on("error", reject);
  });
}

async function waitHealthy(baseURL: string, timeoutMs = 15000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(baseURL + "/healthz");
      if (res.ok) {
        return;
      }
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) {
      throw new Error("server did not become healthy at " + baseURL);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
}

async function stopProcess(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null) {
    return;
  }
  const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
  child.kill("SIGTERM");
  const timeout = new Promise<void>((resolve) =>
    setTimeout(() => {
      child.kill("SIGKILL");
      resolve();
    }, 5000),
  );
  await Promise.race([exited, timeout]);
  await exited.catch(() => undefined);
}

export const test = base.extend<{ app: AppServer }>({
  app: async ({}, use) => {
    const dataDir = mkdtempSync(path.join(tmpdir(), "bahnfrei-e2e-"));
    const port = await freePort();
    const baseURL = `http://127.0.0.1:${port}`;
    let child: ChildProcess | null = null;

    const start = async (): Promise<void> => {
      child = spawn(
        binPath,
        [
          "serve",
          "--addr",
          `127.0.0.1:${port}`,
          "--tls-mode",
          "off",
          "--data-dir",
          dataDir,
        ],
        { stdio: "ignore" },
      );
      await waitHealthy(baseURL);
    };
    const stop = async (): Promise<void> => {
      if (child) {
        await stopProcess(child);
        child = null;
      }
    };

    await start();
    await use({ baseURL, stop, start });
    await stop();
    rmSync(dataDir, { recursive: true, force: true });
  },
});

export { expect } from "@playwright/test";
