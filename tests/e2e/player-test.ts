import { chromium } from "playwright";
import * as fs from "fs";
import * as path from "path";

const BASE = "https://evey.test";
const SCREENSHOT_DIR = path.join(__dirname, "screenshots");
fs.mkdirSync(SCREENSHOT_DIR, { recursive: true });

interface LogEntry {
  type: string;
  text: string;
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  const consoleLogs: LogEntry[] = [];
  const networkErrors: string[] = [];

  page.on("console", (msg) => {
    const type = msg.type();
    if (type === "error" || type === "warning") {
      consoleLogs.push({ type, text: msg.text() });
    }
  });

  page.on("response", (resp) => {
    const status = resp.status();
    const url = resp.url();
    if (status >= 400) {
      networkErrors.push(`${status} ${resp.request().method()} ${url}`);
    }
  });

  page.on("pageerror", (err) => {
    consoleLogs.push({ type: "pageerror", text: err.message });
  });

  // 1. Login
  console.log("--- Step 1: Login ---");
  await page.goto(`${BASE}/login`, { waitUntil: "networkidle" });
  await page.fill("#username", "gravity");
  await page.fill("#password", "REDACTED");
  await page.click('button[type="submit"]');
  await page.waitForURL("**/home", { timeout: 15000 });
  console.log("Logged in, redirected to:", page.url());

  // 2. Navigate to player
  console.log("\n--- Step 2: Navigate to player ---");
  // Test with a long movie (Dust Bunny, ~1h45) to verify knownDuration works
  const MEDIA_ID = process.env.MEDIA_ID ?? "327046e8-621e-4289-8915-e5cf02c07a01";
  await page.goto(`${BASE}/player/${MEDIA_ID}`, {
    waitUntil: "domcontentloaded",
    timeout: 20000,
  });

  // Wait for either the video element or a spinner to appear
  await page.waitForSelector("video, [class*='spinner'], [class*='Spinner']", {
    timeout: 15000,
  });

  // If spinner is shown, wait for video to replace it
  const hasVideo = await page.$("video");
  if (!hasVideo) {
    console.log("Waiting for video element (spinner visible)...");
    await page.waitForSelector("video", { timeout: 30000 });
  }
  console.log("Video element found");

  // Give the player time to load the stream and start playback
  console.log("Waiting for stream to buffer...");
  await page.waitForTimeout(8000);

  // 3. Take screenshot of the player
  await page.screenshot({
    path: path.join(SCREENSHOT_DIR, "player-loaded.png"),
    fullPage: true,
  });
  console.log("Screenshot saved: player-loaded.png");

  // Move mouse to trigger controls visibility
  const videoEl = await page.$("video");
  if (videoEl) {
    const box = await videoEl.boundingBox();
    if (box) {
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      await page.waitForTimeout(500);
    }
  }

  // 4. Check player UI elements
  console.log("\n--- Step 3: Check player UI ---");

  // Seek bar
  const seekBar = await page.$('input[aria-label="Seek"]');
  console.log(`Seek bar exists: ${!!seekBar}`);
  if (seekBar) {
    const max = await seekBar.getAttribute("max");
    const value = await seekBar.getAttribute("value");
    console.log(`Seek bar max=${max}, value=${value}`);
  }

  // Time display — the <span> with "X:XX / X:XX" pattern
  const timeSpan = await page.$("span.tabular-nums");
  let timeText = "";
  if (timeSpan) {
    timeText = (await timeSpan.textContent()) ?? "";
    console.log(`Time display: "${timeText}"`);
    // Validate it's not 0:00 / 0:00 or contains Infinity
    if (timeText.includes("Infinity")) {
      console.log("WARNING: Time display contains Infinity");
    }
    if (timeText.trim() === "0:00 / 0:00") {
      console.log("WARNING: Duration is 0:00 — stream may not have loaded");
    }
    const durationMatch = timeText.match(/\/\s*(.+)/);
    if (durationMatch) {
      const dur = durationMatch[1].trim();
      if (dur !== "0:00" && !dur.includes("Infinity") && !dur.includes("NaN")) {
        console.log(`Duration looks valid: ${dur}`);
      } else {
        console.log(`WARNING: Duration looks invalid: ${dur}`);
      }
    }
  } else {
    console.log("WARNING: Time display span not found");
  }

  // Buffered indicator (the div with bg-white/40)
  const bufferedBar = await page.$("div.bg-white\\/40");
  console.log(`Buffered indicator exists: ${!!bufferedBar}`);
  if (bufferedBar) {
    const style = await bufferedBar.getAttribute("style");
    console.log(`Buffered bar style: ${style}`);
  }

  // Play/pause button
  const playPauseBtn = await page.$('button[aria-label="Pause"], button[aria-label="Play"]');
  if (playPauseBtn) {
    const label = await playPauseBtn.getAttribute("aria-label");
    console.log(`Play/Pause button state: ${label}`);
  } else {
    console.log("WARNING: Play/Pause button not found");
  }

  // Volume slider
  const volumeSlider = await page.$('input[aria-label="Volume"]');
  console.log(`Volume slider exists: ${!!volumeSlider}`);

  // Settings button
  const settingsBtn = await page.$('button[aria-label="Settings"]');
  console.log(`Settings button exists: ${!!settingsBtn}`);

  // Fullscreen button
  const fullscreenBtn = await page.$('button[aria-label="Fullscreen"], button[aria-label="Exit fullscreen"]');
  console.log(`Fullscreen button exists: ${!!fullscreenBtn}`);

  // Back button
  const backBtn = await page.$('button[aria-label="Back"]');
  console.log(`Back button exists: ${!!backBtn}`);

  // Take another screenshot with controls visible
  await page.screenshot({
    path: path.join(SCREENSHOT_DIR, "player-controls.png"),
    fullPage: true,
  });
  console.log("Screenshot saved: player-controls.png");

  // 5. Report all findings
  console.log("\n--- Step 4: Console errors/warnings ---");
  if (consoleLogs.length === 0) {
    console.log("No console errors or warnings");
  } else {
    for (const log of consoleLogs) {
      console.log(`[${log.type}] ${log.text}`);
    }
  }

  console.log("\n--- Step 5: Network errors (4xx/5xx) ---");
  if (networkErrors.length === 0) {
    console.log("No network errors");
  } else {
    for (const err of networkErrors) {
      console.log(err);
    }
  }

  console.log("\n--- Summary ---");
  console.log(`Console errors: ${consoleLogs.filter((l) => l.type === "error" || l.type === "pageerror").length}`);
  console.log(`Console warnings: ${consoleLogs.filter((l) => l.type === "warning").length}`);
  console.log(`Network errors: ${networkErrors.length}`);
  console.log(`Time display: "${timeText}"`);

  await browser.close();
  console.log("\nDone.");
})();
