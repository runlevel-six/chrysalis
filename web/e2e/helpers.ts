// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Shared helpers for the end-to-end tests.

import AxeBuilder from "@axe-core/playwright";
import { expect, type Locator, type Page } from "@playwright/test";

export const unique = (label: string) => `${label} ${crypto.randomUUID().slice(0, 8)}`;

/** Create a board with the given columns and open it. Returns its URL. */
/**
 * Go to the board list. A fresh load of "/" may land on the last board you
 * had open (§3.3); the home link always goes to the list.
 */
export async function gotoBoards(page: Page) {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  if (new URL(page.url()).pathname !== "/") await page.getByRole("link", { name: "Chrysalis" }).click();
  await expect(page.getByRole("heading", { name: "Your boards" })).toBeVisible();
}

export async function newBoard(page: Page, columns: string[]) {
  await gotoBoards(page);
  const name = unique("Board");
  await page.getByLabel("New board").fill(name);
  await page.getByLabel("New board").press("Enter");
  await page.getByRole("link", { name, exact: true }).click();
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  for (const col of columns) {
    const add = page.getByRole("button", { name: "Add a column" });
    if (await add.isVisible()) await add.click();
    await page.getByLabel("New column name").fill(col);
    await page.getByLabel("New column name").press("Enter");
    await expect(column(page, col)).toBeVisible();
  }
  await page.getByLabel("New column name").press("Escape");
  return page.url();
}

export const column = (page: Page, name: string) => page.getByRole("region", { name, exact: true });
export const card = (scope: Page | Locator, title: string) => scope.getByRole("group", { name: title, exact: true });
export const cardTitles = (page: Page, col: string) => column(page, col).locator(".card-title").allTextContents();

export async function addCards(page: Page, col: string, titles: string[]) {
  const c = column(page, col);
  await c.getByRole("button", { name: "Add a card" }).click();
  const input = c.getByLabel("New card title");
  for (const t of titles) {
    await input.fill(t);
    await input.press("Enter");
    await expect(card(c, t)).toBeVisible();
  }
  // Quick capture stays open for the next card (§3.6).
  await expect(input).toBeFocused();
  await input.press("Escape");
}

/** Wait until debounced view-state writes have reached the server. */
export const viewSaved = (page: Page) => page.waitForResponse((r) => r.url().endsWith("/view") && r.request().method() === "PUT");

/** Expand a card if it isn't already (expanded state persists across reloads). */
export async function expand(page: Page, title: string) {
  const c = card(page, title);
  // Wait for the card first: isVisible() doesn't wait, so on a board that's
  // still loading it would wrongly report "nothing to expand".
  await expect(c.getByRole("button", { name: /^(Expand|Collapse) card$/ })).toBeVisible();
  if (await c.getByRole("button", { name: "Expand card" }).isVisible()) {
    await c.getByRole("button", { name: "Expand card" }).click();
  }
  return c;
}

/** Fail on serious or critical axe findings (docs/DESIGN.md §3.11). */
export async function expectAccessible(page: Page, what: string) {
  const results = await new AxeBuilder({ page }).analyze();
  const bad = results.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(bad.map((v) => `${what} ${v.id}: ${v.help} (${v.nodes.map((n) => n.target).join(", ")})`)).toEqual([]);
}
