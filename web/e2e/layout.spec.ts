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
  for (const w of await widths(page)) {
    expect(w).toBeGreaterThan(400); // not stuck at a narrow fixed width
    expect(w).toBeLessThanOrEqual(480); // 30rem: lines stay readable
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
