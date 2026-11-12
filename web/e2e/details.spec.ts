// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Card details: labels, dates, archiving and board settings.

import { expect, test, type Page } from "@playwright/test";

import { addCards, card, cardTitles, expectAccessible, newBoard, unique } from "./helpers";

async function expand(page: Page, title: string) {
  const c = card(page, title);
  if (await c.getByRole("button", { name: "Expand card" }).isVisible()) {
    await c.getByRole("button", { name: "Expand card" }).click();
  }
  return c;
}

async function openSettings(page: Page) {
  await page.getByRole("button", { name: "Board" }).click();
  await page.getByRole("menuitem", { name: "Board settings" }).click();
  return page.getByRole("dialog", { name: "Board settings" });
}

test("make a label, put it on a card, and show label colors on collapsed cards", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Tagged"]);
  const c = await expand(page, "Tagged");
  await c.getByRole("button", { name: "Labels…" }).click();
  const dialog = page.getByRole("dialog", { name: "Labels" });
  await dialog.getByLabel("Label name").fill("Urgent");
  await dialog.getByRole("radio", { name: "orange" }).click();
  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await dialog.getByRole("button", { name: "Add label" }).click();
  await expect(dialog.getByRole("checkbox")).toBeChecked();
  await saved;
  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(c.getByRole("list", { name: "Labels" })).toContainText("Urgent");

  // Off by default: a collapsed card shows only its title.
  await c.getByRole("button", { name: "Collapse card" }).click();
  await expect(c.getByRole("img", { name: /Labels:/ })).toHaveCount(0);

  const settings = await openSettings(page);
  await settings.getByLabel("Label colors").check();
  await settings.getByRole("button", { name: "Close" }).click();
  await expect(c.getByRole("img", { name: "Labels: Urgent" })).toBeVisible();

  await page.reload();
  await expect(card(page, "Tagged").getByRole("img", { name: "Labels: Urgent" })).toBeVisible();
});

test("set start and due dates; the due date can show on collapsed cards", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Plant bulbs"]);
  const c = await expand(page, "Plant bulbs");
  await c.getByRole("button", { name: "Dates…" }).click();
  const dialog = page.getByRole("dialog", { name: "Dates" });
  await dialog.getByLabel("Start date").fill("2026-10-01");
  await dialog.getByLabel("Due date").fill("2026-10-15");
  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await dialog.getByRole("button", { name: "Save" }).click();
  await saved;
  await expect(c).toContainText("Starts Oct 1");
  await expect(c).toContainText("Due Oct 15");

  // A start after the due date is caught before saving.
  await c.getByRole("button", { name: "Dates…" }).click();
  await dialog.getByLabel("Start date").fill("2026-10-20");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog.getByRole("alert")).toContainText("start date is after the due date");
  await dialog.getByRole("button", { name: "Close" }).click();

  const settings = await openSettings(page);
  await settings.getByLabel("Due date").check();
  await settings.getByRole("button", { name: "Close" }).click();
  await c.getByRole("button", { name: "Collapse card" }).click();
  await expect(c.locator(".due-text")).toContainText("Oct 15");
  await page.reload();
  await expect(card(page, "Plant bulbs").locator(".due-text")).toContainText("Oct 15");
});

test("archive a card, undo, archive again, then put it back", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Old news", "Current"]);
  const c = await expand(page, "Old news");
  await c.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Archive" }).click();
  await expect(card(page, "Old news")).toHaveCount(0);
  const toast = page.getByRole("status", { name: "Notifications" });
  await expect(toast).toContainText("Archived “Old news”");
  await toast.getByRole("button", { name: "Undo" }).click();
  await expect(card(page, "Old news")).toBeVisible();

  const archived = page.waitForResponse((r) => r.request().method() === "PATCH");
  await (await expand(page, "Old news")).getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Archive" }).click();
  await archived;
  await page.reload();
  expect(await cardTitles(page, "Col")).toEqual(["Current"]);
  await page.getByRole("button", { name: "Board" }).click();
  await page.getByRole("menuitem", { name: "Archived cards" }).click();
  const dialog = page.getByRole("dialog", { name: "Archived cards" });
  await dialog.getByRole("button", { name: "Put back" }).click();
  await expect(dialog).toContainText("Nothing archived.");
  await dialog.getByRole("button", { name: "Close" }).click();
  expect(await cardTitles(page, "Col")).toEqual(["Old news", "Current"]);
});

test("board settings: rename, and open untouched cards expanded", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Never toggled"]);
  await expect(card(page, "Never toggled").getByRole("button", { name: "Expand card" })).toBeVisible();

  const settings = await openSettings(page);
  await settings.getByLabel("Expanded").check();
  const name = unique("Renamed board");
  await settings.getByLabel("Board name").fill(name);
  await settings.getByRole("button", { name: "Rename" }).click();
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(name);
  await settings.getByRole("button", { name: "Close" }).click();
  await expect(card(page, "Never toggled").getByRole("button", { name: "Collapse card" })).toBeVisible();
  await page.reload();
  await expect(card(page, "Never toggled").getByRole("button", { name: "Collapse card" })).toBeVisible();
});

test("no serious accessibility issues in the details dialogs", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Check me"]);
  const c = await expand(page, "Check me");
  await c.getByRole("button", { name: "Labels…" }).click();
  await page.getByRole("dialog", { name: "Labels" }).getByRole("button", { name: "Add label" }).click();
  await expectAccessible(page, "labels dialog");
  await page.getByRole("dialog", { name: "Labels" }).getByRole("button", { name: "Close" }).click();
  await c.getByRole("button", { name: "Dates…" }).click();
  await expectAccessible(page, "dates dialog");
  await page.getByRole("dialog", { name: "Dates" }).getByRole("button", { name: "Close" }).click();
  await openSettings(page);
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `settings dialog (${scheme})`);
  }
});
