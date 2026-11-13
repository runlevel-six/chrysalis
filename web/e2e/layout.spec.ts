// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Desktop layout: columns use the space a wide screen gives them, and an
// expanded card's contents get the card's full width.

import { expect, test } from "@playwright/test";

import { addCards, card, expand, newBoard } from "./helpers";

test.skip(({ isMobile }) => isMobile, "desktop layout");

async function boardWithChecklist(page: import("@playwright/test").Page) {
  await newBoard(page, ["Ingest", "In progress", "In review", "Completed"]);
  await addCards(page, "In progress", ["Plan the bed"]);
  const c = await expand(page, "Plan the bed");
  await c.getByRole("button", { name: "Add a checklist" }).click();
  const input = c.getByLabel("New checklist item");
  await input.fill("Second thing");
  await input.press("Enter");
  await expect(c.getByRole("checkbox", { name: "Second thing" })).toBeVisible();
  await input.press("Escape");
  return c;
}

const widths = (page: import("@playwright/test").Page) =>
  page.locator(".column:not(.is-collapsed)").evaluateAll((els) => els.map((e) => e.getBoundingClientRect().width));

test("columns grow on a wide screen, up to a comfortable maximum", async ({ page }) => {
  await page.setViewportSize({ width: 2560, height: 1440 });
  await boardWithChecklist(page);
  // Four columns fill a wide desktop: the last one ends near the right edge.
  const right = await page.locator(".column").last().evaluate((e) => e.getBoundingClientRect().right);
  expect(right / 2560).toBeGreaterThan(0.85);
  for (const w of await widths(page)) {
    expect(w).toBeGreaterThan(400); // not stuck at a narrow fixed width
    expect(w).toBeLessThanOrEqual(640); // 40rem: lines stay readable
  }
});

test("on a narrower desktop, columns keep their minimum and the board scrolls", async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 });
  await boardWithChecklist(page);
  for (const w of await widths(page)) expect(w).toBeGreaterThanOrEqual(17 * 16 - 1);
});

test("an expanded card's checklist text gets most of the card's width", async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 });
  const c = await boardWithChecklist(page);
  const cardWidth = (await c.boundingBox())!.width;
  const textWidth = (await c.locator(".checklist-text").first().boundingBox())!.width;
  // Before the fix, nested indents left a sliver and words broke mid-word.
  expect(textWidth / cardWidth).toBeGreaterThan(0.4);
  await expect(card(page, "Plan the bed").locator(".checklist-text").first()).toHaveCSS("overflow-wrap", "break-word");
});

test("checklist controls line up with the first line of long items", async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 });
  const c = await boardWithChecklist(page);
  const input = c.getByLabel("New checklist item");
  if (!(await input.isVisible())) await c.getByRole("button", { name: "Add an item" }).click();
  const long = "A really long task with an absurd name so we can see how it wraps across lines";
  await c.getByLabel("New checklist item").fill(long);
  await c.getByLabel("New checklist item").press("Enter");
  const row = c.locator(".checklist-item").filter({ hasText: "absurd name" });
  await expect(row).toBeVisible();

  const m = await row.evaluate((li) => {
    const text = li.querySelector(".checklist-text")!;
    const range = document.createRange();
    range.setStart(text.firstChild!, 0);
    range.setEnd(text.firstChild!, 1);
    const first = range.getBoundingClientRect(); // the first line
    const mid = (r: DOMRect) => r.top + r.height / 2;
    const box = li.querySelector("input[type=checkbox]")!.getBoundingClientRect();
    const del = li.querySelector("button[aria-label^='Delete']")!.getBoundingClientRect();
    return {
      lines: Math.round(text.getBoundingClientRect().height / first.height),
      checkbox: Math.abs(mid(box) - mid(first)),
      remove: Math.abs(mid(del) - mid(first)),
    };
  });
  expect(m.lines).toBeGreaterThan(1); // it really does wrap
  expect(m.checkbox).toBeLessThan(3);
  expect(m.remove).toBeLessThan(4);
});

test("collapsing a column doesn't resize the others", async ({ page }) => {
  await page.setViewportSize({ width: 1800, height: 1000 });
  await newBoard(page, ["Ingest", "In progress", "In review", "Completed"]);
  const before = await widths(page);
  await page.getByRole("button", { name: "Collapse Completed" }).click();
  await expect(page.locator(".column.is-collapsed")).toHaveCount(1);
  const after = await widths(page);
  expect(after).toHaveLength(3);
  for (const w of after) expect(Math.abs(w - before[0])).toBeLessThan(1);
});

test("a card's More menu is fully visible and usable, even in the first column", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await newBoard(page, ["Ingest", "Doing"]);
  await addCards(page, "Ingest", ["Edge card"]);
  const c = await expand(page, "Edge card");
  await c.getByRole("button", { name: "More" }).click();
  const menu = page.getByRole("menu", { name: "More" });
  await expect(menu).toBeVisible();
  for (const name of ["Cover…", "Move to…", "Archive", "Delete"]) {
    const item = menu.getByRole("menuitem", { name });
    const box = (await item.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(1440);
    // Nothing covers it or clips it: the topmost element at its center is the item.
    const hit = await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.textContent?.trim(), [box.x + box.width / 2, box.y + box.height / 2]);
    expect(hit).toBe(name);
  }
  await menu.getByRole("menuitem", { name: "Move to…" }).click();
  await expect(page.getByRole("dialog", { name: "Move “Edge card”" })).toBeVisible();
});
