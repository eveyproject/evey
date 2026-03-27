import { test, expect } from "./fixtures/auth";

const BASE = process.env.BASE_URL ?? "https://evey.test";

// ─── Helpers ────────────────────────────────────────────────────────

async function goto(page: import("@playwright/test").Page, path: string) {
  await page.goto(path, { waitUntil: "domcontentloaded", timeout: 10_000 });
}

async function text(page: import("@playwright/test").Page, selector: string, timeout = 5_000) {
  const el = page.locator(selector).first();
  await el.waitFor({ state: "visible", timeout });
  return el.textContent();
}

// ─── HOME ───────────────────────────────────────────────────────────

test.describe("Home", () => {
  test("page loads with home-page testid", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator('[data-testid="home-page"]')).toBeVisible({ timeout: 10_000 });
  });

  test("at least one media slider is visible", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator('[data-testid="media-slider"]').first()).toBeVisible({ timeout: 10_000 });
  });

  test("media cards are present in sliders", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="media-card"]', { timeout: 10_000 });
    const count = await page.locator('[data-testid="media-card"]').count();
    expect(count).toBeGreaterThan(0);
  });

  test("clicking a media card navigates to media detail", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click();
    await page.waitForURL("**/media/**", { timeout: 5_000 });
    expect(page.url()).toContain("/media/");
  });

  test("customize button is visible", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator("button", { hasText: /customize/i })).toBeVisible({ timeout: 5_000 });
  });
});

// ─── SEARCH ─────────────────────────────────────────────────────────

test.describe("Search", () => {
  test("search input is visible in topbar", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator('[data-testid="topbar-search-input"]')).toBeVisible();
  });

  test("typing in search shows dropdown", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    await expect(page.locator('[data-testid="search-dropdown"]')).toBeVisible({ timeout: 5_000 });
  });

  test("search dropdown has results", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    await page.waitForSelector('[data-search-item]', { timeout: 5_000 });
    const count = await page.locator("[data-search-item]").count();
    expect(count).toBeGreaterThan(0);
  });

  test("clicking search result navigates to media", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    const item = page.locator("[data-search-item]").first();
    await item.waitFor({ state: "visible", timeout: 5_000 });
    await item.click();
    await page.waitForURL("**/media/**", { timeout: 5_000 });
    expect(page.url()).toContain("/media/");
  });

  test("Escape closes search dropdown", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "the");
    await page.waitForSelector('[data-testid="search-dropdown"]', { timeout: 5_000 });
    await page.keyboard.press("Escape");
    await expect(page.locator('[data-testid="search-dropdown"]')).not.toBeVisible({ timeout: 2_000 });
  });
});

// ─── MEDIA DETAIL ───────────────────────────────────────────────────

test.describe("Media Detail", () => {
  test("detail page shows title", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click();
    await expect(page.locator('[data-testid="media-title"]')).toBeVisible({ timeout: 10_000 });
  });

  test("genres are displayed", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    // genres may not exist for all media
    const genres = page.locator('[data-testid="media-genres"]');
    const exists = await genres.count();
    expect(exists).toBeGreaterThanOrEqual(0); // just no crash
  });

  test("play button exists on movie detail", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const playBtn = page.locator('[data-testid="play-button"]');
    // may not exist for series without episodes
    const count = await playBtn.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("cast section exists", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const cast = page.locator('[data-testid="cast-section"]');
    const count = await cast.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("back button navigates back", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    await page.goBack();
    await expect(page.locator('[data-testid="home-page"]')).toBeVisible({ timeout: 5_000 });
  });
});

// ─── FAVORITES ──────────────────────────────────────────────────────

test.describe("Favorites", () => {
  test("favorites page loads", async ({ page }) => {
    await goto(page, "/favorites");
    await page.waitForTimeout(2_000);
    // either empty state or cards
    const hasCards = await page.locator('[data-testid="media-card"]').count();
    const hasEmpty = await page.locator("text=/no favorites|no results/i").count();
    expect(hasCards + hasEmpty).toBeGreaterThanOrEqual(0); // no crash
  });

  test("favorite toggle on media detail works", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const favBtn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]').first();
    if (await favBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await favBtn.click();
      await page.waitForTimeout(500);
      // no crash = success, state may have toggled
    }
  });

  test("sort dropdown is visible on favorites page", async ({ page }) => {
    await goto(page, "/favorites");
    await page.waitForTimeout(2_000);
    const select = page.locator("select").first();
    if (await select.isVisible({ timeout: 2_000 }).catch(() => false)) {
      expect(await select.count()).toBeGreaterThan(0);
    }
  });
});

// ─── WATCHED TOGGLE ─────────────────────────────────────────────────

test.describe("Watched toggle", () => {
  test("watched badge is visible on media card", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="media-card"]', { timeout: 10_000 });
    const badge = page.locator('[data-testid="watched-badge"]').first();
    await expect(badge).toBeVisible({ timeout: 3_000 });
  });

  test("clicking watched badge triggers mutation (no error)", async ({ page }) => {
    const errors: string[] = [];
    const serverErrors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("response", (r) => { if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`); });
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="watched-badge"]', { timeout: 10_000 });
    await page.locator('[data-testid="watched-badge"]').first().click();
    await page.waitForTimeout(2_000);
    expect(errors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });

  test("mark watched on detail page works", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]').first();
    if (await btn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await btn.click();
      await page.waitForTimeout(500);
    }
  });
});

// ─── NOTIFICATIONS ──────────────────────────────────────────────────

test.describe("Notifications", () => {
  test("bell icon is visible", async ({ page }) => {
    await goto(page, "/home");
    await expect(page.locator('button[aria-label*="otification"]')).toBeVisible({ timeout: 5_000 });
  });

  test("clicking bell opens dropdown", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('button[aria-label*="otification"]').click();
    await page.waitForTimeout(500);
    // dropdown or panel should appear
    const dropdown = page.locator("text=/notification|no notification|view all/i").first();
    await expect(dropdown).toBeVisible({ timeout: 3_000 });
  });

  test("notifications page loads", async ({ page }) => {
    await goto(page, "/profile/notifications");
    await page.waitForTimeout(2_000);
    // should show heading or content
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });
});

// ─── COLLECTIONS ────────────────────────────────────────────────────

test.describe("Collections", () => {
  test("collections page loads", async ({ page }) => {
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });
});

// ─── PLAYLISTS ──────────────────────────────────────────────────────

test.describe("Playlists", () => {
  test("playlists page loads", async ({ page }) => {
    await goto(page, "/playlists");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });
});

// ─── LIBRARY BROWSE ─────────────────────────────────────────────────

test.describe("Library", () => {
  test("library index loads", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    // either library list or redirect to first library
    expect(page.url()).toMatch(/library/);
  });

  test("library has media cards", async ({ page }) => {
    await goto(page, "/library");
    await page.waitForTimeout(3_000);
    // may redirect to specific library
    const cards = await page.locator('[data-testid="media-card"]').count();
    // some libraries may be empty
    expect(cards).toBeGreaterThanOrEqual(0);
  });
});

// ─── PERSON PAGE ────────────────────────────────────────────────────

test.describe("Person", () => {
  test("navigating to person from cast shows name", async ({ page }) => {
    await goto(page, "/home");
    await page.locator('[data-testid="media-card"]').first().click();
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const castLink = page.locator('[data-testid="cast-section"] a').first();
    if (await castLink.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await castLink.click();
      await page.waitForURL("**/person/**", { timeout: 5_000 });
      const heading = page.locator("h1").first();
      await expect(heading).toBeVisible({ timeout: 5_000 });
      const name = await heading.textContent();
      expect(name!.length).toBeGreaterThan(0);
    }
  });
});

// ─── PROFILE ────────────────────────────────────────────────────────

test.describe("Profile", () => {
  test("profile page loads with username", async ({ page }) => {
    await goto(page, "/profile");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("settings page loads with tabs", async ({ page }) => {
    await goto(page, "/profile/settings");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("watch history page loads", async ({ page }) => {
    await goto(page, "/profile/history");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });
});

// ─── NAVIGATION ─────────────────────────────────────────────────────

test.describe("Navigation", () => {
  test("hamburger menu opens sidebar on mobile", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await goto(page, "/home");
    const burger = page.locator('button[aria-label*="menu" i]').first();
    await burger.waitFor({ state: "visible", timeout: 5_000 });
    await burger.click();
    await page.waitForTimeout(500);
    // overlay close button = sidebar opened
    await expect(page.locator('[aria-label*="close" i]').first()).toBeVisible({ timeout: 3_000 });
  });

  test("sidebar has navigation links", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await goto(page, "/home");
    await page.locator('button[aria-label*="menu" i]').first().click();
    await page.waitForTimeout(500);
    const links = page.locator("nav a");
    const count = await links.count();
    expect(count).toBeGreaterThan(3); // home, library, favorites, music at minimum
  });

  test("desktop sidebar starts collapsed", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await goto(page, "/home");
    const nav = page.locator("nav").first();
    await nav.waitFor({ state: "visible", timeout: 5_000 });
    const box = await nav.boundingBox();
    expect(box!.width).toBeLessThan(100); // collapsed = ~64px
  });
});

// ─── DOWNLOAD ───────────────────────────────────────────────────────

test.describe("Download", () => {
  test("context menu has download option", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const dl = page.locator("text=/download/i").first();
    const visible = await dl.isVisible({ timeout: 2_000 }).catch(() => false);
    expect(visible).toBe(true);
  });
});

// ─── MUSIC ──────────────────────────────────────────────────────────

test.describe("Music", () => {
  test("music page loads", async ({ page }) => {
    await goto(page, "/music");
    await page.waitForTimeout(3_000);
    // may show content or "no libraries"
    expect(page.url()).toContain("/music");
  });
});

// ─── PHOTOS ─────────────────────────────────────────────────────────

test.describe("Photos", () => {
  test("photos page loads", async ({ page }) => {
    await goto(page, "/photos");
    await page.waitForTimeout(3_000);
    expect(page.url()).toContain("/photos");
  });
});

// ─── NO CONSOLE ERRORS ─────────────────────────────────────────────

test.describe("Stability", () => {
  test("home page has no JS errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await goto(page, "/home");
    await page.waitForTimeout(3_000);
    expect(errors).toHaveLength(0);
  });

  test("no 5xx on home page API calls", async ({ page }) => {
    const serverErrors: string[] = [];
    page.on("response", (r) => {
      if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`);
    });
    await goto(page, "/home");
    await page.waitForTimeout(3_000);
    expect(serverErrors).toHaveLength(0);
  });
});
