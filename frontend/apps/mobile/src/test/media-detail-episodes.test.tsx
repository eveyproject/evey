import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// A 100-episode season is the scenario the audit flagged: the screen's episode
// list used to .map() every item straight into the outer ScrollView. This test
// guards the fix (incremental "Show more" rendering) rather than a nested
// FlatList, since the episode block lives inside one big ScrollView (see the
// screen's own comment) and a nested VirtualizedList there would warn and
// still not get its own scroll container.

// --- React Native mock ----------------------------------------------------------------
const alertMock = vi.fn();
vi.mock("react-native", () => ({
  Alert: { alert: (...a: unknown[]) => alertMock(...a) },
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      null,
      (data ?? []).map((item: any, i: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, i) : i },
          renderItem({ item, index: i }),
        ),
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
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
    i18n: { language: "en" },
  }),
}));

// --- expo-router ---------------------------------------------------------------------
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: SEASON_ID }),
  router: { push: vi.fn(), back: vi.fn(), replace: vi.fn() },
}));

// --- react-native-safe-area-context ---------------------------------------------------
vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));

// --- ThemeProvider -------------------------------------------------------------------
// ONE frozen object, returned by identity. The real provider memoises its
// tokens, so `theme` is referentially stable in production; a mock that built
// a fresh object per call would hand every memo()'d child a changed prop and
// silently re-render the whole episode list on each "Show more" - making the
// test both far slower than the app AND blind to memoisation regressions.
const MOCK_THEME = {
  colors: {
    bg: "#000",
    surface: "#111",
    surfaceRaised: "#222",
    text: "#fff",
    textMuted: "#888",
    accent: "#f00",
    onAccent: "#fff",
    border: "#333",
    error: "#e00",
    warning: "#fa0",
    success: "#0f0",
  },
  radii: { lg: 12, md: 8, sm: 4 },
  spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
};
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => MOCK_THEME }));
vi.mock("../components/media/PlayOnSheet", () => ({ PlayOnSheet: () => null }));

// --- misc hooks / libs the screen depends on, stubbed to keep this a focused
// --- render test of the episode list rather than an integration test -------------------
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
const startDownloadMock = vi.fn();
const getDownloadPathMock = vi.fn().mockResolvedValue(null);
const deleteDownloadMock = vi.fn().mockResolvedValue(undefined);
vi.mock("../lib/downloads", () => ({
  startDownload: (...a: unknown[]) => startDownloadMock(...a),
  getDownloadPath: (...a: unknown[]) => getDownloadPathMock(...a),
  deleteDownload: (...a: unknown[]) => deleteDownloadMock(...a),
  downloadErrorMessageKey: () => "downloads.error",
  subscribeToProgress: () => () => undefined,
  isPreparingCompat: () => false,
  getCompatPreparePermille: () => 0,
}));
// The sliced 720p flow pulls in expo-file-system and the native background
// downloader at import time; the screen only reaches it when the user picks
// the lighter quality, which these tests never do.
vi.mock("../lib/reducedDownload", () => ({ startReducedDownload: vi.fn() }));
// No server offers a reduced copy here, so the quality dialog never opens and
// every download goes straight to the original - today's behavior.
vi.mock("../lib/downloadQualityGate", () => ({
  chooseDownloadQuality: vi.fn(async () => ({ quality: "original" })),
}));
vi.mock("../components/AddToPlaylistSheet", () => ({ AddToPlaylistSheet: () => null }));
vi.mock("../components/ParentalPINModal", () => ({ ParentalPINModal: () => null }));

// --- @evey/store -----------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) =>
    selector({ profileId: "profile-1", isAdmin: false, accessToken: "tok" }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

// --- @evey/api -------------------------------------------------------------------------
const SEASON_ID = "season-1";
const EPISODE_COUNT = 100;

const seasonItem = {
  id: SEASON_ID,
  type: "MEDIA_ITEM_TYPE_SEASON",
  title: "Season 1",
  people: [],
};

const allEpisodes = Array.from({ length: EPISODE_COUNT }, (_, i) => ({
  id: `ep-${i + 1}`,
  type: "MEDIA_ITEM_TYPE_EPISODE",
  parentId: SEASON_ID,
  indexNumber: i + 1,
  title: `Episode ${i + 1}`,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: (itemId: string) => {
    if (itemId === SEASON_ID) return { data: seasonItem, isLoading: false };
    return { data: undefined, isLoading: false };
  },
  useLibraryServiceListMediaItems: (params: { parentId?: string }) => {
    if (params.parentId === SEASON_ID) return { data: { items: allEpisodes } };
    return { data: { items: [] } };
  },
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
});

// EpisodeRow renders its title as two sibling text expressions ("1. " + title)
// inside the same node, so getByText(exact title) never matches; the row's
// accessibilityLabel carries the bare title instead, which is what we assert on.
function episodeButton(title: string): HTMLElement | null {
  return document.querySelector(`button[accessibilitylabel="${title}"]`);
}

describe("MediaDetailScreen episode list", () => {
  it("renders fewer than 25 episodes initially for a 100-episode season", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    const rendered = allEpisodes.filter((ep) => episodeButton(ep.title));
    expect(rendered.length).toBeLessThan(25);
    expect(rendered.length).toBe(20);
    expect(episodeButton("Episode 21")).toBeNull();
  });

  it("reveals more episodes on 'Show more' tap, up to the full season", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    const showMore = screen.getByRole("button", { name: "Show more" });
    fireEvent.click(showMore);

    expect(episodeButton("Episode 40")).toBeTruthy();
    expect(episodeButton("Episode 41")).toBeNull();
  });

  // Explicit timeout, NOT a global testTimeout bump: this one test walks the
  // whole 100-episode season into the DOM (five render passes, ~2.3s locally),
  // and a CI runner at half that speed overran the 5s default. The cost is the
  // jsdom mount of 100 rich rows - measured, not guessed: making the memo()
  // around EpisodeRow genuinely effective moved it by 4%. Raising the default
  // for every test would have hidden the next slow one instead.
  it("hides the 'Show more' button once every episode is visible", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    const showMore = () => screen.getByRole("button", { name: "Show more" });
    for (let i = 0; i < 4; i++) {
      fireEvent.click(showMore());
    }

    expect(episodeButton(`Episode ${EPISODE_COUNT}`)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  }, 20_000);
});

describe("MediaDetailScreen episode downloads", () => {
  // Idle download buttons render icon-only (no accessible text), so this
  // mirrors episodeButton() above and queries the accessibilityLabel prop
  // directly rather than relying on text content for the accessible name.
  function downloadButtons(): NodeListOf<Element> {
    return document.querySelectorAll('button[accessibilitylabel="common.download"]');
  }

  it("shows no per-episode download button when no episode has a media file", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    expect(downloadButtons().length).toBe(0);
  });

  it("hides the bulk 'Download all' button when no episode has a media file", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
    expect(screen.queryByRole("button", { name: "Download all" })).toBeNull();
  });

  it("shows a per-episode download button and the bulk button once an episode has a media file", () => {
    (allEpisodes[0] as { fileSize?: string }).fileSize = "123456";
    try {
      render(<MediaDetailScreen />, { wrapper: makeWrapper() });
      expect(downloadButtons().length).toBe(1);
      expect(screen.getByRole("button", { name: "Download all" })).toBeTruthy();
    } finally {
      delete (allEpisodes[0] as { fileSize?: string }).fileSize;
    }
  });

  it("offers confirmed deletion instead of a dead-end toast when already downloaded", async () => {
    (allEpisodes[0] as { fileSize?: string }).fileSize = "123456";
    getDownloadPathMock.mockResolvedValueOnce("/files/evey-ep-1.mkv");
    try {
      render(<MediaDetailScreen />, { wrapper: makeWrapper() });
      fireEvent.click(downloadButtons()[0] as HTMLElement);

      await waitFor(() => expect(alertMock).toHaveBeenCalled());
      expect(startDownloadMock).not.toHaveBeenCalled();

      // Confirming the destructive action must actually remove the download.
      const dialogButtons = alertMock.mock.calls[0][2] as { style?: string; onPress?: () => Promise<void> }[];
      const destructive = dialogButtons.find((b) => b.style === "destructive");
      expect(destructive).toBeTruthy();
      await destructive!.onPress?.();
      expect(deleteDownloadMock).toHaveBeenCalledWith("ep-1");
    } finally {
      delete (allEpisodes[0] as { fileSize?: string }).fileSize;
    }
  });
});
