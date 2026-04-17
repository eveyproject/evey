import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

test.describe("Media > Metadata edit", () => {
  test("edit button is visible", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const editBtn = page.locator('button[title*="edit" i], button[title*="Edit" i]').first();
    await expect(editBtn).toBeVisible({ timeout: 5_000 });
  });

  test("clicking edit opens edit panel", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const editBtn = page.locator('button[title*="edit" i], button[title*="Edit" i]').first();
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
    const editBtn = page.locator('button[title*="edit" i]').first();
    await editBtn.click();
    await page.waitForTimeout(1_000);
    const titleInput = page.locator('input[value]').first();
    if (await titleInput.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const val = await titleInput.inputValue();
      // title input should have the current title or be non-empty
      expect(val.length).toBeGreaterThan(0);
    }
  });

  test("edit panel has save button", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForTimeout(1_000);
    const saveBtn = page.locator("button", { hasText: /save/i });
    await expect(saveBtn).toBeVisible({ timeout: 3_000 });
  });

  test("edit panel has refresh metadata button", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    await page.locator('button[title*="edit" i]').first().click();
    await page.waitForTimeout(1_000);
    const refreshBtn = page.locator("button", { hasText: /refresh/i });
    await expect(refreshBtn).toBeVisible({ timeout: 3_000 });
  });

  test("closing edit panel (click edit again) hides inputs", async ({ page }) => {
    const id = await gotoRandomMedia(page);
    test.skip(!id, "No media found");
    const editBtn = page.locator('button[title*="edit" i]').first();
    await editBtn.click();
    await page.waitForTimeout(500);
    await editBtn.click();
    await page.waitForTimeout(500);
    // save button should be gone
    const saveBtn = page.locator("button", { hasText: /save/i });
    const visible = await saveBtn.isVisible({ timeout: 1_000 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("edit mode has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      const id = await gotoRandomMedia(page);
      if (!id) return;
      await page.locator('button[title*="edit" i]').first().click();
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
