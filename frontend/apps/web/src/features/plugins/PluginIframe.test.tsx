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
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

beforeEach(() => {
  vi.clearAllMocks();
  mockToken = "USER-ACCESS-TOKEN";
  eveyFetchMock.mockResolvedValue({
    token: "SCOPED-PLUGIN-TOKEN",
    expires_at: new Date(Date.now() + 5 * 60_000).toISOString(),
    scopes: ["media:read"],
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

  it("fetches scoped plugin token from gateway", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    await waitFor(() => {
      // Among parallel fetches (token + admin config) one must be the token call.
      const calls = eveyFetchMock.mock.calls;
      const tokenCall = calls.find(([req]) => (req as { url: string }).url === "/api/v1/plugins/p1/token");
      expect(tokenCall, "token fetch was expected").toBeTruthy();
      expect((tokenCall![0] as { method: string }).method).toBe("POST");
    });
  });

  it("injects scoped plugin token + asset URL into srcdoc", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("SCOPED-PLUGIN-TOKEN");
    expect(srcdoc).not.toContain("USER-ACCESS-TOKEN");
    expect(srcdoc).toContain("/api/v1/plugins/p1/assets/index.js");
  });

  it("injects <base href> pointing at the assets root", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain('<base href="/api/v1/plugins/p1/assets/">');
  });

  it("injects meta CSP with default-src none and script-src host origin", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain('http-equiv="Content-Security-Policy"');
    expect(srcdoc).toContain("default-src 'none'");
    // script-src uses the host origin, not 'self', because the sandboxed
    // iframe has an opaque origin where 'self' matches nothing.
    expect(srcdoc).toMatch(/script-src https?:\/\/[^ ]+ 'unsafe-inline'/);
  });

  it("CSP connect-src reflects the externalFetch list when provided", async () => {
    render(<PluginIframe pluginId="p1" src="index.js" externalFetch={["api.example.com", "cdn.example.org"]} />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("connect-src api.example.com cdn.example.org");
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

  it("shows error fallback when token issuance fails", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("forbidden"));
    render(<PluginIframe pluginId="p1" src="index.js" />);
    await waitFor(() => {
      expect(screen.queryByTitle(/p1/)).toBeNull();
    });
  });
});
