import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { PluginSlot } from "./PluginSlot";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

vi.mock("./plugin-registry", () => ({
  loadPlugin: vi.fn().mockResolvedValue(undefined),
  getPluginsForSlot: vi.fn().mockReturnValue([]),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: vi.fn(),
}));

import { eveyFetch } from "@evey/api";
import { getPluginsForSlot, loadPlugin } from "./plugin-registry";
import { useAuthStore } from "@evey/store";

const mockEveyFetch = eveyFetch as ReturnType<typeof vi.fn>;
const mockGetPlugins = getPluginsForSlot as ReturnType<typeof vi.fn>;
const mockLoadPlugin = loadPlugin as ReturnType<typeof vi.fn>;
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
  mockGetPlugins.mockReturnValue([]);
  setAuthenticated(true);
});

describe("PluginSlot", () => {
  it("renders null when no plugins", async () => {
    const { container } = render(<PluginSlot name="home.section" />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(container.innerHTML).toBe("");
    });
  });

  it("loads and renders plugin widgets", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        {
          id: "p1",
          enabled: true,
          uiWidgets: [{ slot: "player.overlay", src: "widget.jsx" }],
        },
      ],
    });
    mockLoadPlugin.mockResolvedValue(undefined);
    mockGetPlugins.mockReturnValue([
      {
        pluginId: "p1",
        slot: "player.overlay",
        component: () => <div data-testid="plugin-widget">Hello</div>,
      },
    ]);

    render(<PluginSlot name="player.overlay" context={{ mediaId: "m1" }} />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(screen.getByTestId("plugin-widget")).toBeInTheDocument();
    });
    expect(mockLoadPlugin).toHaveBeenCalledWith("p1", "player.overlay", "/api/v1/plugins/p1/widget.js");
  });

  it("wraps widgets in ErrorBoundary — crash does not break host", async () => {
    const CrashWidget = () => { throw new Error("boom"); };

    mockEveyFetch.mockResolvedValue({
      plugins: [{ id: "p1", enabled: true, uiWidgets: [{ slot: "test.slot", src: "w.jsx" }] }],
    });
    mockLoadPlugin.mockResolvedValue(undefined);
    mockGetPlugins.mockReturnValue([
      { pluginId: "p1", slot: "test.slot", component: CrashWidget },
    ]);

    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { container } = render(<PluginSlot name="test.slot" />, { wrapper: Wrapper });
    await waitFor(() => {
      expect(container.innerHTML).toBe("");
    });
    spy.mockRestore();
  });

  it("skips disabled plugins", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        { id: "p1", enabled: false, uiWidgets: [{ slot: "sidebar.bottom", src: "w.jsx" }] },
      ],
    });

    render(<PluginSlot name="sidebar.bottom" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(mockLoadPlugin).not.toHaveBeenCalled();
    });
  });

  it("does NOT call API when unauthenticated", async () => {
    setAuthenticated(false);
    const { container } = render(<PluginSlot name="home.section" />, { wrapper: Wrapper });

    // Give React Query time to potentially fire
    await new Promise((r) => setTimeout(r, 50));
    expect(mockEveyFetch).not.toHaveBeenCalled();
    expect(container.innerHTML).toBe("");
  });

  it("passes context props to widget component", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        { id: "p1", enabled: true, uiWidgets: [{ slot: "media.detail.sidebar", src: "w.jsx" }] },
      ],
    });
    mockLoadPlugin.mockResolvedValue(undefined);
    mockGetPlugins.mockReturnValue([
      {
        pluginId: "p1",
        slot: "media.detail.sidebar",
        component: (props: Record<string, unknown>) => (
          <div data-testid="ctx-widget">{String(props.mediaId)}</div>
        ),
      },
    ]);

    render(
      <PluginSlot name="media.detail.sidebar" context={{ mediaId: "m42" }} />,
      { wrapper: Wrapper },
    );

    await waitFor(() => {
      expect(screen.getByTestId("ctx-widget")).toHaveTextContent("m42");
    });
  });

  it("only loads widgets matching the slot name", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        {
          id: "p1",
          enabled: true,
          uiWidgets: [
            { slot: "home.section", src: "a.jsx" },
            { slot: "player.overlay", src: "b.jsx" },
          ],
        },
      ],
    });
    mockLoadPlugin.mockResolvedValue(undefined);
    mockGetPlugins.mockReturnValue([]);

    render(<PluginSlot name="home.section" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(mockLoadPlugin).toHaveBeenCalledTimes(1);
      expect(mockLoadPlugin).toHaveBeenCalledWith("p1", "home.section", "/api/v1/plugins/p1/widget.js");
    });
  });

  it("renders multiple widgets from different plugins", async () => {
    mockEveyFetch.mockResolvedValue({
      plugins: [
        { id: "p1", enabled: true, uiWidgets: [{ slot: "sidebar", src: "w1.jsx" }] },
        { id: "p2", enabled: true, uiWidgets: [{ slot: "sidebar", src: "w2.jsx" }] },
      ],
    });
    mockLoadPlugin.mockResolvedValue(undefined);
    mockGetPlugins.mockReturnValue([
      { pluginId: "p1", slot: "sidebar", component: () => <div data-testid="w1">Widget 1</div> },
      { pluginId: "p2", slot: "sidebar", component: () => <div data-testid="w2">Widget 2</div> },
    ]);

    render(<PluginSlot name="sidebar" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(screen.getByTestId("w1")).toBeInTheDocument();
      expect(screen.getByTestId("w2")).toBeInTheDocument();
    });
  });
});
