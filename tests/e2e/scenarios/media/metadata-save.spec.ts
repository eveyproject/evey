import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Metadata save (full CRUD)", () => {
  test("edit title, save, verify changed", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    const originalTitle = (await page.locator('[data-testid="media-title"]').textContent())?.trim() ?? "";

    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    const titleInput = page.locator('[data-testid="media-edit-title-input"]');
    await expect(titleInput).toBeVisible({ timeout: 5_000 });
    const testSuffix = " [e2e-test]";

    // keyboard.type fires the React onChange; .fill bypasses it on some
    // controlled inputs and the form stays clean → save no-ops.
    await titleInput.click();
    await page.keyboard.press("End");
    await page.keyboard.type(testSuffix);
    await page.waitForTimeout(300);

    const saveBtn = page.locator(`[data-testid="media-edit-save-btn"]`);
    const { serverErrors } = await collectErrors(page, async () => {
      // wait for the override POST to settle before reload, otherwise the
      // freshly-fetched detail can race the not-yet-committed write.
      await Promise.all([
        page.waitForResponse(
          (r) => r.url().includes("/api/v1/metadata/overrides") && r.request().method() === "POST" && r.ok(),
          { timeout: 5_000 },
        ),
        saveBtn.click(),
      ]);
      await page.waitForTimeout(500);
    });
    expect(serverErrors, `5xx on save: ${serverErrors.join(", ")}`).toHaveLength(0);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const newTitle = await page.locator('[data-testid="media-title"]').textContent();
    expect(newTitle).toContain(testSuffix);

    // cleanup: restore original title
    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    const input2 = page.locator('[data-testid="media-edit-title-input"]');
    await expect(input2).toBeVisible({ timeout: 5_000 });
    await input2.click();
    await page.keyboard.press("ControlOrMeta+a");
    await page.keyboard.press("Delete");
    await page.keyboard.type(originalTitle);
    await page.locator(`[data-testid="media-edit-save-btn"]`).click();
    await page.waitForTimeout(1_500);
  });

  test("save with empty title shows error or rejects", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    const titleInput = page.locator('[data-testid="media-edit-title-input"]');
    await expect(titleInput).toBeVisible({ timeout: 5_000 });

    await titleInput.fill("");
    await page.waitForTimeout(300);

    // Save button is disabled when the form has nothing dirty AND nothing was
    // changed. `force: true` exercises the no-op path without crashing.
    await page.locator(`[data-testid="media-edit-save-btn"]`).click({ force: true });
    await page.waitForTimeout(1_500);

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 10_000 });
    const title = await page.locator('[data-testid="media-title"]').textContent();
    expect(title!.trim().length).toBeGreaterThan(0);
  });

  test("refresh metadata doesn't crash", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");

    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    await page.waitForTimeout(1_000);

    const refreshBtn = page.locator(`[data-testid="media-edit-refresh-btn"]`);
    test.skip(!(await refreshBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No refresh button");

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await refreshBtn.click();
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });
});
