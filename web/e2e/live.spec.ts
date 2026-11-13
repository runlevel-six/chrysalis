// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Live sync (§5.5): a change in one tab shows up in another without a reload.

import { expect, test } from "@playwright/test";

import { addCards, card, newBoard } from "./helpers";

test("changes in one tab appear in another", async ({ page, context }) => {
  await newBoard(page, ["Today"]);
  const other = await context.newPage();
  await other.goto(page.url());
  await expect(other.getByRole("region", { name: "Today", exact: true })).toBeVisible();
  // Let the second tab's stream connect before changing anything.
  await other.waitForResponse((r) => r.url().includes("/api/v1/events"), { timeout: 5000 }).catch(() => undefined);
  await other.waitForTimeout(300);

  await addCards(page, "Today", ["Water the seedlings"]);
  await expect(card(other, "Water the seedlings")).toBeVisible({ timeout: 5000 });

  // And back the other way.
  await addCards(other, "Today", ["Turn the compost"]);
  await expect(card(page, "Turn the compost")).toBeVisible({ timeout: 5000 });
  // The tab that made a change doesn't refetch its own echo into a duplicate.
  await page.waitForTimeout(500);
  await expect(card(other, "Turn the compost")).toHaveCount(1);
});
