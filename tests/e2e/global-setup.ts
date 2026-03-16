import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";
import { chromium } from "@playwright/test";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MARKER = path.join(__dirname, ".e2e-start-time");
export const STORAGE_STATE = path.join(__dirname, ".auth-state.json");

const BASE_URL = process.env.BASE_URL ?? "https://evey.test";
const USER = process.env.EVEY_USER ?? "gravity";
const PASS = process.env.EVEY_PASS ?? "REDACTED";

export default async function globalSetup() {
  fs.writeFileSync(MARKER, new Date().toISOString());

  // Perform login once; save storage state for reuse across all spec files
  const browser = await chromium.launch();
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();
  await page.goto(`${BASE_URL}/login`, { waitUntil: "networkidle" });
  await page.fill("#username", USER);
  await page.fill("#password", PASS);
  await page.click('button[type="submit"]');
  await page.waitForURL("**/home", { timeout: 15_000 });
  await context.storageState({ path: STORAGE_STATE });
  await browser.close();
}
