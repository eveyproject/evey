import { chromium } from '@playwright/test';
const OUT = process.argv[2], URL = process.argv[3];
const b = await chromium.launch();
const p = await b.newPage({ viewport: { width: 1440, height: 1400 } });
await p.goto('http://localhost:5173/login', { waitUntil: 'domcontentloaded' });
await p.fill('input[name="username"], input[type="text"]', process.env.EVEY_E2E_USER ?? 'admin').catch(()=>{});
await p.fill('input[type="password"]', process.env.EVEY_E2E_PASSWORD ?? '').catch(()=>{});
await p.click('button[type="submit"]').catch(()=>{});
await p.waitForTimeout(4000);
await p.goto(URL, { waitUntil: 'networkidle' }).catch(()=>{});
await p.waitForTimeout(2500);
// open the season accordion
const season = p.locator('text=/Saison\\s*\\d+/').first();
await season.click().catch(()=>{});
await p.waitForTimeout(2500);
await p.screenshot({ path: OUT, fullPage: true });
await b.close();
