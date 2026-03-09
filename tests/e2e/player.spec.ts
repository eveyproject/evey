import { test, expect } from "./fixtures/auth";

const MEDIA_ID =
  process.env.MEDIA_ID ?? "327046e8-621e-4289-8915-e5cf02c07a01";

// HEVC demuxer errors are expected in headless Chromium
const KNOWN_ERRORS = [
  "DEMUXER_ERROR_NO_SUPPORTED_STREAMS",
  "PIPELINE_ERROR_DECODE",
];

function isKnownError(text: string) {
  return KNOWN_ERRORS.some((e) => text.includes(e));
}

async function showControls(page: import("@playwright/test").Page) {
  // the player has an overlay div intercepting pointer events, use mouse.move on viewport center
  const viewport = page.viewportSize()!;
  await page.mouse.move(viewport.width / 2, viewport.height / 2);
  await page.waitForTimeout(500);
}

test.describe("Player", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(`/player/${MEDIA_ID}`, {
      waitUntil: "domcontentloaded",
      timeout: 20_000,
    });

    await page.waitForSelector(
      "video, [class*='spinner'], [class*='Spinner']",
      { timeout: 15_000 },
    );

    const hasVideo = await page.$("video");
    if (!hasVideo) {
      await page.waitForSelector("video", { timeout: 30_000 });
    }

    // let stream buffer
    await page.waitForTimeout(8_000);
  });

  test("video element loads with source", async ({ page }) => {
    const video = page.locator("video");
    await expect(video).toBeVisible();

    // verify video has a src (transcode URL with token)
    const src = await video.getAttribute("src");
    expect(src).toBeTruthy();
    expect(src).toContain("/transcode");
  });

  test("seek bar exists with valid max", async ({ page }) => {
    await showControls(page);

    const seekBar = page.locator('input[aria-label="Seek"]');
    await expect(seekBar).toBeVisible();

    const max = await seekBar.getAttribute("max");
    expect(Number(max)).toBeGreaterThan(0);
  });

  test("time display shows valid duration", async ({ page }) => {
    await showControls(page);

    const timeSpan = page.locator("span.tabular-nums");
    await expect(timeSpan).toBeVisible();

    const text = await timeSpan.textContent();
    expect(text).toBeTruthy();
    expect(text).not.toContain("Infinity");
    expect(text).not.toContain("NaN");
    expect(text!.trim()).not.toBe("0:00 / 0:00");

    const durationMatch = text!.match(/\/\s*(.+)/);
    expect(durationMatch).toBeTruthy();
    const dur = durationMatch![1].trim();
    expect(dur).not.toBe("0:00");
  });

  test("player controls are present", async ({ page }) => {
    await showControls(page);

    await expect(
      page.locator(
        'button[aria-label="Pause"], button[aria-label="Play"]',
      ),
    ).toBeVisible();

    await expect(page.locator('input[aria-label="Volume"]')).toBeVisible();
    await expect(
      page.locator('button[aria-label="Settings"]'),
    ).toBeVisible();
    await expect(
      page.locator(
        'button[aria-label="Fullscreen"], button[aria-label="Exit fullscreen"]',
      ),
    ).toBeVisible();
    await expect(
      page.locator('button[aria-label="Back"]'),
    ).toBeVisible();
  });

  test("no unexpected console errors during playback", async ({ page }) => {
    const errors: string[] = [];

    page.on("pageerror", (err) => {
      if (!isKnownError(err.message)) errors.push(err.message);
    });
    page.on("console", (msg) => {
      if (msg.type() === "error" && !isKnownError(msg.text()))
        errors.push(msg.text());
    });

    await page.goto(`/player/${MEDIA_ID}`, {
      waitUntil: "domcontentloaded",
    });
    await page.waitForSelector("video", { timeout: 30_000 });
    await page.waitForTimeout(5_000);

    expect(errors).toHaveLength(0);
  });

  test("no 4xx/5xx network errors", async ({ page }) => {
    const networkErrors: string[] = [];

    page.on("response", (resp) => {
      if (resp.status() >= 400) {
        networkErrors.push(
          `${resp.status()} ${resp.request().method()} ${resp.url()}`,
        );
      }
    });

    await page.goto(`/player/${MEDIA_ID}`, {
      waitUntil: "domcontentloaded",
    });
    await page.waitForSelector("video", { timeout: 30_000 });
    await page.waitForTimeout(5_000);

    expect(networkErrors).toHaveLength(0);
  });

  test("buffered bar element exists", async ({ page }) => {
    await showControls(page);

    // bar may be hidden (width:0) if codec not supported in headless, just check it exists in DOM
    const bufferedBar = page.locator("div.bg-white\\/40");
    const count = await bufferedBar.count();
    expect(count).toBeGreaterThan(0);
  });
});
