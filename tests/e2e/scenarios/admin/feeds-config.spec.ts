import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Admin > Feeds config", () => {
  test("feed rows are visible", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(2_000);
    // should show toggle switches for feeds
    const toggles = page.locator("button[aria-label]");
    expect(await toggles.count()).toBeGreaterThan(0);
  });

  test("toggling a feed doesn't crash", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(2_000);
    const toggle = page.locator('button[aria-label*="able" i]').first();
    test.skip(!(await toggle.isVisible({ timeout: 2_000 }).catch(() => false)), "No toggle");

    const { jsErrors } = await collectErrors(page, async () => {
      await toggle.click();
      await page.waitForTimeout(500);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("save button sends request without 5xx", async ({ page }) => {
    await goto(page, "/admin/feeds");
    const saveBtn = page.locator("button", { hasText: /save/i }).first();
    test.skip(!(await saveBtn.isVisible({ timeout: 3_000 }).catch(() => false)), "No save button");

    const { serverErrors } = await collectErrors(page, async () => {
      await saveBtn.click();
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("no UUID displayed in feed names", async ({ page }) => {
    await goto(page, "/admin/feeds");
    await page.waitForTimeout(2_000);
    const allText = await page.locator("body").textContent();
    const uuids = allText!.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi) ?? [];
    // UUIDs in URLs are ok, but displayed as text = bug
    expect(uuids.length, `UUIDs found in page: ${uuids.join(", ")}`).toBe(0);
  });
});
