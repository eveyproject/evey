import { describe, it, expect, vi, beforeEach } from "vitest";
// The filter header renders once the grid has painted; here the paint is immediate.
vi.mock("../../../src/hooks/useAfterFirstPaint", () => ({ useAfterFirstPaint: (key: string | undefined) => key !== undefined }));
// One object, like the real hook's useCallback handlers.
const { PREFETCH } = vi.hoisted(() => ({ PREFETCH: { prefetchOnFocus: () => {}, cancelPrefetch: () => {} } }));
vi.mock("../../../src/hooks/useMediaPrefetch", () => ({ useMediaPrefetch: () => PREFETCH }));
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";

// Genre filter parity with mobile (a param on the list request) and shuffle
// parity with web: this screen has no local-only facet (the genre filter is
// server-side), so a shuffle always draws from the WHOLE library via a
// random page offset (pageSize 1) rather than the loaded slice, with the
// genre filter, if any, baked into the probe request.

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  useWindowDimensions: () => ({ width: 1200, height: 675, scale: 1.6, fontScale: 1 }),
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  TextInput: ({ onChangeText, value, accessibilityLabel, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      value,
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops a boolean value passed through an unrecognized DOM prop, so
  // spreading it through as-is would make it unobservable in jsdom.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent, ListHeaderComponent }: any) => {
    if (!data || data.length === 0) {
      return React.createElement(React.Fragment, null, ListHeaderComponent ?? null, ListEmptyComponent ?? null);
    }
    return React.createElement(
      "div",
      null,
      ListHeaderComponent ?? null,
      data.map((item: any, index: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, index) : index },
          renderItem({ item, index }),
        ),
      ),
    );
  },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {}, absoluteFill: {} },
}));

// --- expo-image mock ---------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- @expo/vector-icons mock ---------------------------------------------------
// See player/[id].test.tsx: the real build can't be resolved by vitest's SSR
// module resolution, unrelated to app logic.
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- expo-router mock ---------------------------------------------------------
const { pushMock, LIBRARY_ID } = vi.hoisted(() => ({ pushMock: vi.fn(), LIBRARY_ID: "lib-1" }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ libraryId: LIBRARY_ID }),
  useRouter: () => ({ push: pushMock }),
  // The title bar's right side is rendered: it carries the selection summary.
  Stack: { Screen: ({ options }: any) => (options?.headerRight ? options.headerRight() : null) },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      surface: "#111",
      surfaceRaised: "#222",
      border: "#333",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// Counts the posters drawn: one per card render.
const { posterRenders, AUTH_HEADERS } = vi.hoisted(() => ({ posterRenders: { count: 0 }, AUTH_HEADERS: { Authorization: "Bearer TOKEN" } }));
vi.mock("../../../src/components/PosterImage", () => ({
  PosterImage: () => {
    posterRenders.count += 1;
    return null;
  },
}));

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv.test" }));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  // One object, like the real hook (memoised on the token): a fresh one per
  // render would defeat the grid cards' memo here and nowhere else.
  useAuthHeaders: () => AUTH_HEADERS,
}));

// --- @evey/store mock -----------------------------------------------------------------
const { toastErrorMock, toastInfoMock } = vi.hoisted(() => ({
  toastErrorMock: vi.fn(),
  toastInfoMock: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: toastErrorMock, info: toastInfoMock },
}));

// --- @evey/api mock --------------------------------------------------------------------
const { listMediaItemsMock, listGenresMock, rawListMediaItemsMock, searchMediaMock, watchHistoryMock, getLibraryMock } = vi.hoisted(() => ({
  listMediaItemsMock: vi.fn(),
  listGenresMock: vi.fn(),
  rawListMediaItemsMock: vi.fn(),
  searchMediaMock: vi.fn(),
  watchHistoryMock: vi.fn(),
  getLibraryMock: vi.fn(),
}));
// importOriginal, not a hand-written object: the pure helpers must be the REAL
// ones, so the screen cannot pass against a stub that disagrees with shipped
// behaviour. Only the hooks are faked.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetLibrary: (...args: unknown[]) => getLibraryMock(...args),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
  useLibraryServiceListGenres: (...args: unknown[]) => listGenresMock(...args),
  useLibraryServiceSearchMedia: (...args: unknown[]) => searchMediaMock(...args),
  usePlaybackServiceGetWatchHistory: (...args: unknown[]) => watchHistoryMock(...args),
  libraryServiceListMediaItems: (...args: unknown[]) => rawListMediaItemsMock(...args),
}));

// -------------------------------------------------------------------------------------
import LibraryDetailScreen from "./[libraryId]";

const ITEMS = [
  { id: "mi-1", title: "Alpha", genres: ["Action"], type: "MEDIA_ITEM_TYPE_MOVIE" },
  { id: "mi-2", title: "Beta", genres: ["Drama"], type: "MEDIA_ITEM_TYPE_MOVIE" },
];

beforeEach(() => {
  vi.clearAllMocks();
  getLibraryMock.mockReturnValue({ data: { id: LIBRARY_ID, name: "My Library", type: "LIBRARY_TYPE_MOVIES" } });
  listMediaItemsMock.mockImplementation(() => ({
    // Shaped like the real server response: totalPages is ALWAYS 0 (never
    // populated server-side) and the end of the list is an empty nextCursor.
    data: { items: ITEMS, pagination: { totalItems: ITEMS.length, totalPages: 0, nextCursor: "" } },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }));
  listGenresMock.mockReturnValue({ data: { genres: ["Action", "Drama"] } });
  searchMediaMock.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() });
  watchHistoryMock.mockReturnValue({ data: { items: [] } });
});

describe("TV LibraryDetailScreen error state", () => {
  it("gives the remote a focusable retry action instead of leaving it with nowhere to go", () => {
    const refetchMock = vi.fn();
    listMediaItemsMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      isFetching: false,
      refetch: refetchMock,
    });
    render(<LibraryDetailScreen />);

    const retryBtn = screen.getByText("common.retry").closest("button");
    expect(retryBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");

    fireEvent.click(retryBtn!);
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("TV LibraryDetailScreen genre filter", () => {
  it("sends the selected genre as a param on the media list request", () => {
    render(<LibraryDetailScreen />);

    fireEvent.click(screen.getByLabelText("Action"));

    expect(listMediaItemsMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ libraryId: LIBRARY_ID, genre: "Action" }),
      expect.anything(),
    );
  });

  it("omits the genre param again once 'All' is re-selected", () => {
    render(<LibraryDetailScreen />);

    fireEvent.click(screen.getByLabelText("Action"));
    fireEvent.click(screen.getByLabelText("media.search.allGenres"));

    const lastCallParams = listMediaItemsMock.mock.calls.at(-1)?.[0];
    expect(lastCallParams).not.toHaveProperty("genre");
  });
});

describe("TV LibraryDetailScreen selected filters", () => {
  const mark = (label: string) => within(screen.getByLabelText(label)).queryByTestId("icon-checkmark");

  it("marks the chip in effect with a check, apart from the focus, and moves it with the choice", () => {
    render(<LibraryDetailScreen />);
    expect(mark("media.search.allGenres")).not.toBeNull();
    expect(mark("Action")).toBeNull();
    expect(mark("media.library.titleSort")).not.toBeNull();

    fireEvent.click(screen.getByLabelText("Action"));
    expect(mark("Action")).not.toBeNull();
    expect(mark("media.search.allGenres")).toBeNull();
  });

  it("draws the focused chip in the text colour, so focus and selection never look alike", () => {
    render(<LibraryDetailScreen />);
    const chip = screen.getByLabelText("Drama");
    const label = within(chip).getByText("Drama");
    expect(label.style.color).toBe("#fff");
    fireEvent.focus(chip);
    expect(label.style.color).toBe("#000");
    // Focused and selected: the check mark stays.
    fireEvent.click(chip);
    expect(mark("Drama")).not.toBeNull();
  });

  it("names the genre and the sort in effect in the title bar, where they stay once the filters scroll away", () => {
    render(<LibraryDetailScreen />);
    expect(screen.getByText("media.library.titleSort \u2191")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Drama"));
    fireEvent.click(screen.getByLabelText("common.sortAscending"));
    expect(screen.getByText("Drama \u00b7 media.library.titleSort \u2193")).toBeTruthy();
  });
});

describe("TV LibraryDetailScreen grid cards", () => {
  it("leaves the cards alone when the screen re-renders for something they do not show", () => {
    posterRenders.count = 0;
    render(<LibraryDetailScreen />);
    const before = posterRenders.count;
    expect(before).toBeGreaterThan(0);
    // The search field taking the focus re-renders the screen, and the list's
    // renderItem with it: the cards' own props have not changed.
    fireEvent.focus(screen.getByLabelText("media.library.filterByName"));
    expect(posterRenders.count).toBe(before);
  });
});

describe("TV LibraryDetailScreen shuffle", () => {
  it("shuffles across the whole library via a random page offset and navigates to the player", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0); // page = floor(0 * total) + 1 = 1
    rawListMediaItemsMock.mockResolvedValue({ items: [{ id: "mi-9" }], pagination: { totalItems: 7 } });

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("common.shufflePlay"));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/player/mi-9"));
    expect(rawListMediaItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ libraryId: LIBRARY_ID, "pagination.page": 1, "pagination.pageSize": 1 }),
    );
  });

  it("bakes the active genre filter into the shuffle probe", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    rawListMediaItemsMock.mockResolvedValue({ items: [{ id: "mi-9" }], pagination: { totalItems: 1 } });

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("Action"));
    fireEvent.click(screen.getByLabelText("common.shufflePlay"));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/player/mi-9"));
    expect(rawListMediaItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ genre: "Action" }),
    );
  });

  it("toasts a failure when the probe rejects", async () => {
    rawListMediaItemsMock.mockRejectedValue(new Error("network down"));

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("common.shufflePlay"));

    await waitFor(() => expect(toastErrorMock).toHaveBeenCalledWith("common.shuffleFailed"));
    expect(pushMock).not.toHaveBeenCalled();
  });
});

describe("TV LibraryDetailScreen play all", () => {
  // The only selection that genuinely chains: the player's sole auto-advance
  // is useNextEpisode, which carries on through the season.
  it("jumps to the first episode of the current selection, which then chains", () => {
    const episodes = [
      { id: "ep-1", title: "S01E01", type: "MEDIA_ITEM_TYPE_EPISODE" },
      { id: "ep-2", title: "S01E02", type: "MEDIA_ITEM_TYPE_EPISODE" },
    ];
    getLibraryMock.mockReturnValue({ data: { id: LIBRARY_ID, name: "Shows", type: "LIBRARY_TYPE_SHOWS" } });
    listMediaItemsMock.mockImplementation(() => ({
      data: { items: episodes, pagination: { totalItems: 2, totalPages: 0, nextCursor: "" } },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    }));
    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("media.library.episodes"));

    fireEvent.click(screen.getByLabelText("common.playAll"));

    expect(pushMock).toHaveBeenCalledWith("/player/ep-1");
  });

  // A movie selection never chains - the button would play one film and stop,
  // so it must not be offered at all rather than lie about what it does.
  it("does not render on a movies library, where nothing would chain", () => {
    render(<LibraryDetailScreen />);
    expect(screen.queryByLabelText("common.playAll")).toBeNull();
  });

  // A series/season row has no video stream of its own - showing the control
  // over a default shows-library view would always be a dead click, so it is
  // not rendered at all until the type filter narrows to episodes.
  it("does not render on a shows library until filtered down to episodes", () => {
    getLibraryMock.mockReturnValue({ data: { id: LIBRARY_ID, name: "Shows", type: "LIBRARY_TYPE_SHOWS" } });
    render(<LibraryDetailScreen />);

    expect(screen.queryByLabelText("common.playAll")).toBeNull();

    fireEvent.click(screen.getByLabelText("media.library.episodes"));
    expect(screen.getByLabelText("common.playAll")).toBeTruthy();
  });

  it("toasts instead of navigating when nothing in the current selection is playable", () => {
    // A stable outer array, not a fresh literal per call: the accumulate
    // effect keys off `data` identity, and the screen's own state setter
    // (setAllItems) already bails out when the incoming items array is
    // reference-equal to what's there - recreating it on every mock
    // invocation defeats that and spins the component into an infinite
    // render loop.
    const unplayableItems = [{ id: "s-1", title: "A Show", type: "MEDIA_ITEM_TYPE_SERIES" }];
    getLibraryMock.mockReturnValue({ data: { id: LIBRARY_ID, name: "Shows", type: "LIBRARY_TYPE_SHOWS" } });
    listMediaItemsMock.mockImplementation(() => ({
      data: {
        items: unplayableItems,
        pagination: { totalItems: 1, totalPages: 0, nextCursor: "" },
      },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    }));
    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("media.library.episodes"));

    fireEvent.click(screen.getByLabelText("common.playAll"));

    expect(toastInfoMock).toHaveBeenCalledWith("common.playAllEmpty");
    expect(pushMock).not.toHaveBeenCalled();
  });
});
