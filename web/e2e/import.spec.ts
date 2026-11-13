// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

import { cardTitles, column, expectAccessible, gotoBoards, unique } from "./helpers";

// The same synthetic board the Go golden tests use. Every project shares
// one dev user, so each run gives the board its own name.
const fixture = JSON.parse(readFileSync(new URL("../../internal/trello/testdata/garden-board.json", import.meta.url), "utf8"));

test("import a Trello board export and open it", async ({ page }) => {
  const name = unique("Garden");
  await gotoBoards(page);

  const done = page.waitForResponse((r) => r.url().endsWith("/import/trello") && r.request().method() === "POST");
  await page.getByLabel("Trello export file").setInputFiles({
    name: "garden.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify({ ...fixture, name })),
  });
  expect((await done).status()).toBe(201);

  const report = page.getByRole("dialog", { name: "Imported from Trello" });
  await expect(report).toContainText(`${name} is ready: 5 columns, 6 cards`);
  await expect(report.getByRole("heading", { name: "Left out" })).toBeVisible();
  await expect(report).toContainText("Power-Up data");
  await expect(report).toContainText("collapsed column");
  await expectAccessible(page, "import report");

  await report.getByRole("link", { name: "Open the board" }).click();
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  for (const col of ["Last year", "Ideas", "To plant", "Growing", "Harvested"]) {
    await expect(column(page, col)).toBeVisible();
  }
  expect(await cardTitles(page, "To plant")).toEqual(["Sow basil", "Start tomato seeds"]);
  expect(await cardTitles(page, "Harvested")).toEqual(["Build raised beds"]);
  // Archived in Trello: out of sight here too.
  expect(await cardTitles(page, "Growing")).toEqual([]);
  await expectAccessible(page, "imported board");

  // It's in the board list as well.
  await gotoBoards(page);
  await expect(page.getByRole("link", { name, exact: true })).toBeVisible();
});

test("a file that isn't a Trello export gets a calm message", async ({ page }) => {
  await gotoBoards(page);
  await page.getByLabel("Trello export file").setInputFiles({
    name: "notes.json",
    mimeType: "application/json",
    buffer: Buffer.from('{"hello":"world"}'),
  });
  await expect(page.locator(".trello-import").getByRole("status")).toContainText("doesn't look like a Trello board export");
});
