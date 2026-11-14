// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// docs/DESIGN.md §9 required flows for columns and cards.

import AxeBuilder from "@axe-core/playwright";
import { devices, expect, test } from "@playwright/test";

import { addCards, card, cardTitles, column, expectAccessible, newBoard, viewSaved } from "./helpers";

test("create columns and cards with quick capture", async ({ page }) => {
  await newBoard(page, ["To do", "Doing", "Done"]);
  await addCards(page, "To do", ["Order seeds", "Water tomatoes", "Mulch beds"]);
  await page.reload();
  expect(await cardTitles(page, "To do")).toEqual(["Order seeds", "Water tomatoes", "Mulch beds"]);
  // Collapsed by default: title only, no inline actions.
  await expect(card(page, "Order seeds").getByRole("button", { name: "Mark done" })).toHaveCount(0);
});

test("cards expand independently and stay expanded across reloads and devices", async ({ page, browser }) => {
  const url = await newBoard(page, ["Ideas"]);
  await addCards(page, "Ideas", ["One", "Two", "Three", "Four"]);

  const saved = viewSaved(page);
  for (const t of ["One", "Two", "Four"]) {
    await card(page, t).getByRole("button", { name: "Expand card" }).click();
  }
  for (const t of ["One", "Two", "Four"]) {
    await expect(card(page, t).getByRole("button", { name: "Mark done" })).toBeVisible();
  }
  await expect(card(page, "Three").getByRole("button", { name: "Mark done" })).toHaveCount(0);
  await saved;

  await page.reload();
  for (const t of ["One", "Two", "Four"]) {
    await expect(card(page, t).getByRole("button", { name: "Collapse card" })).toBeVisible();
  }
  await expect(card(page, "Three").getByRole("button", { name: "Expand card" })).toBeVisible();

  // Same user on another "device": a fresh browser context.
  const other = await browser.newContext();
  const page2 = await other.newPage();
  await page2.goto(url);
  for (const t of ["One", "Two", "Four"]) {
    await expect(card(page2, t).getByRole("button", { name: "Collapse card" })).toBeVisible();
  }
  await expect(card(page2, "Three").getByRole("button", { name: "Expand card" })).toBeVisible();
  await other.close();
});

test("keyboard: Space expands, Enter opens the full view, E edits the title", async ({ page }) => {
  test.skip(test.info().project.name !== "chromium" && test.info().project.name !== "webkit", "desktop keyboard");
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Keyboard card"]);
  const c = card(page, "Keyboard card");
  await c.focus();
  await page.keyboard.press("Space");
  await expect(c.getByRole("button", { name: "Collapse card" })).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Keyboard card" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await c.focus();
  await page.keyboard.press("e");
  await page.getByLabel("Title").fill("Renamed by keyboard");
  await page.getByLabel("Title").press("Enter");
  await expect(card(page, "Renamed by keyboard")).toBeVisible();
});

test("collapse a column to a strip and back; it stays collapsed", async ({ page }) => {
  await newBoard(page, ["Noisy", "Calm"]);
  await addCards(page, "Noisy", ["a", "b", "c"]);
  const saved = viewSaved(page);
  await page.getByRole("button", { name: "Collapse Noisy" }).click();
  await expect(column(page, "Noisy").locator(".column-strip")).toContainText("3");
  await expect(card(page, "a")).toHaveCount(0);
  await saved;
  await page.reload();
  await expect(card(page, "a")).toHaveCount(0);
  await column(page, "Noisy").locator(".column-strip").click();
  await expect(card(page, "a")).toBeVisible();
});

test("drag a card to another column with the mouse", async ({ page }) => {
  test.skip(!!test.info().project.use.hasTouch, "mouse only");
  await newBoard(page, ["To do", "Done"]);
  await addCards(page, "To do", ["Drag me", "Stay"]);
  await addCards(page, "Done", ["Already done"]);

  const from = await card(page, "Drag me").boundingBox();
  const to = await card(page, "Already done").boundingBox();
  if (!from || !to) throw new Error("no boxes");
  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await page.mouse.move(from.x + 40, from.y + from.height / 2);
  await page.mouse.down();
  for (let i = 1; i <= 10; i++) {
    await page.mouse.move(from.x + 40 + ((to.x - from.x) * i) / 10, from.y + ((to.y + to.height * 0.75 - from.y) * i) / 10);
  }
  await page.mouse.up();
  await saved;
  await expect(card(column(page, "Done"), "Drag me")).toBeVisible();
  await page.reload();
  expect(await cardTitles(page, "To do")).toEqual(["Stay"]);
  expect(await cardTitles(page, "Done")).toContain("Drag me");
});

test("drag a card with a touch long-press", async ({ browser }) => {
  // Real touch input through the browser's input pipeline (Chrome DevTools
  // Protocol), on an emulated phone. WebKit can't synthesize touches.
  test.skip(test.info().project.name !== "chromium", "needs CDP touch input");
  const ctx = await browser.newContext({ ...devices["Pixel 7"] });
  const page = await ctx.newPage();
  await newBoard(page, ["To do", "Done"]);
  await addCards(page, "To do", ["Press and hold", "Stays put"]);
  await addCards(page, "To do", ["Landing spot"]);

  const from = (await card(page, "Press and hold").boundingBox())!;
  const to = (await card(page, "Landing spot").boundingBox())!;
  const cdp = await ctx.newCDPSession(page);
  const touch = (type: "touchStart" | "touchMove" | "touchEnd", x: number, y: number) =>
    cdp.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y }] });

  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  const x = from.x + 60, y0 = from.y + from.height / 2, y1 = to.y + to.height * 0.8;
  await touch("touchStart", x, y0);
  await page.waitForTimeout(400); // past the 250 ms long-press delay
  for (let i = 1; i <= 12; i++) {
    await touch("touchMove", x, y0 + ((y1 - y0) * i) / 12);
    await page.waitForTimeout(16);
  }
  await touch("touchEnd", x, y1);
  await saved;
  await page.reload();
  expect(await cardTitles(page, "To do")).toEqual(["Stays put", "Landing spot", "Press and hold"]);

  // A quick swipe (no long press) scrolls instead of dragging.
  const box = (await card(page, "Stays put").boundingBox())!;
  await touch("touchStart", box.x + 60, box.y + 10);
  await touch("touchMove", box.x + 60, box.y + 80);
  await touch("touchEnd", box.x + 60, box.y + 80);
  await page.waitForTimeout(300);
  expect(await cardTitles(page, "To do")).toEqual(["Stays put", "Landing spot", "Press and hold"]);
  await ctx.close();
});

test("Move to… on a phone moves a card to another column", async ({ page }) => {
  test.skip(test.info().project.name !== "iphone", "phone viewport");
  await newBoard(page, ["Inbox", "Later"]);
  await addCards(page, "Inbox", ["Move me"]);
  await card(page, "Move me").getByRole("button", { name: "Expand card" }).click();
  await card(page, "Move me").getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Move to…" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Later" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("navigation", { name: "Columns" }).getByRole("button", { name: "Later" }).click();
  await expect(card(column(page, "Later"), "Move me")).toBeVisible();
  await page.reload();
  expect(await cardTitles(page, "Later")).toEqual(["Move me"]);
  expect(await cardTitles(page, "Inbox")).toEqual([]);
});

test("delete a card, then undo from the toast", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Keep", "Oops"]);
  await card(page, "Oops").getByRole("button", { name: "Expand card" }).click();
  await card(page, "Oops").getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Delete" }).click();
  await expect(card(page, "Oops")).toHaveCount(0);
  await expect(page.getByRole("status", { name: "Notifications" })).toContainText("Moved “Oops” to the trash");
  await page.getByRole("status", { name: "Notifications" }).getByRole("button", { name: "Undo" }).click();
  await expect(card(page, "Oops")).toBeVisible();
  await page.reload();
  expect(await cardTitles(page, "Col")).toEqual(["Keep", "Oops"]);
});

test("delete a card, then restore it from the trash", async ({ page }) => {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", ["Recoverable"]);
  await card(page, "Recoverable").getByRole("button", { name: "Open card" }).click();
  const deleted = page.waitForResponse((r) => r.request().method() === "DELETE");
  await page.getByRole("dialog").getByRole("button", { name: "Delete" }).click();
  await deleted;
  await page.reload();
  await expect(card(page, "Recoverable")).toHaveCount(0);
  await page.getByRole("button", { name: "Board" }).click();
  await page.getByRole("menuitem", { name: "Trash" }).click();
  const trash = page.getByRole("dialog", { name: "Trash" });
  await expect(trash).toContainText("Recoverable");
  await trash.getByRole("button", { name: "Restore" }).click();
  await expect(trash).toContainText("The trash is empty.");
  await trash.getByRole("button", { name: "Close" }).click();
  await expect(card(page, "Recoverable")).toBeVisible();
});

test("deleting a column with cards asks first; an empty column doesn't", async ({ page }) => {
  await newBoard(page, ["Full", "Empty"]);
  await addCards(page, "Full", ["x"]);
  await page.getByRole("button", { name: "Empty options" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete column" }).click();
  await expect(column(page, "Empty")).toHaveCount(0);
  await page.getByRole("button", { name: "Full options" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete column" }).click();
  await expect(page.getByRole("alert")).toContainText("and its 1 card to the trash?");
  await page.getByRole("button", { name: "Keep it" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Close" }).click();
  await expect(column(page, "Full")).toBeVisible();
});

test("no serious accessibility issues on a board with cards", async ({ page }) => {
  await newBoard(page, ["To do", "Done"]);
  await addCards(page, "To do", ["One", "Two"]);
  await card(page, "One").getByRole("button", { name: "Expand card" }).click();
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    const results = await new AxeBuilder({ page }).analyze();
    const bad = results.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(bad.map((v) => `${scheme} ${v.id}: ${v.help} (${v.nodes.map((n) => n.target).join(", ")})`)).toEqual([]);
  }
});

test("move a card up and down from its menu and with Alt+arrow keys", async ({ page, isMobile }) => {
  await newBoard(page, ["List"]);
  await addCards(page, "List", ["First", "Second", "Third"]);
  const third = card(page, "Third");
  await third.getByRole("button", { name: "Expand card" }).click();
  await third.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Move up" }).click();
  await expect.poll(() => cardTitles(page, "List")).toEqual(["First", "Third", "Second"]);
  await page.reload();
  await expect.poll(() => cardTitles(page, "List")).toEqual(["First", "Third", "Second"]);
  if (isMobile) return; // the rest is keyboard
  await card(page, "First").focus();
  await page.keyboard.press("Alt+ArrowDown");
  await expect.poll(() => cardTitles(page, "List")).toEqual(["Third", "First", "Second"]);
  await expect(card(page, "First")).toBeFocused();
});

test("copy a card, save it as a template, and start a card from the template", async ({ page }) => {
  await newBoard(page, ["To do", "Done"]);
  await addCards(page, "To do", ["Weekly shop", "Something else"]);
  const c = card(page, "Weekly shop");
  await c.getByRole("button", { name: "Expand card" }).click();
  await c.getByRole("button", { name: "Add a checklist" }).click();
  const input = c.getByLabel("New checklist item");
  for (const t of ["Milk", "Bread"]) {
    await input.fill(t);
    await input.press("Enter");
  }
  await input.press("Escape");
  await c.getByRole("checkbox", { name: "Milk" }).check();

  // Copy: lands just after the original, with its checklist as it was.
  await c.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Copy…" }).click();
  let dialog = page.getByRole("dialog", { name: "Copy card" });
  await dialog.getByLabel("Title").fill("Weekly shop (next week)");
  await dialog.getByRole("button", { name: "Copy", exact: true }).click();
  await expect.poll(() => cardTitles(page, "To do")).toEqual(["Weekly shop", "Weekly shop (next week)", "Something else"]);
  const copy = card(page, "Weekly shop (next week)");
  await copy.getByRole("button", { name: "Expand card" }).click();
  await expect(copy.getByRole("checkbox", { name: "Milk" })).toBeChecked();

  // Save as a template: not in the columns, listed in Board settings.
  await c.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Save as template…" }).click();
  dialog = page.getByRole("dialog", { name: "Save as a template" });
  await dialog.getByLabel("Template name").fill("Shopping list");
  await dialog.getByRole("button", { name: "Save template" }).click();
  await expect(card(page, "Shopping list")).toHaveCount(0); // not a card in a column
  await expectAccessible(page, "board with a template");

  // Start a card from it in Done: at the bottom, items unticked.
  await page.getByRole("button", { name: "Add a card to Done from a template" }).click();
  await page.getByRole("menuitem", { name: "Shopping list" }).click();
  const made = card(page, "Shopping list");
  await expect(made).toBeVisible();
  await expect(made.getByRole("checkbox", { name: "Milk" })).not.toBeChecked();
  expect(await cardTitles(page, "Done")).toEqual(["Shopping list"]);

  await page.reload();
  expect(await cardTitles(page, "To do")).toEqual(["Weekly shop", "Weekly shop (next week)", "Something else"]);
  await expect(card(page, "Shopping list")).toBeVisible();
});
