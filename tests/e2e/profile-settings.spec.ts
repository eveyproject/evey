import { test, expect } from "./fixtures/auth";

const TAB_KEYS = ["appearance", "notifications", "parental", "quick-connect"] as const;

test.describe("Profile settings", () => {
  test("loads with heading and tabs", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "domcontentloaded" });

    const heading = page.locator("h1").first();
    await expect(heading).toBeVisible({ timeout: 10_000 });
    expect((await heading.textContent())?.trim().length ?? 0).toBeGreaterThan(0);

    for (const key of TAB_KEYS) {
      await expect(page.locator(`[data-testid="profile-settings-tab-${key}"]`)).toBeVisible({ timeout: 5_000 });
    }
  });

  test("clicking each tab does not crash", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "domcontentloaded" });
    for (const key of TAB_KEYS) {
      const tab = page.locator(`[data-testid="profile-settings-tab-${key}"]`);
      await expect(tab).toBeVisible({ timeout: 5_000 });
      await tab.click();
      await page.waitForTimeout(300);
    }
  });
});
