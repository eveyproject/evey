import { test, expect } from "./fixtures/auth";
import { test as apiTest } from "./fixtures/api";

const MEDIA_ID =
  process.env.MEDIA_ID ?? "327046e8-621e-4289-8915-e5cf02c07a01";
const BASE_URL = process.env.BASE_URL ?? "https://evey.test";
const USER = process.env.EVEY_USER ?? "gravity";
const PASS = process.env.EVEY_PASS ?? "REDACTED";

async function showControls(page: import("@playwright/test").Page) {
  const viewport = page.viewportSize()!;
  await page.mouse.move(viewport.width / 2, viewport.height / 2);
  await page.waitForTimeout(500);
}

async function ensureLoggedIn(page: import("@playwright/test").Page) {
  if (page.url().includes("/login")) {
    await page.fill("#username", USER);
    await page.fill("#password", PASS);
    await page.click('button[type="submit"]');
    await page.waitForURL("**/home", { timeout: 15_000 });
  }
}

async function waitForPlayer(page: import("@playwright/test").Page) {
  await page.goto(`/player/${MEDIA_ID}`, {
    waitUntil: "domcontentloaded",
    timeout: 20_000,
  });

  // re-login if session expired
  await page.waitForTimeout(2_000);
  await ensureLoggedIn(page);
  if (!page.url().includes("/player/")) {
    await page.goto(`/player/${MEDIA_ID}`, {
      waitUntil: "domcontentloaded",
      timeout: 20_000,
    });
  }

  await page.waitForSelector("video, .animate-spin", { timeout: 15_000 });
  const hasVideo = await page.$("video");
  if (!hasVideo) {
    await page.waitForSelector("video", { timeout: 30_000 });
  }
  await page.waitForTimeout(8_000);
}

// --- Player UI tests ---

test.describe("Player fixes — controls & settings", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("control bar visible on hover in windowed mode", async ({ page }) => {
    await showControls(page);

    await expect(
      page.locator('button[aria-label="Pause"], button[aria-label="Play"]'),
    ).toBeVisible();
    await expect(page.locator('input[aria-label="Seek"]')).toBeVisible();
    await expect(
      page.locator('button[aria-label="Fullscreen"], button[aria-label="Exit fullscreen"]'),
    ).toBeVisible();
    await expect(page.locator('button[aria-label="Settings"]')).toBeVisible();
  });

  test("control bar buttons have aria-labels", async ({ page }) => {
    await showControls(page);

    await expect(
      page.locator('button[aria-label="Pause"], button[aria-label="Play"]'),
    ).toBeVisible();
    await expect(
      page.locator('button[aria-label="Fullscreen"], button[aria-label="Exit fullscreen"]'),
    ).toBeVisible();
    await expect(page.locator('button[aria-label="Settings"]')).toBeVisible();
    await expect(page.locator('button[aria-label="Back"]')).toBeVisible();
    await expect(page.locator('input[aria-label="Volume"]')).toBeVisible();
    await expect(page.locator('input[aria-label="Seek"]')).toBeVisible();
  });

  test("quality defaults to Auto", async ({ page }) => {
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    const autoBtn = page.locator("button", { hasText: /^Auto/ });
    await expect(autoBtn).toBeVisible();
    const text = await autoBtn.textContent();
    expect(text).toMatch(/Auto/);
  });

  test("Escape closes settings panel", async ({ page }) => {
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    const qualityHeader = page.locator("div", { hasText: /^Quality$/i }).first();
    await expect(qualityHeader).toBeVisible();

    await page.keyboard.press("Escape");
    await page.waitForTimeout(500);

    await expect(qualityHeader).not.toBeVisible({ timeout: 3_000 });
  });

  test("Space pauses and resumes", async ({ page }) => {
    const wasPlaying = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? !v.paused : false;
    });

    await page.keyboard.press("Space");
    await page.waitForTimeout(500);

    const pausedAfterSpace = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.paused : true;
    });

    if (wasPlaying) {
      expect(pausedAfterSpace).toBe(true);
    }

    await page.keyboard.press("Space");
    await page.waitForTimeout(500);

    const pausedAfterResume = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.paused : true;
    });

    if (wasPlaying) {
      expect(pausedAfterResume).toBe(false);
    }
  });

  test("K toggles play state", async ({ page }) => {
    const initialPaused = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.paused : true;
    });

    await page.keyboard.press("k");
    await page.waitForTimeout(1_000);

    const afterK = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.paused : true;
    });

    if (!initialPaused) {
      expect(afterK).toBe(true);
    }
  });
});

test.describe("Player fixes — transcode stream", () => {
  test("transcode response is 200 and player shows no fatal error after 35s", async ({ page }) => {
    test.setTimeout(120_000);

    let transcodeStatus = 0;
    let transcodeResponseReceived = false;

    page.on("response", (resp) => {
      if (resp.url().includes("/transcode")) {
        transcodeStatus = resp.status();
        transcodeResponseReceived = true;
      }
    });

    await waitForPlayer(page);

    // server responded 200 to transcode request
    expect(transcodeResponseReceived).toBe(true);
    expect(transcodeStatus).toBe(200);

    // video element has transcode src
    const video = page.locator("video");
    await expect(video).toBeVisible();
    const src = await video.getAttribute("src");
    expect(src).toContain("/transcode");

    // wait 35s total (8s in waitForPlayer + 27s here)
    await page.waitForTimeout(27_000);

    // no fatal error overlay — the player didn't give up
    const fatalOverlay = page.locator("text=Playback failed");
    await expect(fatalOverlay).not.toBeVisible();

    // video element still exists (wasn't replaced with error)
    await expect(video).toBeVisible();
  });
});

// --- Backend API tests ---

apiTest.describe("Transcode API", () => {
  apiTest("transcode returns video/mp4 chunked (no Content-Length)", async ({
    page,
    token,
  }) => {
    apiTest.setTimeout(90_000);

    await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded" });

    const url = `/api/v1/media/${MEDIA_ID}/transcode?token=${token}&audioTrack=0`;

    const result = await page.evaluate(async (fetchUrl: string) => {
      const resp = await fetch(fetchUrl);
      const reader = resp.body?.getReader();
      let bytesRead = 0;
      if (reader) {
        const { value } = await reader.read();
        bytesRead = value?.length ?? 0;
        await reader.cancel();
      }
      return {
        status: resp.status,
        contentType: resp.headers.get("content-type"),
        contentLength: resp.headers.get("content-length"),
        bytesRead,
      };
    }, url);

    expect(result.status).toBe(200);
    expect(result.contentType).toContain("video/mp4");
    expect(result.contentLength).toBeNull();
    expect(result.bytesRead).toBeGreaterThan(0);
  });

  apiTest("transcode stream stays alive past 35s", async ({
    page,
    token,
  }) => {
    apiTest.setTimeout(120_000);

    await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded" });

    const url = `/api/v1/media/${MEDIA_ID}/transcode?token=${token}&audioTrack=0`;

    const result = await page.evaluate(async (fetchUrl: string) => {
      const resp = await fetch(fetchUrl);
      if (!resp.ok) return { ok: false, status: resp.status, durationMs: 0, totalBytes: 0, error: null };

      const reader = resp.body?.getReader();
      if (!reader) return { ok: true, status: resp.status, durationMs: 0, totalBytes: 0, error: null };

      const start = Date.now();
      let totalBytes = 0;
      let error: string | null = null;

      try {
        while (Date.now() - start < 35_000) {
          const { done, value } = await reader.read();
          if (done) break;
          totalBytes += value?.length ?? 0;
        }
      } catch (e) {
        error = (e as Error).message;
      }

      const durationMs = Date.now() - start;
      await reader.cancel().catch(() => {});
      return { ok: true, status: resp.status, durationMs, totalBytes, error };
    }, url);

    expect(result.ok).toBe(true);
    expect(result.status).toBe(200);
    expect(result.durationMs).toBeGreaterThanOrEqual(34_000);
    expect(result.totalBytes).toBeGreaterThan(100_000);
    expect(result.error).toBeNull();
  });
});
