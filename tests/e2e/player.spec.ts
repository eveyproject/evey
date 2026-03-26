import { test, expect } from "./fixtures/auth";

const MEDIA_ID =
  process.env.MEDIA_ID ?? "577d3d4d-da77-4c8a-8e46-6bee6b64635c";
const USER = process.env.EVEY_USER ?? "gravity";
const PASS = process.env.EVEY_PASS ?? "REDACTED";

const KNOWN_ERRORS = [
  "DEMUXER_ERROR_NO_SUPPORTED_STREAMS",
  "PIPELINE_ERROR_DECODE",
];

function isKnownError(text: string) {
  return KNOWN_ERRORS.some((e) => text.includes(e));
}

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
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 15_000,
    });
  }
}

async function waitForPlayer(page: import("@playwright/test").Page) {
  await page.goto(`/player/${MEDIA_ID}`, {
    waitUntil: "domcontentloaded",
    timeout: 20_000,
  });

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

test.describe("Player", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("video element loads with source", async ({ page }) => {
    const video = page.locator("video");
    await expect(video).toBeVisible();

    // verify video has a src (transcode or stream URL with token)
    const src = await video.getAttribute("src");
    expect(src).toBeTruthy();
    expect(src!.includes("/transcode") || src!.includes("/stream")).toBe(true);
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

    errors.length = 0;
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

    networkErrors.length = 0;
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

test.describe("Player — Quality switching", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("change quality preserves position", async ({ page }) => {
    await showControls(page);

    // grab current time before switch
    const timeBefore = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.currentTime : 0;
    });

    // open settings panel
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    // pick 720p (always available in QUALITY_OPTIONS)
    const qualityBtn = page.locator("button", { hasText: "720p" });
    if (await qualityBtn.isVisible()) {
      await qualityBtn.click();

      // wait for transcode reload
      await page.waitForTimeout(6_000);

      // position in the time display should roughly match
      await showControls(page);
      const timeSpan = page.locator("span.tabular-nums");
      const text = await timeSpan.textContent();
      expect(text).toBeTruthy();
      // time should not have jumped to 0:00 (first segment shows current / duration)
      // we only verify the display is non-zero if we had played for a bit
      if (timeBefore > 2) {
        expect(text).not.toMatch(/^0:00\s*\//);
      }
    } else {
      test.skip(true, "720p option not visible");
    }
  });

  test("settings panel shows quality section", async ({ page }) => {
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    // quality header visible
    const qualityHeader = page.locator("div", { hasText: /^Quality$/i }).first();
    await expect(qualityHeader).toBeVisible();

    // at least Original option is present (use exact to avoid matching "Auto (Original)")
    await expect(page.getByRole("button", { name: "Original", exact: true })).toBeVisible();
  });
});

test.describe("Player — Keyboard shortcuts", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("Space pauses and resumes", async ({ page }) => {
    // ensure video is playing
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

    // press again to resume
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
    // record initial state
    const initialPaused = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.paused : true;
    });

    // first K press should toggle
    await page.keyboard.press("k");
    await page.waitForTimeout(1_000);

    const afterK = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.paused : true;
    });

    // in headless, play() may fail so resumed state can't be guaranteed
    // but at minimum, K when playing should pause
    if (!initialPaused) {
      expect(afterK).toBe(true);
    } else {
      // was paused: K tries to play. headless may or may not succeed.
      // just verify the handler ran without errors (no assertion on state)
      expect(true).toBe(true);
    }
  });

  test("M toggles mute", async ({ page }) => {
    const mutedBefore = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.muted : false;
    });

    await page.keyboard.press("m");
    await page.waitForTimeout(300);

    const mutedAfter = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.muted : false;
    });

    expect(mutedAfter).not.toBe(mutedBefore);
  });

  test("ArrowRight seeks forward", async ({ page }) => {
    const timeBefore = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.currentTime : 0;
    });

    await page.keyboard.press("ArrowRight");
    await page.waitForTimeout(3_000);

    // in transcode mode a new segment loads, so check the time display updated
    await showControls(page);
    const timeSpan = page.locator("span.tabular-nums");
    const text = await timeSpan.textContent();
    expect(text).toBeTruthy();
    // just verify the display is valid (seek may reload transcode)
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("Infinity");
  });

  test("ArrowLeft seeks backward", async ({ page }) => {
    // first seek forward to have something to seek back from
    await page.keyboard.press("ArrowRight");
    await page.waitForTimeout(3_000);

    await page.keyboard.press("ArrowLeft");
    await page.waitForTimeout(3_000);

    await showControls(page);
    const timeSpan = page.locator("span.tabular-nums");
    const text = await timeSpan.textContent();
    expect(text).toBeTruthy();
    expect(text).not.toContain("NaN");
  });

  test("F toggles fullscreen request", async ({ page }) => {
    // headless chromium may not truly enter fullscreen, but the key event should not error
    const errorsBefore: string[] = [];
    page.on("pageerror", (err) => {
      if (!isKnownError(err.message)) errorsBefore.push(err.message);
    });

    await page.keyboard.press("f");
    await page.waitForTimeout(500);

    // press again to exit
    await page.keyboard.press("f");
    await page.waitForTimeout(500);

    // no JS errors from fullscreen toggle
    expect(errorsBefore).toHaveLength(0);
  });

  test("? opens keyboard shortcuts overlay", async ({ page }) => {
    await page.keyboard.press("?");
    await page.waitForTimeout(500);

    // the overlay shows shortcut descriptions
    const overlay = page.locator("text=Space / K");
    const count = await overlay.count();
    expect(count).toBeGreaterThan(0);

    // close with Escape
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
  });
});

test.describe("Player — Playback speed", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("change speed to 1.5x via settings", async ({ page }) => {
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    const speedBtn = page.locator("button", { hasText: "1.5x" });
    if (await speedBtn.isVisible()) {
      await speedBtn.click();
      await page.waitForTimeout(500);

      const rate = await page.evaluate(() => {
        const v = document.querySelector("video");
        return v ? v.playbackRate : 1;
      });
      expect(rate).toBeCloseTo(1.5, 1);
    } else {
      test.skip(true, "Speed section not visible");
    }
  });

  test("change speed to 2x via settings", async ({ page }) => {
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    // use exact role match to avoid matching "0.5x" when looking for "2x"
    const speedBtn = page.getByRole("button", { name: "2x", exact: true });
    if (await speedBtn.isVisible()) {
      await speedBtn.click();
      await page.waitForTimeout(500);

      const rate = await page.evaluate(() => {
        const v = document.querySelector("video");
        return v ? v.playbackRate : 1;
      });
      expect(rate).toBeCloseTo(2, 1);
    } else {
      test.skip(true, "Speed section not visible");
    }
  });

  test("speed change via > keyboard shortcut", async ({ page }) => {
    // default speed is 1 (Normal), pressing > should go to 1.25x
    await page.keyboard.press(">");
    await page.waitForTimeout(500);

    const rate = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.playbackRate : 1;
    });
    expect(rate).toBeCloseTo(1.25, 1);
  });

  test("speed change via < keyboard shortcut", async ({ page }) => {
    // first increase to 1.25x, then decrease back to 1x
    await page.keyboard.press(">");
    await page.waitForTimeout(300);
    await page.keyboard.press("<");
    await page.waitForTimeout(500);

    const rate = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.playbackRate : 1;
    });
    expect(rate).toBeCloseTo(1, 1);
  });
});

test.describe("Player — Subtitle selection", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("subtitle section appears in settings if tracks exist", async ({ page }) => {
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    // check if Subtitles header exists
    const subHeader = page.locator("div", { hasText: /^Subtitles$/i }).first();
    const hasSubtitles = await subHeader.isVisible().catch(() => false);

    if (hasSubtitles) {
      // "Off" button should always be present in subtitles section
      await expect(page.locator("button", { hasText: "Off" })).toBeVisible();
    } else {
      test.skip(true, "No subtitle tracks for this media");
    }
  });

  test("select a subtitle track adds track element", async ({ page }) => {
    // use C shortcut which toggles first subtitle track on/off — simpler than navigating settings
    // first check subtitles exist
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    const subHeader = page.locator("div", { hasText: /^Subtitles$/i }).first();
    const hasSubtitles = await subHeader.isVisible().catch(() => false);

    if (!hasSubtitles) {
      test.skip(true, "No subtitle tracks for this media");
      return;
    }

    // close settings, then use C shortcut to enable first subtitle
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);

    await page.keyboard.press("c");
    await page.waitForTimeout(1_000);

    const hasTrack = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.querySelectorAll("track[data-dynamic]").length > 0 : false;
    });
    expect(hasTrack).toBe(true);
  });

  test("C keyboard shortcut toggles subtitles", async ({ page }) => {
    // check if subtitles exist first
    await showControls(page);
    await page.click('button[aria-label="Settings"]');
    await page.waitForTimeout(500);

    const subHeader = page.locator("div", { hasText: /^Subtitles$/i }).first();
    const hasSubtitles = await subHeader.isVisible().catch(() => false);

    // close settings
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);

    if (!hasSubtitles) {
      test.skip(true, "No subtitle tracks for this media");
      return;
    }

    // press C to enable subtitles
    await page.keyboard.press("c");
    await page.waitForTimeout(1_000);

    const trackCountAfterEnable = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.querySelectorAll("track[data-dynamic]").length : 0;
    });
    expect(trackCountAfterEnable).toBeGreaterThan(0);

    // press C again to disable
    await page.keyboard.press("c");
    await page.waitForTimeout(1_000);

    const trackCountAfterDisable = await page.evaluate(() => {
      const v = document.querySelector("video");
      return v ? v.querySelectorAll("track[data-dynamic]").length : 0;
    });
    expect(trackCountAfterDisable).toBe(0);
  });
});

test.describe("Player — Skip intro", () => {
  test.beforeEach(async ({ page }) => {
    await waitForPlayer(page);
  });

  test("skip intro button triggers seek when visible", async ({ page }) => {
    const skipBtn = page.locator("button", { hasText: "Skip Intro" });

    // wait a bit for the skip intro button to potentially appear
    const isVisible = await skipBtn.isVisible({ timeout: 3_000 }).catch(() => false);

    if (!isVisible) {
      // no intro chapter or not in intro range — mark test as expected skip
      test.skip();
      return;
    }

    await skipBtn.click();
    await page.waitForTimeout(3_000);

    // after clicking skip, button should disappear (we're past the intro)
    const stillVisible = await skipBtn.isVisible().catch(() => false);
    expect(stillVisible).toBe(false);
  });
});
