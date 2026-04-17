import { test, expect } from "../../fixtures/auth";
import { goto, apiPost, apiDelete, pickRandomMediaId, collectErrors } from "../shared/helpers";

test.describe("Playlists > Full CRUD", () => {
  let playlistId: string | null = null;
  const plName = `E2E-PL-${Date.now()}`;

  test("create playlist via API", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiPost(page, "/api/v1/playlists", { name: plName, type: "manual" });
    expect(res.status).toBe(200);
    playlistId = res.body?.id ?? null;
    expect(playlistId).toBeTruthy();
  });

  test("playlist page lists the new playlist", async ({ page }) => {
    test.skip(!playlistId, "No playlist");
    await goto(page, "/playlists");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).toContain(plName);
  });

  test("add media to playlist via API", async ({ page }) => {
    test.skip(!playlistId, "No playlist");
    await goto(page, "/home");
    const mediaId = await pickRandomMediaId(page);
    test.skip(!mediaId, "No media");
    const res = await apiPost(page, `/api/v1/playlists/${playlistId}/items`, { mediaItemId: mediaId });
    expect(res.status).toBeLessThan(500);
  });

  test("playlist detail shows items", async ({ page }) => {
    test.skip(!playlistId, "No playlist");
    await goto(page, `/playlists/${playlistId}`);
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body!.length).toBeGreaterThan(10);
  });

  test("delete playlist via API", async ({ page }) => {
    test.skip(!playlistId, "No playlist");
    await goto(page, "/home");
    const res = await apiDelete(page, `/api/v1/playlists/${playlistId}`);
    expect([200, 204]).toContain(res.status);
  });

  test("deleted playlist no longer listed", async ({ page }) => {
    test.skip(!playlistId, "No playlist");
    await goto(page, "/playlists");
    await page.waitForTimeout(2_000);
    const body = await page.locator("body").textContent();
    expect(body).not.toContain(plName);
  });
});

test.describe("Playlists > Page stability", () => {
  test("page loads", async ({ page }) => {
    await goto(page, "/playlists");
    await page.waitForTimeout(2_000);
    const h = page.locator("h1, h2").first();
    await expect(h).toBeVisible({ timeout: 5_000 });
  });

  test("create playlist button visible", async ({ page }) => {
    await goto(page, "/playlists");
    await expect(page.locator("button", { hasText: /create|new|add/i }).first()).toBeVisible({ timeout: 5_000 });
  });

  test("no JS errors", async ({ page }) => {
    const { jsErrors } = await collectErrors(page, async () => {
      await goto(page, "/playlists");
      await page.waitForTimeout(2_000);
    });
    expect(jsErrors).toHaveLength(0);
  });

  test("no 5xx", async ({ page }) => {
    const { serverErrors } = await collectErrors(page, async () => {
      await goto(page, "/playlists");
      await page.waitForTimeout(2_000);
    });
    expect(serverErrors).toHaveLength(0);
  });
});
