import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { PluginSlot } from "./PluginSlot";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

vi.mock("./plugin-registry", () => ({
  loadPlugin: vi.fn().mockResolvedValue(undefined),
  getPluginsForSlot: vi.fn().mockReturnValue([]),
}));

import { eveyFetch } from "@evey/api";
import { getPluginsForSlot, loadPlugin } from "./plugin-registry";

const mockEveyFetch = eveyFetch as ReturnType<typeof vi.fn>;
const mockGetPlugins = getPluginsForSlot as ReturnType<typeof vi.fn>;
const mockLoadPlugin = loadPlugin as ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.clearAllMocks();
  mockEveyFetch.mockResolvedValue({ plugins: [] });
  mockGetPlugins.mockReturnValue([]);
});

describe("PluginSlot", () => {
  it("renders null when no plugins", async () => {
    const { container } = render(<PluginSlot name="home.section" />);
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

    render(<PluginSlot name="player.overlay" context={{ mediaId: "m1" }} />);

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

    // Suppress console.error from React error boundary
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { container } = render(<PluginSlot name="test.slot" />);
    await waitFor(() => {
      // crashed widget renders null via ErrorBoundary
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

    render(<PluginSlot name="sidebar.bottom" />);

    await waitFor(() => {
      expect(mockLoadPlugin).not.toHaveBeenCalled();
    });
  });
});
