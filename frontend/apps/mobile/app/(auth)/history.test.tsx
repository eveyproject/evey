import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

// History pages 30 at a time as the list scrolls, appending; a page shorter
// than 30 is the last one; a row opens its title and its own action removes
// it; clearing everything is confirmed first; a failed FIRST fetch is a
// retry, but a failure after some rows are shown keeps them.
const list = vi.hoisted(() => ({ onEndReached: null as null | (() => void) }));
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  SectionList: ({ sections, renderItem, keyExtractor, renderSectionHeader, ListEmptyComponent, onEndReached }: any) => {
    list.onEndReached = onEndReached;
    const all = (sections as any[]).flatMap((s) => s.data);
    return React.createElement("div", null, all.length === 0
      ? ListEmptyComponent
      : (sections as any[]).map((section, si) => React.createElement("div", { key: si }, renderSectionHeader({ section }), section.data.map((item: any, index: number) => React.createElement(React.Fragment, { key: keyExtractor(item, index) }, renderItem({ item, index }))))));
  },
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel, disabled }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (title: string, message: string, buttons: any[]) => { alerts.last = { title, message, buttons }; } },
  RefreshControl: () => null,
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: () => null }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({
  router,
  Stack: { Screen: ({ options }: any) => React.createElement("div", null, options.headerRight?.()) },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const api = vi.hoisted(() => ({
  pages: {} as Record<number, unknown[]>,
  // One response object per page: a fresh object every render would re-run
  // the screen's append effect and duplicate rows.
  responses: {} as Record<number, { items: unknown[] }>,
  error: false,
  lastArgs: null as any,
  refetch: vi.fn(),
  unwatch: vi.fn(), unwatchOpts: null as any,
  clear: vi.fn(), clearOpts: null as any,
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  usePlaybackServiceGetWatchHistory: (p: { "pagination.page": number }) => {
    api.lastArgs = p;
    const page = p["pagination.page"];
    const items = api.pages[page];
    if (items && !api.responses[page]) api.responses[page] = { items };
    return { data: items && !api.error ? api.responses[page] : undefined, isLoading: false, isFetching: false, isError: api.error, refetch: api.refetch };
  },
  usePlaybackServiceMarkAsUnwatched: (o: any) => { api.unwatchOpts = o.mutation; return { mutate: api.unwatch, isPending: false }; },
  usePlaybackServiceClearWatchHistory: (o: any) => { api.clearOpts = o.mutation; return { mutate: api.clear, isPending: false }; },
  getPlaybackServiceGetWatchHistoryQueryKey: () => ["history"],
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", success: "#0f0", danger: "#f33" }, radii: { md: 8, sm: 4 }, spacing: { xs: 4, sm: 8, md: 16, xl: 32 } }),
}));
vi.mock("../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "https://s" }));
vi.mock("../../src/hooks/useAuthenticatedImage", () => ({ useAuthenticatedImage: (uri?: string) => (uri ? { uri } : undefined) }));
vi.mock("../../src/components/ProgressBar", () => ({ ProgressBar: () => null }));
vi.mock("../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));
vi.mock("../../src/components/Skeleton", () => ({ Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }) }));
vi.mock("../../src/lib/historyGrouping", () => ({
  groupHistoryByDate: (items: unknown[]) => (items.length ? [{ title: "Today", data: items }] : []),
}));

import HistoryScreen from "./history";

const row = (id: string) => ({ id, mediaItemId: `m-${id}`, title: `Title ${id}`, percentComplete: 10, lastPlayedAt: "2026-09-16T04:00:00Z" });
const rows = (prefix: string, n: number) => Array.from({ length: n }, (_, i) => row(`${prefix}${i}`));

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  api.pages = {};
  api.responses = {};
  api.error = false;
  list.onEndReached = null;
});

describe("history screen (mobile)", () => {
  it("pages 30 at a time as the list scrolls, appends, and stops after a short page", () => {
    api.pages = { 1: rows("a", 30), 2: rows("b", 2) };
    render(<HistoryScreen />);
    expect(api.lastArgs).toEqual({ "pagination.page": 1, "pagination.pageSize": 30 });
    expect(screen.getAllByRole("button", { name: /^Title / })).toHaveLength(30);
    act(() => list.onEndReached?.());
    expect(api.lastArgs["pagination.page"]).toBe(2);
    expect(screen.getAllByRole("button", { name: /^Title / })).toHaveLength(32);
    act(() => list.onEndReached?.());
    expect(api.lastArgs["pagination.page"]).toBe(2);
  });

  it("opens a title, removes one through its own action, and confirms before clearing everything", () => {
    api.pages = { 1: rows("a", 2) };
    render(<HistoryScreen />);
    fireEvent.click(screen.getByRole("button", { name: "Title a1" }));
    expect(router.push).toHaveBeenCalledWith("/media/m-a1");
    fireEvent.click(screen.getAllByRole("button", { name: "Remove from history" })[0]);
    expect(api.unwatch).toHaveBeenCalledWith({ mediaItemId: "m-a0" });
    api.unwatchOpts.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["history"] });
    api.unwatchOpts.onError();
    expect(toast.error).toHaveBeenCalledWith("Failed to remove from history");

    fireEvent.click(screen.getByRole("button", { name: "auth.watchHistory.clearHistory" }));
    expect(api.clear).not.toHaveBeenCalled();
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(api.clear).toHaveBeenCalledWith({});
    api.clearOpts.onSuccess();
    expect(toast.success).toHaveBeenCalledWith("History cleared");
  });

  it("a failed first fetch is a retry; an empty history says so", () => {
    api.error = true;
    const first = render(<HistoryScreen />);
    expect(screen.queryByText("No watch history")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(api.refetch).toHaveBeenCalledTimes(1);
    first.unmount();

    api.error = false;
    api.pages = { 1: [] };
    render(<HistoryScreen />);
    expect(screen.getByText("No watch history")).toBeTruthy();
  });
});
