import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  Text: ({ children, testID }: any) => React.createElement("span", { "data-testid": testID }, children),
  Pressable: ({ children, onPress, disabled, testID }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
const store = vi.hoisted(() => ({ locked: false }));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (st: unknown) => boolean) => sel({}),
  demoLocks: () => store.locked,
}));
const api = vi.hoisted(() => ({ data: undefined as any, save: vi.fn(), refresh: vi.fn() }));
vi.mock("@evey/api", () => ({
  useAdminPluginServiceGetPluginMetadataSettings: () => ({ data: api.data, isLoading: false }),
  useAdminPluginServiceUpdatePluginMetadataSettings: () => ({ mutate: api.save, isPending: false }),
  useAdminPluginServiceRefreshPluginMetadata: () => ({ mutate: api.refresh, isPending: false }),
  getAdminPluginServiceGetPluginMetadataSettingsQueryKey: (id: string) => ["metadata", id],
  apiErrorMessage: (_e: unknown, _t: unknown, f: string) => f,
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", bg: "#000" }, radii: { sm: 4, md: 8 } }),
}));

import { PluginMetadataSection } from "./PluginMetadataSection";

beforeEach(() => {
  vi.clearAllMocks();
  store.locked = false;
  api.data = { fields: ["overview", "tagline"], availableFields: ["title", "overview", "tagline"], refresh: {} };
});

// The section toggles the allowed fields and saves them, starts the library
// fetch, and shows where the fetch stands.
describe("PluginMetadataSection", () => {
  it("saves the toggled fields", () => {
    render(<PluginMetadataSection pluginId="allocine" />);
    fireEvent.click(screen.getByTestId("metadata-field-title"));
    fireEvent.click(screen.getByTestId("metadata-field-overview"));
    fireEvent.click(screen.getByTestId("metadata-save"));
    expect(api.save).toHaveBeenCalledWith({ pluginId: "allocine", data: { fields: ["tagline", "title"] } });
  });

  it("starts the fetch, then shows it running with the button off", () => {
    render(<PluginMetadataSection pluginId="allocine" />);
    fireEvent.click(screen.getByTestId("metadata-refresh"));
    expect(api.refresh).toHaveBeenCalledWith({ pluginId: "allocine", data: {} });
  });

  it("shows a running fetch and the last finished one", () => {
    api.data = { ...api.data, refresh: { running: true, done: 1, total: 4 } };
    const { unmount } = render(<PluginMetadataSection pluginId="allocine" />);
    expect(screen.getByTestId("metadata-refresh-running")).toBeTruthy();
    expect((screen.getByTestId("metadata-refresh") as HTMLButtonElement).disabled).toBe(true);
    unmount();
    api.data = { ...api.data, refresh: { done: 4, total: 4, covered: 3, failed: 0, finishedAt: new Date(0).toISOString() } };
    render(<PluginMetadataSection pluginId="allocine" />);
    expect(screen.getByTestId("metadata-refresh-finished")).toBeTruthy();
  });

  it("locks every write on the demo", () => {
    store.locked = true;
    render(<PluginMetadataSection pluginId="allocine" />);
    expect((screen.getByTestId("metadata-save") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("metadata-refresh") as HTMLButtonElement).disabled).toBe(true);
  });
});
