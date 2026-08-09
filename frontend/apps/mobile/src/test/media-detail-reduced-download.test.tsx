import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The sliced 720p download fails its own way - a slice the server never
// produced, a join the device refused - and reducedDownloadMessage names each
// of those. The screen used to send every failure through the generic download
// mapping instead, so the user was told "download failed" with no reason.

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
  useFocusEffect: (cb: () => void | (() => void)) => {
    React.useEffect(() => cb(), []);
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
  downloadErrorMessageKey: () => "media.detail.downloadFailed",
  subscribeToProgress: () => () => undefined,
  isPreparingCompat: () => false,
  getCompatPreparePermille: () => 0,
}));
// Fully mocked: importing the real module pulls expo-file-system and the native
// background downloader at import time. reducedDownloadMessage is the seam
// under test, so it answers with the key it would really return for a refused
// join - what matters here is which of the two mappings the screen consults.
const { startReducedDownloadMock, reducedDownloadMessageMock } = vi.hoisted(() => ({
  startReducedDownloadMock: vi.fn(),
  reducedDownloadMessageMock: vi.fn(() => "downloads.reduced.errorJoin"),
}));
vi.mock("../lib/reducedDownload", () => ({
  startReducedDownload: startReducedDownloadMock,
  reducedDownloadMessage: reducedDownloadMessageMock,
}));
const { chooseDownloadQualityMock } = vi.hoisted(() => ({ chooseDownloadQualityMock: vi.fn() }));
vi.mock("../lib/downloadQualityGate", () => ({ chooseDownloadQuality: chooseDownloadQualityMock }));
vi.mock("../lib/mobileDataGate", () => ({ confirmDownloadOnMobileData: vi.fn(async () => true) }));
vi.mock("../components/AddToPlaylistSheet", () => ({ AddToPlaylistSheet: () => null }));
vi.mock("../components/ParentalPINModal", () => ({ ParentalPINModal: () => null }));
vi.mock("../components/media/MetadataEditSheet", () => ({ MetadataEditSheet: () => null }));
vi.mock("../components/library/BulkAddSheets", () => ({ BulkAddToCollectionSheet: () => null }));

// --- @evey/store -----------------------------------------------------------------------
const { toastMock } = vi.hoisted(() => ({
  toastMock: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) =>
    selector({ profileId: "profile-1", isAdmin: false, accessToken: "tok" }),
  toast: toastMock,
}));

// --- @evey/api -------------------------------------------------------------------------
// fileSize is what makes the download button render at all (hasMediaFile).
const movieItem = {
  id: MOVIE_ID,
  type: "MEDIA_ITEM_TYPE_MOVIE",
  title: "Some Movie",
  fileSize: "8000000000",
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

const REDUCED_PLAN = { durationMs: 1_800_000, slices: 3, height: 720 };

beforeEach(() => {
  vi.clearAllMocks();
  reducedDownloadMessageMock.mockReturnValue("downloads.reduced.errorJoin");
});

describe("MediaDetailScreen reduced download failure", () => {
  it("names the reduced-download reason instead of the generic download failure", async () => {
    chooseDownloadQualityMock.mockResolvedValue({ quality: "reduced", plan: REDUCED_PLAN });
    startReducedDownloadMock.mockRejectedValue(new Error("join-failed"));

    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    fireEvent.click(screen.getByLabelText("common.download"));

    await waitFor(() => expect(toastMock.error).toHaveBeenCalled());
    expect(reducedDownloadMessageMock).toHaveBeenCalled();
    expect(toastMock.error).toHaveBeenCalledWith("downloads.reduced.errorJoin");
  });

  it("keeps the generic mapping for an ordinary download", async () => {
    chooseDownloadQualityMock.mockResolvedValue({ quality: "original" });
    const { startDownload } = await import("../lib/downloads");
    vi.mocked(startDownload).mockRejectedValue(new Error("boom"));

    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    fireEvent.click(screen.getByLabelText("common.download"));

    await waitFor(() => expect(toastMock.error).toHaveBeenCalledWith("media.detail.downloadFailed"));
    expect(reducedDownloadMessageMock).not.toHaveBeenCalled();
  });
});
