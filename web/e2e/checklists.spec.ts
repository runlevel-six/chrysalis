// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { expect, test, type Locator, type Page } from "@playwright/test";

import { addCards, card, cardTitles, column, expand, expectAccessible, newBoard } from "./helpers";

async function cardWithChecklist(page: Page, title: string, items: string[]) {
  await newBoard(page, ["To do", "Done"]);
  await addCards(page, "To do", [title]);
  const c = card(page, title);
  await c.getByRole("button", { name: "Expand card" }).click();
  await c.getByRole("button", { name: "Add a checklist" }).click();
  const input = c.getByLabel("New checklist item");
  await expect(input).toBeFocused(); // a new checklist is ready for items
  for (const t of items) {
    await input.fill(t);
    await input.press("Enter");
    await expect(c.getByRole("checkbox", { name: t })).toBeVisible();
  }
  await input.press("Escape");
  return c;
}

const itemTexts = (c: Locator) => c.locator(".checklist-text").allTextContents();
const itemSaved = (page: Page) =>
  page.waitForResponse((r) => r.url().includes("/checklist-items/") && r.request().method() === "PATCH");

test("add a checklist, tick items, and see progress on the collapsed card", async ({ page }) => {
  const c = await cardWithChecklist(page, "Trip", ["Socks", "Charger", "Water bottle"]);
  const saved = itemSaved(page);
  await c.getByRole("checkbox", { name: "Charger" }).check();
  await saved;
  await expect(c.getByLabel("1 of 3 done")).toBeVisible();

  await page.getByRole("button", { name: "Board" }).click();
  await page.getByRole("menuitem", { name: "Board settings" }).click();
  await page.getByRole("dialog", { name: "Board settings" }).getByLabel("Checklist progress").check();
  await page.getByRole("dialog", { name: "Board settings" }).getByRole("button", { name: "Close" }).click();
  await c.getByRole("button", { name: "Collapse card" }).click();
  await expect(c.getByLabel("Checklist: 1 of 3 done")).toBeVisible();

  await page.reload();
  await expect(card(page, "Trip").getByLabel("Checklist: 1 of 3 done")).toBeVisible();
  await expand(page, "Trip");
  await expect(card(page, "Trip").getByRole("checkbox", { name: "Charger" })).toBeChecked();
});

test("expanding a card with an empty checklist doesn't move focus", async ({ page }) => {
  const c = await cardWithChecklist(page, "Quiet", []);
  await c.getByRole("button", { name: "Collapse card" }).click();
  await page.reload();
  await card(page, "Quiet").getByRole("button", { name: "Expand card" }).click();
  await expect(card(page, "Quiet").getByRole("region", { name: "Checklist" })).toBeVisible();
  await expect(page.getByLabel("New checklist item")).toHaveCount(0);
});

test("delete an item and undo it", async ({ page }) => {
  const c = await cardWithChecklist(page, "Chores", ["Dishes", "Laundry"]);
  await c.getByRole("button", { name: "Delete Dishes" }).click();
  await expect(c.getByRole("checkbox", { name: "Dishes" })).toHaveCount(0);
  await page.getByRole("status", { name: "Notifications" }).getByRole("button", { name: "Undo" }).click();
  await expect(c.getByRole("checkbox", { name: "Dishes" })).toBeVisible();
  await page.reload();
  await expand(page, "Chores");
  await expect.poll(() => itemTexts(card(page, "Chores"))).toEqual(["Dishes", "Laundry"]);
});

test("reorder items with the keyboard", async ({ page }) => {
  test.skip(!!test.info().project.use.hasTouch, "desktop keyboard");
  const c = await cardWithChecklist(page, "Steps", ["First", "Second", "Third"]);
  const handle = c.getByRole("button", { name: "Reorder Third" });
  await handle.focus();
  const saved = itemSaved(page);
  // dnd-kit announces each step in a live region; wait for each announcement
  // so every key lands on settled positions, as it would for a person.
  const live = page.locator("[id^=DndLiveRegion]");
  const step = async (key: string) => {
    const before = (await live.allTextContents()).join();
    await page.keyboard.press(key);
    await expect.poll(async () => (await live.allTextContents()).join()).not.toBe(before);
    // The announcement lands a frame before dnd-kit's drop target updates;
    // a person never presses keys this fast, so give it a moment.
    await page.waitForTimeout(200);
  };
  await step("Space");
  await step("ArrowUp");
  await step("ArrowUp");
  await page.keyboard.press("Space");
  await saved;
  expect(await itemTexts(c)).toEqual(["Third", "First", "Second"]);
  await page.reload();
  await expand(page, "Steps");
  await expect.poll(() => itemTexts(card(page, "Steps"))).toEqual(["Third", "First", "Second"]);
});

test("dragging an item reorders it without dragging the card", async ({ page }) => {
  test.skip(!!test.info().project.use.hasTouch, "mouse only");
  const c = await cardWithChecklist(page, "Recipe", ["Chop", "Simmer", "Serve"]);
  const handle = (await c.getByRole("button", { name: "Reorder Serve" }).boundingBox())!;
  const target = (await c.getByRole("checkbox", { name: "Chop" }).boundingBox())!;
  const saved = itemSaved(page);
  await page.mouse.move(handle.x + handle.width / 2, handle.y + handle.height / 2);
  await page.mouse.down();
  for (let i = 1; i <= 8; i++) {
    await page.mouse.move(handle.x + handle.width / 2, handle.y + ((target.y - 4 - handle.y) * i) / 8);
  }
  await page.mouse.up();
  await saved;
  expect(await itemTexts(c)).toEqual(["Serve", "Chop", "Simmer"]);
  expect(await cardTitles(page, "To do")).toEqual(["Recipe"]); // the card stayed put
  await expect(column(page, "Done").locator(".card")).toHaveCount(0);
});

test("no serious accessibility issues with a checklist open", async ({ page }) => {
  const c = await cardWithChecklist(page, "Accessible", ["One", "Two"]);
  await c.getByRole("checkbox", { name: "One" }).check();
  await expectAccessible(page, "checklist");
});
