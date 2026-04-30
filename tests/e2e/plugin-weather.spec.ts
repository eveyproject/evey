// E2E for the weather plugin: install zip, enable, navigate to /home,
// assert the iframe loads its bundle without console errors and the
// canvas renders. Uses the auth state from globalSetup.

import { test, expect } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";
import type { APIRequestContext } from "@playwright/test";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const STORAGE_STATE = path.join(__dirname, ".auth-state.json");
const ZIP_PATH = path.resolve(__dirname, "../../tests/plugins/weather.zip");
const BASE_URL = process.env.BASE_URL ?? "https://evey.test";

test.use({
  storageState: STORAGE_STATE,
  ignoreHTTPSErrors: true,
});

function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` };
}

// Extract a fresh access token by hitting /refresh with the cookies +
// CSRF token Playwright inherits from storageState (set by globalSetup).
async function getAccessToken(request: APIRequestContext): Promise<string> {
  const cookies = await request.storageState().then((s) => s.cookies);
  const csrfCookie = cookies.find((c) => c.name === "_csrf" || c.name === "_csrf_token" || c.name === "csrf_token");
  const headers: Record<string, string> = {};
  if (csrfCookie) headers["X-CSRF-Token"] = csrfCookie.value;
  const res = await request.post(`${BASE_URL}/api/v1/auth/refresh`, { headers });
  if (!res.ok()) throw new Error(`refresh failed ${res.status()}: ${await res.text()}`);
  const body = await res.json();
  const tok = body.accessToken ?? body.access_token;
  if (!tok) throw new Error("refresh returned no token");
  return tok;
}

async function listPlugins(request: APIRequestContext, token: string) {
  const res = await request.get(`${BASE_URL}/api/v1/admin/plugins`, {
    headers: authHeaders(token),
  });
  if (!res.ok()) return [];
  const body = await res.json();
  return body.plugins ?? [];
}

async function uninstallExistingWeather(request: APIRequestContext, token: string) {
  const plugins = await listPlugins(request, token);
  for (const p of plugins) {
    if (p.name === "weather" || p.name === "weather-plugin") {
      await request.delete(`${BASE_URL}/api/v1/admin/plugins/${p.id}`, {
        headers: authHeaders(token),
      });
    }
  }
}

test.describe("Weather plugin", () => {
  test("uploads, installs, and renders the R3F scene with no console errors", async ({ page, request }) => {
    expect(fs.existsSync(ZIP_PATH), `weather.zip not found at ${ZIP_PATH} — run scripts/build-test-plugins.sh first`).toBeTruthy();
    const token = await getAccessToken(request);
    await uninstallExistingWeather(request, token);

    // 1) Upload via multipart form to the admin upload endpoint.
    const zipBytes = fs.readFileSync(ZIP_PATH);
    const upload = await request.post(`${BASE_URL}/api/v1/admin/plugins/upload`, {
      headers: authHeaders(token),
      multipart: {
        plugin: { name: "weather.zip", mimeType: "application/zip", buffer: zipBytes },
      },
    });
    expect(upload.ok(), `upload failed: ${upload.status()} ${await upload.text()}`).toBeTruthy();
    const uploaded = await upload.json();
    expect(uploaded.id).toMatch(/^plg-/);

    // 2) Wait for the plugin process to come up (running status).
    let status = "";
    for (let i = 0; i < 30; i++) {
      const list = await listPlugins(request, token);
      const me = list.find((p: { id: string }) => p.id === uploaded.id);
      status = me?.state ?? "";
      if (status === "PLUGIN_STATE_RUNNING") break;
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(status, "plugin should reach RUNNING state").toBe("PLUGIN_STATE_RUNNING");

    // 3) Capture console + page errors during home page render.
    const consoleErrors: string[] = [];
    const pageErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });
    page.on("pageerror", (err) => pageErrors.push(err.message));

    // 4) Navigate home and wait for the iframe.
    await page.goto(`${BASE_URL}/home`, { waitUntil: "networkidle" });
    const iframeEl = page.locator('iframe[title*="' + uploaded.id + '"]');
    await expect(iframeEl).toBeAttached({ timeout: 15_000 });

    // 5) Drop into the iframe content and confirm the canvas mounted.
    const frameLocator = iframeEl.contentFrame();
    const canvas = frameLocator.locator("canvas");
    await expect(canvas.first()).toBeAttached({ timeout: 15_000 });
    const canvasCount = await canvas.count();
    expect(canvasCount).toBeGreaterThan(0);

    // 6) Wait a beat for shaders to compile + first frame to draw, then
    // verify the canvas has been sized by R3F. Do NOT call getContext()
    // here — R3F has already taken either webgl or webgl2 and asking for
    // the OTHER kind triggers a real "context type mismatch" GL error.
    await page.waitForTimeout(2000);
    const dims = await canvas.first().evaluate((c: HTMLCanvasElement) => ({ w: c.width, h: c.height }));
    expect(dims.w, "canvas must have non-zero width").toBeGreaterThan(0);
    expect(dims.h, "canvas must have non-zero height").toBeGreaterThan(0);

    // 7) The text overlay must show either the temperature or the
    // "Unavailable" fallback once the Open-Meteo flow resolves.
    const overlay = frameLocator.locator("text=/[°C°F—]/").or(frameLocator.locator("text=/Unavailable/"));
    await expect(overlay.first()).toBeVisible({ timeout: 12_000 });

    // 8) Critical errors must be empty. Filter out known benign noise that
    // is NOT specific to the plugin under test:
    //    - Firefox source-map worker complaints (URL constructor)
    //    - Cross-site cookie warnings (iframe srcdoc oddity)
    //    - SSL handshake transient failures during the page load itself
    //    - Vite dev-server module map fetch noise
    //    - net::ERR_TIMED_OUT (transient external API hit during test)
    const fatalErrors = [...consoleErrors, ...pageErrors].filter((msg) => {
      if (/source[- ]?map/i.test(msg)) return false;
      if (/cookie.*samesite|samesite.*cookie/i.test(msg)) return false;
      if (/_csrf/i.test(msg)) return false;
      if (/networkerror.*node_modules\/\.vite\/deps/i.test(msg)) return false;
      if (/ssl certificate error/i.test(msg)) return false;
      if (/err_timed_out/i.test(msg)) return false;
      if (/failed to load resource/i.test(msg)) return false;
      return true;
    });
    expect(fatalErrors, `unexpected errors:\n${fatalErrors.join("\n")}`).toEqual([]);
  });
});
