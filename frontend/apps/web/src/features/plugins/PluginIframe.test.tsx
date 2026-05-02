import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import { PluginIframe, isAllowedNavPath } from "./PluginIframe";

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
// the sandboxed iframe — it cannot be exercised from jsdom tests. The source-
// origin guard (`e.source !== window.parent`) added in that script is a
// static code review and cannot be integration-tested here without a real
// browser environment.
vi.mock("./front-bridge", () => ({
  BRIDGE_SCRIPT: "/* bridge */",
}));

const eveyFetchMock = vi.fn();
const eveyFetchRawMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
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
});

describe("PluginIframe", () => {
  it("renders nothing when no access token", () => {
    mockToken = null;
    const { container } = render(
      <PluginIframe pluginId="p1" src="index.js" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("fetches the plugin bundle via Bearer header (no token in URL)", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    await waitFor(() => {
      const calls = eveyFetchRawMock.mock.calls;
      const bundleCall = calls.find(([req]) => (req as { url: string }).url === "/api/v1/plugins/p1/assets/index.js");
      expect(bundleCall, "bundle fetch was expected").toBeTruthy();
      expect((bundleCall![0] as { method: string }).method).toBe("GET");
      // The URL must never carry a ?token= query — eveyFetchRaw attaches the
      // user access token via the Authorization header automatically.
      expect((bundleCall![0] as { url: string }).url).not.toContain("token=");
    });
  });

  it("inlines the plugin bundle into the iframe srcdoc (no <script src>)", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
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
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    // Defused form keeps the original JS string semantics but prevents the
    // HTML parser from terminating the inline <script> early.
    expect(srcdoc).toContain("<\\/script");
    expect(srcdoc).not.toContain("</script><script>evil");
  });

  it("injects <base href> pointing at the assets root (absolute URL)", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toMatch(/<base href="https?:\/\/[^/]+\/api\/v1\/plugins\/p1\/assets\/">/);
  });

  it("injects meta CSP with default-src none and script-src host origin", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain('http-equiv="Content-Security-Policy"');
    expect(srcdoc).toContain("default-src 'none'");
    expect(srcdoc).toMatch(/script-src https?:\/\/[^ ]+ 'unsafe-inline'/);
  });

  it("CSP connect-src reflects the externalFetch list when provided", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" externalFetch={["api.example.com", "cdn.example.org"]} />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("connect-src https://api.example.com https://cdn.example.org");
  });

  it("CSP connect-src is 'none' when externalFetch is empty", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("connect-src 'none'");
  });

  it("uses sandbox=allow-scripts", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = await screen.findByTitle(/p1/);
    expect(iframe).toHaveAttribute("sandbox", "allow-scripts");
  });

  it("shows error fallback when bundle fetch fails", async () => {
    eveyFetchRawMock.mockRejectedValueOnce(new Error("forbidden"));
    render(<PluginIframe pluginId="p1" src="index.js" />);
    await waitFor(() => {
      expect(screen.queryByTitle(/p1/)).toBeNull();
    });
  });
});

// ── handleMessage end-to-end ────────────────────────────────────────────────
//
// We dispatch a real MessageEvent and override the read-only `source` and
// `origin` properties to spoof the iframe's contentWindow. That's the only
// way to satisfy the strict source/origin checks the host applies.

async function mountIframe(externalFetch?: string[]) {
  render(<PluginIframe pluginId="p1" src="index.js" externalFetch={externalFetch} />);
  const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
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
    // "/homepage" must NOT match "/home" — only exact, "/home/", or "/home?"
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
