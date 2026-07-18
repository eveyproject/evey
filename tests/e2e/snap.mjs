import { chromium } from "playwright";
const browser = await chromium.launch({ headless: true });
const ctx = await browser.newContext({ viewport: { width: 414, height: 896 } });
const page = await ctx.newPage();
await page.goto("http://localhost:5173/login");
// Credentials come from the environment - never hardcode them here.
await page.fill('input[type="text"]', process.env.EVEY_E2E_USER ?? "admin");
await page.fill('input[type="password"]', process.env.EVEY_E2E_PASSWORD ?? "");
await page.click('button[type="submit"]');
await page.waitForURL(/home|^http:\/\/localhost:5173\/?$/, { timeout: 15000 });
await page.waitForTimeout(4500);
await page.screenshot({ path: "/tmp/web.png", fullPage: false });
console.log("OK", await page.title());
await browser.close();
