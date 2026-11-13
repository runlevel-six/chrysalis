// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Overwhelm controls (docs/DESIGN.md §3.4): collapse/expand all, hide
// completed, focus mode, filter; and landing where you left off (§3.3).

import { expect, test, type Page } from "@playwright/test";

import { addCards, card, column, expand, expectAccessible, newBoard } from "./helpers";

const viewSaved = (page: Page) => page.waitForResponse((r) => r.url().endsWith("/view") && r.request().method() === "PUT");

async function viewMenu(page: Page, item: string) {
  await page.getByRole("button", { name: "View" }).click();
  await page.getByRole("menuitem", { name: item }).click();
}

test("collapse and expand every card at once", async ({ page }) => {
  await newBoard(page, ["A", "B"]);
  await addCards(page, "A", ["One", "Two"]);
  await addCards(page, "B", ["Three"]);
  await viewMenu(page, "Expand all cards");
  for (const t of ["One", "Two", "Three"]) await expect(card(page, t).getByRole("button", { name: "Collapse card" })).toBeVisible();
  const saved = viewSaved(page);
  await viewMenu(page, "Collapse all cards");
  for (const t of ["One", "Two", "Three"]) await expect(card(page, t).getByRole("button", { name: "Expand card" })).toBeVisible();
  await saved;
  await page.reload();
  await expect(card(page, "One").getByRole("button", { name: "Expand card" })).toBeVisible();
});

test("hide completed cards, including everything in a done column", async ({ page }) => {
  await newBoard(page, ["To do", "Done"]);
  await addCards(page, "To do", ["Open task", "Finished task"]);
  await addCards(page, "Done", ["Already there"]);
  const c = await expand(page, "Finished task");
  await c.getByRole("button", { name: "Mark done" }).click();
  await page.getByRole("button", { name: "Done options" }).click();
  await page.getByRole("dialog").getByLabel("Cards here count as done").check();
  await page.getByRole("dialog").getByRole("button", { name: "Close" }).click();

  const saved = viewSaved(page);
  await viewMenu(page, "Hide completed cards");
  await expect(card(page, "Finished task")).toHaveCount(0);
  await expect(card(page, "Already there")).toHaveCount(0);
  await expect(card(page, "Open task")).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "Completed cards are hidden." })).toBeVisible();
  await saved;

  await page.reload();
  await expect(card(page, "Finished task")).toHaveCount(0);
  await page.getByRole("button", { name: "Show completed" }).click();
  await expect(card(page, "Finished task")).toBeVisible();
  await expect(card(page, "Already there")).toBeVisible();
});

test("focus on one column with F, and leave with Esc", async ({ page }) => {
  test.skip(!!test.info().project.use.hasTouch, "desktop keyboard");
  await newBoard(page, ["Today", "Later", "Someday"]);
  await addCards(page, "Today", ["Call the clinic"]);
  await addCards(page, "Later", ["Plan the garden"]);
  await card(page, "Call the clinic").focus();
  await page.keyboard.press("f");
  await expect(column(page, "Later")).toHaveCount(0);
  await expect(column(page, "Someday")).toHaveCount(0);
  await expect(page.getByRole("status").filter({ hasText: "Focused on Today." })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(column(page, "Later")).toBeVisible();
});

test("focus from a column's options; it's remembered; one click shows everything", async ({ page }) => {
  await newBoard(page, ["Inbox", "Doing"]);
  await addCards(page, "Doing", ["Write the report"]);
  const saved = viewSaved(page);
  await page.getByRole("button", { name: "Doing options" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Focus on this column" }).click();
  await expect(column(page, "Inbox")).toHaveCount(0);
  await saved;
  await page.reload();
  await expect(column(page, "Inbox")).toHaveCount(0);
  await page.getByRole("button", { name: "Show all columns" }).click();
  await expect(column(page, "Inbox")).toBeVisible();
});

test("filter cards by title and by label", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Buy seeds", "Buy mulch", "Call plumber"]);
  // Give "Call plumber" a label.
  const c = await expand(page, "Call plumber");
  await c.getByRole("button", { name: "Labels…" }).click();
  const dialog = page.getByRole("dialog", { name: "Labels" });
  await dialog.getByLabel("Label name").fill("House");
  await dialog.getByRole("button", { name: "Add label" }).click();
  await expect(dialog.getByRole("checkbox")).toBeChecked();
  await dialog.getByRole("button", { name: "Close" }).click();

  await viewMenu(page, "Filter cards…");
  await page.getByLabel("Filter cards").fill("buy");
  await expect(card(page, "Call plumber")).toHaveCount(0);
  await expect(card(page, "Buy seeds")).toBeVisible();
  await expect(card(page, "Buy mulch")).toBeVisible();

  await page.getByLabel("Filter cards").fill("");
  await page.getByRole("group", { name: "Filter by label" }).getByRole("button").click();
  await expect(card(page, "Buy seeds")).toHaveCount(0);
  await expect(card(page, "Call plumber")).toBeVisible();

  await page.getByRole("button", { name: "Clear filter" }).click();
  await expect(card(page, "Buy seeds")).toBeVisible();
  await expect(page.getByLabel("Filter cards")).toHaveCount(0);
});

test("opening the app lands on the last board; reloading the list doesn't", async ({ page, browser }) => {
  const url = await newBoard(page, ["Col"]);
  const name = await page.getByRole("heading", { level: 1 }).textContent();
  const boardId = new URL(url).pathname.split("/").pop();
  // Opening a board is recorded on the server (other tests share this user,
  // so it may already be a newer board than ours).
  await expect.poll(async () => (await (await page.request.get("/api/v1/me")).json()).lastBoardId).toBeTruthy();

  // A fresh tab opening "/" goes straight to the last board opened. Every
  // test shares one dev user and other tests open boards in parallel, so
  // "last" can be someone else's board by now: check it lands on a board,
  // and on this one when nothing else has been opened since.
  const fresh = await browser.newContext();
  const p2 = await fresh.newPage();
  const lastNow = async () => (await (await page.request.get("/api/v1/me")).json()).lastBoardId;
  const expected = await lastNow();
  await p2.goto("/");
  await expect(p2).toHaveURL(/\/b\/[0-9A-Z]{26}$/);
  await expect(p2.getByRole("heading", { level: 1 })).toBeVisible();
  if (expected === boardId && (await lastNow()) === boardId) {
    await expect(p2).toHaveURL(url);
    await expect(p2.getByRole("heading", { level: 1 })).toHaveText(name!);
  }

  // Going to the list from there, and reloading it, stays on the list.
  await p2.getByRole("link", { name: "Chrysalis" }).click();
  await expect(p2.getByRole("heading", { name: "Your boards" })).toBeVisible();
  await p2.reload();
  await expect(p2.getByRole("heading", { name: "Your boards" })).toBeVisible();
  await fresh.close();
});

test("no serious accessibility issues with focus, filter and hide-completed on", async ({ page }) => {
  await newBoard(page, ["A", "B"]);
  await addCards(page, "A", ["One"]);
  await viewMenu(page, "Hide completed cards");
  await viewMenu(page, "Filter cards…");
  await page.getByLabel("Filter cards").fill("o");
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `overwhelm controls (${scheme})`);
  }
});
