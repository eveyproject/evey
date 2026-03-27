import { test, expect } from "../../fixtures/auth";
import { goto, apiGet, apiPost, apiDelete, collectErrors } from "../shared/helpers";

test.describe("Collections > Full CRUD", () => {
  let collectionId: string | null = null;
  const collName = `E2E-Coll-${Date.now()}`;

  test("create collection via API", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiPost(page, "/api/v1/collections", {
      name: collName,
      type: "manual",
    });
    expect(res.status).toBeLessThan(500);
    collectionId = res.body?.id ?? null;
  });

  test("collections page lists the new collection", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(collName);
  });

  test("collection detail page loads", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, `/collections/${collectionId}`);
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.length).toBeGreaterThan(10);
  });

  test("collection detail has name", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, `/collections/${collectionId}`);
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(collName);
  });

  test("delete collection via API", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/home");
    const res = await apiDelete(page, `/api/v1/collections/${collectionId}`);
    expect(res.status).toBeLessThan(500);
  });

  test("deleted collection no longer listed", async ({ page }) => {
    test.skip(!collectionId, "No collection");
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).not.toContain(collName);
  });
});

test.describe("Collections > Page stability", () => {
  test("collections page loads", async ({ page }) => {
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const h = page.locator("h1, h2").first();
    await expect(h).toBeVisible({ timeout: 5_000 });
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/collections");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/collections");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
