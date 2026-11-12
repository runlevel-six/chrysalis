// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// End-to-end tests run against the real server binary in dev auth mode with a
// throwaway data directory. Build it first: `make build` (or `make e2e`).

import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { defineConfig, devices } from "@playwright/test";

const port = 18090;
const baseURL = `http://localhost:${port}`;

export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    // Traces can capture page content; they're gitignored (docs/DESIGN.md §0.1).
    trace: "retain-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
    { name: "iphone", use: { ...devices["iPhone 15"] } },
    { name: "ipad", use: { ...devices["iPad Pro 11"] } },
  ],
  webServer: {
    command: "../bin/chrysalis",
    url: `${baseURL}/readyz`,
    reuseExistingServer: false,
    env: {
      CHRYSALIS_BASE_URL: baseURL,
      CHRYSALIS_LISTEN_ADDR: `127.0.0.1:${port}`,
      CHRYSALIS_DATA_DIR: mkdtempSync(join(tmpdir(), "chrysalis-e2e-")),
      CHRYSALIS_AUTH_MODE: "dev",
      CHRYSALIS_DEV_MODE: "true",
      CHRYSALIS_LOG_LEVEL: "warn",
    },
  },
});
