import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Metadata edit", () => {
  test("edit button is visible", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const editBtn = page.locator('[data-testid="media-edit-toggle-btn"]').first();
    await expect(editBtn).toBeVisible({ timeout: 5_000 });
  });

  test("clicking edit opens edit panel", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const editBtn = page.locator('[data-testid="media-edit-toggle-btn"]').first();
    await editBtn.click();
    await page.waitForTimeout(1_000);
    // edit panel should show input fields
    const inputs = page.locator("input, textarea");
    const count = await inputs.count();
    expect(count).toBeGreaterThan(0);
  });

  test("edit panel shows title input with current value", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    const titleInput = page.locator('[data-testid="media-edit-title-input"]');
    await expect(titleInput).toBeVisible({ timeout: 5_000 });
    const val = await titleInput.inputValue();
    expect(val.length).toBeGreaterThan(0);
  });

  test("edit panel has save button", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    await page.waitForTimeout(1_000);
    const saveBtn = page.locator(`[data-testid="media-edit-save-btn"]`);
    await expect(saveBtn).toBeVisible({ timeout: 3_000 });
  });

  test("edit panel has refresh metadata button", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    await page.waitForTimeout(1_000);
    const refreshBtn = page.locator(`[data-testid="media-edit-refresh-btn"]`);
    await expect(refreshBtn).toBeVisible({ timeout: 3_000 });
  });

  test("closing edit panel via cancel hides inputs", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    const cancelBtn = page.locator('[data-testid="media-edit-cancel-btn"]');
    await expect(cancelBtn).toBeVisible({ timeout: 5_000 });
    await cancelBtn.click();
    const saveBtn = page.locator('[data-testid="media-edit-save-btn"]');
    await expect(saveBtn).toBeHidden({ timeout: 3_000 });
  });

  test("edit mode has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
