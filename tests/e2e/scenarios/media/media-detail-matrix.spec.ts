import { test, expect } from "../../fixtures/auth";
import { goto, getToken, collectErrors } from "../shared/helpers";

/**
 * Media detail matrix: test N random media IDs × multiple checks
 * Generates ~10 media × 15 checks = 150 tests
 */

const NUM_MEDIA = 10;

// Fetch media IDs once before all tests
let mediaIds: string[] = [];

test.beforeAll(async ({ browser }) => {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await ctx.newPage();
  await page.goto(process.env.BASE_URL ?? "https://evey.test", { waitUntil: "domcontentloaded" });
  // inject auth
  const raw = await page.evaluate(() => localStorage.getItem("evey-auth"));
  if (!raw) {
    await ctx.close();
    return;
  }
  const token = JSON.parse(raw).state?.accessToken;
  if (!token) { await ctx.close(); return; }
  const res = await page.evaluate(async (tk) => {
    const r = await fetch(`/api/v1/media?pagination.pageSize=50&type=MEDIA_ITEM_TYPE_MOVIE`, {
      headers: { Authorization: `Bearer ${tk}` },
    });
    if (!r.ok) return [];
    const d = await r.json();
    return (d.items ?? []).map((i: { id?: string }) => i.id).filter(Boolean);
  }, token);
  mediaIds = (res as string[]).slice(0, NUM_MEDIA);
  await ctx.close();
});

for (let i = 0; i < NUM_MEDIA; i++) {
  const label = `media[${i}]`;

  test(`${label} - detail page loads`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await expect(page.locator('[data-testid="media-detail"]')).toBeVisible({ timeout: 10_000 });
  });

  test(`${label} - title visible`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await expect(page.locator('[data-testid="media-title"]')).toBeVisible({ timeout: 10_000 });
  });

  test(`${label} - title non-empty`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    const t = await page.locator('[data-testid="media-title"]').textContent();
    expect(t!.trim().length).toBeGreaterThan(0);
  });

  test(`${label} - no NaN`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await page.waitForTimeout(1_000);
    expect(await page.locator("body").textContent()).not.toContain("NaN");
  });

  test(`${label} - no [object Object]`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await page.waitForTimeout(1_000);
    expect(await page.locator("body").textContent()).not.toContain("[object Object]");
  });

  test(`${label} - no untranslated keys`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await page.waitForTimeout(1_000);
    const body = await page.locator("body").textContent();
    const keys = (body!.match(/\b(?:media|common|player)\.[a-z]+\.[a-zA-Z]+/g) ?? [])
      .filter(k => !k.includes("http"));
    expect(keys).toHaveLength(0);
  });

  test(`${label} - favorite button exists`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const btn = page.locator('[data-testid="favorite-btn"], [data-testid="favorite-btn-active"]');
    await expect(btn.first()).toBeVisible({ timeout: 5_000 });
  });

  test(`${label} - watched button exists`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const btn = page.locator('[data-testid="mark-watched-btn"], [data-testid="mark-unwatched-btn"]');
    await expect(btn.first()).toBeVisible({ timeout: 5_000 });
  });

  test(`${label} - edit button exists`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, `/media/${mediaIds[i]}`);
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    // By test id, not by English title - the label is translated.
    await expect(page.locator('[data-testid="media-edit-toggle-btn"]').first()).toBeVisible({ timeout: 5_000 });
  });

  test(`${label} - no JS errors`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, `/media/${mediaIds[i]}`);
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test(`${label} - no 5xx`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, `/media/${mediaIds[i]}`);
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test(`${label} - API returns valid data`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, "/home");
    const token = await getToken(page);
    const res = await page.evaluate(async ({ id, tk }) => {
      const r = await fetch(`/api/v1/media/${id}`, { headers: { Authorization: `Bearer ${tk}` } });
      return { status: r.status, body: await r.json() };
    }, { id: mediaIds[i], tk: token });
    expect(res.status).toBe(200);
    expect(res.body.title?.length).toBeGreaterThan(0);
  });

  test(`${label} - artwork endpoint responds`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ id, tk }) => {
      const r = await fetch(`/api/v1/media/${id}/artwork/poster?token=${encodeURIComponent(tk)}`, { method: "HEAD" });
      return r.status;
    }, { id: mediaIds[i], tk: token });
    expect([200, 404]).toContain(status);
  });

  test(`${label} - chapters endpoint responds`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ id, tk }) => {
      const r = await fetch(`/api/v1/media/${id}/chapters?token=${encodeURIComponent(tk)}`);
      return r.status;
    }, { id: mediaIds[i], tk: token });
    expect([200, 404]).toContain(status);
  });

  test(`${label} - streams endpoint responds`, async ({ page }) => {
    test.skip(!mediaIds[i], "Not enough media");
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ id, tk }) => {
      const r = await fetch(`/api/v1/media/${id}/playback-info?token=${encodeURIComponent(tk)}`);
      return r.status;
    }, { id: mediaIds[i], tk: token });
    expect(status).toBeLessThan(500);
  });
}
