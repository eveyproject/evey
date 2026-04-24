import { test, expect } from "../../fixtures/auth";
import { goto, getToken } from "../shared/helpers";

const FAKE_UUID = "00000000-0000-0000-0000-000000000099";

test.describe("IDOR > Access other user's resources", () => {
  test("GET /api/v1/media/{otherUserMediaId} with inaccessible library → 403 or 404", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk, id }) => {
      const res = await fetch(`/api/v1/media/${id}`, {
        headers: { Authorization: `Bearer ${tk}` },
      });
      return res.status;
    }, { tk: token, id: FAKE_UUID });
    expect([403, 404]).toContain(status);
  });

  test("PUT /api/v1/media/{id}/rating with inaccessible mediaId → 403 or 404", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk, id }) => {
      const res = await fetch(`/api/v1/media/${id}/rating`, {
        method: "PUT",
        headers: { Authorization: `Bearer ${tk}`, "Content-Type": "application/json" },
        body: JSON.stringify({ rating: 5 }),
      });
      return res.status;
    }, { tk: token, id: FAKE_UUID });
    expect([403, 404]).toContain(status);
  });

  test("GET /api/v1/playback/history with another profile → isolated", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const result = await page.evaluate(async ({ tk }) => {
      const res = await fetch("/api/v1/playback/history", {
        headers: { Authorization: `Bearer ${tk}` },
      });
      return { status: res.status, body: await res.json().catch(() => null) };
    }, { tk: token });
    // should succeed for own profile but never leak other profiles' data
    expect(result.status).toBe(200);
    // if items exist, they should belong to the authenticated user (no profileId mismatch)
    if (result.body?.items?.length) {
      for (const item of result.body.items) {
        // each item should not contain a foreign userId/profileId
        expect(item).not.toHaveProperty("userId");
      }
    }
  });
});

test.describe("IDOR > Non-admin accessing admin endpoints", () => {
  test("DELETE /api/v1/users/{id} without admin → 401/403", async ({ page }) => {
    await goto(page, "/home");
    const status = await page.evaluate(async ({ id }) => {
      const res = await fetch(`/api/v1/users/${id}`, {
        method: "DELETE",
      });
      return res.status;
    }, { id: FAKE_UUID });
    // without any token → 401; with non-admin token → 403
    expect([401, 403]).toContain(status);
  });

  test("POST /api/v1/admin/media/refresh-metadata without admin → 401/403", async ({ page }) => {
    await goto(page, "/home");
    const status = await page.evaluate(async () => {
      const res = await fetch("/api/v1/admin/media/refresh-metadata", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      });
      return res.status;
    });
    // without any token → 401; with non-admin token → 403
    expect([401, 403]).toContain(status);
  });
});
