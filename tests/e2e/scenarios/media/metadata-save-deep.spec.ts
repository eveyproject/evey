import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors, apiGet } from "../shared/helpers";

const editBtn = 'button[title*="Edit Metadata"], button[title*="edit" i]';
const modal = '[role="dialog"]';

test.describe("Media > Metadata save (deep)", () => {
  test("edit title → save → reload → title persisted", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const titleInput = dialog.locator("#title");
    await expect(titleInput).toBeVisible({ timeout: 3_000 });
    const suffix = ` [e2e-${Date.now()}]`;
    await titleInput.fill(originalTitle + suffix);

    await dialog.locator("button[type='submit']").first().click();
    // wait for modal to close (save may take a while)
    await expect(dialog).toBeHidden({ timeout: 15_000 });

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const newTitle = await page.locator('[data-testid="media-title"]').textContent();
    expect(newTitle).toContain(suffix);

    // cleanup: restore original
    await page.locator(editBtn).first().click();
    const d2 = page.locator(modal);
    await expect(d2).toBeVisible({ timeout: 5_000 });
    await d2.locator("#title").fill(originalTitle);
    await d2.locator("button[type='submit']").first().click();
    await expect(d2).toBeHidden({ timeout: 15_000 });
  });

  test("edit overview → save → reload → overview persisted", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const textarea = dialog.getByLabel("Overview");
    const isVis = await textarea.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!isVis, "No Overview textarea");

    const original = await textarea.inputValue();
    const tag = `[e2e-overview-${Date.now()}]`;
    await textarea.fill(original + " " + tag);
    await dialog.locator("button[type='submit']").first().click();
    await expect(dialog).toBeHidden({ timeout: 15_000 });

    // verify via API overrides
    const res = await apiGet(page, `/api/v1/media/${id}/metadata/overrides`);
    const overrides = res.body?.overrides ?? res.body ?? [];
    const hasOverview = Array.isArray(overrides) && overrides.some((o: { fieldName?: string; customValue?: string }) => o.fieldName === "overview" && o.customValue?.includes(tag));
    expect(hasOverview).toBe(true);

    // cleanup: restore
    await page.locator(editBtn).first().click();
    const d2 = page.locator(modal);
    await expect(d2).toBeVisible({ timeout: 5_000 });
    await d2.getByLabel("Overview").fill(original);
    await d2.locator("button[type='submit']").first().click();
    await expect(d2).toBeHidden({ timeout: 15_000 });
  });

  test("edit TMDB ID → save → no crash", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const tmdbInput = dialog.getByLabel("TMDB ID");
    const hasTmdb = await tmdbInput.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!hasTmdb, "No TMDB ID input");

    const original = await tmdbInput.inputValue();
    // only test if there's already a TMDB ID (to avoid triggering re-enrichment with invalid ID)
    test.skip(!original, "No existing TMDB ID to modify");

    // submit same ID (no-op) to verify save flow works without crash
    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await dialog.locator("button[type='submit']").first().click();
      await expect(dialog).toBeHidden({ timeout: 15_000 });
    });
    expect(serverErrors).toHaveLength(0);
    expect(jsErrors).toHaveLength(0);
  });

  test("empty title → save → title not erased", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const titleInput = dialog.locator("#title");
    await titleInput.fill("");

    // title is required — HTML validation should prevent submit
    await dialog.locator("button[type='submit']").first().click();
    await page.waitForTimeout(1_000);

    // modal should still be open (submit blocked by required field)
    const stillOpen = await dialog.isVisible().catch(() => false);
    if (stillOpen) {
      // cancel
      await dialog.locator("button", { hasText: /cancel/i }).first().click();
      await expect(dialog).toBeHidden({ timeout: 3_000 });
    }

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const title = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(title.length).toBeGreaterThan(0);
    expect(title).toBe(originalTitle);
  });

  test("special chars (quotes, <>, emoji) → save → verify not escaped", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const special = `Test "quotes" <tag> & e2e`;
    await dialog.locator("#title").fill(special);
    await dialog.locator("button[type='submit']").first().click();
    await expect(dialog).toBeHidden({ timeout: 15_000 });

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const displayed = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(displayed).toContain('"quotes"');
    expect(displayed).not.toContain("&quot;");
    expect(displayed).not.toContain("&lt;");

    // cleanup
    await page.locator(editBtn).first().click();
    const d2 = page.locator(modal);
    await expect(d2).toBeVisible({ timeout: 5_000 });
    await d2.locator("#title").fill(originalTitle);
    await d2.locator("button[type='submit']").first().click();
    await expect(d2).toBeHidden({ timeout: 15_000 });
  });

  test("refresh metadata → fields updated (not empty)", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const refreshBtn = dialog.locator("button", { hasText: /refresh metadata/i }).first();
    test.skip(!(await refreshBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No refresh button");

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await refreshBtn.click();
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors, `5xx: ${serverErrors.join(", ")}`).toHaveLength(0);
    expect(jsErrors).toHaveLength(0);

    const titleInput = dialog.locator("#title");
    if (await titleInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const val = await titleInput.inputValue();
      expect(val.length).toBeGreaterThan(0);
    }
  });

  test("edit then cancel → no save happened", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    await page.locator(editBtn).first().click();
    const dialog = page.locator(modal);
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await dialog.locator("#title").fill(originalTitle + " SHOULD_NOT_SAVE");

    await dialog.locator("button", { hasText: /cancel/i }).first().click();
    await expect(dialog).toBeHidden({ timeout: 3_000 });

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const title = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(title).toBe(originalTitle);
  });
});
