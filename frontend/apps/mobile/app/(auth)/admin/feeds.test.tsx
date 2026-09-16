import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Home sections in display order, library rows named after their library; a
// plugin with a home.section slot but no admin row yet appears at the end,
// enabled, and can be switched off but never reordered (each user keeps
// their own order); max-items is clamped to 1..100 and a blank keeps the old
// value; Save appears only once dirty and sends every row, then refreshes
// both the admin and the merged home config.
const header = vi.hoisted(() => ({ right: null as null | (() => React.ReactNode) }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  RefreshControl: () => null,
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("input", { type: "checkbox", role: "switch", checked: value, onChange: () => onValueChange(!value) }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: disabled ? undefined : onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({
  Stack: { Screen: ({ options }: any) => (options.headerRight ? React.createElement("div", { "data-testid": "header" }, options.headerRight()) : null) },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const api = vi.hoisted(() => ({
  feeds: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  libraries: undefined as unknown,
  plugins: undefined as unknown,
  batchOpts: null as any,
  batch: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  useFeedServiceGetAdminFeedConfigs: () => api.feeds,
  useLibraryServiceListLibraries: () => ({ data: api.libraries }),
  useAdminPluginServiceListPlugins: () => ({ data: api.plugins }),
  useFeedServiceBatchUpdateAdminFeedConfigs: (o: any) => {
    api.batchOpts = o.mutation;
    return { mutateAsync: api.batch, isPending: false };
  },
  getFeedServiceGetAdminFeedConfigsQueryKey: () => ["admin-feeds"],
  getFeedServiceGetFeedConfigQueryKey: () => ["feed-config"],
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" }, radii: { md: 8, sm: 4 } }),
}));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import AdminFeedsScreen from "./feeds";

beforeEach(() => {
  vi.clearAllMocks();
  header.right = null;
  api.feeds = {
    data: { configs: [
      { feedType: "next_up", enabled: true, displayOrder: 1, maxItems: 20 },
      { feedType: "resume_playback", enabled: true, displayOrder: 0, maxItems: 10 },
      { feedType: "library:lib-1", enabled: false, displayOrder: 2, maxItems: 30 },
    ] },
    isLoading: false, isError: false, refetch: vi.fn(),
  };
  api.libraries = { libraries: [{ id: "lib-1", name: "Films" }] };
  api.plugins = { plugins: [
    { id: "weather", name: "Weather", enabled: true, frontend: [{ slot: "home.section" }] },
    { id: "hidden", name: "Hidden", enabled: true, frontend: [{ slot: "settings.user" }] },
    { id: "off", name: "Off", enabled: false, frontend: [{ slot: "home.section" }] },
  ] };
  api.batch.mockImplementation(async (vars: unknown) => { api.batchOpts.onSuccess({}, vars); return {}; });
});

describe("admin feeds screen (mobile)", () => {
  it("orders rows by display order, names library rows, and parks a new home plugin at the end", () => {
    render(<AdminFeedsScreen />);
    expect(screen.getByText("Films")).toBeTruthy();
    expect(screen.getByText("Weather")).toBeTruthy();
    expect(screen.queryByText("Hidden")).toBeNull();
    expect(screen.queryByText("Off")).toBeNull();
    expect(screen.getByText("Plugin")).toBeTruthy();
    const switches = screen.getAllByRole("switch") as HTMLInputElement[];
    expect(switches.map((s) => s.checked)).toEqual([true, true, false, true]);
    // Move arrows: the plugin row's are disabled, and the row above it cannot move down onto it.
    const downs = screen.getAllByRole("button", { name: "common.feeds.moveDown" }) as HTMLButtonElement[];
    expect(downs.at(-1)!.disabled).toBe(true);
    expect(screen.queryByTestId("header")).toBeNull();
  });

  it("toggles, reorders regular rows, clamps max-items, and saves every row then refreshes both configs", async () => {
    render(<AdminFeedsScreen />);
    fireEvent.click(screen.getAllByRole("button", { name: "common.feeds.moveUp" })[1]);
    expect(screen.getByTestId("header")).toBeTruthy();
    const max = screen.getByLabelText("Éléments max: common.feeds.labels.resume_playback") as HTMLInputElement;
    fireEvent.change(max, { target: { value: "500" } });
    expect(max.value).toBe("100");
    fireEvent.change(max, { target: { value: "" } });
    expect(max.value).toBe("100");
    fireEvent.change(max, { target: { value: "0" } });
    expect(max.value).toBe("1");
    fireEvent.click(screen.getAllByRole("switch")[3]);

    fireEvent.click(screen.getByText("common.save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("common.feeds.saved"));
    const sent = api.batch.mock.calls[0][0].data.configs;
    expect(sent.map((r: { feedType: string }) => r.feedType)).toEqual(["next_up", "resume_playback", "library:lib-1", "plugin:weather"]);
    expect(sent.map((r: { displayOrder: number }) => r.displayOrder)).toEqual([0, 1, 2, 3]);
    expect(sent[1].maxItems).toBe(1);
    expect(sent[3].enabled).toBe(false);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["admin-feeds"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["feed-config"] });
    await waitFor(() => expect(screen.queryByTestId("header")).toBeNull());
  });

  it("keeps a background refetch from clobbering unsaved edits, and separates empty, loading and failure", () => {
    const { rerender } = render(<AdminFeedsScreen />);
    fireEvent.click(screen.getAllByRole("switch")[0]);
    api.feeds = { ...api.feeds, data: { configs: [{ feedType: "next_up", enabled: true, displayOrder: 0, maxItems: 5 }] } };
    rerender(<AdminFeedsScreen />);
    expect(screen.getAllByRole("switch")).toHaveLength(4);
    expect((screen.getAllByRole("switch")[0] as HTMLInputElement).checked).toBe(false);

    api.feeds = { data: { configs: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    api.plugins = { plugins: [] };
    rerender(<AdminFeedsScreen />);
    // Still dirty: the local rows stand until saved.
    expect(screen.getAllByRole("switch")).toHaveLength(4);

    api.feeds = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    rerender(<AdminFeedsScreen />);
    fireEvent.click(screen.getByText("retry"));
    expect(api.feeds.refetch).toHaveBeenCalledTimes(1);
  });
});
