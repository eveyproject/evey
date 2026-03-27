import { test, expect } from "../../fixtures/auth";
import { goto, gotoRandomMedia, apiGet, apiPost, apiDelete, getToken, collectErrors } from "../shared/helpers";

test.describe("Media > Playlist full CRUD", () => {
  let playlistId: string | null = null;
  let mediaId: string | null = null;

  test("create playlist via API", async ({ page }) => {
    await goto(page, "/home"); // need page context for API calls
    const res = await apiPost(page, "/api/v1/playlists", { name: `E2E-Playlist-${Date.now()}`, type: "manual" });
    expect(res.status).toBe(200);
    playlistId = res.body?.id ?? null;
    expect(playlistId).toBeTruthy();
  });

  test("playlist appears on /playlists page", async ({ page }) => {
    test.skip(!playlistId, "No playlist created");
    await goto(page, "/playlists");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain("E2E-Playlist");
  });

  test("add media to playlist from context menu", async ({ page }) => {
    test.skip(!playlistId, "No playlist");
    mediaId = await gotoRandomMedia(page);
    test.skip(!mediaId, "No media");
    // go back to home to use context menu on a card
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const playlistOption = page.locator("text=/playlist/i").first();
    test.skip(!(await playlistOption.isVisible({ timeout: 2_000 }).catch(() => false)), "No playlist option");
    await playlistOption.click();
    await page.waitForTimeout(1_000);
    // modal should show playlists
    const modal = page.locator("text=/E2E-Playlist/i").first();
    const hasModal = await modal.isVisible({ timeout: 3_000 }).catch(() => false);
    if (hasModal) {
      await modal.click();
      await page.waitForTimeout(1_500);
    }
  });

  test("cleanup: delete playlist via API", async ({ page }) => {
    test.skip(!playlistId, "No playlist to delete");
    await goto(page, "/home");
    const res = await apiDelete(page, `/api/v1/playlists/${playlistId}`);
    expect([200, 204]).toContain(res.status);
  });
});

test.describe("Media > Collection interaction", () => {
  test("collections page loads", async ({ page }) => {
    await goto(page, "/collections");
    await page.waitForTimeout(2_000);
    const heading = page.locator("h1, h2").first();
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test("add to collection from context menu opens modal", async ({ page }) => {
    await goto(page, "/home");
    const card = page.locator('[data-testid="media-card"]').first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.click({ button: "right" });
    await page.waitForTimeout(500);
    const collOption = page.locator("text=/collection/i").first();
    test.skip(!(await collOption.isVisible({ timeout: 2_000 }).catch(() => false)), "No collection option");
    const { jsErrors } = await collectErrors(page, async () => {
      await collOption.click();
      await page.waitForTimeout(1_500);
    });
    expect(jsErrors).toHaveLength(0);
  });
});
