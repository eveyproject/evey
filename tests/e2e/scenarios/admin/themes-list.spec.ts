import { request as playwrightRequest } from "@playwright/test";
import { test, expect } from "../../fixtures/auth";
import { getAccessToken } from "../../helpers/plugin-helpers";

// Themes are listed through `GET /api/v1/plugins/themes` which the host
// frontend hits via the regular auth middleware. The endpoint is not covered
// elsewhere in the suite — this spec asserts the response contract
// (200 + `{ themes: [{id, name}] }` shape) and that unauthenticated callers
// are rejected before they can enumerate installed themes.

const BASE_URL = process.env.BASE_URL ?? "https://evey.test";

test.describe("Admin > Themes list", () => {
  test("authenticated GET returns 200 + themes array", async ({ page }) => {
    const token = await getAccessToken(page);
    const api = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
    try {
      const res = await api.get(`${BASE_URL}/api/v1/plugins/themes`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      expect(res.status()).toBe(200);
      const body = (await res.json()) as { themes?: Array<{ id?: string; name?: string }> };
      expect(Array.isArray(body.themes)).toBe(true);
      for (const t of body.themes ?? []) {
        expect(typeof t.id).toBe("string");
        expect(t.id?.length).toBeGreaterThan(0);
        expect(typeof t.name).toBe("string");
        expect(t.name?.length).toBeGreaterThan(0);
      }
    } finally {
      await api.dispose();
    }
  });

  test("unauthenticated request is rejected before enumeration", async () => {
    const api = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
    try {
      const res = await api.get(`${BASE_URL}/api/v1/plugins/themes`);
      // 4xx (typically 401 from the http auth middleware) — must never be 200
      // (would mean themes were listed to an anonymous caller) and must not be
      // 5xx (would mean the endpoint is degraded on missing auth).
      expect(res.status()).toBeGreaterThanOrEqual(400);
      expect(res.status()).toBeLessThan(500);
    } finally {
      await api.dispose();
    }
  });
});
