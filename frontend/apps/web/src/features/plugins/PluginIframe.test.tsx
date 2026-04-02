import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { PluginIframe } from "./PluginIframe";

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => vi.fn(),
}));

let mockToken: string | null = "test-token";

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  useAuthStore: (sel: (s: Record<string, unknown>) => unknown) =>
    sel({ accessToken: mockToken }),
}));

vi.mock("./widget-bridge", () => ({
  BRIDGE_SCRIPT: "/* bridge */",
}));

beforeEach(() => {
  vi.clearAllMocks();
  mockToken = "test-token";
});

describe("PluginIframe", () => {
  it("renders nothing when no access token", () => {
    mockToken = null;
    const { container } = render(
      <PluginIframe pluginId="p1" src="/widget.js" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders iframe with sandbox=allow-scripts", () => {
    render(<PluginIframe pluginId="p1" src="/widget.js" />);
    const iframe = screen.getByTitle(/p1/);
    expect(iframe).toBeInTheDocument();
    expect(iframe).toHaveAttribute("sandbox", "allow-scripts");
  });

  it("sets correct title attribute", () => {
    render(<PluginIframe pluginId="my-plugin" src="/w.js" />);
    const iframe = screen.getByTitle(/my-plugin/);
    expect(iframe.tagName).toBe("IFRAME");
  });

  it("renders srcdoc containing plugin script URL", () => {
    render(<PluginIframe pluginId="p1" src="/widget.js" />);
    const iframe = screen.getByTitle(/p1/) as HTMLIFrameElement;
    expect(iframe.getAttribute("srcdoc")).toContain(
      "/api/v1/plugins/p1/widget.js",
    );
  });
});
