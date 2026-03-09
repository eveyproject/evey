import { test as base, expect } from "@playwright/test";
import { test as authedTest } from "./fixtures/auth";

const USER = process.env.EVEY_USER ?? "gravity";
const PASS = process.env.EVEY_PASS ?? "REDACTED";

base.describe("Auth — login", () => {
  base.beforeEach(async ({ page }) => {
    await page.goto("/login", { waitUntil: "networkidle" });
  });

  base("valid credentials redirect to /home", async ({ page }) => {
    await page.waitForSelector("#username", { state: "visible" });
    await page.fill("#username", USER);
    await page.fill("#password", PASS);
    await page.click('[data-testid="login-submit"]');
    await page.waitForURL("**/home", { timeout: 15_000 });
    expect(page.url()).toContain("/home");
  });

  base("wrong password shows error", async ({ page }) => {
    await page.fill("#username", USER);
    await page.fill("#password", "wrongpassword123");
    await page.click('[data-testid="login-submit"]');

    const error = page.locator('[data-testid="login-error"]');
    await expect(error).toBeVisible({ timeout: 10_000 });
    const text = await error.textContent();
    expect(text!.length).toBeGreaterThan(0);
  });

  base("empty fields shows validation / browser prevents submit", async ({ page }) => {
    // HTML required attr prevents submit — the form should stay on /login
    await page.click('[data-testid="login-submit"]');
    expect(page.url()).toContain("/login");

    // username input should have required attribute
    const usernameRequired = await page.locator("#username").getAttribute("required");
    expect(usernameRequired).not.toBeNull();
  });
});

base.describe("Auth — protected routes", () => {
  base("unauthenticated access to /home redirects to /login", async ({ page }) => {
    await page.goto("/home");
    await page.waitForURL("**/login", { timeout: 15_000 });
    expect(page.url()).toContain("/login");
  });

  base("unauthenticated access to /library redirects to /login", async ({ page }) => {
    await page.goto("/library");
    await page.waitForURL("**/login", { timeout: 15_000 });
    expect(page.url()).toContain("/login");
  });
});

authedTest.describe("Auth — logout", () => {
  authedTest("sign out redirects to /login", async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });

    // Open user menu
    const userMenuBtn = page.locator('button[aria-label="User menu"]');
    await expect(userMenuBtn).toBeVisible({ timeout: 10_000 });
    await userMenuBtn.click();

    const logoutBtn = page.locator('[data-testid="logout-btn"]');
    await expect(logoutBtn).toBeVisible({ timeout: 3_000 });
    await logoutBtn.click();

    await page.waitForURL("**/login", { timeout: 15_000 });
    expect(page.url()).toContain("/login");
  });
});

authedTest.describe("Auth — session persistence", () => {
  authedTest("page refresh keeps user logged in", async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });
    expect(page.url()).toContain("/home");

    await page.reload({ waitUntil: "networkidle" });
    // Should still be on /home, not redirected to /login
    expect(page.url()).toContain("/home");

    // User menu should still be visible
    const userMenuBtn = page.locator('button[aria-label="User menu"]');
    await expect(userMenuBtn).toBeVisible({ timeout: 10_000 });
  });
});
