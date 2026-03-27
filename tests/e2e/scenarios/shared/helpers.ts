import type { Page } from "@playwright/test";

const BASE = process.env.BASE_URL ?? "https://evey.test";
const USER = process.env.EVEY_USER ?? "gravity";
const PASS = process.env.EVEY_PASS ?? "REDACTED";

/** Navigate with short timeout */
export async function goto(page: Page, path: string) {
  await page.goto(path, { waitUntil: "domcontentloaded", timeout: 10_000 });
}

/** Get a random media ID from the home page */
export async function pickRandomMediaId(page: Page): Promise<string | null> {
  await goto(page, "/home");
  await page.waitForSelector('[data-testid="media-card"]', { timeout: 10_000 });
  const cards = page.locator('[data-testid="media-card"]');
  const count = await cards.count();
  if (count === 0) return null;
  const idx = Math.floor(Math.random() * count);
  const href = await cards.nth(idx).getAttribute("href");
  if (!href) return null;
  const match = href.match(/\/media\/([^/]+)/);
  return match ? match[1] : null;
}

/** Navigate to a random media detail page, return the media ID */
export async function gotoRandomMedia(page: Page): Promise<string | null> {
  const id = await pickRandomMediaId(page);
  if (!id) return null;
  await goto(page, `/media/${id}`);
  await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
  return id;
}

/** Get auth token from storage state */
export async function getToken(page: Page): Promise<string> {
  return page.evaluate(() => {
    const raw = localStorage.getItem("evey-auth");
    if (!raw) return "";
    const parsed = JSON.parse(raw);
    return parsed?.state?.accessToken ?? "";
  });
}

/** API helper: GET with auth */
export async function apiGet(page: Page, path: string) {
  const token = await getToken(page);
  return page.evaluate(async ({ url, tk }) => {
    const res = await fetch(url, { headers: { Authorization: `Bearer ${tk}` } });
    return { status: res.status, body: await res.json().catch(() => null) };
  }, { url: `${BASE}${path}`, tk: token });
}

/** API helper: POST with auth */
export async function apiPost(page: Page, path: string, data: unknown) {
  const token = await getToken(page);
  return page.evaluate(async ({ url, tk, body }) => {
    const res = await fetch(url, {
      method: "POST",
      headers: { Authorization: `Bearer ${tk}`, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return { status: res.status, body: await res.json().catch(() => null) };
  }, { url: `${BASE}${path}`, tk: token, body: data });
}

/** Collect JS errors and 5xx during a callback */
export async function collectErrors(page: Page, fn: () => Promise<void>) {
  const jsErrors: string[] = [];
  const serverErrors: string[] = [];
  const onError = (e: Error) => jsErrors.push(e.message);
  const onResponse = (r: { status: () => number; url: () => string }) => {
    if (r.status() >= 500) serverErrors.push(`${r.status()} ${r.url()}`);
  };
  page.on("pageerror", onError);
  page.on("response", onResponse);
  await fn();
  page.off("pageerror", onError);
  page.off("response", onResponse);
  return { jsErrors, serverErrors };
}
