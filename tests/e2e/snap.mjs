import { chromium } from "playwright";
// Target + credentials come from the environment - never hardcode them here.
const BASE_URL = process.env.BASE_URL ?? "http://localhost:5173";
const browser = await chromium.launch({ headless: true });
const ctx = await browser.newContext({ viewport: { width: 414, height: 896 } });
const page = await ctx.newPage();
await page.goto(`${BASE_URL}/login`);
await page.fill('input[type="text"]', process.env.EVEY_E2E_USER ?? "admin");
await page.fill('input[type="password"]', process.env.EVEY_E2E_PASSWORD ?? "");
await page.click('button[type="submit"]');
await page.waitForURL(new RegExp(`home|^${BASE_URL.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/?$`), { timeout: 15000 });
await page.waitForTimeout(4500);
await page.screenshot({ path: "/tmp/web.png", fullPage: false });
console.log("OK", await page.title());
await browser.close();
