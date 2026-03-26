import { test, expect } from "./fixtures/auth";

const PLAYLIST_NAME = `E2E-Playlist-${Date.now()}`;
const MOVIE_ID = process.env.MEDIA_ID ?? "577d3d4d-da77-4c8a-8e46-6bee6b64635c";

test.describe("Playlists", () => {
  test.describe.configure({ mode: "serial" });

  let playlistId: string;

  test("playlists page loads", async ({ page }) => {
    await page.goto("/playlists", { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: "Playlists" }),
    ).toBeVisible({ timeout: 15_000 });
  });

  test("create manual playlist", async ({ page }) => {
    await page.goto("/playlists", { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: "Playlists" }),
    ).toBeVisible({ timeout: 15_000 });

    // Click "+ New Playlist" button
    await page.locator("button", { hasText: "New Playlist" }).click();

    // Wait for modal
    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill name
    await modal.locator("#name").fill(PLAYLIST_NAME);

    // Fill description
    await modal.locator("#description").fill("E2E test playlist");

    // Type should default to "manual"
    const typeSelect = modal.locator("#type");
    await expect(typeSelect).toHaveValue("manual");

    // Intercept the create API call
    const [createResp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes("/api/v1/playlists") && r.request().method() === "POST",
        { timeout: 10_000 },
      ),
      modal.locator('button[type="submit"]', { hasText: "Create" }).click(),
    ]);
    expect(createResp.status()).toBeLessThan(400);

    // Extract playlist ID from response
    const body = await createResp.json();
    playlistId = body.id;
    expect(playlistId).toBeTruthy();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5_000 });

    // The playlist card should appear in the list (use link to avoid matching toast text)
    await expect(
      page.locator(`a:has-text("${PLAYLIST_NAME}")`),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("add item to playlist from media detail", async ({ page }) => {
    test.skip(!playlistId, "No playlist created in previous test");

    await page.goto(`/media/${MOVIE_ID}`, { waitUntil: "networkidle" });
    await page.waitForSelector('[data-testid="media-detail"]', { timeout: 15_000 });

    // Click "Add to Playlist" button
    await page.locator("button", { hasText: "Add to Playlist" }).click();

    // Wait for the "Add to Playlist" modal
    const modal = page.locator('[role="dialog"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Should see our playlist in the list
    const playlistBtn = modal.locator("button", { hasText: PLAYLIST_NAME });
    await expect(playlistBtn).toBeVisible({ timeout: 10_000 });

    // Click to add to that playlist
    const [addResp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes(`/api/v1/playlists/${playlistId}/items`) && r.request().method() === "POST",
        { timeout: 10_000 },
      ),
      playlistBtn.click(),
    ]);
    expect(addResp.status()).toBeLessThan(400);

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5_000 });
  });

  test("playlist detail shows added item", async ({ page }) => {
    test.skip(!playlistId, "No playlist created in previous test");

    await page.goto(`/playlists/${playlistId}`, { waitUntil: "networkidle" });

    // Playlist name visible
    await expect(
      page.locator("h1", { hasText: PLAYLIST_NAME }),
    ).toBeVisible({ timeout: 15_000 });

    // Manual badge visible
    await expect(
      page.locator("text=manual"),
    ).toBeVisible({ timeout: 5_000 });

    // At least one item row with a link to the media
    const itemLinks = page.locator(`a[href*='/media/']`);
    await expect(itemLinks.first()).toBeVisible({ timeout: 10_000 });

    const count = await itemLinks.count();
    expect(count).toBeGreaterThan(0);

    // Item count footer
    await expect(
      page.locator("text=/\\d+ items?/"),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("remove item from playlist", async ({ page }) => {
    test.skip(!playlistId, "No playlist created in previous test");

    await page.goto(`/playlists/${playlistId}`, { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: PLAYLIST_NAME }),
    ).toBeVisible({ timeout: 15_000 });

    // Click the remove button (X icon) on the first item
    const removeBtn = page.locator('button[title="Remove"]').first();
    await expect(removeBtn).toBeVisible({ timeout: 10_000 });

    const [removeResp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes(`/api/v1/playlists/${playlistId}/items/`) && r.request().method() === "DELETE",
        { timeout: 10_000 },
      ),
      removeBtn.click(),
    ]);
    expect(removeResp.status()).toBeLessThan(400);

    // Should show empty state after removal
    await expect(
      page.locator("text=No items"),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("delete playlist from list page", async ({ page }) => {
    test.skip(!playlistId, "No playlist created in previous test");

    await page.goto("/playlists", { waitUntil: "networkidle" });
    await expect(
      page.locator("h1", { hasText: "Playlists" }),
    ).toBeVisible({ timeout: 15_000 });

    // Find the playlist card (the .group div containing our playlist name)
    const playlistCard = page.locator(".group", { has: page.locator(`a:has-text("${PLAYLIST_NAME}")`) });

    // Handle the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Hover to reveal the delete button, then click it
    await playlistCard.hover();
    const deleteBtn = playlistCard.locator('button[title="Delete playlist"]');

    const [deleteResp] = await Promise.all([
      page.waitForResponse(
        (r) => r.url().includes(`/api/v1/playlists/${playlistId}`) && r.request().method() === "DELETE",
        { timeout: 10_000 },
      ),
      deleteBtn.click(),
    ]);
    expect(deleteResp.status()).toBeLessThan(400);

    // Playlist should no longer be visible
    await expect(
      page.locator(`a:has-text("${PLAYLIST_NAME}")`),
    ).not.toBeVisible({ timeout: 10_000 });
  });
});
