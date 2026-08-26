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
    t: (key: string, fallback?: unknown) => {
      // Interpolated so the season breadcrumb proves it went through i18n.
      if (key === "media.season") return `Saison ${(fallback as { number?: number } | undefined)?.number}`;
      return typeof fallback === "string" ? fallback : key;
    },
    i18n: { language: "en" },
  }),
}));

// --- expo-router ---------------------------------------------------------------------
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: currentId }),
  router: { push: vi.fn(), back: vi.fn(), replace: vi.fn() },
  // No real navigator in this test - just run the focus callback on mount,
  // matching downloads-screen.test.tsx's mock of the same hook.
  useFocusEffect: (cb: () => void | (() => void)) => {
    React.useEffect(() => cb(), []);
  },
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
// Mutable so the parental-gate tests below can flip the gate on; everything
// else runs with the gate open, as before.
let pinRequired = false;
vi.mock("../hooks/useParentalGate", () => ({
  useParentalGate: () => ({
    needsPin: () => pinRequired,
    profileId: pinRequired ? "profile-1" : null,
  }),
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
vi.mock("../components/ParentalPINModal", () => ({
  ParentalPINModal: ({ visible }: { visible: boolean }) =>
    visible ? React.createElement("div", { "data-testid": "pin-modal" }) : null,
}));

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

// Mutable so the episode-navigation and watched-badge tests below can drive
// which media the screen renders and per-episode watch progress. Defaults keep
// every existing test rendering the 100-episode season with no progress.
let currentId = SEASON_ID;
const extraItems: Record<string, Record<string, unknown>> = {};
let progressById: Record<string, { played?: boolean; positionMs?: string; durationMs?: string }> = {};
// Batch source for the episode badges/bars (audit 25-08 H3): one history
// fetch replaces one GetWatchProgress per row. Counting mocks let the tests
// pin the request pattern, not just the pixels.
let historyItems: { mediaItemId: string; played?: boolean; percentComplete?: number }[] = [];
const watchProgressCalls: string[] = [];
const historyCalls = { n: 0 };

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: (itemId: string) => {
    if (itemId === SEASON_ID) return { data: seasonItem, isLoading: false };
    if (extraItems[itemId]) return { data: extraItems[itemId], isLoading: false };
    return { data: undefined, isLoading: false };
  },
  useLibraryServiceListMediaItems: (params: { parentId?: string }) => {
    if (params.parentId === SEASON_ID) return { data: { items: allEpisodes } };
    return { data: { items: [] } };
  },
  usePlaybackServiceGetWatchProgress: (itemId: string) => {
    watchProgressCalls.push(itemId);
    return { data: progressById[itemId] };
  },
  usePlaybackServiceGetWatchHistory: () => {
    historyCalls.n++;
    return { data: { items: historyItems } };
  },
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
import { router } from "expo-router";
import MediaDetailScreen from "../../app/(auth)/media/[id]";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  pinRequired = false;
  currentId = SEASON_ID;
  progressById = {};
  historyItems = [];
  watchProgressCalls.length = 0;
  historyCalls.n = 0;
  for (const k of Object.keys(extraItems)) delete extraItems[k];
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

describe("MediaDetailScreen episode parental gate", () => {
  // An episode tap is a PLAY action: it must pass the same PIN gate as the
  // Play button. Before the fix the row navigated straight to the player -
  // which itself has zero PIN check - so the gate only protected the one
  // button that happened to call handlePlay.
  it("routes an episode tap through the PIN prompt instead of straight to the player", () => {
    pinRequired = true;
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    fireEvent.click(episodeButton("Episode 1")!);

    expect(router.push).not.toHaveBeenCalled();
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
  });

  it("opens the player directly when no PIN is required", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    fireEvent.click(episodeButton("Episode 1")!);

    expect(router.push).toHaveBeenCalledWith("/(auth)/player/ep-1");
  });

  // A fast double-tap fires the row's onPress twice before the first push
  // unmounts the screen; without a latch that mounts two player screens.
  it("navigates only once on a double-tap of the same episode row", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    const row = episodeButton("Episode 1")!;
    fireEvent.click(row);
    fireEvent.click(row);

    expect(router.push).toHaveBeenCalledTimes(1);
  });
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

describe("MediaDetailScreen episode → season navigation", () => {
  // An episode reached from the Resume row had no way back up to its season.
  // The SxEy · season line is now a tap-back to the season page.
  const EP_ID = "ep-solo";
  const SEASON_PARENT = "season-9";
  function mountEpisode(parentId: string | undefined) {
    currentId = EP_ID;
    extraItems[EP_ID] = {
      id: EP_ID,
      type: "MEDIA_ITEM_TYPE_EPISODE",
      parentId,
      indexNumber: 4,
      title: "Some Episode",
      people: [],
    };
    if (parentId) {
      extraItems[parentId] = { id: parentId, type: "MEDIA_ITEM_TYPE_SEASON", indexNumber: 3, title: "Some Season", people: [] };
    }
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });
  }

  // The season on that line is labelled from its number, never from the
  // stored title: "Some Season" is a provider name the enrichment used to
  // write, and a season has no name.
  it("navigates to the season when the SxEy · season line is tapped", () => {
    mountEpisode(SEASON_PARENT);
    expect(document.querySelector('button[accessibilitylabel="Some Season"]')).toBeNull();
    const back = document.querySelector('button[accessibilitylabel="Saison 3"]');
    expect(back).toBeTruthy();
    fireEvent.click(back!);
    expect(router.push).toHaveBeenCalledWith(`/(auth)/media/${SEASON_PARENT}`);
  });

  it("renders the label as plain text (no navigation) when the episode has no parent", () => {
    mountEpisode(undefined);
    expect(document.querySelector('button[accessibilitylabel="Saison 3"]')).toBeNull();
    expect(router.push).not.toHaveBeenCalled();
  });
});

describe("MediaDetailScreen episode watched indicator", () => {
  // A fully-watched episode resets its resume position, so a percent-only bar
  // would make it indistinguishable from a never-watched one. `played` drives
  // a check badge; a mid-way episode keeps the partial bar (no badge).
  function badgeInRow(title: string): Element | null {
    const row = document.querySelector(`button[accessibilitylabel="${title}"]`);
    return row?.querySelector('[data-testid="icon-checkmark"]') ?? null;
  }

  it("shows a check badge on a fully-watched episode and none on a partial one", () => {
    historyItems = [
      { mediaItemId: "ep-1", played: true, percentComplete: 100 },
      { mediaItemId: "ep-2", played: false, percentComplete: 30 },
    ];
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    expect(badgeInRow("Episode 1")).toBeTruthy();
    expect(badgeInRow("Episode 2")).toBeNull();
    // ...and an untouched episode has neither badge nor progress.
    expect(badgeInRow("Episode 3")).toBeNull();
  });

  // Audit 25-08 H3: opening a season fired one GetWatchProgress per visible
  // row - a 20-request burst against a weak NAS for data GetWatchHistory
  // serves in ONE call (the exact pattern library/[libraryId].tsx already
  // uses). The rows must read the shared map, never fetch for themselves.
  it("feeds every row from ONE watch-history call, zero per-episode requests", () => {
    render(<MediaDetailScreen />, { wrapper: makeWrapper() });

    expect(historyCalls.n).toBeGreaterThanOrEqual(1);
    const perEpisode = watchProgressCalls.filter((id) => id.startsWith("ep-"));
    expect(perEpisode).toEqual([]);
  });
});
