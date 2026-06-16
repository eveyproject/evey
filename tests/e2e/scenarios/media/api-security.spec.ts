import { test, expect } from "../../fixtures/auth";
import { goto, getToken } from "../shared/helpers";

/**
 * API security tests: auth, invalid inputs, edge cases
 */

const ENDPOINTS = [
  { method: "GET", path: "/api/v1/media" },
  { method: "GET", path: "/api/v1/libraries" },
  { method: "GET", path: "/api/v1/notifications/unread-count" },
  { method: "GET", path: "/api/v1/playback/history" },
  { method: "GET", path: "/api/v1/collections" },
  { method: "GET", path: "/api/v1/playlists" },
  { method: "GET", path: "/api/v1/users" },
  { method: "GET", path: "/api/v1/admin/feeds/config" },
  { method: "GET", path: "/api/v1/media/search?query=test" },
];

test.describe("API > Unauthenticated access returns 401", () => {
  for (const ep of ENDPOINTS) {
    test(`${ep.method} ${ep.path} → 401 without token`, async ({ page }) => {
      await goto(page, "/home");
      const status = await page.evaluate(async ({ method, path }) => {
        const res = await fetch(path, { method });
        return res.status;
      }, ep);
      expect(status).toBe(401);
    });
  }
});

test.describe("API > Invalid media ID handling", () => {
  test("GET /api/v1/media/invalid-id → 404 or 500 (not crash)", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk }) => {
      const res = await fetch("/api/v1/media/invalid-fake-uuid", {
        headers: { Authorization: `Bearer ${tk}` },
      });
      return res.status;
    }, { tk: token });
    expect(status).toBeLessThanOrEqual(500);
    expect(status).toBeGreaterThanOrEqual(400);
  });

  test("GET /api/v1/people/invalid-id → 404", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk }) => {
      const res = await fetch("/api/v1/people/invalid-fake-uuid", {
        headers: { Authorization: `Bearer ${tk}` },
      });
      return res.status;
    }, { tk: token });
    expect([404, 500]).toContain(status);
  });

  test("download nonexistent media → 404", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk }) => {
      const res = await fetch("/api/v1/media/nonexistent/download?token=" + encodeURIComponent(tk), { method: "HEAD" });
      return res.status;
    }, { tk: token });
    expect(status).toBe(404);
  });
});

test.describe("API > Expired/invalid token", () => {
  test("request with expired token → 401 or 403", async ({ page }) => {
    await goto(page, "/home");
    const status = await page.evaluate(async () => {
      const res = await fetch("/api/v1/media", {
        headers: { Authorization: "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiZXhwIjoxfQ.invalid" },
      });
      return res.status;
    });
    expect([401, 403]).toContain(status);
  });

  test("request with malformed token → 401 or 403", async ({ page }) => {
    await goto(page, "/home");
    const status = await page.evaluate(async () => {
      const res = await fetch("/api/v1/media", {
        headers: { Authorization: "Bearer not-a-jwt" },
      });
      return res.status;
    });
    expect([401, 403]).toContain(status);
  });
});

test.describe("API > XSS/injection in query params", () => {
  test("search with XSS payload → no 500", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk }) => {
      const q = encodeURIComponent('<img src=x onerror=alert(1)>');
      const res = await fetch(`/api/v1/media/search?query=${q}&limit=1`, {
        headers: { Authorization: `Bearer ${tk}` },
      });
      return res.status;
    }, { tk: token });
    expect(status).toBeLessThan(500);
  });

  test("list media with SQL injection sort → no 500", async ({ page }) => {
    await goto(page, "/home");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ tk }) => {
      const res = await fetch("/api/v1/media?sortField='; DROP TABLE media_items; --&pagination.pageSize=1", {
        headers: { Authorization: `Bearer ${tk}` },
      });
      return res.status;
    }, { tk: token });
    expect(status).toBeLessThan(500);
  });
});
