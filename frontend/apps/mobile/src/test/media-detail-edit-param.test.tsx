import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The library's long-press bar offers "Edit" on one selected title (the web
// card menu's "Edit metadata"): it opens the title with ?edit=1, and the
// metadata editor must open on arrival - for an admin only.

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  Alert: { alert: vi.fn() },
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      null,
      (data ?? []).map((item: any, i: number) =>
        React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, i) : i }, renderItem({ item, index: i })),
      ),
    ),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Linking: { openURL: vi.fn() },
  Dimensions: {
    get: () => ({ width: 400, height: 800 }),
    addEventListener: () => ({ remove: vi.fn() }),
  },
  Share: { share: vi.fn() },
  Platform: { OS: "android", select: (o: any) => o.android },
  AppState: {
    addEventListener: (_e: string, cb: (s: string) => void) => {
      appStateRef.current = cb;
      return { remove: vi.fn() };
    },
  },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("react-native-svg", () => ({
  default: () => null,
  Defs: () => null,
  LinearGradient: () => null,
  Rect: () => null,
  Stop: () => null,
}));
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
    i18n: { language: "en" },
  }),
}));

// --- expo-router ---------------------------------------------------------------------
const { MOVIE_ID } = vi.hoisted(() => ({ MOVIE_ID: "movie-1" }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: MOVIE_ID, edit: editParamRef.current }),
  router: { push: vi.fn(), back: vi.fn(), replace: vi.fn() },
  // The screen resets its double-tap navigation latch on focus; no real
  // navigator here, so run the callback once on mount (same shape as
  // media-detail-episodes.test.tsx).
  useFocusEffect: (cb: () => void | (() => void)) => {
    React.useEffect(() => cb(), [cb]);
  },
}));

// --- react-native-safe-area-context ---------------------------------------------------
vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));

// --- ThemeProvider -------------------------------------------------------------------
const MOCK_THEME = {
  colors: {
    bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
    textMuted: "#888", accent: "#f00", onAccent: "#fff", border: "#333",
    error: "#e00", warning: "#fa0", success: "#0f0",
  },
  radii: { lg: 12, md: 8, sm: 4 },
  spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
};
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => MOCK_THEME }));
vi.mock("../components/media/PlayOnSheet", () => ({ PlayOnSheet: () => null }));

// --- misc hooks/libs the screen depends on, stubbed to keep this a focused
// --- render test of the "add to collection" button rather than an integration test ---
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => undefined,
  useAuthenticatedImage: () => undefined,
}));
vi.mock("../hooks/useFavorite", () => ({
  useIsFavorite: () => ({ data: false }),
  useToggleFavorite: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("../hooks/useParentalGate", () => ({
  useParentalGate: () => ({ needsPin: () => false, profileId: null }),
}));
vi.mock("../lib/downloads", () => ({
  startDownload: vi.fn(),
  getDownloadPath: vi.fn().mockResolvedValue(null),
  deleteDownload: vi.fn(),
  downloadErrorMessageKey: () => "downloads.error",
  subscribeToProgress: () => () => undefined,
  isPreparingCompat: () => false,
  getCompatPreparePermille: () => 0,
}));
vi.mock("../lib/reducedDownload", () => ({ startReducedDownload: vi.fn() }));
vi.mock("../lib/downloadQualityGate", () => ({
  chooseDownloadQuality: vi.fn(async () => ({ quality: "original" })),
}));
vi.mock("../components/AddToPlaylistSheet", () => ({ AddToPlaylistSheet: () => null }));
vi.mock("../components/ParentalPINModal", () => ({ ParentalPINModal: () => null }));
vi.mock("../components/media/MetadataEditSheet", () => ({
  MetadataEditSheet: (p: { visible: boolean }) => metadataSheetProps(p),
}));

const { collectionSheetProps } = vi.hoisted(() => ({ collectionSheetProps: vi.fn() }));
vi.mock("../components/library/BulkAddSheets", () => ({
  BulkAddToCollectionSheet: (props: any) => {
    collectionSheetProps(props);
    return null;
  },
}));

// --- @evey/store -----------------------------------------------------------------------
const { isAdminRef, editParamRef, metadataSheetProps, appStateRef, getWatchProgress, refetchProgress } = vi.hoisted(() => {
  const refetchProgress = vi.fn();
  return {
    appStateRef: { current: null as ((s: string) => void) | null },
    refetchProgress,
    getWatchProgress: vi.fn((..._args: unknown[]) => ({ data: undefined, refetch: refetchProgress })),
    isAdminRef: { current: true },
    editParamRef: { current: undefined as string | undefined },
    metadataSheetProps: vi.fn((_p: { visible: boolean }) => null),
  };
});
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) =>
    selector({ profileId: "profile-1", isAdmin: isAdminRef.current, accessToken: "tok" }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  useDemoStore: (selector: (s: any) => unknown) => selector({ demoMode: false }),
}));

// --- @evey/api -------------------------------------------------------------------------
const movieItem = {
  id: MOVIE_ID,
  type: "MEDIA_ITEM_TYPE_MOVIE",
  title: "The Movie",
  people: [],
};

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: () => ({ data: movieItem, isLoading: false }),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] } }),
  usePlaybackServiceGetWatchProgress: (...args: unknown[]) => getWatchProgress(...args),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), isPending: false }),
  useFeedServiceSimilarItems: () => ({ data: { items: [] } }),
  useUserServiceGetCurrentUser: () => ({ data: { id: "user-1" } }),
  useUserServiceListProfiles: () => ({ data: { profiles: [{ id: "profile-1", isDefault: true, region: "US" }] } }),
  useShareServiceCreateShareLink: () => ({ mutate: vi.fn(), isPending: false }),
  useRatingServiceGetMyThumbRating: () => ({ data: undefined }),
  ratingServicePutThumbRating: vi.fn(() => Promise.resolve({})),
  getRatingServiceGetMyThumbRatingQueryKey: () => ["rating"],
}));

// -------------------------------------------------------------------------------------
import MediaDetailScreen from "../../app/(auth)/media/[id]";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  isAdminRef.current = true;
  editParamRef.current = undefined;
});

// The library's long-press "Edit" on one title lands here with ?edit=1: the
// editor opens on arrival for an admin, never for a viewer.
describe("MediaDetailScreen ?edit=1", () => {
  it("opens the metadata editor on arrival for an admin", () => {
    editParamRef.current = "1";
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    expect(metadataSheetProps).toHaveBeenLastCalledWith(expect.objectContaining({ visible: true }));
  });

  it("stays closed without the parameter", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    expect(metadataSheetProps).toHaveBeenLastCalledWith(expect.objectContaining({ visible: false }));
  });

  it("never mounts the editor for a viewer, even with ?edit=1", () => {
    isAdminRef.current = false;
    editParamRef.current = "1";
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    expect(metadataSheetProps).not.toHaveBeenCalled();
  });
});

// 27-09 (the owner, on the TV; same stack here): a Resume label read before the
// player, and never again, outlived autoplay and a night in the background.
describe("MediaDetailScreen resume progress freshness", () => {
  it("reads the progress with no stale window", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    const opts = getWatchProgress.mock.calls[0]?.[2] as { query?: { staleTime?: number } };
    expect(opts?.query?.staleTime).toBe(0);
  });

  it("re-reads the progress when the app comes back to the foreground", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    expect(refetchProgress).not.toHaveBeenCalled();
    act(() => appStateRef.current?.("active"));
    expect(refetchProgress).toHaveBeenCalledTimes(1);
  });
});
