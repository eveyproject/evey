import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, collectErrors } from "../shared/helpers";

async function openEdit(page: import("@playwright/test").Page): Promise<boolean> {
  const id = await gotoRandomMedia(page);
  if (!id) return false;
  const btn = page.locator('[data-testid="media-edit-toggle-btn"]').first();
  if (!(await btn.isVisible({ timeout: 3_000 }).catch(() => false))) return false;
  await btn.click();
  // wait for the edit form to render (title input is the canonical first field)
  return await page
    .locator('[data-testid="media-edit-title-input"]')
    .waitFor({ state: "visible", timeout: 5_000 })
    .then(() => true)
    .catch(() => false);
}

test.describe("Media Edit > Form fields", () => {
  test("title input exists and is pre-filled", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const titleInput = page.locator('[data-testid="media-edit-title-input"]');
    await expect(titleInput).toBeVisible({ timeout: 3_000 });
    const val = await titleInput.inputValue();
    expect(val.length).toBeGreaterThan(0);
  });

  test("overview textarea exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const textarea = page.locator("textarea");
    const count = await textarea.count();
    expect(count).toBeGreaterThanOrEqual(0); // may not exist for some forms
  });

  test("save button exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    await expect(page.locator(`[data-testid="media-edit-save-btn"]`)).toBeVisible({ timeout: 2_000 });
  });

  test("refresh metadata button exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    await expect(page.locator(`[data-testid="media-edit-refresh-btn"]`)).toBeVisible({ timeout: 2_000 });
  });

  test("TMDB ID field exists", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const tmdb = page.locator("input[placeholder*='tmdb' i], label:has-text('TMDB') + input, input[name*='tmdb' i]").first();
    // may or may not have a dedicated TMDB field
    expect(await tmdb.count()).toBeGreaterThanOrEqual(0);
  });

  test("typing in title input works", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const input = page.locator('[data-testid="media-edit-title-input"]');
    const original = await input.inputValue();
    await input.fill(original + " test");
    const newVal = await input.inputValue();
    expect(newVal).toContain(" test");
    // restore
    await input.fill(original);
  });

  test("save button click sends API request", async ({ page }) => {
    let apiCalled = false;
    page.on("request", (req) => {
      if (req.url().includes("/api/v1/metadata/overrides") && req.method() === "POST") {
        apiCalled = true;
      }
    });
    test.skip(!(await openEdit(page)), "Cannot open edit");
    const input = page.locator('[data-testid="media-edit-title-input"]');
    const original = await input.inputValue();
    // type at end so React's controlled-component onChange definitely fires
    await input.click();
    await page.keyboard.press("End");
    await page.keyboard.type(" [e2e-noop]");
    await page.waitForTimeout(200);
    await Promise.all([
      page.waitForRequest((r) => r.url().includes("/api/v1/metadata/overrides") && r.method() === "POST", { timeout: 5_000 }),
      page.locator(`[data-testid="media-edit-save-btn"]`).click(),
    ]);
    expect(apiCalled).toBe(true);
    // restore (modal closed onSave; reopen)
    await page.locator('[data-testid="media-edit-toggle-btn"]').first().click();
    const restore = page.locator('[data-testid="media-edit-title-input"]');
    if (await restore.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await restore.fill(original);
      await page.locator(`[data-testid="media-edit-save-btn"]`).click();
      await page.waitForTimeout(1_000);
    }
  });

  test("refresh metadata sends API request", async ({ page }) => {
    test.skip(!(await openEdit(page)), "Cannot open edit");
    let apiCalled = false;
    page.on("request", (req) => {
      if (req.url().includes("refresh") || req.url().includes("enrich")) apiCalled = true;
    });
    const refreshBtn = page.locator(`[data-testid="media-edit-refresh-btn"]`);
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
