import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Profile > Theme settings", () => {
  test("settings page loads with tabs", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("theme selector is visible", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    const themeBtn = page.locator("button", { hasText: /dark|light|amoled/i }).first();
    await expect(themeBtn).toBeVisible({ timeout: 5_000 });
  });

  test("clicking theme button changes active state", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    const lightBtn = page.locator("button", { hasText: /^light$/i }).first();
    test.skip(!(await lightBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No light theme button");
    await lightBtn.click();
    await page.waitForTimeout(500);
    // the button should now have accent styling
    const classes = await lightBtn.getAttribute("class");
    expect(classes).toContain("accent");
  });

  test("theme persists after reload", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    // click AMOLED
    const amoledBtn = page.locator("button", { hasText: /amoled/i }).first();
    test.skip(!(await amoledBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No AMOLED button");
    await amoledBtn.click();
    await page.waitForTimeout(500);
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForTimeout(1_500);
    // AMOLED button should still be active
    const classes = await amoledBtn.getAttribute("class");
    expect(classes).toContain("accent");
    // restore default
    const darkBtn = page.locator("button", { hasText: /^dark$/i }).first();
    if (await darkBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await darkBtn.click();
      await page.waitForTimeout(300);
    }
  });

  test("settings page has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile/settings");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("settings page has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile/settings");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});

test.describe("Profile > Password change", () => {
  test("password form is visible on account tab", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    // click Account tab
    const accountTab = page.locator("button, a", { hasText: /account/i }).first();
    if (await accountTab.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await accountTab.click();
      await page.waitForTimeout(500);
    }
    const pwdHeading = page.locator("text=/password/i").first();
    await expect(pwdHeading).toBeVisible({ timeout: 3_000 });
  });

  test("submit empty password form shows error or does nothing", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(1_000);
    const accountTab = page.locator("button, a", { hasText: /account/i }).first();
    if (await accountTab.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await accountTab.click();
      await page.waitForTimeout(500);
    }
    const submitBtn = page.locator("button", { hasText: /update.*password|change.*password/i }).first();
    test.skip(!(await submitBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No password submit button");
    const { jsErrors } = await collectErrors(page, async () => {
      await submitBtn.click();
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});

test.describe("Profile > Watch history", () => {
  test("watch history page loads", async ({ page }) => {
    await goto(page, "/profile/history");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("history shows items or empty state", async ({ page }) => {
    await goto(page, "/profile/history");
    await page.waitForTimeout(2_000);
    const hasItems = await page.locator('[data-testid="history-item"]').count();
    const hasEmpty = await page.locator('[data-testid="history-empty"]').count();
    expect(hasItems + hasEmpty).toBeGreaterThan(0);
  });

  test("no 5xx on watch history", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile/history");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Profile > Notifications page", () => {
  test("notifications page loads", async ({ page }) => {
    await goto(page, "/profile/notifications");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("mark all read button doesn't crash", async ({ page }) => {
    await goto(page, "/profile/notifications");
    await page.waitForTimeout(2_000);
    const markBtn = page.locator("button", { hasText: /mark.*read|tout.*lu/i }).first();
    if (await markBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const { jsErrors, serverErrors } = await collectErrors(page, async () => {
        await markBtn.click();
        await page.waitForTimeout(1_000);
      });
      expect(jsErrors).toHaveLength(0);
      expect(serverErrors).toHaveLength(0);
    }
  });

  test("no 5xx on notifications page", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/profile/notifications");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
