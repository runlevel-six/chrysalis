// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// The centering line (docs/DESIGN.md §3.13).
//
// Turning the line on is a setting of the one shared dev user, and it shows
// on every board, so these run in the "preferences" project after the
// others, one at a time, and switch it back off when they finish.

import { expect, test, type Page } from "@playwright/test";

import { expectAccessible, newBoard, unique, viewSaved } from "./helpers";

test.describe.configure({ mode: "serial" });

const OFF = { enabled: false, mode: "rotate", interval: "daily", order: "sequential" };
const made: string[] = [];

test.afterEach(async ({ page }) => {
  await page.request.post("/api/v1/me/centering/line/show");
  await page.request.patch("/api/v1/me/centering", { data: OFF });
  for (const id of made.splice(0)) await page.request.delete(`/api/v1/centering/collections/${id}`);
});

const line = (page: Page) => page.getByRole("region", { name: "Centering line" });

/** Make a collection through the API and have it alone feed the line, turned on. */
async function collectionOnly(page: Page, texts: string[]) {
  const res = await page.request.post("/api/v1/centering/collections", { data: { name: unique("Morning") } });
  const col = (await res.json()) as { id: string; name: string };
  made.push(col.id);
  for (const text of texts) {
    await page.request.post(`/api/v1/centering/collections/${col.id}/entries`, { data: { text, attribution: "A gardener" } });
  }
  await page.request.patch("/api/v1/me/centering", { data: { ...OFF, enabled: true, collectionIds: [col.id] } });
  return col;
}

test("add a collection and entries in settings, and turn the line on", async ({ page }) => {
  const name = unique("Evening");
  await page.goto("/settings");
  await page.getByRole("link", { name: "Collections and entries" }).click();
  await expect(page.getByRole("heading", { name: "Centering line" })).toBeVisible();

  await page.getByLabel("New collection name").fill(name);
  await page.getByRole("button", { name: "Add collection" }).click();
  await expect(page.getByRole("button", { name: new RegExp(`^${name}`) })).toHaveAttribute("aria-expanded", "true");
  const id = ((await (await page.request.get("/api/v1/centering/collections")).json()) as { collections: { id: string; name: string }[] }).collections.find(
    (c) => c.name === name,
  )!.id;
  made.push(id);

  await page.getByText("Add an entry").click();
  for (const text of ["Be still, and breathe.", "Rest is part of the work."]) {
    await page.getByLabel("Text", { exact: true }).fill(text);
    await page.getByLabel(/Said or written by/).fill("A gardener");
    await page.getByRole("button", { name: "Add entry" }).click();
    await expect(page.getByRole("list", { name: `Entries in ${name}` })).toContainText(text);
  }
  // Reorder and edit.
  const entries = page.getByRole("list", { name: `Entries in ${name}` }).getByRole("listitem");
  await entries.nth(1).getByRole("button", { name: "Move up" }).click();
  await expect(entries.first()).toContainText("Rest is part of the work.");
  await entries.first().getByRole("button", { name: "Edit" }).click();
  await entries.first().getByLabel(/Reference/).fill("Notebook 3");
  await entries.first().getByRole("button", { name: "Save" }).click();
  await expect(entries.first()).toContainText("A gardener, Notebook 3");

  // Delete, with undo.
  await entries.nth(1).getByRole("button", { name: "Delete entry" }).click();
  await expect(entries).toHaveCount(1);
  await page.getByRole("button", { name: "Undo" }).click();
  await expect(entries).toHaveCount(2);
  await expectAccessible(page, "centering settings");

  // Turn it on, fed by just this collection.
  await page.getByRole("link", { name: "Back to settings" }).click();
  await page.getByLabel("Show the centering line on my boards").check();
  await expect(page.getByLabel("Every day, at midnight")).toBeChecked();
  const others = page.locator("fieldset", { hasText: "Entries come from" }).getByRole("checkbox");
  for (const box of await others.all()) {
    const label = await box.evaluate((el) => el.closest("label")?.textContent ?? "");
    if (label.startsWith(name) !== (await box.isChecked())) await box.click();
  }
  await expectAccessible(page, "settings with the centering line on");

  await newBoard(page, ["Col"]);
  await expect(line(page)).toContainText("Rest is part of the work.");
});

test("the line expands, steps, pins and hides for today", async ({ page }) => {
  await collectionOnly(page, ["Be still, and breathe.", "Water the roots, not the leaves.", "One row at a time."]);
  await newBoard(page, ["Col"]);
  const text = line(page).locator(".centering-text");
  await expect(text).toHaveText("Be still, and breathe.");
  // One line, collapsed.
  expect((await line(page).boundingBox())!.height).toBeLessThanOrEqual(32);

  // Expand inline: the source shows, and the controls.
  await text.click();
  await expect(text).toHaveAttribute("aria-expanded", "true");
  await expect(text).toContainText("— A gardener");
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `centering line (${scheme})`);
  }
  await text.press("Enter");
  await expect(text).toHaveAttribute("aria-expanded", "false");

  // Controls appear on focus.
  await text.focus();
  await line(page).getByRole("button", { name: "Next entry" }).click();
  await expect(text).toContainText("Water the roots, not the leaves.");
  await line(page).getByRole("button", { name: "Previous entry" }).click();
  await expect(text).toContainText("Be still, and breathe.");
  await line(page).getByRole("button", { name: "Previous entry" }).click();
  await expect(text).toContainText("One row at a time."); // wraps around

  // Every device agrees: a reload shows the same entry.
  await page.reload();
  await expect(text).toContainText("One row at a time.");

  // Pin: it stays, and stepping is put away until it's unpinned.
  await text.click();
  await line(page).getByRole("button", { name: "Pin this entry" }).click();
  await expect(line(page).getByRole("button", { name: "Unpin this entry" })).toBeVisible();
  await expect(line(page).getByRole("button", { name: "Next entry" })).toHaveCount(0);
  await page.reload();
  await text.click();
  await expect(line(page).getByRole("button", { name: "Unpin this entry" })).toBeVisible();
  await line(page).getByRole("button", { name: "Unpin this entry" }).click();
  await expect(line(page).getByRole("button", { name: "Next entry" })).toBeVisible();

  // Hide for today, with undo; then it stays hidden after a reload.
  await line(page).getByRole("button", { name: "Hide for today" }).click();
  await expect(line(page)).toHaveCount(0);
  await page.getByRole("button", { name: "Undo" }).click();
  await expect(line(page)).toBeVisible();
  await text.click();
  await line(page).getByRole("button", { name: "Hide for today" }).click();
  await expect(line(page)).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("button", { name: "Add a column" }).or(page.getByLabel("New column name"))).toBeVisible();
  await expect(line(page)).toHaveCount(0);
});

test("each board can hide the line, and it reads well on a background", async ({ page }) => {
  await collectionOnly(page, ["Be still, and breathe."]);
  const first = await newBoard(page, ["Col"]);
  await expect(line(page)).toBeVisible();

  await page.getByRole("button", { name: "Board", exact: true }).click();
  await page.getByRole("menuitem", { name: "Board settings" }).click();
  const dialog = page.getByRole("dialog", { name: "Board settings" });
  const saved = viewSaved(page);
  await dialog.getByLabel(/Show the centering line on this board/).uncheck();
  await saved;
  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(line(page)).toHaveCount(0);

  // Other boards still show it.
  await newBoard(page, ["Col"]);
  await expect(line(page)).toBeVisible();
  const boardId = new URL(page.url()).pathname.split("/").pop()!;
  await page.request.patch(`/api/v1/boards/${boardId}`, { data: { background: { type: "gradient", value: "meadow" } } });
  await page.reload();
  await expect(page.locator(".board.has-background")).toBeVisible();
  await expect(line(page)).toBeVisible();
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `centering line on a background (${scheme})`);
  }

  // And the hidden board stays hidden, and can bring it back from the View menu.
  await page.goto(first);
  await expect(page.getByRole("button", { name: "Add a column" }).or(page.getByLabel("New column name"))).toBeVisible();
  await expect(line(page)).toHaveCount(0);
  await page.getByRole("button", { name: "View" }).click();
  await page.getByRole("menuitem", { name: "Show the centering line here" }).click();
  await expect(line(page)).toBeVisible();
});

test("import a YAML file and export it again", async ({ page }) => {
  const name = unique("Imported");
  await page.goto("/settings/centering");
  const done = page.waitForResponse((r) => r.url().includes("/centering/import") && r.request().method() === "POST");
  await page.getByLabel("File to import").setInputFiles({
    name: "morning.yaml",
    mimeType: "application/yaml",
    buffer: Buffer.from(`collection: ${name}\nentries:\n  - text: Be still, and breathe.\n    attribution: A gardener\n  - text: Rest is part of the work.\n`),
  });
  const res = await done;
  expect(res.status()).toBe(201);
  made.push(((await res.json()) as { collection: { id: string } }).collection.id);
  await expect(page.getByRole("status").filter({ hasText: "Added 2 entries" })).toContainText(`a new collection, “${name}”`);
  await expect(page.getByRole("list", { name: `Entries in ${name}` }).getByRole("listitem")).toHaveCount(2);

  const href = await page.getByRole("link", { name: "YAML" }).getAttribute("href");
  const file = await page.request.get(href!);
  expect(file.headers()["content-disposition"]).toContain("attachment");
  expect(await file.text()).toContain("text: Be still, and breathe.");

  // A file that isn't one gets a calm message.
  await page.getByLabel("File to import").setInputFiles({ name: "notes.yaml", mimeType: "application/yaml", buffer: Buffer.from("entries:\n  - txt: hi\n") });
  await expect(page.getByRole("status").filter({ hasText: "couldn't be read" })).toBeVisible();
});
