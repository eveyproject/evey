import type { Locator, Page } from "@playwright/test";

const BASE = process.env.BASE_URL ?? "https://evey.test";

/** Navigate with short timeout */
export async function goto(page: Page, path: string) {
  await page.goto(path, { waitUntil: "domcontentloaded", timeout: 10_000 });
}

/**
 * Set a controlled-input's value so React's onChange actually fires.
 * Playwright's `fill` / `keyboard.type` / `pressSequentially` all bypass
 * React 18's value setter on some inputs, leaving the form state stale and
 * silently turning subsequent saves into no-ops. The native-setter +
 * synthetic 'input' event below is the canonical workaround.
 */
export async function setReactInput(input: Locator, value: string): Promise<void> {
  await input.evaluate((el, v) => {
    const node = el as HTMLInputElement;
    node.focus();
    const proto = Object.getPrototypeOf(node);
    const desc = Object.getOwnPropertyDescriptor(proto, "value");
    desc?.set?.call(node, v);
    node.dispatchEvent(new Event("input", { bubbles: true }));
    node.dispatchEvent(new Event("change", { bubbles: true }));
    node.blur();
  }, value);
}

/** Get auth token from storage state */
export async function getToken(page: Page): Promise<string> {
  // Navigate to origin first if on about:blank (localStorage inaccessible)
  if (page.url() === "about:blank") {
    await page.goto(BASE, { waitUntil: "domcontentloaded", timeout: 10_000 });
  }
  return page.evaluate(() => {
    const raw = localStorage.getItem("evey-auth");
    if (!raw) return "";
    const parsed = JSON.parse(raw);
    return parsed?.state?.accessToken ?? "";
  });
}

/** Fetch JSON with retry on 429 */
async function fetchWithRetry(page: Page, url: string, token: string, retries = 3): Promise<{ ok: boolean; data: unknown }> {
  for (let i = 0; i < retries; i++) {
    const result = await page.evaluate(async ({ u, tk }) => {
      const res = await fetch(u, { headers: { Authorization: `Bearer ${tk}` } });
      if (res.status === 429) return { ok: false, status: 429, data: null };
      if (!res.ok) return { ok: false, status: res.status, data: null };
      return { ok: true, status: res.status, data: await res.json() };
    }, { u: url, tk: token });
    if (result.ok) return { ok: true, data: result.data };
    if (result.status === 429) {
      await page.waitForTimeout(1500 * (i + 1));
      continue;
    }
    return { ok: false, data: null };
  }
  return { ok: false, data: null };
}

/** Fetch a list of valid media IDs from the API (movies + series) */
export async function fetchMediaIds(page: Page, limit = 50): Promise<string[]> {
  const token = await getToken(page);
  // small delay to let SPA initial requests settle
  await page.waitForTimeout(500);
  const movieResult = await fetchWithRetry(page, `/api/v1/media?pagination.pageSize=${limit}&type=MEDIA_ITEM_TYPE_MOVIE`, token);
  if (movieResult.ok) {
    const items = (movieResult.data as { items?: { id?: string }[] })?.items ?? [];
    const ids = items.map((i) => i.id).filter(Boolean) as string[];
    if (ids.length > 0) return ids;
  }
  // fallback: try series
  const seriesResult = await fetchWithRetry(page, `/api/v1/media?pagination.pageSize=${limit}&type=MEDIA_ITEM_TYPE_SERIES`, token);
  if (seriesResult.ok) {
    const items = (seriesResult.data as { items?: { id?: string }[] })?.items ?? [];
    return items.map((i) => i.id).filter(Boolean) as string[];
  }
  return [];
}

/** Pick a random valid media ID via API */
export async function pickRandomMediaId(page: Page): Promise<string | null> {
  const ids = await fetchMediaIds(page);
  if (ids.length === 0) return null;
  return ids[Math.floor(Math.random() * ids.length)];
}

/** Navigate to a random media detail page, return the media ID */
export async function gotoRandomMedia(page: Page): Promise<string | null> {
  const ids = await fetchMediaIds(page);
  if (ids.length === 0) return null;
  // shuffle and try IDs until one loads
  const shuffled = [...ids].sort(() => Math.random() - 0.5);
  for (const id of shuffled.slice(0, 5)) {
    await goto(page, `/media/${id}`);
    const found = await page.waitForSelector('[data-testid="media-detail"]', { timeout: 8_000 }).catch(() => null);
    if (found) return id;
  }
  return null;
}

/** Navigate to a random MOVIE detail (has file → playable) */
export async function gotoRandomMovie(page: Page): Promise<string | null> {
  const token = await getToken(page);
  await page.waitForTimeout(500);
  const result = await fetchWithRetry(page, "/api/v1/media?pagination.pageSize=50&type=MEDIA_ITEM_TYPE_MOVIE", token);
  if (!result.ok) return null;
  const items = (result.data as { items?: { id?: string; filePath?: string }[] })?.items ?? [];
  const ids = items.filter((i) => i.id && i.filePath).map((i) => i.id!) as string[];
  if (ids.length === 0) return null;
  const shuffled = [...ids].sort(() => Math.random() - 0.5);
  for (const id of shuffled.slice(0, 5)) {
    await goto(page, `/media/${id}`);
    const found = await page.waitForSelector('[data-testid="media-detail"]', { timeout: 8_000 }).catch(() => null);
    if (found) return id;
  }
  return null;
}

/** Navigate to a random SERIES detail */
export async function gotoRandomSeries(page: Page): Promise<string | null> {
  const token = await getToken(page);
  await page.waitForTimeout(500);
  const result = await fetchWithRetry(page, "/api/v1/media?pagination.pageSize=50&type=MEDIA_ITEM_TYPE_SERIES", token);
  if (!result.ok) return null;
  const items = (result.data as { items?: { id?: string }[] })?.items ?? [];
  const ids = items.map((i) => i.id).filter(Boolean) as string[];
  if (ids.length === 0) return null;
  const shuffled = [...ids].sort(() => Math.random() - 0.5);
  for (const id of shuffled.slice(0, 5)) {
    await goto(page, `/media/${id}`);
    const found = await page.waitForSelector('[data-testid="media-detail"]', { timeout: 8_000 }).catch(() => null);
    if (found) return id;
  }
  return null;
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
    // Mutating requests must echo the _csrf cookie in X-CSRF-Token (double-submit
    // pattern). Playwright's fetch sends the cookie automatically; without the
    // header the server returns 403 "csrf token missing".
    const csrf = document.cookie.match(/(?:^|;\s*)_csrf=([^;]+)/)?.[1] ?? "";
    const res = await fetch(url, {
      method: "POST",
      headers: { Authorization: `Bearer ${tk}`, "Content-Type": "application/json", "X-CSRF-Token": decodeURIComponent(csrf) },
      body: JSON.stringify(body),
    });
    return { status: res.status, body: await res.json().catch(() => null) };
  }, { url: `${BASE}${path}`, tk: token, body: data });
}

/** API helper: DELETE with auth */
export async function apiDelete(page: Page, path: string) {
  const token = await getToken(page);
  return page.evaluate(async ({ url, tk }) => {
    const csrf = document.cookie.match(/(?:^|;\s*)_csrf=([^;]+)/)?.[1] ?? "";
    const res = await fetch(url, { method: "DELETE", headers: { Authorization: `Bearer ${tk}`, "X-CSRF-Token": decodeURIComponent(csrf) } });
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
