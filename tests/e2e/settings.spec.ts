import { test, expect } from "./fixtures/auth";

test.describe("Settings / Profile", () => {
  test("profile page loads and shows username", async ({ page }) => {
    await page.goto("/profile", { waitUntil: "networkidle" });

    const heading = page.getByRole("heading", { name: "Profile" });
    await expect(heading).toBeVisible({ timeout: 10_000 });

    const username = page.getByText("gravity");
    await expect(username).toBeVisible({ timeout: 5_000 });
  });

  test("profile page shows navigation cards", async ({ page }) => {
    await page.goto("/profile", { waitUntil: "networkidle" });

    const settingsLink = page.getByText("Settings", { exact: true });
    await expect(settingsLink).toBeVisible({ timeout: 10_000 });

    const historyLink = page.getByText("Watch History", { exact: true });
    await expect(historyLink).toBeVisible();

    const notifsLink = page.getByText("Notifications", { exact: true }).first();
    await expect(notifsLink).toBeVisible();
  });

  test("settings page loads with tabs", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    const heading = page.getByRole("heading", { name: "Settings" });
    await expect(heading).toBeVisible({ timeout: 10_000 });

    // Scope to main to avoid TopBar notification bell ambiguity
    const main = page.getByRole("main");
    const tabs = ["Appearance", "Notifications", "Account"];
    for (const tab of tabs) {
      const tabBtn = main.getByRole("button", { name: tab });
      await expect(tabBtn).toBeVisible();
    }
  });

  test("theme selector is visible on appearance tab", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    // Appearance tab is active by default
    const themeLabel = page.getByText("Theme", { exact: true });
    await expect(themeLabel).toBeVisible({ timeout: 10_000 });

    const themes = ["Dark", "AMOLED", "Light", "Light Blue"];
    for (const t of themes) {
      const btn = page.getByRole("button", { name: t, exact: true });
      await expect(btn).toBeVisible();
    }
  });

  test("changing theme updates background", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    await page.waitForSelector('button:has-text("Light")', { timeout: 10_000 });

    const bgBefore = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--bg").trim(),
    );

    // Click "Light" theme
    const lightBtn = page.getByRole("button", { name: "Light", exact: true });
    await lightBtn.click();
    // Wait for theme CSS var to update
    await page.waitForFunction(
      () => getComputedStyle(document.documentElement).getPropertyValue("--bg").trim() !== "",
    ).catch(() => {});

    const bgAfter = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--bg").trim(),
    );

    expect(bgAfter).not.toBe(bgBefore);

    // Restore dark theme
    const darkBtn = page.getByRole("button", { name: "Dark", exact: true });
    await darkBtn.click();
  });

  test("account tab shows password change form", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    const main = page.getByRole("main");
    const accountTab = main.getByRole("button", { name: "Account" });
    await expect(accountTab).toBeVisible({ timeout: 10_000 });
    await accountTab.click();

    const changePassword = page.getByText("Change Password");
    await expect(changePassword).toBeVisible({ timeout: 5_000 });

    const updateBtn = page.getByRole("button", { name: "Update password" });
    await expect(updateBtn).toBeVisible();
  });
});
