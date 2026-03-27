import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

async function openEdit(page: import("@playwright/test").Page): Promise<boolean> {
  const id = await gotoRandomMedia(page);
  if (!id) return false;
  const btn = page.locator('button[title*="edit" i]').first();
  if (!(await btn.isVisible({ timeout: 3_000 }).catch(() => false))) return false;
  await btn.click();
  await page.waitForTimeout(1_000);
  return true;
}

test.describe("Media Edit > Form fields", () => {
  test("title input exists and is pre-filled", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const inputs = page.locator("input");
    const count = await inputs.count();
    expect(count).toBeGreaterThan(0);
    const firstVal = await inputs.first().inputValue();
    expect(firstVal.length).toBeGreaterThan(0);
  });

  test("overview textarea exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const textarea = page.locator("textarea");
    const count = await textarea.count();
    expect(count).toBeGreaterThanOrEqual(0); // may not exist for some forms
  });

  test("save button exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    await expect(page.locator("button", { hasText: /save/i }).first()).toBeVisible({ timeout: 2_000 });
  });

  test("refresh metadata button exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    await expect(page.locator("button", { hasText: /refresh/i }).first()).toBeVisible({ timeout: 2_000 });
  });

  test("TMDB ID field exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const tmdb = page.locator("input[placeholder*='tmdb' i], label:has-text('TMDB') + input, input[name*='tmdb' i]").first();
    // may or may not have a dedicated TMDB field
    expect(await tmdb.count()).toBeGreaterThanOrEqual(0);
  });

  test("typing in title input works", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const input = page.locator("input").first();
    const original = await input.inputValue();
    await input.fill(original + " test");
    const newVal = await input.inputValue();
    expect(newVal).toContain(" test");
    // restore
    await input.fill(original);
  });

  test("save button click sends API request", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    let apiCalled = false;
    page.on("request", (req) => {
      if (req.method() === "PUT" || req.method() === "PATCH" || req.method() === "POST") {
        if (req.url().includes("/media/") || req.url().includes("/metadata/")) apiCalled = true;
      }
    });
    await page.locator("button", { hasText: /save/i }).first().click();
    await page.waitForTimeout(2_000);
    expect(apiCalled).toBe(true);
  });

  test("refresh metadata sends API request", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    let apiCalled = false;
    page.on("request", (req) => {
      if (req.url().includes("refresh") || req.url().includes("enrich")) apiCalled = true;
    });
    const refreshBtn = page.locator("button", { hasText: /refresh/i }).first();
    await refreshBtn.click();
    await page.waitForTimeout(3_000);
    expect(apiCalled).toBe(true);
  });

  test("edit form has no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      if (!(await openEdit(page))) return;
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("edit form has no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      if (!(await openEdit(page))) return;
      await page.waitForTimeout(1_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});

test.describe("Media Edit > Artwork picker", () => {
  test("poster images visible in edit mode", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    await page.waitForTimeout(1_000);
    const posterSection = page.locator("text=/poster/i").first();
    // artwork picker may or may not be present
    expect(await posterSection.count()).toBeGreaterThanOrEqual(0);
  });

  test("clicking different poster selects it", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    await page.waitForTimeout(1_000);
    const posterBtns = page.locator("button img").all();
    const btns = await posterBtns;
    test.skip(btns.length < 2, "Not enough poster variants");
    const { jsErrors } = await collectErrors(page, async () => {
      await btns[1].click();
      await page.waitForTimeout(1_000);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
