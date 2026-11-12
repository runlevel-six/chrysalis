// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

import { expect, test, type Locator, type Page } from "@playwright/test";

import { addCards, card, expand, expectAccessible, newBoard } from "./helpers";

// A real 2x2 PNG, so the server can decode it and make a thumbnail.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGM4UWEDRAwQCgArzgXx0HrtWgAAAABJRU5ErkJggg==",
  "base64",
);

async function expandedCard(page: Page, title: string) {
  await newBoard(page, ["Col"]);
  await addCards(page, "Col", [title]);
  return expand(page, title);
}

const attachmentsOf = (c: Locator) => c.getByRole("region", { name: "Attachments" });
const uploaded = (page: Page) =>
  page.waitForResponse((r) => /\/attachments$/.test(r.url()) && r.request().method() === "POST");

test("attach a file; it persists and downloads as a file", async ({ page }) => {
  const c = await expandedCard(page, "Paperwork");
  const done = uploaded(page);
  await attachmentsOf(c).locator("input[type=file]").setInputFiles({ name: "plan.txt", mimeType: "text/plain", buffer: Buffer.from("seed order") });
  await done;
  const link = attachmentsOf(c).getByRole("link", { name: "plan.txt" });
  await expect(link).toBeVisible();
  await expect(attachmentsOf(c)).toContainText("10 B");

  await page.reload();
  const href = await attachmentsOf(card(page, "Paperwork")).getByRole("link", { name: "plan.txt" }).getAttribute("href");
  const res = await page.request.get(href!);
  expect(res.status()).toBe(200);
  expect(res.headers()["content-disposition"]).toMatch(/^attachment/);
  expect(await res.text()).toBe("seed order");
});

test("attach an image, make it the cover, then switch to a color and remove it", async ({ page }) => {
  const c = await expandedCard(page, "Garden photo");
  const done = uploaded(page);
  await attachmentsOf(c).locator("input[type=file]").setInputFiles({ name: "bed.png", mimeType: "image/png", buffer: PNG });
  await done;
  await expect(attachmentsOf(c).getByRole("img", { name: "bed.png" })).toBeVisible();

  const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().includes("/cards/"));
  await attachmentsOf(c).getByRole("button", { name: "Make cover" }).click();
  await saved;
  await expect(c.locator(".card-cover.is-image img")).toHaveAttribute("src", /\?thumb=1$/);

  // The cover shows on the collapsed card too, and after a reload.
  await c.getByRole("button", { name: "Collapse card" }).click();
  await page.reload();
  await expect(card(page, "Garden photo").locator(".card-cover.is-image")).toBeVisible();

  const c2 = await expand(page, "Garden photo");
  await c2.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Cover…" }).click();
  const dialog = page.getByRole("dialog", { name: "Cover" });
  await dialog.getByRole("radio", { name: "sky" }).click();
  await expect(card(page, "Garden photo").locator(".card-cover.label-sky")).toBeVisible();

  await c2.getByRole("button", { name: "More" }).click();
  await page.getByRole("menuitem", { name: "Cover…" }).click();
  await dialog.getByRole("button", { name: "Remove cover" }).click();
  await expect(card(page, "Garden photo").locator(".card-cover")).toHaveCount(0);
});

test("add a link; unsafe addresses are refused", async ({ page }) => {
  const c = await expandedCard(page, "Reading");
  const a = attachmentsOf(c);
  await a.getByRole("button", { name: "Add a link" }).click();
  await a.getByLabel("Link address").fill("javascript:alert(1)");
  await a.getByRole("button", { name: "Add link" }).click();
  await expect(a.getByRole("alert")).toContainText("http:// or https://");

  await a.getByLabel("Link address").fill("example.com/seeds");
  await a.getByLabel("Link name (optional)").fill("Seed list");
  await a.getByRole("button", { name: "Add link" }).click();
  const link = a.getByRole("link", { name: "Seed list" });
  await expect(link).toHaveAttribute("href", "https://example.com/seeds");
  await expect(link).toHaveAttribute("rel", /noopener/);
});

test("remove an attachment and undo it", async ({ page }) => {
  const c = await expandedCard(page, "Undo me");
  const done = uploaded(page);
  await attachmentsOf(c).locator("input[type=file]").setInputFiles({ name: "notes.txt", mimeType: "text/plain", buffer: Buffer.from("x") });
  await done;
  await attachmentsOf(c).getByRole("button", { name: "Remove notes.txt" }).click();
  await expect(attachmentsOf(c).getByRole("link", { name: "notes.txt" })).toHaveCount(0);
  await page.getByRole("status", { name: "Notifications" }).getByRole("button", { name: "Undo" }).click();
  await expect(attachmentsOf(c).getByRole("link", { name: "notes.txt" })).toBeVisible();
});

test("a file over the upload limit gets a calm message", async ({ page }) => {
  const c = await expandedCard(page, "Too big");
  await attachmentsOf(c)
    .locator("input[type=file]")
    .setInputFiles({ name: "huge.bin", mimeType: "application/octet-stream", buffer: Buffer.alloc(2 * 1024 * 1024 + 1) });
  await expect(page.getByRole("status", { name: "Notifications" })).toContainText("too big to attach (the limit is 2 MB)");
});

test("insert an image into a description; it's stored as an attachment", async ({ page }) => {
  const c = await expandedCard(page, "Illustrated");
  const editor = page.getByRole("textbox", { name: "Description for Illustrated" }).first();
  await editor.click();
  await page.keyboard.type("Here's the bed: ");
  // Listen before acting: autosave can fire before slower checks finish.
  const savedWithImage = page.waitForRequest(
    (r) => r.url().endsWith("/description") && r.method() === "PUT" && /"src":"attachment:/.test(r.postData() ?? ""),
  );
  const done = uploaded(page);
  await c.locator(".toolbar input[type=file]").setInputFiles({ name: "inline.png", mimeType: "image/png", buffer: PNG });
  await done;
  const img = editor.locator("img");
  await expect(img).toHaveAttribute("src", /^\/api\/v1\/attachments\/[0-9A-Z]{26}$/);
  await expect(attachmentsOf(c).getByRole("img", { name: "inline.png" })).toBeVisible();

  const body = (await savedWithImage).postData() ?? "";
  expect(body).toMatch(/"src":"attachment:[0-9A-Z]{26}"/); // stored by ID, not URL
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Description for Illustrated" }).first().locator("img")).toBeVisible();
});

test("images from other sites are never loaded in a description", async ({ page }) => {
  test.skip(test.info().project.name !== "chromium", "synthetic paste events");
  const external: string[] = [];
  page.on("request", (r) => !r.url().startsWith("http://localhost") && external.push(r.url()));
  const c = await expandedCard(page, "Pasted");
  const editor = page.getByRole("textbox", { name: "Description for Pasted" }).first();
  await editor.click();
  await editor.evaluate((el) => {
    const dt = new DataTransfer();
    dt.setData("text/html", '<p>hello <img src="https://tracker.example/pixel.png"></p>');
    dt.setData("text/plain", "hello");
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
  });
  await expect(editor).toContainText("hello");
  await expect(editor.locator("img[src]")).toHaveCount(0);
  await page.waitForTimeout(300);
  expect(external).toEqual([]);
  void c;
});

test("no serious accessibility issues with attachments and a cover", async ({ page }) => {
  const c = await expandedCard(page, "Accessible");
  const done = uploaded(page);
  await attachmentsOf(c).locator("input[type=file]").setInputFiles([
    { name: "photo.png", mimeType: "image/png", buffer: PNG },
    { name: "list.txt", mimeType: "text/plain", buffer: Buffer.from("x") },
  ]);
  await done;
  await expect(attachmentsOf(c).getByRole("link", { name: "list.txt" })).toBeVisible();
  await attachmentsOf(c).getByRole("button", { name: "Make cover" }).click();
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    await expectAccessible(page, `attachments (${scheme})`);
  }
});
