import { test, expect } from "./fixtures/auth";

async function findPersonPage(page: import("@playwright/test").Page): Promise<boolean> {
  await page.goto("/home", { waitUntil: "networkidle" });

  const mediaLinks = page.locator("a[href*='/media/']");
  const linkCount = await mediaLinks.count();

  for (let i = 0; i < Math.min(linkCount, 8); i++) {
    const href = await mediaLinks.nth(i).getAttribute("href");
    if (!href) continue;

    await page.goto(href, { waitUntil: "networkidle" });

    const castSection = page.locator('[data-testid="cast-section"]');
    const hasCast = await castSection.isVisible({ timeout: 5_000 }).catch(() => false);
    if (!hasCast) continue;

    const personLink = castSection.locator("a[href*='/person/']").first();
    const hasLink = await personLink.isVisible({ timeout: 3_000 }).catch(() => false);
    if (!hasLink) continue;

    await personLink.click();
    await page.waitForURL("**/person/**", { timeout: 10_000 });
    await page.waitForLoadState("networkidle");
    return true;
  }
  return false;
}

test.describe("Person Page", () => {
  test("person page loads with name displayed", async ({ page }) => {
    const found = await findPersonPage(page);
    if (!found) {
      test.skip(true, "No media with cast/person links found");
      return;
    }

    const name = page.locator("h1").first();
    await expect(name).toBeVisible({ timeout: 10_000 });
    const nameText = await name.textContent();
    expect(nameText!.trim().length).toBeGreaterThan(0);
  });

  test("person page shows avatar area", async ({ page }) => {
    const found = await findPersonPage(page);
    if (!found) {
      test.skip(true, "No media with cast/person links found");
      return;
    }

    // Avatar circle (photo or initial letter)
    const avatar = page.locator(".w-32.h-32");
    await expect(avatar).toBeVisible({ timeout: 10_000 });
  });

  test("person page shows title count", async ({ page }) => {
    const found = await findPersonPage(page);
    if (!found) {
      test.skip(true, "No media with cast/person links found");
      return;
    }

    // Wait for filmography data to load
    await page.waitForLoadState("networkidle");

    // "N title(s)" text
    const titleCount = page.locator("text=/\\d+ titles?/");
    await expect(titleCount).toBeVisible({ timeout: 10_000 });
  });

  test("filmography sections or loading state visible", async ({ page }) => {
    const found = await findPersonPage(page);
    if (!found) {
      test.skip(true, "No media with cast/person links found");
      return;
    }

    await page.waitForLoadState("networkidle");

    // Either filmography sections, "No filmography found", or spinner
    const sections = page.locator("section");
    const noFilmo = page.getByText("No filmography found");
    const sectionCount = await sections.count();
    const hasNoFilmo = await noFilmo.isVisible().catch(() => false);

    expect(sectionCount > 0 || hasNoFilmo).toBe(true);
  });

  test("filmography items are clickable links", async ({ page }) => {
    const found = await findPersonPage(page);
    if (!found) {
      test.skip(true, "No media with cast/person links found");
      return;
    }

    await page.waitForLoadState("networkidle");

    const sections = page.locator("section");
    const sectionCount = await sections.count();
    if (sectionCount === 0) {
      test.skip(true, "No filmography sections found");
      return;
    }

    const filmoLink = sections.first().locator("a[href*='/media/']").first();
    const hasLinks = await filmoLink.isVisible().catch(() => false);
    if (!hasLinks) {
      test.skip(true, "No media links in filmography");
      return;
    }

    await filmoLink.click();
    await page.waitForURL("**/media/**", { timeout: 10_000 });
  });
});
