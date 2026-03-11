import { test, expect } from "./fixtures/auth";

test.describe("Admin sessions (Now Playing)", () => {
  test("page loads with heading", async ({ page }) => {
    await page.goto("/admin/sessions", { waitUntil: "networkidle" });

    const heading = page.locator("h1", { hasText: "Now Playing" });
    await expect(heading).toBeVisible({ timeout: 15_000 });
  });

  test("shows empty state or active session list", async ({ page }) => {
    const sessionsResponse = page.waitForResponse(
      (r) => r.url().includes("/api/v1/admin/sessions/active") && r.status() === 200,
    );
    await page.goto("/admin/sessions", { waitUntil: "networkidle" });
    const res = await sessionsResponse;
    const body = await res.json();
    const sessions: unknown[] = body.sessions ?? [];

    if (sessions.length === 0) {
      // Empty state
      await expect(
        page.locator("text=No active sessions"),
      ).toBeVisible({ timeout: 10_000 });
    } else {
      // At least one session card with profile initial avatar
      await expect(
        page.locator("text=active session").first(),
      ).toBeVisible({ timeout: 10_000 });
    }
  });

  test("session cards show device info and stop button", async ({ page }) => {
    const sessionsResponse = page.waitForResponse(
      (r) => r.url().includes("/api/v1/admin/sessions/active") && r.status() === 200,
    );
    await page.goto("/admin/sessions", { waitUntil: "networkidle" });
    const res = await sessionsResponse;
    const body = await res.json();
    const sessions: unknown[] = body.sessions ?? [];

    test.skip(sessions.length === 0, "No active sessions to inspect");

    // Each session card has a stop button (StopIcon svg inside a button)
    const stopButtons = page.locator('button[title="Stop session"]');
    await expect(stopButtons.first()).toBeVisible({ timeout: 10_000 });

    // Play method badge (Direct Play / Transcode / Direct Stream)
    const badges = page.locator("text=/Direct Play|Transcode|Direct Stream/");
    await expect(badges.first()).toBeVisible({ timeout: 5_000 });
  });
});
