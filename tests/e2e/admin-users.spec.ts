import { test, expect } from "./fixtures/auth";

test.describe("Admin users", () => {
  test("loads with users heading and create button", async ({ page }) => {
    await page.goto("/admin/users", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Users" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Create User" }),
    ).toBeVisible();
  });

  test("shows users table with column headers", async ({ page }) => {
    await page.goto("/admin/users", { waitUntil: "networkidle" });

    for (const h of ["Username", "Email", "Role", "Status", "Actions"]) {
      await expect(
        page.locator("th", { hasText: h }).first(),
      ).toBeVisible({ timeout: 5_000 });
    }
  });

  test("shows invite links section", async ({ page }) => {
    await page.goto("/admin/users", { waitUntil: "networkidle" });

    await expect(
      page.locator("h2", { hasText: "Invite Links" }),
    ).toBeVisible({ timeout: 10_000 });

    await expect(
      page.locator("button", { hasText: "Create Invite Link" }),
    ).toBeVisible();
  });

  test("create user modal opens", async ({ page }) => {
    await page.goto("/admin/users", { waitUntil: "networkidle" });

    await page.click("button:has-text('Create User')");

    await expect(page.locator("label", { hasText: "Username" })).toBeVisible({ timeout: 5_000 });
    await expect(page.locator("label", { hasText: "Password" })).toBeVisible();
  });
});
