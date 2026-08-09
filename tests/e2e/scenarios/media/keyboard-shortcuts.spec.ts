import { test, expect } from "../../fixtures/auth";
import { goto, collectErrors } from "../shared/helpers";

test.describe("Keyboard shortcuts > Global", () => {
  test("/ focuses search", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(1_000);
    await page.keyboard.press("/");
    await page.waitForTimeout(300);
    const focused = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
    expect(focused).toBe("topbar-search-input");
  });

  test("Ctrl+K focuses search", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(1_000);
    await page.keyboard.press("Control+k");
    await page.waitForTimeout(300);
    const focused = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
    expect(focused).toBe("topbar-search-input");
  });

  test("Escape blurs search", async ({ page }) => {
    await goto(page, "/home");
    await page.fill('[data-testid="topbar-search-input"]', "test");
    await page.keyboard.press("Escape");
    await page.waitForTimeout(200);
    const focused = await page.evaluate(() => document.activeElement?.tagName);
    expect(focused).not.toBe("INPUT");
  });

  test("h navigates to home", async ({ page }) => {
    await goto(page, "/favorites");
    await page.waitForTimeout(1_000);
    await page.keyboard.press("h");
    await page.waitForTimeout(1_000);
    expect(page.url()).toContain("/home");
  });

  test("? opens keyboard shortcuts help", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(1_000);
    await page.keyboard.press("?");
    await page.waitForTimeout(500);
    // Match the dialog by ROLE, not by English text: the overlay title is
    // translated (the shared dev profile is often left non-English by other
    // e2e runs), and a /keyboard|shortcut/i regex silently missed a German or
    // French title and failed a working feature.
    const overlay = page.locator('[role="dialog"]');
    const visible = await overlay.isVisible({ timeout: 2_000 }).catch(() => false);
    expect(visible).toBe(true);
  });

  test("Escape closes shortcuts help", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForTimeout(500);
    await page.keyboard.press("?");
    await page.waitForTimeout(500);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(300);
    // Role, not English text - see the open test above.
    const overlay = page.locator('[role="dialog"]');
    const visible = await overlay.isVisible({ timeout: 500 }).catch(() => false);
    expect(visible).toBe(false);
  });

  test("no JS errors from keyboard shortcuts", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/home");
      await page.waitForTimeout(500);
      for (const key of ["/", "Escape", "h", "?", "Escape"]) {
        await page.keyboard.press(key);
        await page.waitForTimeout(200);
      }
    });
    expect(jsErrors).toHaveLength(0);
  });
});
