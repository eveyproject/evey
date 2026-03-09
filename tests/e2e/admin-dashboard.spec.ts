import { test, expect } from "./fixtures/auth";

test.describe("Admin Dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/admin", { waitUntil: "networkidle" });
  });

  test("page loads without crash", async ({ page }) => {
    const heading = page.getByRole("heading", { name: "Admin" });
    await expect(heading).toBeVisible({ timeout: 10_000 });
  });

  test("stat cards are displayed", async ({ page }) => {
    const labels = ["Movies", "Series", "Users", "Active Streams", "Total Media"];
    for (const label of labels) {
      const card = page.locator("span.uppercase", { hasText: label });
      await expect(card).toBeVisible({ timeout: 10_000 });
    }
  });

  test("quick actions are visible", async ({ page }) => {
    const main = page.getByRole("main");
    const scanAll = main.getByRole("button", { name: /Scan All Libraries/ });
    await expect(scanAll).toBeVisible({ timeout: 10_000 });

    const manageLibs = main.getByRole("link", { name: /Manage Libraries/ });
    await expect(manageLibs).toBeVisible();

    const manageUsers = main.getByRole("link", { name: /Manage Users/ });
    await expect(manageUsers).toBeVisible();
  });

  test("system info section is visible", async ({ page }) => {
    const sysInfo = page.getByText("System Info");
    await expect(sysInfo).toBeVisible({ timeout: 10_000 });
  });

  test("admin nav links are visible", async ({ page }) => {
    // Scope to main to avoid sidebar nav ambiguity
    const main = page.getByRole("main");
    const navLinks = ["Plugins", "Webhooks", "Audit Log", "Metadata", "Activity", "Scheduled Tasks", "Logs"];
    for (const label of navLinks) {
      const link = main.getByRole("link", { name: label, exact: true });
      await expect(link).toBeVisible({ timeout: 5_000 });
    }
  });

  test("navigation to libraries sub-page works", async ({ page }) => {
    const main = page.getByRole("main");
    const libLink = main.locator('a[href="/admin/libraries"]');
    await expect(libLink.first()).toBeVisible({ timeout: 10_000 });
    await libLink.first().click();

    await page.waitForURL("**/admin/libraries", { timeout: 10_000 });
    const heading = page.getByRole("heading", { name: "Libraries" });
    await expect(heading).toBeVisible({ timeout: 10_000 });
  });

  test("navigation to users sub-page works", async ({ page }) => {
    const main = page.getByRole("main");
    const usersLink = main.locator('a[href="/admin/users"]');
    await expect(usersLink.first()).toBeVisible({ timeout: 10_000 });
    await usersLink.first().click();

    await page.waitForURL("**/admin/users", { timeout: 10_000 });
  });
});
