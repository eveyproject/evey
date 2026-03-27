import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Metadata read", () => {
  test("title is displayed", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const title = page.locator('[data-testid="media-title"]');
    await expect(title).toBeVisible({ timeout: 5_000 });
    const text = await title.textContent();
    expect(text!.trim().length).toBeGreaterThan(0);
  });

  test("title is not 'undefined' or 'null'", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const title = await page.locator('[data-testid="media-title"]').textContent();
    expect(title).not.toContain("undefined");
    expect(title).not.toContain("null");
  });

  test("backdrop or placeholder exists", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const backdrop = page.locator('[data-testid="media-backdrop"]');
    const placeholder = page.locator(".bg-\\[var\\(--surface\\)\\]");
    const hasBackdrop = await backdrop.isVisible({ timeout: 3_000 }).catch(() => false);
    const hasPlaceholder = await placeholder.count();
    expect(hasBackdrop || hasPlaceholder > 0).toBe(true);
  });

  test("year badge or duration is shown", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.waitForTimeout(1_000);
    // at least one metadata info should be visible
    const body = await page.locator('[data-testid="media-detail"]').textContent();
    // year (4 digits) or duration (Xh Xm) or rating
    const hasMetadata = /\d{4}|\d+h|\d+m|\d\.\d/.test(body ?? "");
    expect(hasMetadata).toBe(true);
  });

  test("no 'NaN' or 'Infinity' in metadata", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const body = await page.locator('[data-testid="media-detail"]').textContent();
    expect(body).not.toContain("NaN");
    expect(body).not.toContain("Infinity");
  });

  test("no untranslated i18n keys visible", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const body = await page.locator("body").textContent();
    // i18n keys look like "media.detail.something" or "common.something"
    const untranslated = body!.match(/\b(media|common|player|admin|auth|errors)\.[a-z]+\.[a-zA-Z]+/g) ?? [];
    // filter out false positives (URLs, code)
    const real = untranslated.filter((k) => !k.includes("http") && !k.includes("api"));
    expect(real, `Untranslated keys: ${real.join(", ")}`).toHaveLength(0);
  });

  test("page has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await gotoRandomMedia(page);
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("page has no 5xx errors", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await gotoRandomMedia(page);
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
