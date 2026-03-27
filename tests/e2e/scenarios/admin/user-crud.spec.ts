import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > User management", () => {
  test("create user modal opens", async ({ page }) => {
    await goto(page, "/admin/users");
    const createBtn = page.locator("button", { hasText: /create.*user|add.*user/i }).first();
    await expect(createBtn).toBeVisible({ timeout: 5_000 });
    await createBtn.click();
    await page.waitForTimeout(500);
    // should see username/password inputs
    const inputs = page.locator("input");
    expect(await inputs.count()).toBeGreaterThan(0);
  });

  test("create invite link button works", async ({ page }) => {
    await goto(page, "/admin/users");
    const inviteBtn = page.locator("button", { hasText: /invite/i }).first();
    await expect(inviteBtn).toBeVisible({ timeout: 5_000 });

    const { jsErrors, serverErrors } = await collectErrors(page, async () => {
      await inviteBtn.click();
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });

  test("admin user is listed", async ({ page }) => {
    await goto(page, "/admin/users");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.toLowerCase()).toContain("gravity");
  });

  test("page has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/users");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
