import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Library CRUD", () => {
  test("add library button opens modal", async ({ page }) => {
    await goto(page, "/admin/libraries");
    const addBtn = page.locator("button", { hasText: /add|create|new/i }).first();
    await expect(addBtn).toBeVisible({ timeout: 5_000 });
    await addBtn.click();
    await page.waitForTimeout(500);
    // modal should show
    const modal = page.locator('[role="dialog"], .modal, [data-testid="modal"]');
    const hasModal = await modal.isVisible({ timeout: 2_000 }).catch(() => false);
    // or inline form
    const hasForm = await page.locator("input, select").count() > 1;
    expect(hasModal || hasForm).toBe(true);
  });

  test("scan library triggers API call without error", async ({ page }) => {
    await goto(page, "/admin/libraries");
    const scanBtn = page.locator("button", { hasText: /scan/i }).first();
    test.skip(!(await scanBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No scan button");

    const { serverErrors } = await collectErrors(page, async () => {
      await scanBtn.click();
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("library page has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/libraries");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
