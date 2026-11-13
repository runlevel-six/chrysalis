// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Read-only offline (§5.6): the app and the last synced copy of a board open
// without a network, with a calm banner, and editing is put away.

import { expect, test } from "@playwright/test";

import { addCards, card, newBoard } from "./helpers";

test.use({ serviceWorkers: "allow" });
test.skip(({ browserName, isMobile }) => browserName !== "chromium" || !!isMobile, "service worker offline emulation: chromium");

test("a board opens offline, read-only, from the last synced copy", async ({ page, context }) => {
  await newBoard(page, ["Today"]);
  await addCards(page, "Today", ["Water the seedlings"]);
  // Wait for the service worker to take control, then load once more so the
  // app shell and this board are both saved on the device.
  await page.evaluate(() => navigator.serviceWorker.ready);
  await page.reload();
  await expect(card(page, "Water the seedlings")).toBeVisible();
  await page.waitForTimeout(300);

  await context.setOffline(true);
  await page.reload();
  await expect(page.getByRole("status").filter({ hasText: "Offline — viewing last synced copy" })).toBeVisible();
  await expect(card(page, "Water the seedlings")).toBeVisible();
  await expect(page.getByRole("button", { name: "Add a card" })).toHaveCount(0);

  await context.setOffline(false);
  await page.reload();
  await expect(page.getByText("Offline — viewing last synced copy")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Add a card" }).first()).toBeVisible();
});

test("the app is installable", async ({ page }) => {
  await page.goto("/");
  const href = await page.locator('link[rel="manifest"]').getAttribute("href");
  const res = await page.request.get(href!);
  expect(res.ok()).toBe(true);
  const m = await res.json();
  expect(m.display).toBe("standalone");
  expect(m.icons.length).toBeGreaterThan(0);
});
