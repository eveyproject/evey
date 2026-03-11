import { test, expect } from "./fixtures/auth";

const COLLECTION_NAME = `E2E-Collection-${Date.now()}`;

test.describe("Collections", () => {
  test.describe.configure({ mode: "serial" });

  let collectionId: string;

  test("collections page loads", async ({ page }) => {
    await page.goto("/collections", { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: "Collections" }),
    ).toBeVisible({ timeout: 15_000 });
  });

  test("create smart collection with genre filter", async ({ page }) => {
    await page.goto("/collections", { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: "Collections" }),
    ).toBeVisible({ timeout: 15_000 });

    // Click "New Collection" button
    await page.locator("button", { hasText: "New Collection" }).click();

    // Wait for modal
    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill name
    await modal.locator("#name").fill(COLLECTION_NAME);

    // Fill description
    await modal.locator("#description").fill("E2E test collection");

    // Type should default to "smart" — verify
    const typeSelect = modal.locator("#type");
    await expect(typeSelect).toHaveValue("smart");

    // The first rule row should already exist (genre / contains / empty value)
    // Fill the value input for the genre rule
    const ruleValueInput = modal.locator('input[placeholder="e.g. Action"]');
    await ruleValueInput.fill("Action");

    // Intercept the create API call
    const [createResp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes("/api/v1/collections") && r.request().method() === "POST",
        { timeout: 10_000 },
      ),
      modal.locator('button[type="submit"]', { hasText: "Create" }).click(),
    ]);
    expect(createResp.status()).toBeLessThan(400);

    // Extract collection ID from response
    const body = await createResp.json();
    collectionId = body.id;
    expect(collectionId).toBeTruthy();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5_000 });

    // The collection card should appear in the list
    await expect(
      page.locator("text=" + COLLECTION_NAME),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("collection detail page loads and shows rules", async ({ page }) => {
    test.skip(!collectionId, "No collection created in previous test");

    await page.goto(`/collections/${collectionId}`, { waitUntil: "networkidle" });

    // Collection name visible
    await expect(
      page.locator("h1", { hasText: COLLECTION_NAME }),
    ).toBeVisible({ timeout: 15_000 });

    // Smart badge visible
    await expect(
      page.locator("text=smart"),
    ).toBeVisible({ timeout: 5_000 });

    // Rule description should mention genre contains Action
    await expect(
      page.locator("text=/genre.*contains.*Action/"),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("collection detail shows items or empty state", async ({ page }) => {
    test.skip(!collectionId, "No collection created in previous test");

    const apiResp = page.waitForResponse(
      (r) => r.url().includes(`/api/v1/collections/${collectionId}`) && r.request().method() === "GET",
      { timeout: 10_000 },
    );
    await page.goto(`/collections/${collectionId}`, { waitUntil: "networkidle" });
    const res = await apiResp;
    const body = await res.json();
    const items: unknown[] = body.items ?? [];

    if (items.length === 0) {
      await expect(
        page.locator("text=No matching items"),
      ).toBeVisible({ timeout: 10_000 });
    } else {
      // At least one media card
      const cards = page.locator("a[href*='/media/']");
      await expect(cards.first()).toBeVisible({ timeout: 10_000 });
    }
  });

  test("edit collection modal opens and works", async ({ page }) => {
    test.skip(!collectionId, "No collection created in previous test");

    await page.goto(`/collections/${collectionId}`, { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: COLLECTION_NAME }),
    ).toBeVisible({ timeout: 15_000 });

    // Click Edit button
    await page.locator("button", { hasText: "Edit" }).click();

    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Title should say "Edit Collection"
    await expect(
      modal.locator("text=Edit Collection"),
    ).toBeVisible({ timeout: 3_000 });

    // Close without saving
    await modal.locator("button", { hasText: "Cancel" }).click();
    await expect(modal).not.toBeVisible({ timeout: 5_000 });
  });

  test("delete collection", async ({ page }) => {
    test.skip(!collectionId, "No collection created in previous test");

    await page.goto(`/collections/${collectionId}`, { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: COLLECTION_NAME }),
    ).toBeVisible({ timeout: 15_000 });

    // Handle the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    const [deleteResp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes(`/api/v1/collections/${collectionId}`) && r.request().method() === "DELETE",
        { timeout: 10_000 },
      ),
      page.locator("button", { hasText: "Delete" }).click(),
    ]);
    expect(deleteResp.status()).toBeLessThan(400);

    // Should navigate back to /collections
    await page.waitForURL("**/collections", { timeout: 10_000 });

    // Collection should no longer appear
    await expect(
      page.locator("text=" + COLLECTION_NAME),
    ).not.toBeVisible({ timeout: 10_000 });
  });
});
