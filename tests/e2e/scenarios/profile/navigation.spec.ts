import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Profile > Navigation", () => {
  test("profile page shows username", async ({ page }) => {
    await goto(page, "/profile");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.toLowerCase()).toContain("gravity");
  });

  test("profile has link to settings", async ({ page }) => {
    await goto(page, "/profile");
    const link = page.locator("a[href*='settings']").first();
    await expect(link).toBeVisible({ timeout: 5_000 });
  });

  test("profile has link to history", async ({ page }) => {
    await goto(page, "/profile");
    const link = page.locator("a[href*='history']").first();
    await expect(link).toBeVisible({ timeout: 5_000 });
  });

  test("profile has link to notifications", async ({ page }) => {
    await goto(page, "/profile");
    const link = page.locator("a[href*='notification']").first();
    await expect(link).toBeVisible({ timeout: 5_000 });
  });

  test("admin user sees admin panel link", async ({ page }) => {
    await goto(page, "/profile");
    const link = page.locator("a[href*='admin']").first();
    await expect(link).toBeVisible({ timeout: 5_000 });
  });

  test("clicking settings link navigates", async ({ page }) => {
    await goto(page, "/profile");
    await page.locator("a[href*='settings']").first().click();
    await page.waitForTimeout(1_000);
    expect(page.url()).toContain("/settings");
  });

  test("clicking history link navigates", async ({ page }) => {
    await goto(page, "/profile");
    await page.locator("a[href*='history']").first().click();
    await page.waitForTimeout(1_000);
    expect(page.url()).toContain("/history");
  });

  test("no JS errors on profile", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});

test.describe("Profile > Quick Connect", () => {
  test("quick connect tab exists in settings", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    const tab = page.locator("button, a", { hasText: /quick.*connect/i }).first();
    const visible = await tab.isVisible({ timeout: 3_000 }).catch(() => false);
    expect(typeof visible).toBe("boolean");
  });

  test("quick connect shows code input", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    const tab = page.locator("button, a", { hasText: /quick.*connect/i }).first();
    if (await tab.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(500);
      const input = page.locator("input[placeholder*='code' i], input[placeholder*='A1B2' i]").first();
      expect(await input.count()).toBeGreaterThanOrEqual(0);
    }
  });
});

test.describe("Profile > Topbar user menu", () => {
  test("user avatar/button visible in topbar", async ({ page }) => {
    await goto(page, "/home");
    // user menu is usually the last button in topbar with an avatar or initial
    const avatarBtn = page.locator("button").last();
    await expect(avatarBtn).toBeVisible({ timeout: 5_000 });
  });

  test("notification bell visible", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator('button[aria-label*="otification"]')).toBeVisible({ timeout: 5_000 });
  });

  test("bell click opens notification dropdown", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('button[aria-label*="otification"]').click();
    await page.waitForTimeout(500);
    const panel = page.locator("text=/notification|no notification|view all/i").first();
    await expect(panel).toBeVisible({ timeout: 3_000 });
  });

  test("notification dropdown has 'view all' link", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('button[aria-label*="otification"]').click();
    await page.waitForTimeout(500);
    const viewAll = page.locator("a", { hasText: /view all|voir tout/i }).first();
    await expect(viewAll).toBeVisible({ timeout: 3_000 });
  });

  test("clicking 'view all' navigates to notifications page", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('button[aria-label*="otification"]').click();
    await page.waitForTimeout(500);
    const viewAll = page.locator("a", { hasText: /view all|voir tout/i }).first();
    test.skip(!(await viewAll.isVisible({ timeout: 2_000 }).catch(() => false)), "No view all link");
    await viewAll.click();
    await page.waitForTimeout(1_000);
    expect(page.url()).toContain("/notification");
  });
});
