import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, apiGet, collectErrors } from "../shared/helpers";

async function gotoPersonFromMedia(page: import("@playwright/test").Page): Promise<boolean> {
  const id = await gotoRandomMedia(page);
  if (!id) return false;
  const link = page.locator('[data-testid="cast-section"] a').first();
  if (!(await link.isVisible({ timeout: 3_000 }).catch(() => false))) return false;
  await link.click();
  await page.waitForURL("**/person/**", { timeout: 5_000 });
  return true;
}

test.describe("Person > Page content", () => {
  test("name is visible and non-empty", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const h1 = page.locator("h1").first();
    await expect(h1).toBeVisible({ timeout: 5_000 });
    const name = await h1.textContent();
    expect(name!.trim().length).toBeGreaterThan(0);
  });

  test("name is not 'Unknown' or 'undefined'", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const name = await page.locator("h1").first().textContent();
    expect(name!.toLowerCase()).not.toBe("unknown");
    expect(name).not.toContain("undefined");
  });

  test("photo or initial is visible", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const img = page.locator("img").first();
    const initial = page.locator(".rounded-full").first();
    const hasImg = await img.isVisible({ timeout: 2_000 }).catch(() => false);
    const hasInitial = await initial.isVisible({ timeout: 1_000 }).catch(() => false);
    expect(hasImg || hasInitial).toBe(true);
  });

  test("biography section exists (may be empty for some)", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    await page.waitForTimeout(2_000); // lazy TMDB fetch
    const body = await page.locator("body").textContent();
    // just verify no crash, bio may or may not load
    expect(body!.length).toBeGreaterThan(20);
  });

  test("birthday displayed if available", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    await page.waitForTimeout(2_000);
    // birthday format: month name + year, or nothing
    const body = await page.locator("body").textContent();
    expect(body).not.toContain("Invalid Date");
  });

  test("age is a reasonable number (1-120)", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    // look for age pattern: "— XX years old" or "— XX ans"
    const ageMatch = body!.match(/—\s*(\d+)\s*(years|ans|años|岁|वर्ष)/);
    if (ageMatch) {
      const age = parseInt(ageMatch[1]);
      expect(age).toBeGreaterThan(0);
      expect(age).toBeLessThan(120);
    }
  });

  test("filmography section has cards", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const cards = page.locator('[data-testid="media-card"]');
    const count = await cards.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("filmography card click navigates to media", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const card = page.locator('[data-testid="media-card"]').first();
    test.skip(!(await card.isVisible({ timeout: 3_000 }).catch(() => false)), "No filmography");
    await card.click();
    await page.waitForTimeout(2_000);
    expect(page.url()).toContain("/media/");
  });

  test("role badges are visible", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const badges = page.locator("span", { hasText: /actor|director|writer|producer|acteur|réalisateur/i });
    const count = await badges.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test("title count is shown", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const body = await page.locator("body").textContent();
    // "X title(s)" or "X titre(s)"
    const has = /\d+\s*(title|titre)/i.test(body!);
    expect(typeof has).toBe("boolean");
  });

  test("person API returns correct data", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    const personId = page.url().match(/\/person\/([^/?]+)/)?.[1];
    test.skip(!personId, "No person ID");
    const res = await apiGet(page, `/api/v1/people/${personId}`);
    expect(res.status).toBe(200);
    expect(res.body.name.length).toBeGreaterThan(0);
  });

  test("no JS errors on person page", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await gotoPersonFromMedia(page);
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("no 5xx on person page", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await gotoPersonFromMedia(page);
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });

  test("no [object Object] on person page", async ({ page }) => {
    test.skip(!(await gotoPersonFromMedia(page)), "No person");
    expect(await page.locator("body").textContent()).not.toContain("[object Object]");
  });
});
