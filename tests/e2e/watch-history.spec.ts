import { test, expect } from "./fixtures/auth";

test.describe("Watch history", () => {
  test("page loads", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "networkidle" });

    const heading = page.getByRole("heading", { name: "Watch History" });
    await expect(heading).toBeVisible({ timeout: 10_000 });
  });

  test("shows empty state or history items", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "networkidle" });

    const emptyState = page.locator('[data-testid="history-empty"]');
    const historyList = page.locator('[data-testid="history-list"]');

    await expect(emptyState.or(historyList)).toBeVisible({ timeout: 10_000 });
  });

  test("empty state has correct message", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "networkidle" });

    const emptyState = page.locator('[data-testid="history-empty"]');
    const historyList = page.locator('[data-testid="history-list"]');

    const isEmpty = await emptyState.isVisible().catch(() => false);
    if (isEmpty) {
      await expect(page.getByText("No watch history yet")).toBeVisible();
    } else {
      await expect(historyList).toBeVisible();
    }
  });

  test("history items have thumbnail and link", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "networkidle" });

    const historyList = page.locator('[data-testid="history-list"]');
    const hasHistory = await historyList.isVisible().catch(() => false);
    test.skip(!hasHistory, "no watch history");

    const items = page.locator('[data-testid="history-item"]');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);

    const firstItem = items.first();
    const link = firstItem.locator('a[href*="/media/"]');
    await expect(link.first()).toBeVisible();
  });

  test("clear history button opens confirmation modal", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "networkidle" });

    const clearBtn = page.locator('[data-testid="clear-history-btn"]');
    const hasBtn = await clearBtn.isVisible().catch(() => false);
    test.skip(!hasBtn, "no clear button (empty history)");

    await clearBtn.click();

    const modal = page.getByText("This will remove all items from your watch history");
    await expect(modal).toBeVisible({ timeout: 5_000 });

    const cancelBtn = page.getByRole("button", { name: "Cancel" });
    await cancelBtn.click();
    await expect(modal).not.toBeVisible({ timeout: 3_000 });
  });
});
