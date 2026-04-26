import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";
import { chromium } from "@playwright/test";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MARKER = path.join(__dirname, ".e2e-start-time");
export const STORAGE_STATE = path.join(__dirname, ".auth-state.json");

const BASE_URL = process.env.BASE_URL ?? "https://evey.test";
const USER = process.env.EVEY_USER;
const PASS = process.env.EVEY_PASS;

export default async function globalSetup() {
  if (!USER || !PASS) {
    throw new Error("EVEY_USER and EVEY_PASS env vars are required for e2e (no hardcoded credentials)");
  }
  fs.writeFileSync(MARKER, new Date().toISOString());

  const browser = await chromium.launch();
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();
  await page.goto(`${BASE_URL}/login`);
  await page.waitForSelector("#username", { state: "visible", timeout: 15_000 });
  await page.fill("#username", USER);
  await page.fill("#password", PASS);
  await page.click('button[type="submit"]');
  await page.waitForURL("**/home", { timeout: 15_000 });

  // Zustand persists only profileId (not accessToken) to localStorage.
  // Patch the persisted state to include the token so restored contexts work.
  await page.evaluate(async (creds) => {
    const res = await fetch("/api/v1/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: creds.user, password: creds.pass }),
    });
    if (!res.ok) return;
    const data = (await res.json()) as { accessToken?: string };
    if (!data.accessToken) return;
    const tokenParts = data.accessToken.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
    localStorage.setItem("evey-auth", JSON.stringify({
      state: {
        accessToken: data.accessToken,
        profileId: typeof payload.profile_id === "string" ? payload.profile_id : null,
        isAdmin: payload.is_admin === true,
      },
      version: 0,
    }));
  }, { user: USER, pass: PASS });

  await context.storageState({ path: STORAGE_STATE });
  await browser.close();
}
