import { test as base, expect } from "@playwright/test";
import { test, expect as authExpect } from "./fixtures/auth";

const USER = process.env.EVEY_USER ?? "gravity";
const PASS = process.env.EVEY_PASS ?? "REDACTED";
const MOVIE_ID = process.env.MEDIA_ID ?? "577d3d4d-da77-4c8a-8e46-6bee6b64635c";

const PAGES = [
  "/home", "/library", "/favorites", "/music", "/photos",
  "/collections", "/playlists", "/profile", "/profile/settings",
  "/search", "/admin",
];

// ── 1. Rapid navigation ────────────────────────────────────────────

test.describe("Monkey — rapid navigation", () => {
  test("blitz through 12 pages without waiting for full load", async ({ page }) => {
    const jsErrors: string[] = [];
    const serverErrors: string[] = [];

    page.on("pageerror", (e) => jsErrors.push(e.message));
    page.on("response", (r) => {
      if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`);
    });

    for (const route of PAGES) {
      try {
        await page.goto(route, { waitUntil: "domcontentloaded", timeout: 8_000 });
      } catch {}
      // don't wait for networkidle — stress test
      await page.waitForTimeout(200);

      // page must not be blank
      const len = await page.evaluate(() => document.body.innerText.length);
      expect(len, `blank page on ${route}`).toBeGreaterThan(5);
    }

    expect(jsErrors, `JS errors: ${jsErrors.join("; ")}`).toHaveLength(0);
    expect(serverErrors, `5xx errors: ${serverErrors.join("; ")}`).toHaveLength(0);
  });
});

// ── 2. Search injection ────────────────────────────────────────────

const INJECTION_INPUTS = [
  { label: "SQL injection", value: "'; DROP TABLE users; --" },
  { label: "XSS script tag", value: "<script>alert(1)</script>" },
  { label: "emoji", value: "🎬🍿🎥" },
  { label: "empty string", value: "" },
  { label: "500 chars", value: "A".repeat(500) },
  { label: "path traversal", value: "../../../etc/passwd" },
];

test.describe("Monkey — search injection", () => {
  for (const { label, value } of INJECTION_INPUTS) {
    test(`search handles ${label}`, async ({ page }) => {
      const jsErrors: string[] = [];
      const serverErrors: string[] = [];

      page.on("pageerror", (e) => jsErrors.push(e.message));
      page.on("response", (r) => {
        if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`);
      });

      await page.goto("/home", { waitUntil: "networkidle" });
      const input = page.locator('[data-testid="topbar-search-input"]');
      await expect(input).toBeVisible({ timeout: 10_000 });
      await input.click();
      await input.fill(value);

      // Wait for any dropdown/response
      await page.waitForTimeout(1_000);

      // XSS: script must never execute — check no alert dialog appeared
      // Playwright auto-dismisses dialogs, but we can listen
      let dialogFired = false;
      page.on("dialog", async (d) => {
        dialogFired = true;
        await d.dismiss();
      });

      // Injected content must not render as raw HTML
      const bodyHtml = await page.evaluate(() => document.body.innerHTML);
      expect(bodyHtml).not.toContain("<script>alert(1)</script>");

      expect(dialogFired, "XSS dialog must not fire").toBe(false);
      expect(jsErrors, `JS errors on "${label}": ${jsErrors.join("; ")}`).toHaveLength(0);
      expect(serverErrors, `5xx on "${label}": ${serverErrors.join("; ")}`).toHaveLength(0);
    });
  }
});

// ── 3. Login/logout cycle ──────────────────────────────────────────

base.describe("Monkey — login/logout cycle", () => {
  base("login → home → logout × 3", async ({ page }) => {
    for (let i = 0; i < 3; i++) {
      // Login
      await page.goto("/login", { waitUntil: "networkidle" });
      await page.waitForSelector("#username", { state: "visible", timeout: 10_000 });
      await page.fill("#username", USER);
      await page.fill("#password", PASS);
      await page.click('[data-testid="login-submit"]');
      await page.waitForURL("**/home", { timeout: 15_000 });
      expect(page.url(), `cycle ${i + 1}: not on /home after login`).toContain("/home");

      // Verify content loaded
      const bodyLen = await page.evaluate(() => document.body.innerText.length);
      expect(bodyLen).toBeGreaterThan(10);

      // Logout
      const userMenuBtn = page.locator('button[aria-label="User menu"]');
      await expect(userMenuBtn).toBeVisible({ timeout: 10_000 });
      await userMenuBtn.click();

      const logoutBtn = page.locator('[data-testid="logout-btn"]');
      await expect(logoutBtn).toBeVisible({ timeout: 3_000 });
      await logoutBtn.click();

      await page.waitForURL("**/login", { timeout: 15_000 });
      expect(page.url(), `cycle ${i + 1}: not on /login after logout`).toContain("/login");
    }
  });
});

// ── 4. Favorites spam ──────────────────────────────────────────────

test.describe("Monkey — favorites spam", () => {
  test("toggle favorite 10 times rapidly", async ({ page }) => {
    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="media-actions"]', { timeout: 15_000 });

    const actions = page.locator('[data-testid="media-actions"]');
    const anyFavBtn = actions.locator(
      '[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]',
    );
    await expect(anyFavBtn).toBeVisible({ timeout: 10_000 });

    // Record initial state
    const wasActive = await actions
      .locator('[data-testid="favorite-btn-active"]')
      .isVisible()
      .catch(() => false);

    const serverErrors: string[] = [];
    page.on("response", (r) => {
      if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`);
    });

    // Rapid toggle 10 times
    for (let i = 0; i < 10; i++) {
      const btn = actions.locator(
        '[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]',
      );
      try {
        await btn.click({ timeout: 3_000 });
      } catch {}
      // minimal wait to let UI react
      await page.waitForTimeout(300);
    }

    // Wait for all API calls to settle
    await page.waitForTimeout(2_000);

    expect(serverErrors, `5xx during favorites spam: ${serverErrors.join("; ")}`).toHaveLength(0);

    // Final state: either favorite-btn or favorite-btn-active must be visible (consistent)
    const finalActive = actions.locator('[data-testid="favorite-btn-active"]');
    const finalInactive = actions.locator('[data-testid="favorite-btn"]');
    const hasActive = await finalActive.isVisible().catch(() => false);
    const hasInactive = await finalInactive.isVisible().catch(() => false);
    expect(hasActive || hasInactive, "favorite button must still be rendered").toBe(true);

    // 10 toggles = even count → should return to original state
    if (wasActive) {
      expect(hasActive, "should return to active after even toggles").toBe(true);
    } else {
      expect(hasInactive, "should return to inactive after even toggles").toBe(true);
    }
  });
});

// ── 5. Theme switching ─────────────────────────────────────────────

test.describe("Monkey — theme switching", () => {
  test("cycle all themes 3 times", async ({ page }) => {
    await page.goto("/profile/settings", { waitUntil: "networkidle" });

    const themes = ["Dark", "AMOLED", "Light", "Light Blue"];

    // Wait for theme buttons to render
    await expect(
      page.getByRole("button", { name: themes[0], exact: true }),
    ).toBeVisible({ timeout: 10_000 });

    const jsErrors: string[] = [];
    page.on("pageerror", (e) => jsErrors.push(e.message));

    for (let cycle = 0; cycle < 3; cycle++) {
      for (const theme of themes) {
        const btn = page.getByRole("button", { name: theme, exact: true });
        await btn.click();
        await page.waitForTimeout(200);
      }
    }

    // After cycling, CSS vars must still be defined
    const bg = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--bg").trim(),
    );
    expect(bg.length, "CSS --bg must be set").toBeGreaterThan(0);

    // Page must not be blank
    const bodyLen = await page.evaluate(() => document.body.innerText.length);
    expect(bodyLen).toBeGreaterThan(10);

    expect(jsErrors, `JS errors during theme cycling: ${jsErrors.join("; ")}`).toHaveLength(0);

    // Restore dark theme
    await page.getByRole("button", { name: "Dark", exact: true }).click();
  });
});

// ── 6. Profile switching ───────────────────────────────────────────

test.describe("Monkey — profile switching", () => {
  test("switch profiles if multiple exist", async ({ page }) => {
    await page.goto("/home", { waitUntil: "networkidle" });

    // Open user menu to check for profile switcher
    const userMenuBtn = page.locator('button[aria-label="User menu"]');
    await expect(userMenuBtn).toBeVisible({ timeout: 10_000 });
    await userMenuBtn.click();

    // Look for profile entries in the menu
    const profileItems = page.locator('[data-testid="profile-switch-item"]');
    const profileCount = await profileItems.count().catch(() => 0);

    if (profileCount < 2) {
      // Only one profile — skip with a soft pass
      test.skip();
      return;
    }

    const jsErrors: string[] = [];
    page.on("pageerror", (e) => jsErrors.push(e.message));

    // Switch between profiles
    for (let i = 0; i < Math.min(profileCount, 4); i++) {
      // Re-open menu (it may close after selection)
      const menuBtn = page.locator('button[aria-label="User menu"]');
      if (await menuBtn.isVisible().catch(() => false)) {
        await menuBtn.click();
      }

      const items = page.locator('[data-testid="profile-switch-item"]');
      const count = await items.count();
      if (count === 0) break;

      const idx = i % count;
      await items.nth(idx).click();
      await page.waitForTimeout(1_000);

      // Page should still render content
      const bodyLen = await page.evaluate(() => document.body.innerText.length);
      expect(bodyLen, `blank page after profile switch ${i}`).toBeGreaterThan(10);
    }

    expect(jsErrors, `JS errors during profile switch: ${jsErrors.join("; ")}`).toHaveLength(0);
  });
});
