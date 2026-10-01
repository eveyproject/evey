import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// History pages by cursor as the list scrolls (30 a page); each row opens
// its title and has its own focusable Remove (a remote has no swipe), which
// unwatches and refreshes the list; the first row takes the initial focus;
// the empty state keeps a focusable way out; a failed fetch is a retry.
const list = vi.hoisted(() => ({ onEndReached: null as null | (() => void) }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor, onEndReached }: any) => {
    list.onEndReached = onEndReached;
    return React.createElement("div", { "data-testid": "list" }, (data as any[]).map((item, index) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item, index }))));
  },
  StyleSheet: { create: (s: any) => s },
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const api = vi.hoisted(() => ({ history: vi.fn(), unwatchOpts: null as any, unwatch: vi.fn() }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  playbackServiceGetWatchHistory: (...a: unknown[]) => api.history(...a),
  usePlaybackServiceMarkAsUnwatched: (o: any) => {
    api.unwatchOpts = o.mutation;
    return { mutate: api.unwatch, isPending: false };
  },
  getPlaybackServiceGetWatchHistoryQueryKey: () => ["/api/v1/playback/history"],
}));
const toast = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }) }));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", danger: "#f33" }, radii: { md: 8 } }),
}));
vi.mock("../../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, hasTVPreferredFocus, accessibilityLabel, disabled }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, "data-focus": String(!!hasTVPreferredFocus) }, children),
}));
vi.mock("../../src/components/ScreenState", () => ({
  ScreenState: ({ loading, error, onRetry }: any) =>
    React.createElement("div", { "data-testid": "state", "data-loading": String(loading), "data-error": String(error) }, React.createElement("button", { onClick: onRetry }, "retry")),
}));

import HistoryScreen from "./history";

let qc: QueryClient;
function mount() {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><HistoryScreen /></QueryClientProvider>);
}

const row = (id: string, over: Record<string, unknown> = {}) => ({ id, mediaItemId: `m-${id}`, title: `Title ${id}`, percentComplete: 40, lastPlayedAt: "2026-09-16T04:00:00Z", ...over });

beforeEach(() => {
  vi.clearAllMocks();
  // Queued once-values from a failed test must not leak into the next one.
  api.history.mockReset();
  list.onEndReached = null;
});

describe("tv history screen", () => {
  it("pages by cursor as the list scrolls, focuses the first row, opens a title and removes one", async () => {
    api.history
      .mockResolvedValueOnce({ items: [row("a", { playCount: 3, positionMs: "60000", durationMs: "120000", percentComplete: 50 }), row("b")], pagination: { nextCursor: "c1" } })
      .mockResolvedValueOnce({ items: [row("c")], pagination: {} });
    mount();
    await waitFor(() => expect(screen.getByRole("button", { name: "Title a" })).toBeTruthy());
    expect(api.history.mock.calls[0][0]).toEqual({ "pagination.pageSize": 30 });
    expect(screen.getByRole("button", { name: "Title a" }).getAttribute("data-focus")).toBe("true");
    expect(screen.getByRole("button", { name: "Title b" }).getAttribute("data-focus")).toBe("false");
    expect(screen.getByText("Title a (x3)")).toBeTruthy();
    expect(screen.getByText("1min / 2min")).toBeTruthy();
    expect(screen.getByText("50%")).toBeTruthy();

    list.onEndReached?.();
    await waitFor(() => expect(screen.getByRole("button", { name: "Title c" })).toBeTruthy());
    expect(api.history.mock.calls[1][0]).toEqual({ "pagination.pageSize": 30, "pagination.cursor": "c1" });
    list.onEndReached?.();
    expect(api.history).toHaveBeenCalledTimes(2);

    fireEvent.click(screen.getByRole("button", { name: "Title b" }));
    expect(router.push).toHaveBeenCalledWith("/media/m-b");
    fireEvent.click(screen.getAllByRole("button", { name: "Remove from history" })[1]);
    expect(api.unwatch).toHaveBeenCalledWith({ mediaItemId: "m-b" });
    const spy = vi.spyOn(qc, "invalidateQueries");
    api.unwatchOpts.onSuccess();
    expect(spy).toHaveBeenCalledWith({ queryKey: ["/api/v1/playback/history"] });
    api.unwatchOpts.onError();
    expect(toast.error).toHaveBeenCalledWith("Could not remove from history");
  });

  it("keeps a focusable way out of the empty state", async () => {
    api.history.mockResolvedValue({ items: [], pagination: {} });
    mount();
    await waitFor(() => expect(screen.getByText("auth.watchHistory.noHistory")).toBeTruthy());
    const out = screen.getByRole("button", { name: "media.library.libraries" });
    expect(out.getAttribute("data-focus")).toBe("true");
    fireEvent.click(out);
    expect(router.push).toHaveBeenCalledWith("/(auth)/(tabs)/library");
  });

  it("a failed fetch is a retry, not an empty history", async () => {
    api.history.mockRejectedValueOnce(new Error("500")).mockResolvedValueOnce({ items: [], pagination: {} });
    mount();
    await waitFor(() => expect(screen.getByTestId("state").getAttribute("data-error")).toBe("true"));
    expect(screen.queryByText("auth.watchHistory.noHistory")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    await waitFor(() => expect(screen.getByText("auth.watchHistory.noHistory")).toBeTruthy());
  });
});
