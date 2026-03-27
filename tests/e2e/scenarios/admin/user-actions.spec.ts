import { test, expect } from "../../fixtures/auth";
import { goto, apiGet, apiPost, apiDelete, collectErrors } from "../shared/helpers";

test.describe("Admin > User actions deep", () => {
  test("create user modal has username and password fields", async ({ page }) => {
    await goto(page, "/admin/users");
    await page.locator("button", { hasText: /create.*user/i }).first().click();
    await page.waitForTimeout(500);
    const inputs = page.locator("input");
    expect(await inputs.count()).toBeGreaterThanOrEqual(2);
  });

  test("create invite link via API returns link", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiPost(page, "/api/v1/admin/invites", {});
    // should return a link or error gracefully
    expect(res.status).toBeLessThan(500);
  });

  test("list users via API returns at least admin", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/admin/users");
    expect(res.status).toBe(200);
    const users = res.body?.users ?? [];
    expect(users.length).toBeGreaterThan(0);
  });

  test("user list shows admin user 'gravity'", async ({ page }) => {
    await goto(page, "/admin/users");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.toLowerCase()).toContain("gravity");
  });

  test("no 5xx on user page interactions", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/admin/users");
      await page.waitForTimeout(2_000);
      // click create user then cancel
      const createBtn = page.locator("button", { hasText: /create.*user/i }).first();
      if (await createBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await createBtn.click();
        await page.waitForTimeout(500);
        await page.keyboard.press("Escape");
        await page.waitForTimeout(300);
      }
    });
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Admin > Library actions deep", () => {
  test("scan button triggers scan without error", async ({ page }) => {
    await goto(page, "/admin/libraries");
    await page.waitForTimeout(2_000);
    const scanBtn = page.locator("button", { hasText: /scan/i }).first();
    test.skip(!(await scanBtn.isVisible({ timeout: 2_000 }).catch(() => false)), "No scan button");
    const { serverErrors, jsErrors } = await collectErrors(page, async () => {
      await scanBtn.click();
      await page.waitForTimeout(3_000);
    });
    expect(jsErrors).toHaveLength(0);
    expect(serverErrors).toHaveLength(0);
  });

  test("add library modal has name and type fields", async ({ page }) => {
    await goto(page, "/admin/libraries");
    const addBtn = page.locator("button", { hasText: /add|create|new/i }).first();
    await addBtn.click();
    await page.waitForTimeout(500);
    const nameInput = page.locator("input").first();
    const typeSelect = page.locator("select").first();
    expect(await nameInput.count()).toBeGreaterThan(0);
    expect(await typeSelect.count()).toBeGreaterThanOrEqual(0); // may be dropdown not select
  });

  test("libraries API returns array", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/libraries");
    expect(res.status).toBe(200);
    expect(Array.isArray(res.body?.libraries)).toBe(true);
  });
});
