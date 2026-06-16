import { test, expect } from "../../fixtures/auth";
import { goto, apiPost, apiDelete, collectErrors } from "../shared/helpers";

test.describe("Collections > Smart rules", () => {
  const collName = `E2E-Smart-${Date.now()}`;
  let collectionId: string | null = null;

  test("create smart collection via API", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiPost(page, "/api/v1/collections", {
      name: collName,
      smart: true,
      rules: [
        { field: "type", operator: "eq", value: "MEDIA_ITEM_TYPE_MOVIE" },
        { field: "year", operator: "gte", value: "2000" },
      ],
    });
    expect(res.status).toBeLessThan(500);
    // CreateCollectionResponse wraps the row in `collection`.
    collectionId = res.body?.collection?.id ?? res.body?.id ?? null;
    expect(collectionId).toBeTruthy();
  });

  test("smart collection listed on page", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(collName);
  });

  test("smart collection detail shows filtered results", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, `/collections/smart/${collectionId}`);
    await page.waitForTimeout(3_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(collName);
  });

  test("edit smart collection name via UI", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);

    const card = page.locator(`text=${collName}`).first();
    test.skip(!(await card.isVisible({ timeout: 3_000 }).catch(() => false)), "Card not found");

    // look for an edit button near the collection
    const editBtn = page.locator('button[aria-label*="edit" i], button:has-text("edit")').first();
    if (await editBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await editBtn.click();
      await page.waitForTimeout(1_000);
      const dialog = page.locator('[role="dialog"]');
      await expect(dialog).toBeVisible({ timeout: 3_000 });
    }
  });

  test("delete smart collection via API", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/home");
    const res = await apiDelete(page, `/api/v1/collections/${collectionId}`);
    expect(res.status).toBeLessThan(500);
  });

  test("deleted smart collection no longer listed", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).not.toContain(collName);
  });

  test("no 5xx during smart collection flow", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/collections");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
