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
const PREFS_TESTS = /(appearance|centering|reminders)\.spec\.ts/;

export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  // Each worker is a browser plus its pages; on a laptop that also runs other
  // things, a handful is plenty and keeps memory in check.
  workers: process.env.CI ? undefined : 4,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    // Traces can capture page content; they're gitignored (docs/DESIGN.md §0.1).
    trace: "retain-on-failure",
    // Request interception (page.route) can't see through a service worker,
    // so only the offline tests run with it (they opt back in).
    serviceWorkers: "block",
  },
  // Every test signs in as the same dev user, and preferences sync live to
  // every open tab (§5.5). Tests that change preferences therefore run on
  // their own, after everything else, so they can't reflow other tests' pages.
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] }, testIgnore: PREFS_TESTS },
    { name: "webkit", use: { ...devices["Desktop Safari"] }, testIgnore: PREFS_TESTS },
    { name: "iphone", use: { ...devices["iPhone 15"] }, testIgnore: PREFS_TESTS },
    { name: "ipad", use: { ...devices["iPad Pro 11"] }, testIgnore: PREFS_TESTS },
    {
      name: "preferences",
      use: { ...devices["Desktop Chrome"] },
      testMatch: PREFS_TESTS,
      dependencies: ["chromium", "webkit", "iphone", "ipad"],
    },
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
      CHRYSALIS_MAX_UPLOAD_MB: "2", // keeps the over-the-limit test small
    },
  },
});
