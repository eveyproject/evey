// Open Firefox via Playwright, navigate to /home with the existing
// auth-state, find the plugin iframe, and print its srcdoc + the CSP
// directive specifically + any console errors.

import { firefox } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const STORAGE_STATE = "/home/gravity/side_projects/evey/tests/e2e/.auth-state.json";

(async () => {
  const browser = await firefox.launch();
  const context = await browser.newContext({
    storageState: STORAGE_STATE,
    ignoreHTTPSErrors: true,
  });
  const page = await context.newPage();

  page.on("console", (msg) => {
    if (msg.type() === "error" || msg.type() === "warning") {
      console.log(`[FF console.${msg.type()}]`, msg.text());
    }
  });
  page.on("pageerror", (e) => console.log("[FF pageerror]", e.message));

  await page.goto("https://evey.test/home", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(8000);

  const iframes = page.locator("iframe");
  const count = await iframes.count();
  console.log(`\n=== iframes on page: ${count} ===`);
  for (let i = 0; i < count; i++) {
    const f = iframes.nth(i);
    const title = await f.getAttribute("title");
    const srcdoc = await f.getAttribute("srcdoc");
    console.log(`\n--- iframe[${i}] title=${title} ---`);
    if (srcdoc) {
      console.log("srcdoc length:", srcdoc.length);
      // Print just the head section
      const head = srcdoc.match(/<head>([\s\S]*?)<\/head>/);
      if (head) console.log("HEAD:\n" + head[1].slice(0, 2000));
      const body = srcdoc.match(/<body>([\s\S]*?)<\/body>/);
      if (body) console.log("BODY:\n" + body[1].slice(0, 600));
    }
  }

  await browser.close();
})();
