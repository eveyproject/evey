import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { PluginIframe } from "./PluginIframe";

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => vi.fn(),
}));

let mockToken: string | null = "USER-ACCESS-TOKEN";

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  useAuthStore: (sel: (s: Record<string, unknown>) => unknown) =>
    sel({ accessToken: mockToken }),
}));

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
