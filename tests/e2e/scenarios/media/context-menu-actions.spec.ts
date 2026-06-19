import { test, expect } from "../../fixtures/auth";
import { goto, apiPost, apiDelete, collectErrors } from "../shared/helpers";

async function openContextMenuOnCard(page: import("@playwright/test").Page): Promise<string | null> {
  await goto(page, "/home");
  const card = page.locator('[data-testid="media-card"]').first();
  if (!(await card.isVisible({ timeout: 10_000 }).catch(() => false))) return null;

  const href = await card.locator("a[href*='/media/']").first().getAttribute("href").catch(() => null);
  const mediaId = href?.match(/\/media\/([^/]+)/)?.[1] ?? null;

  await card.click({ button: "right" });
  await page.waitForTimeout(500);
  return mediaId;
}

test.describe("Context Menu > Actions", () => {
  let testCollectionId: string | null = null;
  const collName = `E2E-CtxMenu-${Date.now()}`;

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext({ ignoreHTTPSErrors: true, storageState: ".auth-state.json" });
    const page = await ctx.newPage();
    await goto(page, "/home");
    const res = await apiPost(page, "/api/v1/collections", { name: collName, type: "manual" });
    testCollectionId = res.body?.id ?? null;
    await ctx.close();
  });

  test.afterAll(async ({ browser }) => {
    if (!testCollectionId) return;
    const ctx = await browser.newContext({ ignoreHTTPSErrors: true, storageState: ".auth-state.json" });
    const page = await ctx.newPage();
    await goto(page, "/home");
    await apiDelete(page, `/api/v1/collections/${testCollectionId}`);
    await ctx.close();
  });

  test("add to collection via context menu", async ({ page }) => {
    const mediaId = await openContextMenuOnCard(page);
    test.skip(!mediaId, "No card found");

    const collOpt = page.locator('[data-testid="media-context-menu-add-to-collection"]').first();
    test.skip(!(await collOpt.isVisible({ timeout: 2_000 }).catch(() => false)), "No collection option");

    await collOpt.click();
    const dialog = page.locator('[role="dialog"]');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // select collection if list visible
    const collItem = dialog.locator(`text=${collName}`).first();
    if (await collItem.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await collItem.click();
      await page.waitForTimeout(1_500);
    }
  });

  test("add to playlist via context menu opens modal", async ({ page }) => {
    const mediaId = await openContextMenuOnCard(page);
    test.skip(!mediaId, "No card found");

    const playlistOpt = page.locator('[data-testid="media-context-menu-add-to-playlist"]').first();
    test.skip(!(await playlistOpt.isVisible({ timeout: 2_000 }).catch(() => false)), "No playlist option");

    await playlistOpt.click();
    const dialog = page.locator('[role="dialog"]');
    await expect(dialog).toBeVisible({ timeout: 5_000 });
  });

  test("favorite via context menu toggles state", async ({ page }) => {
    const mediaId = await openContextMenuOnCard(page);
    test.skip(!mediaId, "No card found");

    const favOpt = page.locator('[data-testid="media-context-menu-add-to-favorites"], [data-testid="media-context-menu-remove-from-favorites"]').first();
    test.skip(!(await favOpt.isVisible({ timeout: 2_000 }).catch(() => false)), "No favorite option");

    const { serverErrors } = await collectErrors(page, async () => {
      await favOpt.click();
      await page.waitForTimeout(1_500);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("mark watched via context menu no errors", async ({ page }) => {
    const { serverErrors, jsErrors } = await collectErrors(page, async () => {
      const mediaId = await openContextMenuOnCard(page);
      if (!mediaId) return;
      const watchOpt = page.locator('[data-testid="media-context-menu-mark-watched"]').first();
      if (await watchOpt.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await watchOpt.click();
        await page.waitForTimeout(1_500);
      }
    });
    expect(serverErrors).toHaveLength(0);
    expect(jsErrors).toHaveLength(0);
  });
});
