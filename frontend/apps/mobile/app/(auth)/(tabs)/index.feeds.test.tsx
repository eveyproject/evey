import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";

// The customize panel's handlers must serialise: two taps that land in the
// same tick (before React re-renders) must each build on the other's result,
// the way the old setState-updater code did - and still persist only the rows
// each tap changed, from the handler (never from inside an updater, which
// StrictMode runs twice).

// The collections row reads the shared layer (generated API); this screen's
// tests are about the other rows, so it is doubled at the app's own path.
vi.mock("../../../src/api/collections", () => ({
  useCollections: () => ({ data: { collections: [] }, isLoading: false, isError: false, refetch: vi.fn() }),
}));
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  RefreshControl: () => null,
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  Switch: () => null,
  AppState: { addEventListener: () => ({ remove: () => {} }) },
  StyleSheet: { create: (s: any) => s },
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
  Alert: { alert: vi.fn() },
}));
vi.mock("expo-image", () => ({ Image: () => null }));
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useFocusEffect: () => {},
  Link: ({ children }: any) => children,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : k), i18n: { language: "en" } }),
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#aaa", accent: "#0af", border: "#333", onAccent: "#000", error: "#f00" },
    spacing: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../../../src/hooks/useWatchedMap", () => ({ useWatchedMap: () => ({ get: () => false }) }));
vi.mock("../../../src/features/plugins/PluginSlot", () => ({ PluginSlot: () => null }));
vi.mock("../../../src/components/Skeleton", () => ({ Skeleton: () => null, SkeletonRow: () => null }));
vi.mock("../../../src/components/MediaRow", () => ({ MediaRow: () => null, default: () => null }));
vi.mock("../../../src/components/CollectionRow", () => ({ CollectionRow: () => null, default: () => null }));
vi.mock("@evey/store", () => ({ useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }), isDemoMode: () => false,
  useAuthStore: Object.assign((sel: any) => sel({ accessToken: "t", profileId: "p1", user: { id: "u1" } }), {
    getState: () => ({ accessToken: "t", profileId: "p1", user: { id: "u1" } }),
  }),
}));
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn().mockResolvedValue(undefined) }),
  useQuery: (opts: { queryKey: readonly unknown[] }) => {
    if (opts.queryKey[0] === "popular") return { data: [], isLoading: false, isError: false, refetch: vi.fn() };
    if (opts.queryKey[0] === "collections") return { data: { collections: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    return { data: {}, isLoading: false, isError: false, refetch: vi.fn() };
  },
}));
const settled = { data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() };
const updateMutateAsync = vi.fn().mockResolvedValue({});
vi.mock("@evey/api", () => ({
  usePlaybackServiceGetResumeItems: () => settled,
  usePlaybackServiceGetNextUp: () => settled,
  useFeedServiceRecommended: () => settled,
  useFeedServiceGetFeedConfig: () => ({ data: { configs: [] } }),
  useFeedServiceUpdateProfileFeedConfig: () => ({ mutateAsync: updateMutateAsync }),
  useFeedServiceResetProfileFeedConfig: () => ({ mutateAsync: vi.fn().mockResolvedValue({}) }),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] } }),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] }, isLoading: false }),
  useActivityServiceGetPopularMedia: () => ({ data: [], isLoading: false, isError: false, refetch: vi.fn() }),
  eveyFetch: vi.fn().mockResolvedValue({}),
}));

// The long-press sheet has its own test; the home only mounts it.
vi.mock("../../../src/components/MediaActionsSheet", () => ({ MediaActionsSheet: () => null }));

import HomeScreen from "./index";

const FEED_LABEL_KEYS = new Set(["common.feeds.labels.resume_playback", "media.home.upNext", "media.home.collections", "media.home.recommendedForYou", "media.home.mostPopular"]);

beforeEach(() => {
  updateMutateAsync.mockClear();
});

describe("HomeScreen - customize panel handlers serialise fast taps", () => {
  it("two 'move down' taps in one tick move the first row down twice and persist both moves", async () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("common.feeds.customize"));
    const downButtons = screen.getAllByLabelText("Move section down");
    // Two taps in ONE React batch (one act, native clicks): the handlers run
    // back to back before any re-render, each with the closure it captured.
    // Tap 1 moves row 0 (resume_playback) down; tap 2 moves row 1 down - and
    // row 1 is, by then, resume_playback again. Serialised, the result is
    // [next_episode, collections, resume_playback]; computed from the same
    // stale list, the second tap would swap next_episode/collections instead
    // and its state write would silently discard the first tap.
    act(() => {
      downButtons[0].dispatchEvent(new MouseEvent("click", { bubbles: true }));
      downButtons[1].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await Promise.resolve();
    await Promise.resolve();

    const modal = screen.getByTestId("modal");
    const rows = Array.from(modal.querySelectorAll("span"))
      .map((el) => el.textContent ?? "")
      .filter((text) => FEED_LABEL_KEYS.has(text));
    expect(rows.slice(0, 3)).toEqual(["media.home.upNext", "media.home.collections", "common.feeds.labels.resume_playback"]);

    // Every persisted row reflects the cumulative order.
    const persisted = updateMutateAsync.mock.calls.map((c) => c[0] as { feedType: string; data: { displayOrder?: number } });
    const byType = new Map<string, number | undefined>();
    for (const p of persisted) byType.set(p.feedType, p.data.displayOrder); // last write wins
    expect(byType.get("next_episode")).toBe(0);
    expect(byType.get("collections")).toBe(1);
    expect(byType.get("resume_playback")).toBe(2);
  });
});
