import { test, expect } from "./fixtures/auth";

/**
 * Random walk E2E fuzzer.
 * Each test starts from a page and does N random actions.
 * Asserts: no JS errors, no 5xx, no white screen.
 */

const ACTIONS_PER_WALK = 8;
const WALKS = 20;

const ENTRY_POINTS = [
  "/home", "/library", "/favorites", "/music", "/photos",
  "/collections", "/playlists", "/profile", "/profile/settings",
  "/admin", "/admin/libraries", "/admin/users", "/admin/feeds",
];

type ActionFn = (page: import("@playwright/test").Page) => Promise<void>;

const actions: ActionFn[] = [
  // Click a random link
  async (page) => {
    const links = page.locator("a[href]:visible");
    const count = await links.count();
    if (count === 0) return;
    const idx = Math.floor(Math.random() * Math.min(count, 20));
    try { await links.nth(idx).click({ timeout: 2_000 }); } catch {}
    await page.waitForTimeout(500);
  },

  // Click a random visible button
  async (page) => {
    const btns = page.locator("button:visible");
    const count = await btns.count();
    if (count === 0) return;
    const idx = Math.floor(Math.random() * Math.min(count, 15));
    try { await btns.nth(idx).click({ timeout: 2_000 }); } catch {}
    await page.waitForTimeout(500);
  },

  // Type in search
  async (page) => {
    const input = page.locator('[data-testid="topbar-search-input"]');
    if (await input.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await input.fill("test");
      await page.waitForTimeout(500);
      await page.keyboard.press("Escape");
    }
  },

  // Go back
  async (page) => {
    try { await page.goBack({ timeout: 2_000 }); } catch {}
    await page.waitForTimeout(300);
  },

  // Scroll down
  async (page) => {
    await page.evaluate(() => window.scrollBy(0, 300));
    await page.waitForTimeout(200);
  },

  // Press Escape (close any modal)
  async (page) => {
    await page.keyboard.press("Escape");
    await page.waitForTimeout(200);
  },

  // Right-click a card (context menu)
  async (page) => {
    const card = page.locator('[data-testid="media-card"]').first();
    if (await card.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await card.click({ button: "right" });
      await page.waitForTimeout(300);
      await page.keyboard.press("Escape");
    }
  },

  // Navigate to a random entry point
  async (page) => {
    const target = ENTRY_POINTS[Math.floor(Math.random() * ENTRY_POINTS.length)];
    try { await page.goto(target, { waitUntil: "domcontentloaded", timeout: 5_000 }); } catch {}
    await page.waitForTimeout(500);
  },
];

for (let i = 0; i < WALKS; i++) {
  test(`random walk #${i + 1}`, async ({ page }) => {
    const jsErrors: string[] = [];
    const serverErrors: string[] = [];

    page.on("pageerror", (e) => jsErrors.push(e.message));
    page.on("response", (r) => {
      if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`);
    });

    // Start from a random entry point
    const start = ENTRY_POINTS[Math.floor(Math.random() * ENTRY_POINTS.length)];
    await page.goto(start, { waitUntil: "domcontentloaded", timeout: 10_000 });
    await page.waitForTimeout(500);

    // Do N random actions
    for (let a = 0; a < ACTIONS_PER_WALK; a++) {
      const action = actions[Math.floor(Math.random() * actions.length)];
      try { await action(page); } catch {}
    }

    // Final assertions
    // 1. No JS errors
    expect(jsErrors, `JS errors during walk: ${jsErrors.join(", ")}`).toHaveLength(0);
    // 2. No 5xx
    expect(serverErrors, `Server 5xx during walk: ${serverErrors.join(", ")}`).toHaveLength(0);
    // 3. Page not blank (at least some content rendered)
    const bodyText = await page.evaluate(() => document.body.innerText.length);
    expect(bodyText).toBeGreaterThan(10);
  });
}
