// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Themes and personal preferences (docs/DESIGN.md §3.5, §3.6, §3.12).
//
// Every e2e test signs in as the same dev user, so these tests restore the
// default preferences when they finish to keep from changing other tests.

import { expect, test, type Page } from "@playwright/test";

import { addCards, card, expand, expectAccessible, newBoard } from "./helpers";

// One browser, one test at a time: these change the shared user's
// preferences, and running them side by side would overwrite each other.
// What they cover (CSS variables and classes) doesn't vary by engine.
test.describe.configure({ mode: "serial" });
test.skip(({ browserName, isMobile }) => browserName !== "chromium" || !!isMobile, "runs once, serially");

const DEFAULTS = { theme: "default", mode: "system", density: "comfortable", motion: "system", completionFlutter: true, highlightOverdue: false };

test.afterEach(async ({ page }) => {
  await page.request.patch("/api/v1/me/prefs", { data: DEFAULTS });
});

const root = (page: Page) => page.locator("html");
const token = (page: Page, name: string) =>
  page.evaluate((n) => getComputedStyle(document.documentElement).getPropertyValue(`--${n}`).trim(), name);
const prefsSaved = (page: Page) =>
  page.waitForResponse((r) => r.url().endsWith("/me/prefs") && r.request().method() === "PATCH");

async function openSettings(page: Page) {
  await page.goto("/");
  await page.getByRole("link", { name: "Settings" }).click();
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
}

test("choose the Butterfly Garden theme; it applies and is remembered", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await openSettings(page);
  const saved = prefsSaved(page);
  await page.getByRole("radio", { name: "Butterfly Garden" }).click();
  await saved;
  await expect(root(page)).toHaveAttribute("data-theme", "butterfly-garden");
  expect(await token(page, "surface")).toBe("#fcf7f3");
  await page.reload();
  await expect(root(page)).toHaveAttribute("data-theme", "butterfly-garden");
  await expect(page.getByRole("radio", { name: "Butterfly Garden" })).toHaveAttribute("aria-checked", "true");
});

test("always dark, compact spacing, and keeping things still", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light", reducedMotion: "no-preference" });
  await openSettings(page);
  await page.getByLabel("Always dark").check();
  await expect(root(page)).toHaveCSS("color-scheme", "dark");
  expect(await token(page, "surface")).toBe("#16181c");
  await page.getByLabel("Compact (more fits on screen)").check();
  await expect(root(page)).toHaveClass(/density-compact/);
  const saved = prefsSaved(page);
  await page.getByLabel("Keep things still").check();
  await saved;
  await expect(root(page)).toHaveClass(/reduce-motion/);
  await page.getByLabel("Match this device").first().check();
  await expect(root(page)).toHaveCSS("color-scheme", "light");
});

test("overdue cards get a quiet marker only when that's turned on", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Renew library books"]);
  const c = await expand(page, "Renew library books");
  await c.getByRole("button", { name: "Dates…" }).click();
  await page.getByRole("dialog", { name: "Dates" }).getByLabel("Due date").fill("2020-01-15");
  await page.getByRole("dialog", { name: "Dates" }).getByRole("button", { name: "Save" }).click();
  await page.getByRole("button", { name: "Board", exact: true }).click();
  await page.getByRole("menuitem", { name: "Board settings" }).click();
  await page.getByRole("dialog", { name: "Board settings" }).getByLabel("Due date").check();
  await page.getByRole("dialog", { name: "Board settings" }).getByRole("button", { name: "Close" }).click();
  await c.getByRole("button", { name: "Collapse card" }).click();

  // Off by default: the date shows, with no marker.
  await expect(card(page, "Renew library books").locator(".due-text")).toBeVisible();
  await expect(card(page, "Renew library books").locator(".due-text.is-overdue")).toHaveCount(0);

  const board = page.url();
  await openSettings(page);
  const saved = prefsSaved(page);
  await page.getByLabel(/Mark cards that are past their due date/).check();
  await saved;
  await page.goto(board);
  await expect(card(page, "Renew library books").locator(".due-text.is-overdue")).toBeVisible();
});

for (const scheme of ["light", "dark"] as const) {
  test(`Butterfly Garden passes accessibility checks in ${scheme} mode`, async ({ page }) => {
    await page.request.patch("/api/v1/me/prefs", { data: { ...DEFAULTS, theme: "butterfly-garden" } });
    await page.emulateMedia({ colorScheme: scheme });
    await newBoard(page, ["To do", "Done"]);
    await addCards(page, "To do", ["Plant milkweed", "Order seeds"]);
    const c = await expand(page, "Plant milkweed");
    await c.getByRole("button", { name: "Labels…" }).click();
    const dialog = page.getByRole("dialog", { name: "Labels" });
    await dialog.getByLabel("Label name").fill("Outside");
    await dialog.getByRole("button", { name: "Add label" }).click();
    await dialog.getByRole("button", { name: "Close" }).click();
    await expect(root(page)).toHaveAttribute("data-theme", "butterfly-garden");
    await expectAccessible(page, `garden board (${scheme})`);
    await page.getByRole("link", { name: "Settings" }).click();
    await expectAccessible(page, `settings (${scheme})`);
  });
}
