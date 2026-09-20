import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import { PluginIframe, suggestedSlotHeight, isAllowedNavPath } from "./PluginIframe";

const { navigateMock, toastMocks } = vi.hoisted(() => ({
  navigateMock: vi.fn(),
  toastMocks: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => navigateMock,
}));

let mockToken: string | null = "USER-ACCESS-TOKEN";

vi.mock("@evey/store", () => ({
  toast: toastMocks,
  useAuthStore: (sel: (s: Record<string, unknown>) => unknown) =>
    sel({ accessToken: mockToken }),
}));

// front-bridge.ts BRIDGE_SCRIPT is inlined into the srcdoc and runs inside
// the sandboxed iframe - it cannot be exercised from jsdom tests. The source-
// origin guard (`e.source !== window.parent`) added in that script is a
// static code review and cannot be integration-tested here without a real
// browser environment.
vi.mock("./front-bridge", () => ({
  BRIDGE_SCRIPT: "/* bridge */",
}));

const eveyFetchMock = vi.fn();
const eveyFetchRawMock = vi.fn();
const issuePluginTokenMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
  adminPluginServiceIssuePluginToken: (...args: unknown[]) => issuePluginTokenMock(...args),
}));

const PLUGIN_BUNDLE = "console.log('hello from plugin');";

beforeEach(() => {
  vi.clearAllMocks();
  navigateMock.mockClear();
  toastMocks.success.mockClear();
  toastMocks.error.mockClear();
  toastMocks.info.mockClear();
  mockToken = "USER-ACCESS-TOKEN";
  eveyFetchMock.mockResolvedValue({ values: {} });
  eveyFetchRawMock.mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => PLUGIN_BUNDLE,
  });
  issuePluginTokenMock.mockResolvedValue({ token: "PLUGIN-TOKEN" });
});

describe("PluginIframe", () => {
  it("renders nothing when no access token", () => {
    mockToken = null;
    const { container } = render(
      <PluginIframe pluginId="plg-test1" src="index.js" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("fetches the plugin bundle via Bearer header (no token in URL)", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    await waitFor(() => {
      const calls = eveyFetchRawMock.mock.calls;
      const bundleCall = calls.find(([req]) => (req as { url: string }).url === "/api/v1/plugins/plg-test1/assets/index.js");
      expect(bundleCall, "bundle fetch was expected").toBeTruthy();
      expect((bundleCall![0] as { method: string }).method).toBe("GET");
      // The URL must never carry a ?token= query - eveyFetchRaw attaches the
      // user access token via the Authorization header automatically.
      expect((bundleCall![0] as { url: string }).url).not.toContain("token=");
      // cache: "no-cache" forces conditional revalidation (If-None-Match) even
      // on cache entries stored back when the server said immutable - a JS
      // fetch() is NOT reliably covered by the user's hard refresh, so without
      // this, browsers that cached a bundle pre-2026-07-22 would keep running
      // the stale plugin forever.
      expect((bundleCall![0] as { cache?: string }).cache).toBe("no-cache");
    });
  });

  // The redteam-exfil fixture (tests/plugins/redteam-exfil) attempts eight
  // browser-side exfiltration vectors and expects every one to render BLOCKED.
  // This is the in-gate proof that its expectation is real: the containment it
  // relies on - a strict CSP and a locked-down sandbox attribute - is actually
  // emitted. If any of these assertions breaks, that fixture would start
  // showing BYPASSED in the browser.
  it("denies the browser exfiltration vectors at the sandbox boundary", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";

    // default-src none is the floor: nothing is allowed unless re-granted.
    expect(srcdoc).toContain("default-src 'none'");
    // No external_fetch here, so connect-src is none: fetch/sendBeacon/WebSocket
    // to an attacker sink cannot leave the iframe (vectors 1, 2, 4).
    expect(srcdoc).toContain("connect-src 'none'");
    // img-src never includes an arbitrary host, so the pixel beacon dies (3).
    expect(srcdoc).toMatch(/img-src [^;]*data: blob:/);
    expect(srcdoc).not.toContain("img-src *");

    // The sandbox attribute is allow-scripts and nothing else: no
    // allow-same-origin (opaque origin -> no parent DOM read, vector 5), no
    // allow-top-navigation (URL exfil via top.location, vector 7), no
    // allow-popups. This is what keeps the iframe from reaching the host page.
    const sandbox = iframe.getAttribute("sandbox") ?? "";
    expect(sandbox).toBe("allow-scripts");
  });

  it("inlines the plugin bundle into the iframe srcdoc (no <script src>)", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain(PLUGIN_BUNDLE);
    expect(srcdoc).not.toMatch(/<script\s+src=/i);
    // No token of any kind should appear in the rendered HTML.
    expect(srcdoc).not.toContain("USER-ACCESS-TOKEN");
    expect(srcdoc).not.toContain("token=");
  });

  it("escapes literal </script> in the bundle so HTML parsing is safe", async () => {
    eveyFetchRawMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: async () => "var s = '</script><script>evil()</script>';",
    });
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    // Defused form keeps the original JS string semantics but prevents the
    // HTML parser from terminating the inline <script> early.
    expect(srcdoc).toContain("<\\/script");
    expect(srcdoc).not.toContain("</script><script>evil");
  });

  it("injects <base href> pointing at the assets root (absolute URL)", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toMatch(/<base href="https?:\/\/[^/]+\/api\/v1\/plugins\/plg-test1\/assets\/">/);
  });

  it("injects meta CSP with default-src none and script-src host origin", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain('http-equiv="Content-Security-Policy"');
    expect(srcdoc).toContain("default-src 'none'");
    expect(srcdoc).toMatch(/script-src https?:\/\/[^ ]+ 'unsafe-inline'/);
  });

  it("CSP connect-src reflects the externalFetch list when provided", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" externalFetch={["api.example.com", "cdn.example.org"]} />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("connect-src https://api.example.com https://cdn.example.org");
  });

  it("CSP connect-src is 'none' when externalFetch is empty", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("connect-src 'none'");
  });

  it("uses sandbox=allow-scripts", async () => {
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    const iframe = await screen.findByTitle(/plg-test1/);
    expect(iframe).toHaveAttribute("sandbox", "allow-scripts");
  });

  it("shows error fallback when bundle fetch fails", async () => {
    eveyFetchRawMock.mockRejectedValueOnce(new Error("forbidden"));
    render(<PluginIframe pluginId="plg-test1" src="index.js" />);
    await waitFor(() => {
      expect(screen.queryByTitle(/plg-test1/)).toBeNull();
    });
  });

  it("renders nothing when pluginId has an HTML-special character (defense-in-depth)", async () => {
    // pluginId is server-issued (`plg-<24 hex>`), but a malformed value
    // - typo, manipulated state, future bug - must never be interpolated
    // into the srcdoc HTML. The boundary check returns an empty srcdoc;
    // the component renders nothing.
    render(<PluginIframe pluginId={'plg-foo"><script>1</script>'} src="index.js" />);
    // No iframe titled with the bad id appears.
    await waitFor(() => {
      expect(screen.queryByTitle(/plg-foo/)).toBeNull();
    });
  });

  it("renders nothing when pluginId is missing the plg- prefix (defense-in-depth)", async () => {
    render(<PluginIframe pluginId="weather" src="index.js" />);
    await waitFor(() => {
      expect(screen.queryByTitle(/weather/)).toBeNull();
    });
  });
});

// ── handleMessage end-to-end ────────────────────────────────────────────────
//
// We dispatch a real MessageEvent and override the read-only `source` and
// `origin` properties to spoof the iframe's contentWindow. That's the only
// way to satisfy the strict source/origin checks the host applies.

async function mountIframe(externalFetch?: string[]) {
  render(<PluginIframe pluginId="plg-test1" slot="home.section" src="index.js" externalFetch={externalFetch} />);
  const iframe = (await screen.findByTitle(/plg-test1/)) as HTMLIFrameElement;
  // jsdom assigns iframe.contentWindow lazily; nudge it.
  if (!iframe.contentWindow) {
    Object.defineProperty(iframe, "contentWindow", {
      value: { postMessage: vi.fn() },
      configurable: true,
    });
  }
  return iframe;
}

function dispatchPluginMessage(iframe: HTMLIFrameElement, payload: unknown, opts?: { origin?: string; source?: Window | null }) {
  const evt = new MessageEvent("message", {
    data: payload,
    origin: opts?.origin ?? "null",
  });
  Object.defineProperty(evt, "source", {
    value: opts?.source === undefined ? iframe.contentWindow : opts.source,
  });
  act(() => {
    window.dispatchEvent(evt);
  });
}

describe("PluginIframe handleMessage", () => {
  it("invokes navigate() on whitelisted path", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "navigate", path: "/library/movies" });
    expect(navigateMock).toHaveBeenCalledWith({ to: "/library/movies" });
  });

  it("ignores navigate() on forbidden path", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "navigate", path: "/admin/users" });
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("ignores navigate() on protocol-relative URL", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "navigate", path: "//evil.com/library" });
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("rejects messages from a foreign source (other window)", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "navigate", path: "/library" }, { source: null });
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("rejects messages from a foreign origin", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "navigate", path: "/library" }, { origin: "https://evil.com" });
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("clamps a giant resize request to 800px", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "resize", height: 99999 });
    await waitFor(() => {
      expect(iframe.style.height).toBe("800px");
    });
  });

  it("opens a home section at the height the host recommends, before any resize", async () => {
    const iframe = await mountIframe();
    expect(iframe.style.height).toBe(`${suggestedSlotHeight("home.section", window.innerWidth)}px`);
  });

  it("recommends a banner height for home.section only, by viewport width", () => {
    expect(suggestedSlotHeight("home.section", 360)).toBe(216);
    expect(suggestedSlotHeight("home.section", 600)).toBe(248);
    expect(suggestedSlotHeight("home.section", 1280)).toBe(280);
    expect(suggestedSlotHeight("settings.user.appearance", 1280)).toBeUndefined();
  });

  it("clamps a tiny resize request to 50px", async () => {
    const iframe = await mountIframe();
    dispatchPluginMessage(iframe, { type: "resize", height: 5 });
    await waitFor(() => {
      expect(iframe.style.height).toBe("50px");
    });
  });

  it("truncates toast messages to 200 chars", async () => {
    const iframe = await mountIframe();
    const long = "x".repeat(500);
    dispatchPluginMessage(iframe, { type: "toast", level: "info", message: long });
    expect(toastMocks.info).toHaveBeenCalledTimes(1);
    const arg = toastMocks.info.mock.calls[0][0];
    expect(arg).toHaveLength(200);
  });

  it("rate-limits toasts past 3 per second", async () => {
    const iframe = await mountIframe();
    for (let i = 0; i < 10; i++) {
      dispatchPluginMessage(iframe, { type: "toast", level: "info", message: `msg-${i}` });
    }
    // 3 within the window, 7 dropped silently.
    expect(toastMocks.info).toHaveBeenCalledTimes(3);
  });

  it("allows new toasts after the rate-limit window slides", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const iframe = await mountIframe();
    for (let i = 0; i < 3; i++) {
      dispatchPluginMessage(iframe, { type: "toast", level: "info", message: `m-${i}` });
    }
    expect(toastMocks.info).toHaveBeenCalledTimes(3);
    // 4th rejected immediately
    dispatchPluginMessage(iframe, { type: "toast", level: "info", message: "m-blocked" });
    expect(toastMocks.info).toHaveBeenCalledTimes(3);
    // After the window slides, a new toast is accepted again.
    vi.advanceTimersByTime(1100);
    dispatchPluginMessage(iframe, { type: "toast", level: "info", message: "m-after" });
    expect(toastMocks.info).toHaveBeenCalledTimes(4);
    vi.useRealTimers();
  });
});

describe("isAllowedNavPath", () => {
  it("allows whitelisted top-level paths", () => {
    expect(isAllowedNavPath("/home")).toBe(true);
    expect(isAllowedNavPath("/library")).toBe(true);
    expect(isAllowedNavPath("/library/movies")).toBe(true);
    expect(isAllowedNavPath("/media/123")).toBe(true);
    expect(isAllowedNavPath("/player/abc")).toBe(true);
    expect(isAllowedNavPath("/settings/appearance")).toBe(true);
    expect(isAllowedNavPath("/search?q=foo")).toBe(true);
  });

  it("blocks sensitive areas", () => {
    expect(isAllowedNavPath("/admin")).toBe(false);
    expect(isAllowedNavPath("/admin/users")).toBe(false);
    expect(isAllowedNavPath("/auth/login")).toBe(false);
    expect(isAllowedNavPath("/setup")).toBe(false);
    expect(isAllowedNavPath("/profile/security")).toBe(false);
    expect(isAllowedNavPath("/profile/security/password")).toBe(false);
  });

  it("blocks protocol-relative and absolute URLs", () => {
    expect(isAllowedNavPath("//evil.com")).toBe(false);
    expect(isAllowedNavPath("//evil.com/library")).toBe(false);
    expect(isAllowedNavPath("https://evil.com/library")).toBe(false);
    expect(isAllowedNavPath("http://evil.com")).toBe(false);
  });

  it("blocks javascript: and data: schemes", () => {
    expect(isAllowedNavPath("javascript:alert(1)")).toBe(false);
    expect(isAllowedNavPath("data:text/html,<script>")).toBe(false);
  });

  it("blocks paths that share a prefix but are not whitelisted", () => {
    // "/homepage" must NOT match "/home" - only exact, "/home/", or "/home?"
    expect(isAllowedNavPath("/homepage")).toBe(false);
    expect(isAllowedNavPath("/librarypriv")).toBe(false);
  });

  it("rejects empty, relative, or oversized paths", () => {
    expect(isAllowedNavPath("")).toBe(false);
    expect(isAllowedNavPath("home")).toBe(false);
    expect(isAllowedNavPath("../admin")).toBe(false);
    expect(isAllowedNavPath("/" + "library/" + "x".repeat(3000))).toBe(false);
  });
});

describe("PluginIframe assets.read bridge", () => {
  // Routes an rpc("assets.read") to the plugin's OWN asset endpoint and hands
  // the bytes back as a data: URL. This is the path the weather-v2 plugin uses
  // to display the sun image its backend downloaded at runtime.
  function assetAwareFetch() {
    eveyFetchRawMock.mockImplementation(async (req: { url: string }) => {
      if (req.url.includes("/assets/") && req.url.includes("sun-live")) {
        return {
          ok: true,
          status: 200,
          blob: async () => new Blob([new Uint8Array([0xff, 0xd8, 0xff, 0x01])], { type: "image/jpeg" }),
        };
      }
      return { ok: true, status: 200, text: async () => PLUGIN_BUNDLE };
    });
  }

  // jsdom gives the iframe a real contentWindow; replace it with a stub so we
  // can observe the rpc-result postMessage. The source guard still matches
  // because dispatchPluginMessage uses this same object as the event source.
  function stubPostMessage(iframe: HTMLIFrameElement) {
    const post = vi.fn();
    Object.defineProperty(iframe, "contentWindow", { value: { postMessage: post }, configurable: true });
    return post;
  }

  it("fetches the plugin's own asset with the plugin token and returns a data URL", async () => {
    assetAwareFetch();
    const iframe = await mountIframe();
    const post = stubPostMessage(iframe);
    dispatchPluginMessage(iframe, { type: "rpc", id: 7, method: "assets.read", args: { path: "sun-live-aia171.jpg" } });

    await waitFor(() => {
      const call = post.mock.calls.find(([m]) => (m as { type?: string; id?: number }).type === "rpc-result" && (m as { id?: number }).id === 7);
      expect(call, "rpc-result for id 7 expected").toBeTruthy();
      const msg = call![0] as { ok: boolean; data: string };
      expect(msg.ok).toBe(true);
      expect(msg.data).toMatch(/^data:image\/jpeg;base64,/);
    });

    const assetCall = eveyFetchRawMock.mock.calls.find(([r]) => (r as { url: string }).url.includes("sun-live-aia171.jpg"));
    expect(assetCall, "asset endpoint should have been called").toBeTruthy();
    const req = assetCall![0] as { url: string; authToken: string };
    expect(req.url).toContain("/api/v1/plugins/plg-test1/assets/sun-live-aia171.jpg");
    expect(req.authToken).toBe("PLUGIN-TOKEN");
  });

  it("rejects a path-traversal asset request before any fetch", async () => {
    assetAwareFetch();
    const iframe = await mountIframe();
    const post = stubPostMessage(iframe);
    dispatchPluginMessage(iframe, { type: "rpc", id: 8, method: "assets.read", args: { path: "../../etc/passwd" } });

    await waitFor(() => {
      const call = post.mock.calls.find(([m]) => (m as { type?: string; id?: number }).type === "rpc-result" && (m as { id?: number }).id === 8);
      expect(call, "rpc-result for id 8 expected").toBeTruthy();
      expect((call![0] as { ok: boolean }).ok).toBe(false);
    });
    const traversalCall = eveyFetchRawMock.mock.calls.find(([r]) => (r as { url: string }).url.includes("etc/passwd"));
    expect(traversalCall).toBeFalsy();
  });
});
