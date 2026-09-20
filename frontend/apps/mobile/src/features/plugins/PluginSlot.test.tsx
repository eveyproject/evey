import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor, act } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------------------
// View fires onLayout synchronously on mount (height > 0) so PluginWebView's
// "inViewport" gate opens without needing a real layout pass.
vi.mock("react-native", () => ({
  View: ({ children, onLayout, ...p }: any) => {
    React.useEffect(() => {
      onLayout?.({ nativeEvent: { layout: { height: 100 } } });
    }, []);
    return React.createElement("div", p, children);
  },
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s },
  useWindowDimensions: () => ({ width: 390, height: 844, scale: 3, fontScale: 1 }),
  InteractionManager: {
    runAfterInteractions: (cb: () => void) => {
      cb();
      return { cancel: () => {} };
    },
  },
  Platform: { OS: "android", select: (o: any) => o.android },
}));

// --- react-native-webview mock -----------------------------------------------------------
// Captures the last onMessage handler + exposes an injectJavaScript spy via
// useImperativeHandle so the test can simulate the WebView->host and
// host->WebView directions of the bridge without a real WebView.
const injectJavaScriptMock = vi.fn();
let capturedOnMessage: ((e: { nativeEvent: { data: string } }) => void) | null = null;
vi.mock("react-native-webview", () => ({
  WebView: React.forwardRef((props: any, ref: any) => {
    capturedOnMessage = props.onMessage;
    React.useImperativeHandle(ref, () => ({ injectJavaScript: injectJavaScriptMock }));
    return React.createElement("div", { "data-testid": "plugin-webview" });
  }),
}));

// --- expo-file-system mock (bundle disk cache; always a miss in tests) -----------------
vi.mock("expo-file-system/legacy", () => ({
  cacheDirectory: "file:///cache/",
  getInfoAsync: vi.fn().mockResolvedValue({ exists: false }),
  readAsStringAsync: vi.fn().mockResolvedValue(""),
  makeDirectoryAsync: vi.fn().mockResolvedValue(undefined),
  writeAsStringAsync: vi.fn().mockResolvedValue(undefined),
}));

// --- expo-router ------------------------------------------------------------------------
vi.mock("expo-router", () => ({
  router: { push: vi.fn() },
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key, i18n: { language: "en" } }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00" } }),
}));

// --- useServerUrl -----------------------------------------------------------------------
vi.mock("../../hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));

// --- @evey/store --------------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  useAuthStore: (sel: (s: Record<string, unknown>) => unknown) => sel({ accessToken: "USER-ACCESS-TOKEN" }),
}));

// --- @tanstack/react-query ----------------------------------------------------------------
const PLUGIN_INFO = {
  id: "plg-test1",
  name: "Test Plugin",
  enabled: true,
  frontend: [{ slot: "test.slot", src: "index.js" }],
  webRoot: ".",
  externalFetch: [] as string[],
};
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: { plugins: [PLUGIN_INFO] } }),
}));

// --- @evey/api ------------------------------------------------------------------------------
const PLUGIN_BUNDLE = "console.log('hello from plugin');";
const eveyFetchMock = vi.fn();
const eveyFetchRawMock = vi.fn();
const issuePluginTokenMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
  adminPluginServiceIssuePluginToken: (...args: unknown[]) => issuePluginTokenMock(...args),
}));

// -------------------------------------------------------------------------------------------
import { PluginSlot, isAllowedNavPath, suggestedSlotHeight } from "./PluginSlot";

function dispatchToWebView(payload: unknown) {
  act(() => {
    capturedOnMessage?.({ nativeEvent: { data: JSON.stringify(payload) } });
  });
}

// injectJavaScript is called with `window.postMessage(<json>, "*"); true;` -
// pull the JSON payload back out to assert on it.
function lastInjectedMessage(): Record<string, unknown> | undefined {
  const call = injectJavaScriptMock.mock.calls.at(-1);
  if (!call) return undefined;
  const script = call[0] as string;
  const match = script.match(/^window\.postMessage\((.*), "\*"\); true;$/);
  if (!match) return undefined;
  return JSON.parse(match[1]);
}

beforeEach(() => {
  vi.clearAllMocks();
  capturedOnMessage = null;
  eveyFetchMock.mockResolvedValue({ values: {} });
  eveyFetchRawMock.mockResolvedValue({ ok: true, status: 200, text: async () => "{}" });
  issuePluginTokenMock.mockResolvedValue({ token: "PLUGIN-TOKEN" });
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ etag: '"bundle-v1"' }),
      text: async () => PLUGIN_BUNDLE,
    }),
  );
});

async function mountPluginWebView() {
  render(<PluginSlot name="test.slot" pluginId="plg-test1" />);
  await waitFor(() => expect(capturedOnMessage).toBeTruthy());
}

describe("PluginSlot init payload", () => {
  it("names the surface, and stays silent on the height of a slot it has no opinion about", async () => {
    await mountPluginWebView(); // "test.slot", not a home section
    dispatchToWebView({ type: "ready" });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { type?: string; surface?: string };
      expect(msg?.type).toBe("init");
      expect(msg?.surface).toBe("mobile");
      expect("suggestedHeight" in (msg ?? {})).toBe(false);
    });
  });

  it("recommends a banner height for home.section only, by viewport width", () => {
    expect(suggestedSlotHeight("home.section", 360)).toBe(216);
    expect(suggestedSlotHeight("home.section", 600)).toBe(248);
    expect(suggestedSlotHeight("home.section", 1024)).toBe(280);
    expect(suggestedSlotHeight("settings.user.appearance", 360)).toBeUndefined();
  });
});

describe("PluginSlot assets.read bridge", () => {
  it("fetches the plugin's own asset with the plugin token and returns a data URL", async () => {
    eveyFetchRawMock.mockImplementation(async (req: { url: string }) => {
      if (req.url.includes("/assets/") && req.url.includes("sun-live")) {
        return {
          ok: true,
          status: 200,
          blob: async () => new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0x01])], { type: "image/jpeg" }),
        };
      }
      return { ok: true, status: 200, text: async () => "{}" };
    });

    await mountPluginWebView();
    dispatchToWebView({ type: "rpc", id: 7, method: "assets.read", args: { path: "sun-live-aia171.jpg" } });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { type?: string; id?: number; ok?: boolean; data?: string };
      expect(msg?.type).toBe("rpc-result");
      expect(msg?.id).toBe(7);
      expect(msg?.ok).toBe(true);
      expect(msg?.data).toMatch(/^data:image\/jpeg;base64,/);
    });

    const assetCall = eveyFetchRawMock.mock.calls.find(([r]) => (r as { url: string }).url.includes("sun-live-aia171.jpg"));
    expect(assetCall, "asset endpoint should have been called").toBeTruthy();
    const req = assetCall![0] as { url: string; authToken: string };
    expect(req.url).toContain("/api/v1/plugins/plg-test1/assets/sun-live-aia171.jpg");
    expect(req.authToken).toBe("PLUGIN-TOKEN");
  });

  it("rejects a path-traversal asset request before any fetch", async () => {
    await mountPluginWebView();
    dispatchToWebView({ type: "rpc", id: 8, method: "assets.read", args: { path: "../../etc/passwd" } });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { type?: string; id?: number; ok?: boolean };
      expect(msg?.type).toBe("rpc-result");
      expect(msg?.id).toBe(8);
      expect(msg?.ok).toBe(false);
    });

    const traversalCall = eveyFetchRawMock.mock.calls.find(([r]) => (r as { url: string }).url.includes("etc/passwd"));
    expect(traversalCall).toBeFalsy();
  });
});

describe("PluginSlot media.poster bridge", () => {
  it("fetches the media poster with the user's session token (not the plugin token) and returns a data URL", async () => {
    const fetchMock = vi.fn().mockImplementation(async (url: string) => {
      if (String(url).includes("/artwork/poster")) {
        return {
          ok: true,
          status: 200,
          blob: async () => new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0x01])], { type: "image/jpeg" }),
        };
      }
      return {
        ok: true,
        status: 200,
        headers: new Headers({ etag: '"bundle-v1"' }),
        text: async () => PLUGIN_BUNDLE,
      };
    });
    vi.stubGlobal("fetch", fetchMock);

    await mountPluginWebView();
    dispatchToWebView({ type: "rpc", id: 9, method: "media.poster", args: { id: "mi-1" } });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { type?: string; id?: number; ok?: boolean; data?: string };
      expect(msg?.type).toBe("rpc-result");
      expect(msg?.id).toBe(9);
      expect(msg?.ok).toBe(true);
      expect(msg?.data).toMatch(/^data:image\/jpeg;base64,/);
    });

    const posterCall = fetchMock.mock.calls.find(([url]) => String(url).includes("/artwork/poster"));
    expect(posterCall, "poster endpoint should have been called").toBeTruthy();
    const [url, init] = posterCall!;
    expect(url).toContain("/api/v1/media/mi-1/artwork/poster");
    expect((init as RequestInit).headers).toEqual({ Authorization: "Bearer USER-ACCESS-TOKEN" });
  });
});

// A plugin update keeps its plg-… id, so the disk cache key can never
// invalidate a stale bundle by itself: every cache hit must revalidate
// against the server's ETag (stale-while-revalidate).
describe("PluginSlot bundle cache revalidation", () => {
  const cachePath = "file:///cache/evey-plugins/plg-test1_index.js";

  async function primeCache(code: string, etag: string) {
    const FS = await import("expo-file-system/legacy");
    vi.mocked(FS.getInfoAsync).mockResolvedValue({ exists: true } as never);
    vi.mocked(FS.readAsStringAsync).mockImplementation(async (p: string) =>
      p === cachePath + ".etag" ? etag : code,
    );
    return FS;
  }

  it("revalidates a cache hit with If-None-Match and swaps in an updated bundle", async () => {
    const FS = await primeCache("console.log('stale');", '"bundle-v0"');
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ etag: '"bundle-v1"' }),
      text: async () => PLUGIN_BUNDLE,
    });
    vi.stubGlobal("fetch", fetchMock);

    await mountPluginWebView();

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/plugins/plg-test1/assets/index.js"),
        expect.objectContaining({
          headers: expect.objectContaining({ "If-None-Match": '"bundle-v0"' }),
        }),
      );
      // Updated bundle + its ETag are persisted for the next launch.
      expect(vi.mocked(FS.writeAsStringAsync)).toHaveBeenCalledWith(cachePath, PLUGIN_BUNDLE);
      expect(vi.mocked(FS.writeAsStringAsync)).toHaveBeenCalledWith(cachePath + ".etag", '"bundle-v1"');
    });
  });

  it("keeps the cached bundle on 304 without rewriting the cache", async () => {
    const FS = await primeCache(PLUGIN_BUNDLE, '"bundle-v1"');
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 304,
      headers: new Headers(),
      text: async () => "",
    });
    vi.stubGlobal("fetch", fetchMock);

    await mountPluginWebView();

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(vi.mocked(FS.writeAsStringAsync)).not.toHaveBeenCalled();
  });

  it("does not blank an already-rendered cached bundle when revalidation fails", async () => {
    await primeCache(PLUGIN_BUNDLE, '"bundle-v1"');
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));

    // The WebView mounts from cache despite the network failure...
    await mountPluginWebView();
    // ...and stays mounted (no "Failed to load plugin" fallback).
    const { screen } = await import("@testing-library/react");
    expect(screen.queryByText(/failed to load plugin/i)).toBeNull();
  });
});

describe("isAllowedNavPath", () => {
  it("allows non-sensitive app paths", () => {
    for (const p of ["/home", "/library/lib-1", "/media/mi-1", "/player/mi-1", "/settings", "/search?q=x"]) {
      expect(isAllowedNavPath(p)).toBe(true);
    }
  });

  it("rejects /admin and other sensitive or off-origin destinations (parity with web, anti-phishing)", () => {
    for (const p of ["/admin", "/admin/api-keys", "/auth/login", "/setup", "//evil.com", "https://evil.com", "relative"]) {
      expect(isAllowedNavPath(p)).toBe(false);
    }
  });
});
