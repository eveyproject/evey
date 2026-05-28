import { request as playwrightRequest } from "@playwright/test";
import { test, expect } from "../../fixtures/auth";
import { getAccessToken } from "../../helpers/plugin-helpers";

// Coverage for two endpoints touched in the 2026-05-28 overnight batch:
//   * GET /api/v1/push/devices       — ListDevices used to return 501
//                                       (Unimplemented), now returns 200 with
//                                       an empty list so the profile Devices
//                                       tab never 5xxes (fix 45b976002).
//   * GET /api/v1/i18n/{locale}       — backend-served translations, must
//                                       resolve every supported locale and
//                                       include the new keys added tonight
//                                       (commits a7aba91c2 / 345350234 /
//                                       2b19413a2 / 0cd28d6ac).

const BASE_URL = process.env.BASE_URL ?? "https://evey.test";

test.describe("Push > Devices list (deferred-feature graceful stub)", () => {
  test("authenticated GET returns 200 with an empty devices array (no 501)", async ({ page }) => {
    const token = await getAccessToken(page);
    const api = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
    try {
      const res = await api.get(`${BASE_URL}/api/v1/push/devices`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      expect(res.status()).toBe(200);
      const body = (await res.json()) as { devices?: unknown };
      // Either an empty array or the field is omitted entirely (proto3 omits
      // empty repeated fields). Both are accepted; we forbid any populated
      // entry because native device-token registration isn't wired yet.
      const devices = (body.devices as unknown[] | undefined) ?? [];
      expect(Array.isArray(devices)).toBe(true);
      expect(devices.length).toBe(0);
    } finally {
      await api.dispose();
    }
  });

  test("unauthenticated GET is rejected (not 501)", async () => {
    const api = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
    try {
      const res = await api.get(`${BASE_URL}/api/v1/push/devices`);
      // Must be a 4xx auth rejection, never the old 501 nor 5xx.
      expect(res.status()).toBeGreaterThanOrEqual(400);
      expect(res.status()).toBeLessThan(500);
    } finally {
      await api.dispose();
    }
  });
});

test.describe("i18n > backend translations contract", () => {
  // The host frontend loads `/api/v1/i18n/{lng}` and grep-asserts the absence
  // of raw `key.with.dots` strings in the rendered DOM. The endpoint must
  // resolve every supported locale and surface the keys we shipped tonight.
  const LOCALES = ["en", "de", "es", "fr", "hi", "zh"] as const;
  // A handful of keys added in this batch that the smoke endpoint must serve.
  // We don't assert exact translations — only presence + a non-empty string.
  const REQUIRED_KEYS = [
    "auth.settings.parental.pin.changeTitle",
    "auth.settings.parental.pin.currentPin",
    "admin.serverConfig.parserTokensTitle",
    "admin.plugin.update",
    "media.photos.lightbox",
  ];

  for (const locale of LOCALES) {
    test(`GET /api/v1/i18n/${locale} returns flat map with the new keys`, async () => {
      const api = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
      try {
        const res = await api.get(`${BASE_URL}/api/v1/i18n/${locale}`);
        expect(res.status()).toBe(200);
        const body = (await res.json()) as Record<string, unknown>;
        expect(typeof body).toBe("object");
        // Sanity: at least 100 keys (real catalog has 1700+).
        expect(Object.keys(body).length).toBeGreaterThan(100);
        for (const k of REQUIRED_KEYS) {
          const v = body[k];
          expect(typeof v, `${locale}:${k} must be a string`).toBe("string");
          expect((v as string).length, `${locale}:${k} must be non-empty`).toBeGreaterThan(0);
        }
      } finally {
        await api.dispose();
      }
    });
  }
});
