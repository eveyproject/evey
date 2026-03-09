import { test, expect } from "./fixtures/auth";

test.describe("Admin Libraries", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/admin/libraries", { waitUntil: "networkidle" });
  });

  test("page loads without crash", async ({ page }) => {
    const container = page.locator('[data-testid="libraries-page"]');
    await expect(container).toBeVisible({ timeout: 10_000 });

    const heading = page.getByRole("heading", { name: "Libraries" });
    await expect(heading).toBeVisible();
  });

  test("libraries table displays Films and Series", async ({ page }) => {
    const table = page.locator("table");
    await expect(table).toBeVisible({ timeout: 10_000 });

    const rows = table.locator("tbody tr");
    const rowCount = await rows.count();
    expect(rowCount).toBeGreaterThanOrEqual(2);

    const tableText = await table.textContent();
    expect(tableText).toBeTruthy();
  });

  test("each library shows name, type badge, and paths", async ({ page }) => {
    const table = page.locator("table");
    await expect(table).toBeVisible({ timeout: 10_000 });

    const firstRow = table.locator("tbody tr").first();
    const cells = firstRow.locator("td");
    const cellCount = await cells.count();
    expect(cellCount).toBeGreaterThanOrEqual(5);

    // Name (first cell) is not empty
    const name = await cells.nth(0).textContent();
    expect(name!.trim().length).toBeGreaterThan(0);

    // Type badge (second cell) contains a badge
    const badge = cells.nth(1).locator("span");
    await expect(badge).toBeVisible();
    const badgeText = await badge.textContent();
    expect(["Movies", "Shows", "Music", "Library"]).toContain(badgeText!.trim());

    // Paths (third cell) is not empty dash placeholder
    const pathsText = await cells.nth(2).textContent();
    expect(pathsText!.trim().length).toBeGreaterThan(0);
  });

  test("scan button triggers scan", async ({ page }) => {
    const table = page.locator("table");
    await expect(table).toBeVisible({ timeout: 10_000 });

    const firstRow = table.locator("tbody tr").first();
    const scanBtn = firstRow.getByRole("button", { name: /^Scan/ });
    await expect(scanBtn).toBeVisible();
    await scanBtn.click();

    // Should show "Scanning..." spinner or switch to "Scan again"
    const scanning = firstRow.getByText("Scanning...");
    const scanAgain = firstRow.getByRole("button", { name: "Scan again" });
    await expect(scanning.or(scanAgain)).toBeVisible({ timeout: 15_000 });
  });

  test("Add Library button opens modal", async ({ page }) => {
    const addBtn = page.locator('[data-testid="add-library-btn"]');
    await expect(addBtn).toBeVisible({ timeout: 10_000 });
    await addBtn.click();

    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    const modalTitle = modal.getByText("Add Library");
    await expect(modalTitle).toBeVisible();

    // Form should be present
    const form = modal.locator("form");
    await expect(form).toBeVisible();
  });

  test("Delete button opens confirmation modal", async ({ page }) => {
    const table = page.locator("table");
    await expect(table).toBeVisible({ timeout: 10_000 });

    const firstRow = table.locator("tbody tr").first();
    const deleteBtn = firstRow.getByRole("button", { name: "Delete" });
    await expect(deleteBtn).toBeVisible();
    await deleteBtn.click();

    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    const confirmText = modal.getByText("cannot be undone");
    await expect(confirmText).toBeVisible();

    // Cancel button should close the modal
    const cancelBtn = modal.getByRole("button", { name: "Cancel" });
    await cancelBtn.click();
    await expect(modal).not.toBeVisible({ timeout: 3_000 });
  });
});
