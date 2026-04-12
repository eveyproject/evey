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

vi.mock("./widget-bridge", () => ({
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
      <PluginIframe pluginId="p1" src="/widget.js" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("fetches scoped plugin token from gateway", async () => {
    render(<PluginIframe pluginId="p1" src="/widget.js" />);
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalled());
    const [req] = eveyFetchMock.mock.calls[0];
    expect((req as { url: string }).url).toBe("/api/v1/plugins/p1/token");
    expect((req as { method: string }).method).toBe("POST");
  });

  it("injects scoped plugin token into srcdoc (not user access token)", async () => {
    render(<PluginIframe pluginId="p1" src="/widget.js" />);
    const iframe = (await screen.findByTitle(/p1/)) as HTMLIFrameElement;
    const srcdoc = iframe.getAttribute("srcdoc") ?? "";
    expect(srcdoc).toContain("SCOPED-PLUGIN-TOKEN");
    expect(srcdoc).not.toContain("USER-ACCESS-TOKEN");
    expect(srcdoc).toContain("/api/v1/plugins/p1/widget.js");
  });

  it("uses sandbox=allow-scripts", async () => {
    render(<PluginIframe pluginId="p1" src="/widget.js" />);
    const iframe = await screen.findByTitle(/p1/);
    expect(iframe).toHaveAttribute("sandbox", "allow-scripts");
  });

  it("shows error fallback when token issuance fails", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("forbidden"));
    render(<PluginIframe pluginId="p1" src="/widget.js" />);
    await waitFor(() => {
      // error fallback uses translation key as text in test env
      expect(screen.queryByTitle(/p1/)).toBeNull();
    });
  });
});
