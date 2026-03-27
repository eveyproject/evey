import type { Page } from "@playwright/test";

const BASE = process.env.BASE_URL ?? "https://evey.test";

/** Navigate with short timeout */
export async function goto(page: Page, path: string) {
  await page.goto(path, { waitUntil: "domcontentloaded", timeout: 10_000 });
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

/** Fetch a list of valid media IDs from the API (movies + series) */
export async function fetchMediaIds(page: Page, limit = 50): Promise<string[]> {
  const token = await getToken(page);
  const ids = await page.evaluate(async ({ tk, lim }) => {
    const res = await fetch(`/api/v1/media?pagination.pageSize=${lim}&type=MEDIA_ITEM_TYPE_MOVIE`, {
      headers: { Authorization: `Bearer ${tk}` },
    });
    if (!res.ok) return [];
    const data = await res.json();
    return (data.items ?? []).map((i: { id?: string }) => i.id).filter(Boolean) as string[];
  }, { tk: token, lim: limit });
  if (ids.length > 0) return ids;
  // fallback: try series
  return page.evaluate(async ({ tk, lim }) => {
    const res = await fetch(`/api/v1/media?pagination.pageSize=${lim}&type=MEDIA_ITEM_TYPE_SERIES`, {
      headers: { Authorization: `Bearer ${tk}` },
    });
    if (!res.ok) return [];
    const data = await res.json();
    return (data.items ?? []).map((i: { id?: string }) => i.id).filter(Boolean) as string[];
  }, { tk: token, lim: limit });
}

/** Pick a random valid media ID via API */
export async function pickRandomMediaId(page: Page): Promise<string | null> {
  const ids = await fetchMediaIds(page);
  if (ids.length === 0) return null;
  return ids[Math.floor(Math.random() * ids.length)];
}

/** Navigate to a random media detail page, return the media ID */
export async function gotoRandomMedia(page: Page): Promise<string | null> {
  const id = await pickRandomMediaId(page);
  if (!id) return null;
  await goto(page, `/media/${id}`);
  await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
  return id;
}

/** Navigate to a random MOVIE detail (has file → playable) */
export async function gotoRandomMovie(page: Page): Promise<string | null> {
  const token = await getToken(page);
  const ids = await page.evaluate(async ({ tk }) => {
    const res = await fetch("/api/v1/media?pagination.pageSize=50&type=MEDIA_ITEM_TYPE_MOVIE", {
      headers: { Authorization: `Bearer ${tk}` },
    });
    if (!res.ok) return [];
    const data = await res.json();
    return (data.items ?? []).filter((i: { id?: string; filePath?: string }) => i.id && i.filePath).map((i: { id: string }) => i.id) as string[];
  }, { tk: token });
  if (ids.length === 0) return null;
  const id = ids[Math.floor(Math.random() * ids.length)];
  await goto(page, `/media/${id}`);
  await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
  return id;
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

/** API helper: DELETE with auth */
export async function apiDelete(page: Page, path: string) {
  const token = await getToken(page);
  return page.evaluate(async ({ url, tk }) => {
    const res = await fetch(url, { method: "DELETE", headers: { Authorization: `Bearer ${tk}` } });
    return { status: res.status };
  }, { url: `${BASE}${path}`, tk: token });
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

/** Show player controls (mouse move to center) */
export async function showPlayerControls(page: Page) {
  const vp = page.viewportSize()!;
  await page.mouse.move(vp.width / 2, vp.height / 2);
  await page.waitForTimeout(500);
}
