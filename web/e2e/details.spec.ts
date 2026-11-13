// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Card details: labels, dates, archiving and board settings.

import { expect, test, type Page } from "@playwright/test";

import { addCards, card, cardTitles, expand, expectAccessible, newBoard, unique } from "./helpers";

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

const PHOTO = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGM4UWEDRAwQCgArzgXx0HrtWgAAAABJRU5ErkJggg==",
  "base64",
);

test("board background: a gradient, a color, then a photo; each is remembered", async ({ page }) => {
  await newBoard(page, ["Col"]);
  const board = page.locator(".board");
  const settings = await openSettings(page);
  const saved = () => page.waitForResponse((r) => r.request().method() === "PATCH" && /\/boards\/[^/]+$/.test(r.url()));

  let s = saved();
  await settings.getByRole("radio", { name: "Meadow gradient" }).click();
  await s;
  await expect(board).toHaveClass(/has-background/);
  await expect(board).toHaveCSS("background-image", /linear-gradient/);

  s = saved();
  await settings.getByRole("radio", { name: "Sage" }).click();
  await s;
  await expect(board).toHaveCSS("background-color", "rgb(232, 240, 216)");

  const uploaded = page.waitForResponse((r) => r.url().endsWith("/background") && r.request().method() === "POST");
  s = saved();
  await settings.locator("input[type=file]").setInputFiles({ name: "garden.png", mimeType: "image/png", buffer: PHOTO });
  await uploaded;
  await s;
  await expect(board).toHaveCSS("background-image", /\/api\/v1\/attachments\//);
  await settings.getByRole("button", { name: "Close" }).click();

  await page.reload();
  await expect(page.locator(".board")).toHaveCSS("background-image", /\/api\/v1\/attachments\//);

  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `photo background (${scheme})`);
  }

  // Back to the theme's own surface.
  const settings2 = await openSettings(page);
  await settings2.getByRole("radio", { name: "Theme default" }).click();
  await expect(page.locator(".board")).not.toHaveClass(/has-background/);
});

test("column color as a header stripe or a full tint", async ({ page }) => {
  await newBoard(page, ["Today"]);
  await page.getByRole("button", { name: "Today options" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("radio", { name: "sky" }).click();
  const col = page.getByRole("region", { name: "Today", exact: true });
  await expect(col).toHaveClass(/tint-header/);
  await dialog.getByLabel("A light tint over the column").check();
  await expect(col).toHaveClass(/tint-full/);
  await dialog.getByRole("button", { name: "Close" }).click();
  await page.reload();
  await expect(page.getByRole("region", { name: "Today", exact: true })).toHaveClass(/tint-full/);
});
