import { test, expect } from "./fixtures/auth";

const COLLECTION_NAME = `E2E-SmartCol-${Date.now()}`;

test("smart collection CRUD lifecycle", async ({ page }) => {
  // --- Page loads ---
  await page.goto("/collections", { waitUntil: "networkidle" });
  await expect(
    page.locator("h1", { hasText: "Collections" }),
  ).toBeVisible({ timeout: 15_000 });

  // --- Create ---
  await page.locator("button", { hasText: "New smart collection" }).click();

  const modal = page.locator('[role="dialog"]');
  await expect(modal).toBeVisible({ timeout: 5_000 });
  await expect(
    modal.locator("text=New smart collection"),
  ).toBeVisible({ timeout: 3_000 });

  await modal.locator("#name").fill(COLLECTION_NAME);

  // Add genre rule
  await modal.locator("button", { hasText: "Genre" }).click();
  const genreInput = modal.locator('input[placeholder="e.g. Action"]');
  await expect(genreInput).toBeVisible({ timeout: 3_000 });
  await genreInput.fill("Action");

  const [createResp] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes("/api/v1/collections") && r.request().method() === "POST",
      { timeout: 10_000 },
    ),
    modal.locator("button", { hasText: "Create" }).click(),
  ]);
  expect(createResp.status()).toBeLessThan(400);

  const body = await createResp.json();
  const collectionId = body.id;
  expect(collectionId).toBeTruthy();

  await expect(modal).not.toBeVisible({ timeout: 5_000 });
  await expect(
    page.locator("text=" + COLLECTION_NAME),
  ).toBeVisible({ timeout: 10_000 });

  // --- Detail page: rules ---
  await page.goto(`/collections/smart/${collectionId}`, { waitUntil: "networkidle" });
  await expect(
    page.locator("h1", { hasText: COLLECTION_NAME }),
  ).toBeVisible({ timeout: 15_000 });

  await expect(page.getByText("Smart", { exact: true })).toBeVisible({ timeout: 5_000 });
  await expect(page.locator("text=genre = Action")).toBeVisible({ timeout: 5_000 });

  // --- Detail page: items or empty ---
  const emptyState = page.locator("text=No results");
  const itemLink = page.locator('a[href*="/media/"]');
  await expect(emptyState.or(itemLink.first())).toBeVisible({ timeout: 10_000 });

  // --- Delete ---
  page.on("dialog", (dialog) => dialog.accept());

  const [deleteResp] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes(`/api/v1/collections/${collectionId}`) && r.request().method() === "DELETE",
      { timeout: 10_000 },
    ),
    page.locator("button", { hasText: "Delete" }).click(),
  ]);
  expect(deleteResp.status()).toBeLessThan(400);

  await page.waitForURL("**/collections", { timeout: 10_000 });
  await expect(
    page.locator("text=" + COLLECTION_NAME),
  ).not.toBeVisible({ timeout: 10_000 });
});
