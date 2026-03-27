import { chromium } from "@playwright/test";
import { readFileSync } from "fs";

const state = JSON.parse(readFileSync(".auth-state.json", "utf8"));
const browser = await chromium.launch();
const ctx = await browser.newContext({
  storageState: state,
  ignoreHTTPSErrors: true,
  baseURL: "https://evey.test",
});
const page = await ctx.newPage();
await page.goto("/admin/feeds", { waitUntil: "domcontentloaded", timeout: 10000 });
await page.waitForTimeout(4000);

const bodyText = await page.evaluate(() => document.body.innerText.slice(0, 500));
console.log("Body text:", bodyText);

const allBtns = await page.evaluate(() => {
  return Array.from(document.querySelectorAll("button")).map((e) => ({
    text: e.textContent?.trim().slice(0, 40),
    label: e.getAttribute("aria-label"),
    classes: e.className.slice(0, 40),
  }));
});
console.log("All buttons:", JSON.stringify(allBtns, null, 2));

const allDraggable = await page.evaluate(() => {
  return Array.from(document.querySelectorAll("[draggable]")).map((e) => ({
    tag: e.tagName,
    draggable: e.getAttribute("draggable"),
    text: e.textContent?.trim().slice(0, 50),
  }));
});
console.log("All draggable:", JSON.stringify(allDraggable, null, 2));

await browser.close();
