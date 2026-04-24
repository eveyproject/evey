import { test, expect } from "./fixtures/auth";

test.describe("Watch history", () => {
  test("loads with heading", async ({ page }) => {
    await page.goto("/profile/history", { waitUntil: "domcontentloaded" });
    // Use a generic h1 so the assertion is locale-independent.
    const heading = page.locator("h1").first();
    await expect(heading).toBeVisible({ timeout: 10_000 });
    expect((await heading.textContent())?.trim().length ?? 0).toBeGreaterThan(0);
  });
});
