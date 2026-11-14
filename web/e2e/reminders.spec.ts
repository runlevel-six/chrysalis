// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Reminders (§3.9): off until turned on, chosen per card, and delivered to
// the in-app list under the bell, gently, with snooze. These change the
// shared dev user's preferences, so they run in the "preferences" project.

import { expect, test } from "@playwright/test";

import { addCards, card, expectAccessible, newBoard } from "./helpers";

test.describe.configure({ mode: "serial" });
// Each test starts and ends with reminders off and no quiet hours.
const reset = async ({ page }: { page: import("@playwright/test").Page }) => {
  await page.request.patch("/api/v1/me/prefs", { data: { remindersEnabled: false, quietStart: "", quietEnd: "" } });
};
test.beforeEach(reset);
test.afterEach(async ({ page }) => {
  await reset({ page });
  await page.request.post("/api/v1/me/notifications/dismiss");
});

const pad = (n: number) => String(n).padStart(2, "0");
const tomorrow = () => {
  const d = new Date(Date.now() + 86_400_000);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

test("reminders are off until turned on in Settings", async ({ page }) => {
  await newBoard(page, ["To do"]);
  await addCards(page, "To do", ["Call the clinic"]);
  const c = card(page, "Call the clinic");
  await c.getByRole("button", { name: "Expand card" }).click();
  await c.getByRole("button", { name: "Dates…" }).click();
  await expect(page.getByRole("dialog", { name: "Dates" })).toContainText("turn on reminders in Settings");
  await expect(page.getByRole("button", { name: /^Notifications/ })).toHaveCount(0);
});

test("choose a reminder on a card; it arrives under the bell and can be snoozed", async ({ page }) => {
  test.setTimeout(90_000); // the server checks for due reminders every 30 seconds
  await page.goto("/settings");
  await page.getByLabel("Remind me about cards I choose").check();
  await expect(page.getByRole("button", { name: "Notifications" })).toBeVisible();

  await newBoard(page, ["To do"]);
  await addCards(page, "To do", ["Call the clinic"]);
  const c = card(page, "Call the clinic");
  await c.getByRole("button", { name: "Expand card" }).click();
  await c.getByRole("button", { name: "Dates…" }).click();
  let dialog = page.getByRole("dialog", { name: "Dates" });
  await dialog.getByLabel("Due date").fill(tomorrow());
  await dialog.getByLabel("Remind me", { exact: true }).selectOption({ label: "1 day before" });
  const saved = page.waitForResponse((r) => r.url().endsWith("/reminder") && r.request().method() === "PUT");
  await dialog.getByRole("button", { name: "Save" }).click();
  await saved;

  await c.getByRole("button", { name: "Dates…" }).click();
  dialog = page.getByRole("dialog", { name: "Dates" });
  await expect(dialog.getByLabel("Remind me", { exact: true })).toHaveValue("before:1440");
  await expect(dialog).toContainText("Next reminder:");
  await expectAccessible(page, "dates dialog with a reminder");
  await dialog.getByRole("button", { name: "Close" }).click();

  // Make it go off now (two seconds from now), through the API.
  const cardId = await c.getAttribute("data-card-id");
  const soon = new Date(Date.now() + 2000).toISOString();
  const res = await page.request.put(`/api/v1/cards/${cardId}/reminder`, { data: { kind: "at", at: soon, timeZone: "UTC" } });
  expect(res.ok()).toBe(true);

  const bell = page.getByRole("button", { name: "Notifications (new)" });
  await expect(bell).toBeVisible({ timeout: 45_000 });
  await expect(page.getByRole("status", { name: "Notifications" })).toContainText("Heads up: “Call the clinic” is due tomorrow");

  await bell.click();
  const panel = page.getByRole("dialog", { name: "Notifications" });
  await expect(panel).toContainText("Heads up: “Call the clinic” is due tomorrow");
  await expectAccessible(page, "notifications");
  await panel.getByRole("button", { name: "Snooze…" }).click();
  await panel.getByRole("button", { name: "In an hour" }).click();
  await expect(panel).toContainText("Nothing here.");
  await expect(page.getByRole("button", { name: "Notifications" })).toBeVisible(); // no longer "new"
});

test.describe("with a service worker", () => {
  test.use({ serviceWorkers: "allow" });

test("Settings offers reminders on this device and quiet hours", async ({ page, context }) => {
  await context.grantPermissions(["notifications"]);
  await page.goto("/settings");
  await page.getByLabel("Remind me about cards I choose").check();
  await expect(page.getByRole("heading", { name: "On this device" })).toBeVisible();
  // Never asks for notification permission by itself; only this button does.
  // (Headless browsers report notifications as blocked whatever is granted,
  // so either the button or the calm "blocked" explanation is right here.)
  await expect(page.locator(".push-settings")).toContainText(/Send reminders to this device|Notifications are blocked/);

  const saved = page.waitForResponse((r) => r.url().endsWith("/me/prefs") && r.request().method() === "PATCH");
  await page.getByLabel("Quiet hours: no notifications on devices").check();
  await saved;
  await expect(page.getByLabel("From")).toHaveValue("21:00");
  await expect(page.getByLabel("Until")).toHaveValue("07:00");
  await expectAccessible(page, "reminder settings");
  await page.reload();
  await expect(page.getByLabel("Until")).toHaveValue("07:00");
});
});
