import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors, apiGet } from "../shared/helpers";

test.describe("Media > Metadata save (deep)", () => {
  test("edit title → save → reload → title persisted", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    const titleInput = page.locator("input").first();
    const suffix = ` [e2e-${Date.now()}]`;
    await titleInput.fill(originalTitle + suffix);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(2_000);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const newTitle = await page.locator('[data-testid="media-title"]').textContent();
    expect(newTitle).toContain(suffix);

    // cleanup
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });
    await page.locator("input").first().fill(originalTitle);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);
  });

  test("edit overview → save → reload → overview persisted", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    const textarea = page.locator("textarea").first();
    const isVis = await textarea.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!isVis, "No textarea for overview");

    const original = await textarea.inputValue();
    const tag = `[e2e-overview-${Date.now()}]`;
    await textarea.fill(original + " " + tag);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(2_000);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });

    // verify via API overrides
    const res = await apiGet(page, `/api/v1/media/${id}/metadata/overrides`);
    const overrides = res.body?.overrides ?? res.body ?? [];
    const hasOverview = Array.isArray(overrides) && overrides.some((o: { fieldName?: string; customValue?: string }) => o.fieldName === "overview" && o.customValue?.includes(tag));
    expect(hasOverview).toBe(true);

    // cleanup: restore
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });
    await page.locator("textarea").first().fill(original);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);
  });

  test("edit TMDB ID → save → verify via API", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    // find TMDB input by label or placeholder
    const tmdbInput = page.locator('input[name*="tmdb" i], input[placeholder*="tmdb" i]').first();
    const hasTmdb = await tmdbInput.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!hasTmdb, "No TMDB ID input in edit panel");

    const original = await tmdbInput.inputValue();
    const testId = "999999";
    await tmdbInput.fill(testId);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(2_000);

    const res = await apiGet(page, `/api/v1/media/${id}/metadata/overrides`);
    const overrides = res.body?.overrides ?? res.body ?? [];
    const hasTmdbOverride = Array.isArray(overrides) && overrides.some((o: { fieldName?: string; customValue?: string }) => o.fieldName === "tmdb_id" && o.customValue === testId);
    expect(hasTmdbOverride).toBe(true);

    // cleanup
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });
    await page.locator('input[name*="tmdb" i], input[placeholder*="tmdb" i]').first().fill(original);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_000);
  });

  test("empty title → save → title not erased", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    await page.locator("input").first().fill("");
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const title = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(title.length).toBeGreaterThan(0);
    // should still be original (server ignores empty)
    expect(title).toBe(originalTitle);
  });

  test("special chars (quotes, <>, emoji) → save → verify not escaped", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    const special = `Test "quotes" <tag> & 🎬`;
    const titleInput = page.locator("input").first();
    await titleInput.fill(special);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(2_000);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const displayed = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    // should contain raw chars, not HTML entities
    expect(displayed).toContain('"quotes"');
    expect(displayed).not.toContain("&quot;");
    expect(displayed).not.toContain("&lt;");

    // cleanup
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });
    await page.locator("input").first().fill(originalTitle);
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);
  });

  test("refresh metadata → fields updated (not empty)", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    const refreshBtn = page.locator("button", { hasText: /refresh/i }).first();
    test.skip(!(await refreshBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No refresh button");

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await refreshBtn.click();
      // wait for refresh to complete
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors, `5xx: ${serverErrors.join(", ")}`).toHaveLength(0);
    expect(jsErrors).toHaveLength(0);

    // title should not be empty after refresh
    const titleInput = page.locator("input").first();
    if (await titleInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const val = await titleInput.inputValue();
      expect(val.length).toBeGreaterThan(0);
    }
  });

  test("edit then cancel (re-click edit) → no save happened", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    const editBtn = page.locator('button[title*="edit" i]').first();
    await editBtn.click();
    await page.waitForSelector("button:has-text('Save')", { timeout: 5_000 });

    const titleInput = page.locator("input").first();
    await titleInput.fill(originalTitle + " SHOULD_NOT_SAVE");

    // close edit without saving
    await editBtn.click();
    await page.waitForTimeout(500);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const title = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(title).toBe(originalTitle);
  });
});
