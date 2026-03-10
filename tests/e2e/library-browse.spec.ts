import { test, expect } from "./fixtures/auth";

test.describe("Library browse", () => {
  test("library index loads", async ({ page }) => {
    await page.goto("/library", { waitUntil: "networkidle" });

    // should show library content or redirect/show libraries
    const hasContent = await page.locator(
      'h1, h2, [class*="card"], [class*="Card"], [class*="grid"]',
    ).count();
    expect(hasContent).toBeGreaterThan(0);
  });
});
