// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

// Every test signs in as the same dev user and runs in parallel, so each one
// uses board names no other test will.
function uniqueName(label: string) {
  return `${label} ${crypto.randomUUID().slice(0, 8)}`;
}

async function createBoard(page: Page, name: string) {
  const input = page.getByLabel("New board");
  await input.fill(name);
  await input.press("Enter");
  await expect(page.getByRole("link", { name, exact: true })).toBeVisible();
  return input;
}

test("create boards with quick capture; they survive a reload", async ({ page }) => {
  await page.goto("/");
  const first = uniqueName("Garden plans");
  const second = uniqueName("Reading list");

  const input = await createBoard(page, first);
  // The field clears and stays ready for the next board.
  await expect(input).toHaveValue("");
  await expect(input).toBeFocused();
  await createBoard(page, second);

  await page.reload();
  const tiles = page.locator(".board-tile");
  await expect(tiles.filter({ hasText: first })).toHaveCount(1);
  await expect(tiles.filter({ hasText: second })).toHaveCount(1);
  // New boards go after existing ones (docs/DESIGN.md §3.6: things don't move).
  const names = await tiles.allTextContents();
  expect(names.indexOf(first)).toBeLessThan(names.indexOf(second));
});

test("open a board; it survives a reload", async ({ page }) => {
  await page.goto("/");
  const name = uniqueName("Kitchen");
  await createBoard(page, name);

  await page.getByRole("link", { name, exact: true }).click();
  await expect(page).toHaveURL(/\/b\/[0-9A-Z]+$/);
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  await expect(page).toHaveTitle(`${name} · Chrysalis`);

  await page.reload();
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  await expect(page.getByText("A fresh start. Add your first card.")).toBeVisible();

  await page.getByRole("link", { name: "Chrysalis" }).click();
  await expect(page.getByRole("heading", { name: "Your boards" })).toBeVisible();
});

test("an unknown board shows a calm not-found page", async ({ page }) => {
  await page.goto("/b/01NOTAREALBOARD");
  await expect(page.getByText("There's nothing here.")).toBeVisible();
  await page.getByRole("button", { name: "Back to your boards" }).click();
  await expect(page).toHaveURL(/\/$/);
});

test("rejects a blank board name without a request", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("New board").fill("   ");
  await expect(page.getByRole("button", { name: "Create" })).toBeDisabled();
});

test("no horizontal scrolling", async ({ page }) => {
  await page.goto("/");
  await createBoard(page, uniqueName("A board with quite a long name that should wrap politely"));
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(overflow).toBe(false);
});

// docs/DESIGN.md §3.11: fail on serious or critical accessibility issues.
for (const scheme of ["light", "dark"] as const) {
  test(`no serious accessibility issues (${scheme})`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.goto("/");
    const name = uniqueName("Accessible");
    await createBoard(page, name);

    const check = async () => {
      const results = await new AxeBuilder({ page }).analyze();
      const bad = results.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
      expect(bad.map((v) => `${v.id}: ${v.help} (${v.nodes.length})`)).toEqual([]);
    };
    await check();
    await page.getByRole("link", { name, exact: true }).click();
    await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
    await check();
  });
}
