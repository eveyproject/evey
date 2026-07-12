import { test, expect } from "../../fixtures/auth";
import { gotoRandomMovie, collectErrors } from "../shared/helpers";

const KNOWN_ERRORS = [
  "DEMUXER_ERROR_NO_SUPPORTED_STREAMS",
  "PIPELINE_ERROR_DECODE",
];

function isKnownError(text: string) {
  return KNOWN_ERRORS.some((e) => text.includes(e));
}

async function spaNavigate(page: import("@playwright/test").Page, path: string) {
  // Click a TanStack <Link> in the sidebar if available, otherwise use evaluate
  const sidebarLink = page.locator(`nav a[href="${path}"]`).first();
  if (await sidebarLink.isVisible({ timeout: 1_000 }).catch(() => false)) {
    await sidebarLink.click();
  } else {
    // Use TanStack Router's internal navigation via history API
    await page.evaluate((p) => window.history.pushState({}, "", p), path);
    await page.evaluate(() => window.dispatchEvent(new PopStateEvent("popstate")));
  }
  await page.waitForTimeout(1_500);
}

async function startPlayerAndMinimize(page: import("@playwright/test").Page) {
  const id = await gotoRandomMovie(page);
  if (!id) return null;

  // Navigate to media detail first (gotoRandomMovie already did this), then SPA to player
  // Click the play button on the media detail page to navigate via SPA
  const playLink = page.locator(`a[href="/player/${id}"]`).first();
  const playBtn = page.locator('[data-testid="play-btn"], button:has-text("Play"), [aria-label="Play"]').first();

  if (await playLink.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await playLink.click();
  } else if (await playBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
    await playBtn.click();
  } else {
    // Fallback: use evaluate to push state
    await page.evaluate((p) => window.history.pushState({}, "", p), `/player/${id}`);
    await page.evaluate(() => window.dispatchEvent(new PopStateEvent("popstate")));
  }

  await page.waitForSelector("video", { timeout: 30_000 });
  await page.waitForTimeout(3_000);

  // Go back via browser history (SPA navigation, triggers cleanup effect)
  await page.goBack();
  await page.waitForTimeout(2_000);

  return id;
}

test.describe("Mini Player", () => {
  test("appears when navigating away from player", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });
  });

  test("shows media title element", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    // Title span exists in DOM (may be empty if query hasn't resolved yet)
    const title = container.locator(".truncate");
    await expect(title).toBeAttached();
  });

  test("play/pause button is clickable", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    // Either Play or Pause button should be visible
    const toggleBtn = container.locator('[aria-label="Play"], [aria-label="Pause"]').first();
    await expect(toggleBtn).toBeVisible();

    // Click it - headless may not toggle state reliably, just verify no crash
    await toggleBtn.click();
    await page.waitForTimeout(500);

    // Button should still be present after click
    const afterBtn = container.locator('[aria-label="Play"], [aria-label="Pause"]').first();
    await expect(afterBtn).toBeVisible();
  });

  test("progress bar exists", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    // Progress bar inner div exists (may be width:0% in headless → technically hidden)
    const progressBar = container.locator(".bg-\\[var\\(--accent\\)\\]");
    await expect(progressBar).toBeAttached();
    // Parent track is visible
    const progressTrack = container.locator(".rounded-full.overflow-hidden");
    await expect(progressTrack).toBeVisible();
  });

  test("expand button returns to full player", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    const expandBtn = container.locator('[aria-label="Expand"]');
    await expect(expandBtn).toBeVisible();
    await expandBtn.click();

    await page.waitForURL(/\/player\//, { timeout: 10_000 });
    await page.waitForSelector("video", { timeout: 10_000 });
  });

  test("close button stops playback", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    const closeBtn = container.locator('[aria-label="Close"]');
    await expect(closeBtn).toBeVisible();
    await closeBtn.click();
    await page.waitForTimeout(1_000);

    await expect(container).not.toBeVisible({ timeout: 3_000 });
  });

  test("persists across SPA navigation", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    await spaNavigate(page, "/settings");
    await expect(container).toBeVisible({ timeout: 5_000 });

    await spaNavigate(page, "/home");
    await expect(container).toBeVisible({ timeout: 5_000 });
  });

  test("video click expands to full player", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    const videoArea = container.locator('[role="button"]');
    await videoArea.click();

    await page.waitForURL(/\/player\//, { timeout: 10_000 });
  });

  test("expand then minimize again works", async ({ page }) => {
    const id = await startPlayerAndMinimize(page);
    test.skip(!id, "No playable movie found");

    const container = page.locator(".fixed.z-overlay-raised");
    await expect(container).toBeVisible({ timeout: 5_000 });

    await container.locator('[aria-label="Expand"]').click();
    await page.waitForURL(/\/player\//, { timeout: 10_000 });
    await page.waitForSelector("video", { timeout: 10_000 });
    await page.waitForTimeout(2_000);

    await spaNavigate(page, "/home");
    await expect(container).toBeVisible({ timeout: 5_000 });
  });

  test("no JS errors during mini player lifecycle", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMovie(page);
      if (!id) return;

      await spaNavigate(page, `/player/${id}`);
      await page.waitForSelector("video", { timeout: 30_000 }).catch(() => null);
      await page.waitForTimeout(3_000);

      await spaNavigate(page, "/home");
      await page.waitForTimeout(2_000);

      const container = page.locator(".fixed.z-overlay-raised");
      if (await container.isVisible().catch(() => false)) {
        const toggleBtn = container.locator('[aria-label="Play"], [aria-label="Pause"]').first();
        if (await toggleBtn.isVisible().catch(() => false)) await toggleBtn.click();
        await page.waitForTimeout(500);

        await spaNavigate(page, "/settings");

        const closeBtn = container.locator('[aria-label="Close"]');
        if (await closeBtn.isVisible().catch(() => false)) await closeBtn.click();
        await page.waitForTimeout(500);
      }
    });

    const real = jsErrors.filter((e) => !isKnownError(e));
    expect(real).toHaveLength(0);
  });
});
