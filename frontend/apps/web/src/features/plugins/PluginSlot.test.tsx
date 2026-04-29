import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { PluginSlot } from "./PluginSlot";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: vi.fn(),
}));

vi.mock("./PluginIframe", () => ({
  PluginIframe: ({ pluginId, src }: { pluginId: string; src: string }) => (
    <div data-testid={`plugin-iframe-${pluginId}`} data-src={src} />
  ),
}));

import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";

const mockEveyFetch = eveyFetch as ReturnType<typeof vi.fn>;
const mockUseAuthStore = useAuthStore as unknown as ReturnType<typeof vi.fn>;

function Wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function setAuthenticated(auth: boolean) {
  mockUseAuthStore.mockImplementation((selector: (s: { accessToken: string | null }) => unknown) =>
    selector({ accessToken: auth ? "fake-token" : null }),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockEveyFetch.mockResolvedValue({ plugins: [] });
  setAuthenticated(true);
});

describe("PluginSlot", () => {
  it("renders null when no plugins", async () => {
    const { container } = render(<PluginSlot name="home.section" />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(container.innerHTML).toBe("");
    });
  });

  it("renders iframe for matching plugin front module", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        {
          id: "p1",
          enabled: true,
          frontend: [{ slot: "player.overlay", src: "front.jsx" }],
        },
      ],
    });

    render(<PluginSlot name="player.overlay" context={{ mediaId: "m1" }} />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(screen.getByTestId("plugin-iframe-p1")).toBeInTheDocument();
    });
  });

  it("skips disabled plugins", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        { id: "p1", enabled: false, frontend: [{ slot: "sidebar.bottom", src: "w.jsx" }] },
      ],
    });

    const { container } = render(<PluginSlot name="sidebar.bottom" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(container.innerHTML).toBe("");
    });
  });

  it("does NOT call API when unauthenticated", async () => {
    setAuthenticated(false);
    const { container } = render(<PluginSlot name="home.section" />, { wrapper: Wrapper });

    await new Promise((r) => setTimeout(r, 50));
    expect(mockEveyFetch).not.toHaveBeenCalled();
    expect(container.innerHTML).toBe("");
  });

  it("only renders front modules matching the slot name", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        {
          id: "p1",
          enabled: true,
          frontend: [
            { slot: "home.section", src: "a.jsx" },
            { slot: "player.overlay", src: "b.jsx" },
          ],
        },
      ],
    });

    render(<PluginSlot name="home.section" />, { wrapper: Wrapper });

    await waitFor(() => {
      const iframe = screen.getByTestId("plugin-iframe-p1");
      expect(iframe).toBeInTheDocument();
      expect(iframe.getAttribute("data-src")).toBe("/api/v1/plugins/p1/front.js");
    });
  });

  it("renders front modules from multiple plugins", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        { id: "p1", enabled: true, frontend: [{ slot: "sidebar", src: "w1.jsx" }] },
        { id: "p2", enabled: true, frontend: [{ slot: "sidebar", src: "w2.jsx" }] },
      ],
    });

    render(<PluginSlot name="sidebar" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(screen.getByTestId("plugin-iframe-p1")).toBeInTheDocument();
      expect(screen.getByTestId("plugin-iframe-p2")).toBeInTheDocument();
    });
  });
});
