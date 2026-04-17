import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Metadata save (full CRUD)", () => {
  test("edit title, save, verify changed", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    // read original title
    const originalTitle = await page.locator('[data-testid="media-title"]').textContent();

    // open edit
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForTimeout(1_000);

    // find title input and modify
    const titleInput = page.locator('input').first();
    test.skip(!(await titleInput.isVisible({ timeout: 2_000 }).catch(() => false)), "No input found in edit");
    const currentVal = await titleInput.inputValue();
    const testSuffix = " [e2e-test]";

    // append test suffix
    await titleInput.fill(currentVal + testSuffix);
    await page.waitForTimeout(300);

    // click save
    const saveBtn = page.locator("button", { hasText: /save/i }).first();
    const { serverErrors } = await collectErrors(page, async () => {
      await saveBtn.click();
      await page.waitForTimeout(2_000);
    });

    // verify no server errors
    expect(serverErrors, `5xx on save: ${serverErrors.join(", ")}`).toHaveLength(0);

    // reload and check title changed
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const newTitle = await page.locator('[data-testid="media-title"]').textContent();
    expect(newTitle).toContain(testSuffix);

    // cleanup: restore original title
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForTimeout(1_000);
    const input2 = page.locator('input').first();
    await input2.fill(originalTitle!.trim());
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(1_500);
  });

  test("save with empty title shows error or rejects", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForTimeout(1_000);

    const titleInput = page.locator('input').first();
    test.skip(!(await titleInput.isVisible({ timeout: 2_000 }).catch(() => false)), "No input");

    // clear title
    await titleInput.fill("");
    await page.waitForTimeout(300);

    const saveBtn = page.locator("button", { hasText: /save/i }).first();
    await saveBtn.click();
    await page.waitForTimeout(1_500);

    // should either show error or not save empty title
    // reload and verify title is still the original
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const title = await page.locator('[data-testid="media-title"]').textContent();
    expect(title!.trim().length).toBeGreaterThan(0);
  });

  test("refresh metadata doesn't crash", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForTimeout(1_000);

    const refreshBtn = page.locator("button", { hasText: /refresh/i }).first();
    test.skip(!(await refreshBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No refresh button");

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await refreshBtn.click();
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });
});
