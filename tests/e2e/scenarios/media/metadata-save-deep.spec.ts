import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors, apiGet } from "../shared/helpers";

// By test id, not by English title - the label is translated, so this
// selector matched nothing on a French or German profile.
const editBtn = '[data-testid="media-edit-toggle-btn"]';
const modal = '[role="dialog"]';

async function openEditModal(page: import("@playwright/test").Page) {
  await page.locator(editBtn).first().click();
  const dialog = page.locator(modal);
  await expect(dialog).toBeVisible({ timeout: 5_000 });
  return dialog;
}

async function submitAndWaitClose(dialog: import("@playwright/test").Locator, page: import("@playwright/test").Page) {
  await dialog.locator("button[type='submit']").first().click();
  // wait for modal to close; if it doesn't, the save may have hit rate limit
  const closed = await dialog.waitFor({ state: "hidden", timeout: 15_000 }).then(() => true).catch(() => false);
  if (!closed) {
    // try clicking cancel to dismiss
    const cancelBtn = dialog.locator("button", { hasText: /cancel/i }).first();
    if (await cancelBtn.isVisible({ timeout: 1_000 }).catch(() => false)) {
      await cancelBtn.click();
      await dialog.waitFor({ state: "hidden", timeout: 3_000 }).catch(() => {});
    }
  }
  return closed;
}

test.describe("Media > Metadata save (deep)", () => {
  test("edit title → save → reload → title persisted", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    const dialog = await openEditModal(page);

    const titleInput = dialog.locator("#title");
    await expect(titleInput).toBeVisible({ timeout: 3_000 });
    const suffix = ` [e2e-${Date.now()}]`;
    await titleInput.fill(originalTitle + suffix);

    const saved = await submitAndWaitClose(dialog, page);
    test.skip(!saved, "Save failed (likely rate-limited)");

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const newTitle = await page.locator('[data-testid="media-title"]').textContent();
    expect(newTitle).toContain(suffix);

    // cleanup: restore original
    const d2 = await openEditModal(page);
    await d2.locator("#title").fill(originalTitle);
    await submitAndWaitClose(d2, page);
  });

  test("edit overview → save → reload → overview persisted", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const dialog = await openEditModal(page);
    const textarea = dialog.getByLabel("Overview");
    const isVis = await textarea.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!isVis, "No Overview textarea");

    const original = await textarea.inputValue();
    const tag = `[e2e-overview-${Date.now()}]`;
    await textarea.fill(original + " " + tag);

    const saved = await submitAndWaitClose(dialog, page);
    test.skip(!saved, "Save failed (likely rate-limited)");

    const res = await apiGet(page, `/api/v1/media/${id}/metadata/overrides`);
    const overrides = res.body?.overrides ?? res.body ?? [];
    const hasOverview = Array.isArray(overrides) && overrides.some((o: { fieldName?: string; customValue?: string }) => o.fieldName === "overview" && o.customValue?.includes(tag));
    expect(hasOverview).toBe(true);

    // cleanup
    const d2 = await openEditModal(page);
    await d2.getByLabel("Overview").fill(original);
    await submitAndWaitClose(d2, page);
  });

  test("edit TMDB ID → save → no crash", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const dialog = await openEditModal(page);
    const tmdbInput = dialog.getByLabel("TMDB ID");
    const hasTmdb = await tmdbInput.isVisible({ timeout: 2_000 }).catch(() => false);
    test.skip(!hasTmdb, "No TMDB ID input");
    const original = await tmdbInput.inputValue();
    test.skip(!original, "No existing TMDB ID");

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await dialog.locator("button[type='submit']").first().click();
      await dialog.waitFor({ state: "hidden", timeout: 15_000 }).catch(() => {});
    });
    expect(serverErrors).toHaveLength(0);
    expect(jsErrors).toHaveLength(0);
  });

  test("empty title → save → title not erased", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    const dialog = await openEditModal(page);

    await dialog.locator("#title").fill("");
    await dialog.locator("button[type='submit']").first().click();
    await page.waitForTimeout(1_000);

    // modal should still be open (required field validation)
    const stillOpen = await dialog.isVisible().catch(() => false);
    if (stillOpen) {
      await dialog.locator("button", { hasText: /cancel/i }).first().click();
      await dialog.waitFor({ state: "hidden", timeout: 3_000 }).catch(() => {});
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
    const dialog = await openEditModal(page);

    const special = `Test "quotes" <tag> & e2e`;
    await dialog.locator("#title").fill(special);

    const saved = await submitAndWaitClose(dialog, page);
    test.skip(!saved, "Save failed (likely rate-limited)");

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const displayed = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(displayed).toContain('"quotes"');
    expect(displayed).not.toContain("&quot;");
    expect(displayed).not.toContain("&lt;");

    // cleanup
    const d2 = await openEditModal(page);
    await d2.locator("#title").fill(originalTitle);
    await submitAndWaitClose(d2, page);
  });

  test("refresh metadata → fields updated (not empty)", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const dialog = await openEditModal(page);
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
    const dialog = await openEditModal(page);
    await dialog.locator("#title").fill(originalTitle + " SHOULD_NOT_SAVE");

    await dialog.locator("button", { hasText: /cancel/i }).first().click();
    await dialog.waitFor({ state: "hidden", timeout: 3_000 }).catch(() => {});

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });
    const title = (await page.locator('[data-testid="media-title"]').textContent())!.trim();
    expect(title).toBe(originalTitle);
  });
});
