import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// AddCollectionItem RPC requires RoleAdmin server-side (interceptors/rbac.go).
// The media detail page's "add to collection" button - the only place a saga
// is naturally grouped while looking at the title itself - must be admin-only
// like the same control on the library/favorites bulk action bars.

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
  AppState: { addEventListener: () => ({ remove: vi.fn() }) },
}));

// --- expo-image -------------------------------------------------------------------------
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
  useLocalSearchParams: () => ({ id: MOVIE_ID }),
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
vi.mock("../components/media/MetadataEditSheet", () => ({ MetadataEditSheet: () => null }));

const { collectionSheetProps } = vi.hoisted(() => ({ collectionSheetProps: vi.fn() }));
vi.mock("../components/library/BulkAddSheets", () => ({
  BulkAddToCollectionSheet: (props: any) => {
    collectionSheetProps(props);
    return null;
  },
}));

// --- @evey/store -----------------------------------------------------------------------
const { isAdminRef } = vi.hoisted(() => ({ isAdminRef: { current: true } }));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) =>
    selector({ profileId: "profile-1", isAdmin: isAdminRef.current, accessToken: "tok" }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
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
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
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
});

describe("MediaDetailScreen add to collection", () => {
  it("shows the button for an admin and opens the sheet on the right id", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    const button = screen.getByTestId("add-to-collection-button");
    expect(button).toBeTruthy();

    // Closed by default; opening it is what wires the sheet's mediaItemIds.
    expect(collectionSheetProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: false, mediaItemIds: [MOVIE_ID] }),
    );

    fireEvent.click(button);

    expect(collectionSheetProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: true, mediaItemIds: [MOVIE_ID] }),
    );
  });

  it("hides the button entirely for a non-admin", () => {
    isAdminRef.current = false;
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    expect(screen.queryByTestId("add-to-collection-button")).toBeNull();
    // Defense in depth: the sheet itself must not even mount for a non-admin,
    // not just be unreachable via the button.
    expect(collectionSheetProps).not.toHaveBeenCalled();
  });
});
