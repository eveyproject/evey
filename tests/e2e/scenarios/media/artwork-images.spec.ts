import { test, expect } from "../../fixtures/auth";
import { gotoRandomMedia, goto } from "../shared/helpers";

test.describe("Media > Artwork & Images", () => {
  test("poster loads without error (not broken)", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const poster = page.locator('[data-testid="media-poster"] img, img[alt]').first();
    if (await poster.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const broken = await poster.evaluate((el: HTMLImageElement) => el.naturalWidth === 0);
      expect(broken).toBe(false);
    }
  });

  test("backdrop loads without error", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const backdrop = page.locator('[data-testid="media-backdrop"]');
    if (await backdrop.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const broken = await backdrop.evaluate((el: HTMLImageElement) => el.naturalWidth === 0);
      expect(broken).toBe(false);
    }
  });

  test("home page cards have loaded poster images", async ({ page }) => {
    await goto(page, "/home");
    await page.waitForSelector('[data-testid="media-card"]', { timeout: 10_000 });
    await page.waitForTimeout(2_000);
    const broken = await page.evaluate(() => {
      // Cards off-screen in horizontal sliders use loading="lazy" and never load
      // (naturalWidth===0) — that's lazy, not broken. Only judge in-viewport imgs.
      return Array.from(document.querySelectorAll('[data-testid="media-card"] img'))
        .filter((img: Element) => {
          const i = img as HTMLImageElement;
          if (i.offsetParent === null || i.style.display === "none" || i.naturalWidth !== 0) return false;
          const r = i.getBoundingClientRect();
          return r.top < innerHeight && r.bottom > 0 && r.left < innerWidth && r.right > 0;
        }).length;
    });
    expect(broken).toBeLessThan(3);
  });

  test("poster URL contains auth token", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const poster = page.locator('[data-testid="media-poster"]');
    if (await poster.isVisible({ timeout: 2_000 }).catch(() => false)) {
      const src = await poster.getAttribute("src");
      if (src && src.includes("/api/v1/media/")) {
        expect(src).toContain("token=");
      }
    }
  });

  test("artwork API returns valid response", async ({ page }) => {
    test.skip(!(await gotoRandomMedia(page)), "No media");
    const mediaId = page.url().match(/\/media\/([^/?]+)/)?.[1];
    test.skip(!mediaId, "No media ID");
    const resp = await page.evaluate(async (id) => {
      const token = JSON.parse(localStorage.getItem("evey-auth") ?? "{}").state?.accessToken;
      const res = await fetch(`/api/v1/media/${id}/artwork/poster?token=${encodeURIComponent(token)}`, { method: "HEAD" });
      return res.status;
    }, mediaId);
    expect([200, 404]).toContain(resp);
  });
});
