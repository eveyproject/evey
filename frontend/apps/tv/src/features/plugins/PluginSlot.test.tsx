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
    }, [onLayout]);
    return React.createElement("div", p, children);
  },
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s },
  // 1080p panel at the TV's density: the host reports it so a plugin can size
  // itself as a fraction of the screen instead of guessing.
  useWindowDimensions: () => ({ width: 1200, height: 675 }),
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
// host->WebView directions of the bridge without a real WebView. Also
// captures the `focusable` prop so the TV "display-only, never traps the
// D-pad" requirement is asserted.
const injectJavaScriptMock = vi.fn();
let capturedOnMessage: ((e: { nativeEvent: { data: string } }) => void) | null = null;
let capturedProps: Record<string, unknown> = {};
vi.mock("react-native-webview", () => ({
  WebView: React.forwardRef((props: any, ref: any) => {
    capturedOnMessage = props.onMessage;
    capturedProps = props;
    React.useImperativeHandle(ref, () => ({ injectJavaScript: injectJavaScriptMock }));
    return React.createElement("div", { "data-testid": "plugin-webview" });
  }),
}));

// --- expo-router ------------------------------------------------------------------------
// Bundle disk cache, shared with the phone: a miss unless a test primes it.
vi.mock("expo-file-system/legacy", () => ({
  cacheDirectory: "file:///cache/",
  getInfoAsync: vi.fn().mockResolvedValue({ exists: false }),
  readAsStringAsync: vi.fn().mockResolvedValue(""),
  makeDirectoryAsync: vi.fn().mockResolvedValue(undefined),
  writeAsStringAsync: vi.fn().mockResolvedValue(undefined),
}));

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
  frontend: [{ slot: "home.section", src: "index.js" }],
  webRoot: ".",
  externalFetch: [] as string[],
};
const queryData = { current: { plugins: [PLUGIN_INFO] } as { plugins: typeof PLUGIN_INFO[] } };
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: queryData.current }),
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
import { PluginSlot, usePluginSlotEntries, suggestedSlotHeight } from "./PluginSlot";
import { homeWidgetBudget } from "../../lib/tvLayout";
import { renderHook } from "@testing-library/react";

function dispatchToWebView(payload: unknown) {
  act(() => {
    capturedOnMessage?.({ nativeEvent: { data: JSON.stringify(payload) } });
  });
}

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
  capturedProps = {};
  queryData.current = { plugins: [PLUGIN_INFO] };
  eveyFetchMock.mockResolvedValue({ values: {} });
  eveyFetchRawMock.mockResolvedValue({ ok: true, status: 200, text: async () => "{}" });
  issuePluginTokenMock.mockResolvedValue({ token: "PLUGIN-TOKEN" });
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({ ok: true, text: async () => PLUGIN_BUNDLE }),
  );
});

async function mountPluginWebView() {
  render(<PluginSlot name="home.section" pluginId="plg-test1" />);
  await waitFor(() => expect(capturedOnMessage).toBeTruthy());
}

describe("PluginSlot (TV)", () => {
  it("renders nothing when no plugin matches the slot", () => {
    const { container } = render(<PluginSlot name="no.such.slot" />);
    expect(container.innerHTML).toBe("");
  });

  it("mounts the WebView as display-only (focusable=false) for the D-pad", async () => {
    await mountPluginWebView();
    expect(capturedProps.focusable).toBe(false);
  });

  it("sends the init payload once the plugin signals ready", async () => {
    await mountPluginWebView();
    dispatchToWebView({ type: "ready" });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { type?: string };
      expect(msg?.type).toBe("init");
    });
  });

  it("names the surface and recommends a banner height for a home section", async () => {
    await mountPluginWebView();
    dispatchToWebView({ type: "ready" });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { surface?: string; viewportHeight?: number; suggestedHeight?: number };
      expect(msg?.surface).toBe("tv");
      expect(msg?.suggestedHeight).toBeGreaterThan(0);
      expect(msg?.suggestedHeight).toBe(homeWidgetBudget(msg?.viewportHeight ?? 0));
    });
  });

  it("recommends what the home has left under one whole poster row, for home.section only", () => {
    expect(suggestedSlotHeight("home.section", 675)).toBe(homeWidgetBudget(675));
    expect(homeWidgetBudget(675)).toBeGreaterThanOrEqual(170); // a 1080p panel keeps a real banner
    expect(homeWidgetBudget(540)).toBe(160); // a 540dp panel bottoms out at the floor
    expect(suggestedSlotHeight("settings.user.appearance", 675)).toBeUndefined();
  });

  // A plugin with nothing to show (the librarian on a clean shelf) says
  // resize(0); the slot must vanish rather than keep its opening height.
  it("collapses the slot on resize(0) and clamps positive heights to 60-800", async () => {
    await mountPluginWebView();
    // The View mock is a div, so the host's numeric height lands in the DOM as "60px".
    const slotHeight = () => {
      const webview = document.querySelector('[data-testid="plugin-webview"]');
      return parseFloat((webview?.parentElement as HTMLElement | null)?.style.height ?? "NaN");
    };
    dispatchToWebView({ type: "resize", height: 0 });
    await waitFor(() => expect(slotHeight()).toBe(0));
    dispatchToWebView({ type: "resize", height: 5 });
    await waitFor(() => expect(slotHeight()).toBe(60));
    dispatchToWebView({ type: "resize", height: -1 });
    expect(slotHeight()).toBe(60);
  });
});

describe("PluginSlot media.poster bridge (TV)", () => {
  it("fetches the media poster with the user's session token (not the plugin token) and returns a data URL", async () => {
    const fetchMock = vi.fn().mockImplementation(async (url: string) => {
      if (String(url).includes("/artwork/poster")) {
        return {
          ok: true,
          status: 200,
          blob: async () => new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0x01])], { type: "image/jpeg" }),
        };
      }
      return { ok: true, text: async () => PLUGIN_BUNDLE };
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

describe("PluginSlot assets.read bridge (TV)", () => {
  it("fetches the plugin's own asset with the plugin token and returns a data URL", async () => {
    eveyFetchRawMock.mockImplementation(async (req: { url: string }) => {
      if (req.url.includes("/assets/") && req.url.includes("icon")) {
        return {
          ok: true,
          status: 200,
          blob: async () => new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0x01])], { type: "image/jpeg" }),
        };
      }
      return { ok: true, status: 200, text: async () => "{}" };
    });

    await mountPluginWebView();
    dispatchToWebView({ type: "rpc", id: 7, method: "assets.read", args: { path: "icon.jpg" } });

    await waitFor(() => {
      const msg = lastInjectedMessage() as { type?: string; id?: number; ok?: boolean; data?: string };
      expect(msg?.type).toBe("rpc-result");
      expect(msg?.id).toBe(7);
      expect(msg?.ok).toBe(true);
      expect(msg?.data).toMatch(/^data:image\/jpeg;base64,/);
    });

    const assetCall = eveyFetchRawMock.mock.calls.find(([r]) => (r as { url: string }).url.includes("icon.jpg"));
    expect(assetCall, "asset endpoint should have been called").toBeTruthy();
    const req = assetCall![0] as { url: string; authToken: string };
    expect(req.url).toContain("/api/v1/plugins/plg-test1/assets/icon.jpg");
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

describe("usePluginSlotEntries (TV)", () => {
  it("lists only the plugins mounting a module in the requested slot, stripping webRoot from the entry path", () => {
    queryData.current = {
      plugins: [
        PLUGIN_INFO,
        {
          id: "plg-weather",
          name: "Weather",
          enabled: true,
          frontend: [{ slot: "settings.user.appearance", src: "web/settings.js" }],
          webRoot: "web",
          externalFetch: [],
        },
      ],
    };
    const { result } = renderHook(() => usePluginSlotEntries("settings.user.appearance"));
    expect(result.current).toEqual([
      { pluginId: "plg-weather", name: "Weather", entryPath: "settings.js", externalFetch: [] },
    ]);
  });

  it("excludes a disabled plugin even when it mounts the slot", () => {
    queryData.current = {
      plugins: [
        {
          id: "plg-disabled",
          name: "Disabled",
          enabled: false,
          frontend: [{ slot: "settings.user.appearance", src: "index.js" }],
          webRoot: ".",
          externalFetch: [],
        },
      ],
    };
    const { result } = renderHook(() => usePluginSlotEntries("settings.user.appearance"));
    expect(result.current).toEqual([]);
  });

  it("returns an empty list when no plugin mounts the slot", () => {
    const { result } = renderHook(() => usePluginSlotEntries("settings.user.appearance"));
    expect(result.current).toEqual([]);
  });
});

// The television downloaded every plugin bundle at each cold start (1.3 MB for
// the weather) where the phone kept it on disk; both use one cache now.
describe("PluginSlot bundle cache (TV)", () => {
  it("revalidates a bundle kept on disk with If-None-Match and stores the update", async () => {
    const cachePath = "file:///cache/evey-plugins/plg-test1_index.js";
    const FS = await import("expo-file-system/legacy");
    vi.mocked(FS.getInfoAsync).mockResolvedValue({ exists: true } as never);
    vi.mocked(FS.readAsStringAsync).mockImplementation(async (p: string) =>
      p === cachePath + ".etag" ? '"bundle-v0"' : "console.log('stale');",
    );
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
        expect.objectContaining({ headers: expect.objectContaining({ "If-None-Match": '"bundle-v0"' }) }),
      );
      expect(vi.mocked(FS.writeAsStringAsync)).toHaveBeenCalledWith(cachePath, PLUGIN_BUNDLE);
    });
  });
});
