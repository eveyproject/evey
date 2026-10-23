import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, disabled, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: disabled ? undefined : onPress,
        disabled,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor }: any) => {
    if (!data || data.length === 0) return null;
    return React.createElement(
      "div",
      null,
      data.map((item: any, index: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, index) : index },
          renderItem({ item, index }),
        ),
      ),
    );
  },
  Linking: { openURL: vi.fn(() => Promise.resolve()) },
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {}, hairlineWidth: 1 },
  AppState: { addEventListener: () => ({ remove: () => {} }) },
}));

// --- expo-image / @expo/vector-icons mocks ----------------------------------
vi.mock("expo-image", () => ({
  Image: (p: any) => React.createElement("img", p),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: () => null,
}));

// --- react-i18next mock ------------------------------------------------------
// Mirrors this suite's convention (see player/[id].test.tsx): a string second
// arg is the English fallback and is returned as-is; anything else (an
// interpolation object, or nothing) falls back to the raw key so a test can
// assert on it directly without a real catalogue.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: unknown) => (typeof opts === "string" ? opts : key),
  }),
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
const replaceMock = vi.fn();
const backMock = vi.fn();
// The latest focus effect the screen registered: calling it plays "the screen
// is shown", its return value "the screen is covered".
let focusEffect: (() => void | (() => void)) | null = null;
vi.mock("expo-router", () => ({
  router: {
    push: (...args: unknown[]) => pushMock(...args),
    replace: (...args: unknown[]) => replaceMock(...args),
    back: (...args: unknown[]) => backMock(...args),
  },
  useLocalSearchParams: () => ({ id: "media-1" }),
  useFocusEffect: (cb: () => void | (() => void)) => {
    focusEffect = cb;
  },
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      surface: "#111",
      border: "#333",
    },
    radii: { sm: 4, md: 8 },
  }),
}));

// --- @evey/store mock -------------------------------------------------------------
const demoState = { demoMode: false };
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: any) => selector({ profileId: "prof-1" }),
  useDemoStore: (selector: any) => selector(demoState),
  toast: { error: vi.fn(), success: vi.fn() },
}));

// --- @evey/api mock ----------------------------------------------------------
// formatDurationMs/formatTimeMs/itemLabel/seasonLabel and the rating query-key
// builder are kept real (pure helpers); every network-backed hook is stubbed.
const getMediaItemMock = vi.fn();
const listMediaItemsMock = vi.fn();
const getWatchProgressMock = vi.fn();
const markWatchedMock = vi.fn();
const markUnwatchedMock = vi.fn();
const similarItemsMock = vi.fn();
const myThumbRatingMock = vi.fn();
const putThumbRatingMock = vi.fn();
const seriesTargetMock = vi.fn();
let watchHistory: { data: { items: { mediaItemId: string; played: boolean }[] } } = { data: { items: [] } };
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: (...args: unknown[]) => getMediaItemMock(...args),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
  usePlaybackServiceGetWatchProgress: (...args: unknown[]) => getWatchProgressMock(...args),
  usePlaybackServiceMarkAsWatched: (...args: unknown[]) => markWatchedMock(...args),
  usePlaybackServiceMarkAsUnwatched: (...args: unknown[]) => markUnwatchedMock(...args),
  useFeedServiceSimilarItems: (...args: unknown[]) => similarItemsMock(...args),
  useRatingServiceGetMyThumbRating: (...args: unknown[]) => myThumbRatingMock(...args),
  ratingServicePutThumbRating: (...args: unknown[]) => putThumbRatingMock(...args),
  usePlaybackServiceGetSeriesPlayTarget: (...args: unknown[]) => seriesTargetMock(...args),
  usePlaybackServiceGetWatchHistory: () => watchHistory,
}));

// --- local hook mocks ---------------------------------------------------------
// Each of these hits eveyFetch or a query hook that would otherwise need a
// live QueryClient/network - stubbed flat like the @evey/api hooks above.
vi.mock("../../../src/hooks/useFavorite", () => ({
  useIsFavorite: () => ({ data: false }),
  useToggleFavorite: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => undefined,
}));
vi.mock("../../../src/hooks/useParentalGate", () => ({
  useParentalGate: () => ({ needsPin: () => false, profileId: undefined }),
}));

// --- component stubs -----------------------------------------------------------
// None of these are under test here; each pulls in its own network/provider
// requirements the screen's own tests don't need.
vi.mock("../../../src/components/AddToPlaylistSheet", () => ({
  AddToPlaylistSheet: () => null,
}));
vi.mock("../../../src/components/ParentalPINModal", () => ({
  ParentalPINModal: () => null,
}));
vi.mock("../../../src/features/plugins/PluginSlot", () => ({
  PluginSlot: () => null,
}));
vi.mock("../../../src/components/SagaSection", () => ({
  SagaSection: () => null,
}));

import MediaDetailScreen from "./[id]";

const MOVIE = {
  id: "media-1",
  type: "MEDIA_ITEM_TYPE_MOVIE",
  title: "Feature Title",
  year: 2020,
  durationMs: 7_200_000,
  officialRating: "PG-13",
  genres: ["Drama"],
  people: [],
} as any;

const SERIES = {
  id: "media-1",
  type: "MEDIA_ITEM_TYPE_SERIES",
  title: "A Series",
  people: [],
} as any;

const SEASON = {
  id: "media-1",
  type: "MEDIA_ITEM_TYPE_SEASON",
  title: "Season 1",
  people: [],
} as any;

function renderScreen() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    React.createElement(QueryClientProvider, { client: queryClient }, React.createElement(MediaDetailScreen)),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  getMediaItemMock.mockReturnValue({ data: MOVIE, isLoading: false, error: null, refetch: vi.fn() });
  listMediaItemsMock.mockReturnValue({ data: { items: [] } });
  getWatchProgressMock.mockReturnValue({ data: undefined });
  markWatchedMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
  markUnwatchedMock.mockReturnValue({ mutate: vi.fn(), isPending: false });
  similarItemsMock.mockReturnValue({ data: { items: [] } });
  myThumbRatingMock.mockReturnValue({ data: 0 });
  putThumbRatingMock.mockResolvedValue({});
  seriesTargetMock.mockReturnValue({ data: undefined, isLoading: false });
});

// #1 - watched/thumb-rating actions used to be hidden on a series/season with
// a `!isSeries && !isSeason` guard the web equivalent does not have. Marking a
// whole season watched from the couch is a normal TV use.
describe("MediaDetailScreen (TV) watch/rating actions on series and season", () => {
  it("shows the watched and thumb-rating actions on a series", () => {
    getMediaItemMock.mockReturnValue({ data: SERIES, isLoading: false, error: null, refetch: vi.fn() });
    renderScreen();

    expect(screen.getByText("media.detail.markWatched")).toBeTruthy();
    expect(screen.getByText("Like")).toBeTruthy();
    expect(screen.getByText("Dislike")).toBeTruthy();
  });

  it("shows the watched and thumb-rating actions on a season", () => {
    getMediaItemMock.mockReturnValue({ data: SEASON, isLoading: false, error: null, refetch: vi.fn() });
    renderScreen();

    expect(screen.getByText("media.detail.markWatched")).toBeTruthy();
    expect(screen.getByText("Like")).toBeTruthy();
    expect(screen.getByText("Dislike")).toBeTruthy();
  });

  // The demo refuses favourites, watched marks, playlists and ratings on the
  // shared account: the page does not offer them there.
  it("offers none of them on the demo", () => {
    getMediaItemMock.mockReturnValue({ data: SERIES, isLoading: false, error: null, refetch: vi.fn() });
    demoState.demoMode = true;
    try {
      renderScreen();
      for (const label of ["media.detail.favorite", "media.detail.markWatched", "media.detail.addToPlaylist", "Like", "Dislike"]) {
        expect(screen.queryByText(label)).toBeNull();
      }
    } finally {
      demoState.demoMode = false;
    }
  });
});

// 27-09 (the owner): back from a player that had autoplayed two episodes, then
// a night in standby, the page still offered "Resume 7:06" on an episode
// finished since - and the press started it over at 0. The progress is now
// never served from cache and re-read each time the page is shown again.
describe("MediaDetailScreen (TV) resume progress freshness", () => {
  it("reads the progress with no stale window", () => {
    renderScreen();
    const opts = getWatchProgressMock.mock.calls[0]?.[2] as { query?: { staleTime?: number } };
    expect(opts?.query?.staleTime).toBe(0);
  });

  it("re-reads the progress when the page is shown again after the player", () => {
    const refetch = vi.fn();
    getWatchProgressMock.mockReturnValue({ data: { positionMs: "426000" }, refetch });
    renderScreen();

    let cover: (() => void) | void = undefined;
    act(() => { cover = focusEffect?.(); });
    expect(refetch).not.toHaveBeenCalled();
    act(() => { cover?.(); });
    act(() => { focusEffect?.(); });
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

// #9 - a 404 is a dead end no retry can fix; any other failure (offline,
// server error) keeps the retryable ScreenState. The two used to share the
// exact same branch (`error={!isLoading}`).
describe("MediaDetailScreen (TV) load failure", () => {
  it("shows a not-found message with a focusable go-home button on a 404, and no retry", () => {
    getMediaItemMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: { httpStatus: 404 },
      refetch: vi.fn(),
    });
    renderScreen();

    expect(screen.getByText("media.detail.notFound")).toBeTruthy();
    expect(screen.queryByText("Retry")).toBeNull();
    const goHomeBtn = screen.getByText("Go Home").closest("button");
    expect(goHomeBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");

    fireEvent.click(goHomeBtn!);
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/(tabs)");
  });

  it("keeps the generic retryable error for a non-404 failure", () => {
    const refetchMock = vi.fn();
    getMediaItemMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: { httpStatus: 500 },
      refetch: refetchMock,
    });
    renderScreen();

    expect(screen.getByText("Something went wrong")).toBeTruthy();
    expect(screen.queryByText("media.detail.notFound")).toBeNull();
    const retryBtn = screen.getByText("Retry").closest("button");
    fireEvent.click(retryBtn!);
    expect(refetchMock).toHaveBeenCalled();
  });

  it("keeps the generic retryable error when the failure carries no httpStatus (offline)", () => {
    getMediaItemMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("network error"),
      refetch: vi.fn(),
    });
    renderScreen();

    expect(screen.getByText("Something went wrong")).toBeTruthy();
    expect(screen.queryByText("media.detail.notFound")).toBeNull();
  });
});

// Play on a series starts where the profile is in it; the first episode of
// the first season is only the fallback before the server answers.
describe("MediaDetailScreen (TV) series Play", () => {
  beforeEach(() => {
    getMediaItemMock.mockReturnValue({ data: SERIES, isLoading: false, error: null, refetch: vi.fn() });
    listMediaItemsMock.mockImplementation((params: { parentId?: string }) =>
      params.parentId === "media-1"
        ? { data: { items: [{ id: "season-1", indexNumber: 1, type: "MEDIA_ITEM_TYPE_SEASON" }] } }
        : { data: { items: [{ id: "ep-1", indexNumber: 1, type: "MEDIA_ITEM_TYPE_EPISODE" }] } });
  });

  it("plays the server's episode, labelled Resume", () => {
    seriesTargetMock.mockReturnValue({ data: { item: { id: "ep-42" }, started: true }, isLoading: false });
    renderScreen();
    fireEvent.click(screen.getByLabelText("Resume"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/player/ep-42");
  });

  it("falls back to the first episode without an answer", () => {
    renderScreen();
    fireEvent.click(screen.getByLabelText("player.controls.play"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/player/ep-1");
  });
});

// The season list marks each episode both ways from its own stop on the
// remote, for the active profile, and the row's check follows at once.
describe("MediaDetailScreen (TV) episode watched toggle", () => {
  const markCalls: { watched: unknown[]; unwatched: unknown[] } = { watched: [], unwatched: [] };
  const hook = (calls: unknown[]) => (opts?: { mutation?: { onSuccess?: (d: unknown, v: unknown) => void } }) => ({
    isPending: false,
    mutate: (vars: unknown) => { calls.push(vars); opts?.mutation?.onSuccess?.({}, vars); },
  });

  beforeEach(() => {
    markCalls.watched = [];
    markCalls.unwatched = [];
    markWatchedMock.mockImplementation(hook(markCalls.watched));
    markUnwatchedMock.mockImplementation(hook(markCalls.unwatched));
    getMediaItemMock.mockReturnValue({ data: SEASON, isLoading: false, error: null, refetch: vi.fn() });
    listMediaItemsMock.mockReturnValue({
      data: { items: [{ id: "ep-1", indexNumber: 1, title: "Pilot" }, { id: "ep-2", indexNumber: 2, title: "Second" }] },
    });
    watchHistory = { data: { items: [{ mediaItemId: "ep-1", played: true }] } };
  });

  it("marks a watched episode unwatched and an unwatched one watched", () => {
    renderScreen();
    fireEvent.click(screen.getByLabelText("media.detail.markUnwatched: 1. Pilot"));
    expect(markCalls.unwatched).toEqual([{ mediaItemId: "ep-1", params: { profileId: "prof-1" } }]);
    expect(screen.getByLabelText("media.detail.markWatched: 1. Pilot")).toBeTruthy();

    fireEvent.click(screen.getByLabelText("media.detail.markWatched: 2. Second"));
    expect(markCalls.watched).toEqual([{ mediaItemId: "ep-2", params: { profileId: "prof-1" } }]);
    expect(screen.getByLabelText("media.detail.markUnwatched: 2. Second")).toBeTruthy();
  });

  it("offers no toggle on the demo", () => {
    demoState.demoMode = true;
    try {
      renderScreen();
      expect(screen.queryByLabelText("media.detail.markUnwatched: 1. Pilot")).toBeNull();
    } finally {
      demoState.demoMode = false;
    }
  });
});
