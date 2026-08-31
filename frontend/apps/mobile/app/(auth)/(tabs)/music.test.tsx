import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Regression: opening the Music tab crashed the whole screen with "Rendered
// more hooks than during the previous render" the moment the libraries query
// resolved (tester report, 2026-08-31). Hooks declared BELOW the loading/empty
// early returns run on the second render but not the first - the exact class
// the in-file comment already warned about for renderArtist; retryTab
// reintroduced it. This test renders the pending state first, then lets the
// query resolve: with the defect, the re-render throws and waitFor rejects.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  TouchableOpacity: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      null,
      (data ?? []).length === 0
        ? (ListEmptyComponent ?? null)
        : data.map((item: any, i: number) =>
            React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, i) : i }, renderItem({ item, index: i })),
          ),
    ),
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1, absoluteFillObject: {} },
  Alert: { alert: vi.fn() },
}));
vi.mock("expo-image", () => ({ Image: (p: any) => React.createElement("img", { alt: "", ...p }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: (p: any) => React.createElement("span", p) }));
vi.mock("expo-router", () => ({ useLocalSearchParams: () => ({}), router: { push: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", surfaceRaised: "#222", border: "#333" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 4, md: 8, lg: 12, xl: 16 },
  }),
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({}) }));
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv" }));
vi.mock("../../../src/hooks/useDebounce", () => ({ useDebounce: (v: string) => v }));
vi.mock("../../../src/stores/musicQueue", () => ({
  useMusicQueueStore: (sel: (s: any) => unknown) =>
    sel({ queue: [], currentIndex: 0, setQueue: vi.fn(), playTrack: vi.fn(), removeFromQueue: vi.fn() }),
}));
vi.mock("../../../src/components/Skeleton", () => ({ Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }) }));
vi.mock("../../../src/components/AddToPlaylistSheet", () => ({ AddToPlaylistSheet: () => null }));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: () => React.createElement("div", { "data-testid": "load-failed" }) }));
vi.mock("../../../src/components/music/TrackArt", () => ({ TrackArt: () => null }));
vi.mock("../../../src/components/music/MusicPlaylistsTab", () => ({ MusicPlaylistsTab: () => null }));
vi.mock("../../../src/lib/downloads", () => ({
  startDownload: vi.fn(),
  getDownloadPath: vi.fn().mockResolvedValue(null),
  deleteDownload: vi.fn(),
  downloadErrorMessageKey: () => "err",
}));
vi.mock("../../../src/lib/mobileDataGate", () => ({ confirmDownloadOnMobileData: vi.fn().mockResolvedValue(true) }));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  useAuthStore: (sel: (s: any) => unknown) => sel({ profileId: "p1" }),
}));

const { fetchState } = vi.hoisted(() => ({
  fetchState: { resolveLibraries: null as ((v: unknown) => void) | null },
}));
vi.mock("@evey/api", () => ({
  formatTime: (s: number) => String(s),
  hasMoreFromCursor: () => false,
  useUserServiceGetCurrentUser: () => ({ data: undefined }),
  useUserServiceListProfiles: () => ({ data: undefined }),
  eveyFetch: ({ url }: { url: string }) => {
    if (url === "/api/v1/libraries") {
      // Held open so the test controls the pending -> resolved transition.
      return new Promise((resolve) => {
        fetchState.resolveLibraries = resolve;
      });
    }
    return Promise.resolve({ tracks: [], albums: [], artists: [] });
  },
}));

import MusicScreen from "./music";

beforeEach(() => {
  fetchState.resolveLibraries = null;
});

describe("MusicScreen", () => {
  it("survives the libraries query resolving after the loading render (hooks order)", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <MusicScreen />
      </QueryClientProvider>,
    );
    // First render: the loading early return.
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);

    // The query resolves: the full screen must render - with a hook declared
    // below the early returns, React throws right here instead.
    fetchState.resolveLibraries?.({ libraries: [{ id: "l1", name: "Musique", type: "LIBRARY_TYPE_MUSIC" }] });
    await waitFor(() => expect(screen.getByText("media.music.tracks")).toBeTruthy());
    // The tab bar and title are up - the screen survived the render with more
    // hooks. (The tracks tab may legitimately show its own loading skeletons
    // while its query runs; only the hooks crash is under test here.)
    expect(screen.getByText("media.music.title")).toBeTruthy();
  });
});
