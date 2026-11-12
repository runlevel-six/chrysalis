// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { expect, test, type Page } from "@playwright/test";

import { addCards, card, expand, expectAccessible, newBoard } from "./helpers";

const editorOf = (page: Page, title: string) => page.getByRole("textbox", { name: `Description for ${title}` }).first();
const descSaved = (page: Page) =>
  page.waitForResponse((r) => r.url().endsWith("/description") && r.request().method() === "PUT" && r.ok());

async function cardWithEditor(page: Page, title: string) {
  const url = await newBoard(page, ["Col"]);
  await addCards(page, "Col", [title]);
  await expand(page, title);
  const ed = editorOf(page, title);
  await expect(ed).toBeVisible();
  return { url, ed };
}

test("type a description; it autosaves and survives a reload", async ({ page }) => {
  const cspErrors: string[] = [];
  page.on("console", (m) => /Content Security Policy|Refused to/.test(m.text()) && cspErrors.push(m.text()));
  const { ed } = await cardWithEditor(page, "Garden");
  // Calm by default: no formatting tools on an inline card until you edit.
  await expect(page.getByRole("button", { name: "Bold" })).toHaveCount(0);
  await ed.click();
  const saved = descSaved(page);
  await page.keyboard.type("Plant milkweed ");
  await page.getByRole("button", { name: "Bold" }).click();
  await page.keyboard.type("by the fence");
  await saved;
  await expect(card(page, "Garden").getByText("Saved", { exact: true })).toBeVisible();
  await page.reload();
  const ed2 = editorOf(page, "Garden");
  await expect(ed2).toContainText("Plant milkweed by the fence");
  await expect(ed2.locator("strong")).toHaveText("by the fence");
  expect(cspErrors).toEqual([]); // the editor works under the strict CSP
});

test("an unsent edit is kept on the device and sent after a reload", async ({ page }) => {
  const { ed } = await cardWithEditor(page, "Notes");
  // The server is unreachable for saves.
  await page.route("**/description", (r) => (r.request().method() === "PUT" ? r.abort() : r.continue()));
  await ed.click();
  await page.keyboard.type("Written while offline");
  await expect(card(page, "Notes")).toContainText("Not saved yet");
  await page.waitForTimeout(400); // the draft is written to IndexedDB
  await page.unroute("**/description");

  const saved = descSaved(page);
  await page.reload();
  await saved;
  await expect(card(page, "Notes")).toContainText("Restored your unsaved changes. Saved.");
  await page.reload();
  await expect(editorOf(page, "Notes")).toContainText("Written while offline");
});

/**
 * Two devices edit the same description at once. Device 2's save is held at
 * the network until device 1 has saved, so it arrives based on the older
 * version: a real conflict, every time.
 */
async function collide(page: Page, browser: import("@playwright/test").Browser, title: string, mine: string, theirs: string) {
  const { url, ed } = await cardWithEditor(page, title);
  const other = await browser.newContext();
  const page2 = await other.newPage();
  await page2.goto(url);
  // Expand here too: device 1's expanded state may not have synced yet.
  await expand(page2, title);
  const ed2 = editorOf(page2, title);
  await expect(ed2).toBeVisible();

  let release!: () => void;
  const held = new Promise<void>((r) => (release = r));
  let holding = true;
  await page2.route("**/description", async (route) => {
    if (route.request().method() === "PUT" && holding) {
      holding = false;
      await held;
    }
    await route.continue();
  });

  await ed2.click();
  await page2.keyboard.type(mine); // device 2's save is now waiting at the network

  await ed.click();
  const saved = descSaved(page);
  await page.keyboard.type(theirs);
  await saved; // device 1 wins the race

  release();
  const prompt = page2.getByRole("alert").filter({ hasText: "edited on another device" });
  await expect(prompt).toBeVisible();
  return { page2, ed2, prompt, other };
}

test("edits on two devices: keep yours", async ({ page, browser }) => {
  const { page2, prompt, other } = await collide(page, browser, "Shared", "From the phone", "From the laptop");
  await prompt.getByRole("button", { name: "View both" }).click();
  await expect(prompt.getByRole("region", { name: "The other device's version" })).toContainText("From the laptop");
  const saved = descSaved(page2);
  await prompt.getByRole("button", { name: "Keep yours" }).click();
  await saved;
  await expect(prompt).toHaveCount(0);
  await page.reload();
  await expect(editorOf(page, "Shared")).toContainText("From the phone");
  await other.close();
});

test("edits on two devices: use theirs", async ({ page, browser }) => {
  const { page2, ed2, prompt, other } = await collide(page, browser, "Shared too", "Discard me", "Keep this one");
  await prompt.getByRole("button", { name: "Use theirs" }).click();
  await expect(ed2).toContainText("Keep this one");
  await expect(ed2).not.toContainText("Discard me");
  await page2.reload();
  await expect(editorOf(page2, "Shared too")).toContainText("Keep this one");
  await other.close();
});

test("links: ordinary addresses work, javascript: is refused", async ({ page }) => {
  const { ed } = await cardWithEditor(page, "Links");
  await ed.click();
  await page.keyboard.type("seed catalog");
  await page.keyboard.press("Shift+Home");
  await page.getByRole("button", { name: "Link", exact: true }).click();
  await page.getByLabel("Link address").fill("javascript:alert(1)");
  await page.getByLabel("Link address").press("Enter");
  await expect(ed.locator("a")).toHaveCount(0);

  await ed.click();
  await page.keyboard.press("Shift+Home");
  await page.getByRole("button", { name: "Link", exact: true }).click();
  await page.getByLabel("Link address").fill("example.com/seeds");
  const saved = descSaved(page);
  await page.getByLabel("Link address").press("Enter");
  await expect(ed.locator("a")).toHaveAttribute("href", "https://example.com/seeds");
  await saved;
});

test("no serious accessibility issues with the editor open", async ({ page }) => {
  const { ed } = await cardWithEditor(page, "Accessible");
  await ed.click();
  await page.keyboard.type("Some text");
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `editor (${scheme})`);
  }
});
