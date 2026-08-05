// E2E for the weather plugin: install zip, enable, navigate to /home,
// assert the iframe loads its bundle without console errors and the
// canvas renders. Uses the auth state from globalSetup.

import { test, expect, request as playwrightRequest } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";
import type { APIRequestContext } from "@playwright/test";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const STORAGE_STATE = path.join(__dirname, ".auth-state.json");
const ZIP_PATH = path.resolve(__dirname, "../../tests/plugins/weather.zip");
const BASE_URL = process.env.BASE_URL ?? "https://evey.test";

test.use({ storageState: STORAGE_STATE, ignoreHTTPSErrors: true });

function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` };
}

// global-setup persists the accessToken into localStorage under "evey-auth".
// Navigating to /home gives us a same-origin context to read it. No CSRF
// dance: we just grab the JWT and use it as a Bearer header.
async function getAccessToken(page: import("@playwright/test").Page): Promise<string> {
  await page.goto(`${BASE_URL}/home`, { waitUntil: "domcontentloaded" });
  const tok = await page.evaluate(() => {
    const raw = localStorage.getItem("evey-auth");
    if (!raw) return null;
    try {
      const parsed = JSON.parse(raw) as { state?: { accessToken?: string } };
      return parsed.state?.accessToken ?? null;
    } catch {
      return null;
    }
  });
  if (!tok) throw new Error("no accessToken in localStorage - global-setup didn't run?");
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
  // One browser only. The three projects run in parallel against ONE server and
  // this test owns a server-side singleton for its whole length: it uninstalls
  // the weather plugin, installs it, then waits for that install to reach
  // RUNNING. Run on three browsers, they interleave - the second upload gets
  // 500 "already installed", and once that is retried the next project's
  // uninstall drops the plugin the first is still waiting on, so it never
  // reaches RUNNING. Nothing about installing a plugin is browser-specific;
  // only the R3F render below is, and one engine is enough to catch a broken
  // bundle.
  test.skip(({ browserName }) => browserName !== "chromium", "owns a server-side singleton - see comment above");

  test("uploads, installs, and renders the R3F scene with no console errors", async ({ page, request }) => {
    expect(fs.existsSync(ZIP_PATH), `weather.zip not found at ${ZIP_PATH} - run scripts/build-test-plugins.sh first`).toBeTruthy();
    // Auth: read the JWT global-setup persisted in localStorage. We can't
    // hit /refresh because that requires the CSRF double-submit cookie which
    // Playwright's APIRequestContext can't read from the browser context.
    const token = await getAccessToken(page);
    // Use an isolated APIRequestContext for admin calls so they auth via
    // the Bearer header without dragging the browser's session cookies
    // (which would force CSRF on every state-changing call).
    const apiCtx = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
    await uninstallExistingWeather(apiCtx, token);

    // 1) Upload via multipart form to the admin upload endpoint.
    //
    // Retried, because the three browser projects run in PARALLEL against one
    // server and the plugin registry is shared: project A can install between
    // project B's uninstall and its upload, and B then gets
    // 500 "plugin already installed at version 1.0.1, new version must be
    // higher". The zip carries a fixed version, so the only way through is to
    // uninstall again and retry - which is exactly what a second operator
    // hitting the same instance would have to do.
    const zipBytes = fs.readFileSync(ZIP_PATH);
    let upload = await apiCtx.post(`${BASE_URL}/api/v1/admin/plugins/upload`, {
      headers: authHeaders(token),
      multipart: {
        plugin: { name: "weather.zip", mimeType: "application/zip", buffer: zipBytes },
      },
    });
    for (let attempt = 0; attempt < 5 && !upload.ok(); attempt++) {
      await uninstallExistingWeather(apiCtx, token);
      upload = await apiCtx.post(`${BASE_URL}/api/v1/admin/plugins/upload`, {
        headers: authHeaders(token),
        multipart: {
          plugin: { name: "weather.zip", mimeType: "application/zip", buffer: zipBytes },
        },
      });
    }
    expect(upload.ok(), `upload failed: ${upload.status()} ${await upload.text()}`).toBeTruthy();
    const uploaded = await upload.json();
    expect(uploaded.id).toMatch(/^plg-/);

    // 2) Wait for the plugin process to come up (running status).
    let status = "";
    for (let i = 0; i < 30; i++) {
      const list = await listPlugins(apiCtx, token);
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

    // 4) The page is already on /home (loaded by getAccessToken). Force a
    // re-navigation so the just-installed plugin shows in the slot list.
    await page.goto(`${BASE_URL}/home`, { waitUntil: "domcontentloaded" });
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
    // here - R3F has already taken either webgl or webgl2 and asking for
    // the OTHER kind triggers a real "context type mismatch" GL error.
    await page.waitForTimeout(2000);
    const dims = await canvas.first().evaluate((c: HTMLCanvasElement) => ({ w: c.width, h: c.height }));
    expect(dims.w, "canvas must have non-zero width").toBeGreaterThan(0);
    expect(dims.h, "canvas must have non-zero height").toBeGreaterThan(0);

    // 7) The text overlay must show either the temperature or the
    // "Unavailable" fallback once the Open-Meteo flow resolves.
    const overlay = frameLocator.locator("text=/[°C°F-]/").or(frameLocator.locator("text=/Unavailable/"));
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
      // Vite HMR websocket gets cut short by the test's re-navigation.
      if (/connection to wss?:.*was interrupted/i.test(msg)) return false;
      if (/@vite\/client/i.test(msg)) return false;
      // Firefox sometimes aborts in-flight TMDB poster requests on
      // re-navigation and reports "Image corrupt or truncated" - unrelated
      // to the plugin under test.
      if (/image corrupt or truncated/i.test(msg)) return false;
      if (/image\.tmdb\.org/i.test(msg)) return false;
      return true;
    });
    expect(fatalErrors, `unexpected errors:\n${fatalErrors.join("\n")}`).toEqual([]);
  });
});
