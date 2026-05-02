import { test } from "@playwright/test";
import * as path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const STORAGE_STATE = path.join(__dirname, ".auth-state.json");
const BASE_URL = process.env.BASE_URL ?? "https://evey.test";

test.use({ storageState: STORAGE_STATE, ignoreHTTPSErrors: true, viewport: { width: 1280, height: 800 } });

test("DEBUG: screenshot home + iframe", async ({ page }) => {
  await page.goto(`${BASE_URL}/home`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(8000);
  await page.screenshot({ path: "/tmp/home-shot.png", fullPage: false });
  const iframe = page.locator("iframe").first();
  if (await iframe.count()) {
    const box = await iframe.boundingBox();
    if (box) await page.screenshot({ path: "/tmp/iframe-shot.png", clip: box });
  }
  console.log("done");
});
