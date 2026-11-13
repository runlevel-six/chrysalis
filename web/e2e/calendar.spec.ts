// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The calendar view (§3.8): cards by due date, drag to reschedule, undated
// cards panel, and opening a card on its board.

import { expect, test, type Page } from "@playwright/test";

import { addCards, expectAccessible, newBoard } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");
const ymd = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const longDate = (s: string) => {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" });
};

/** A board with one dated and one undated card; returns its id and dates. */
async function boardWithDates(page: Page) {
  await newBoard(page, ["To do"]);
  const boardId = page.url().split("/b/")[1];
  await addCards(page, "To do", ["Plant bulbs", "Sketch the beds"]);
  // A Wednesday in the middle of next month's grid, so it's never at an edge.
  const now = new Date();
  const due = new Date(now.getFullYear(), now.getMonth() + 1, 15);
  const day = ymd(due);
  const detail = await (await page.request.get(`/api/v1/boards/${boardId}/detail`)).json();
  const bulbs = detail.cards.find((c: { title: string }) => c.title === "Plant bulbs");
  const res = await page.request.patch(`/api/v1/cards/${bulbs.id}`, { data: { dueAt: day }, headers: { Origin: new URL(page.url()).origin } });
  expect(res.ok()).toBe(true);
  return { boardId, day, cardId: bulbs.id as string };
}

const dayCell = (page: Page, day: string) => page.getByRole("region", { name: new RegExp(`^${longDate(day)}`) });

test("cards show on their day; open one on its board", async ({ page, isMobile }) => {
  const { boardId, day } = await boardWithDates(page);
  // Phones show the month as dots; the week lists the cards.
  await page.goto(`/calendar?board=${boardId}&view=${isMobile ? "week" : "month"}&date=${day}`);
  await expect(dayCell(page, day).getByRole("button", { name: /^Plant bulbs/ })).toBeVisible();
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `calendar month (${scheme})`);
  }
  await dayCell(page, day).getByRole("button", { name: /^Plant bulbs/ }).click();
  await expect(page).toHaveURL(new RegExp(`/b/${boardId}$`));
  await expect(page.getByRole("dialog", { name: "Plant bulbs" })).toBeVisible();
});

test("drag a card to another day, and an undated card onto a day", async ({ page, isMobile }) => {
  test.skip(isMobile, "mouse drag");
  const { boardId, day } = await boardWithDates(page);
  await page.goto(`/calendar?board=${boardId}&view=week&date=${day}&undated=1`);
  const [y, m, d] = day.split("-").map(Number);
  const next = ymd(new Date(y, m - 1, d + 1));
  const target = await dayCell(page, next).boundingBox();

  const drag = async (name: RegExp) => {
    const chip = await page.getByRole("button", { name }).boundingBox();
    await page.mouse.move(chip!.x + 20, chip!.y + 10);
    await page.mouse.down();
    await page.mouse.move(chip!.x + 40, chip!.y + 20, { steps: 4 });
    await page.mouse.move(target!.x + target!.width / 2, target!.y + 120, { steps: 15 });
    await page.mouse.up();
  };

  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await drag(/^Plant bulbs/);
  await saved;
  await expect(dayCell(page, next).getByRole("button", { name: /^Plant bulbs/ })).toBeVisible();

  const saved2 = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await drag(/^Sketch the beds/);
  await saved2;
  await expect(dayCell(page, next).getByRole("button", { name: /^Sketch the beds/ })).toBeVisible();
  await expect(page.getByRole("complementary", { name: "Undated cards" }).getByText("Every open card has a date.")).toBeVisible();

  await page.reload();
  await expect(dayCell(page, next).getByRole("button", { name: /^Plant bulbs/ })).toBeVisible();
  await expect(dayCell(page, next).getByRole("button", { name: /^Sketch the beds/ })).toBeVisible();
});

test("move a card a day with the keyboard, and undo it", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop keyboard");
  const { boardId, day } = await boardWithDates(page);
  await page.goto(`/calendar?board=${boardId}&view=week&date=${day}`);
  const [y, m, d] = day.split("-").map(Number);
  const next = ymd(new Date(y, m - 1, d + 1));
  await dayCell(page, day).getByRole("button", { name: /^Plant bulbs/ }).focus();
  const live = page.locator("[id^=DndLiveRegion]");
  await page.keyboard.press("m");
  await expect(live).toContainText(`over ${longDate(day)}`);
  // dnd-kit measures the days just after pickup; an arrow pressed within the
  // same few milliseconds (only a test is that quick) can be dropped.
  await page.waitForTimeout(200);
  await page.keyboard.press("ArrowRight");
  // Screen readers hear where it is; wait for that before dropping.
  await expect(live).toContainText(`over ${longDate(next)}`);
  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await page.keyboard.press("m");
  await saved;
  await expect(dayCell(page, next).getByRole("button", { name: /^Plant bulbs/ })).toBeVisible();
  await page.getByRole("status", { name: "Notifications" }).getByRole("button", { name: "Undo" }).click();
  await expect(dayCell(page, day).getByRole("button", { name: /^Plant bulbs/ })).toBeVisible();
});

test("the calendar is reachable from the header and from a board", async ({ page }) => {
  const { boardId } = await boardWithDates(page);
  await page.getByRole("button", { name: "View" }).click();
  await page.getByRole("menuitem", { name: "Calendar" }).click();
  await expect(page).toHaveURL(new RegExp(`/calendar\\?board=${boardId}`));
  await expect(page.getByRole("heading", { name: "Calendar", level: 1 })).toBeVisible();
  await page.goto("/");
  await page.getByRole("link", { name: "Calendar" }).click();
  await expect(page.getByRole("combobox", { name: "Boards shown" })).toHaveValue("");
});
