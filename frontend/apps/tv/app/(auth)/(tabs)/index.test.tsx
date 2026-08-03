import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  // The customize panel the home hosts renders inside a Modal.
  Modal: ({ children, visible }: any) => (visible ? children : null),
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops boolean values passed through an unrecognized DOM prop, so passing
  // it through the ...p spread as-is (as other test files do) makes it
  // unobservable in jsdom - a real gap the per-library-row focus tests below
  // need to assert on.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(ListEmptyComponent) : null;
    }
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
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

// --- expo-image mock ---------------------------------------------------------
// The home now renders an icon on the "Customize" row; the real package pulls
// a native icon set that does not resolve under vitest.
vi.mock("@expo/vector-icons", () => ({
  Ionicons: () => null,
}));

vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args) },
}));

// --- react-i18next mock --------------------------------------------------------
// The per-library row title interpolates {{name}}; render it as "<key>:<name>"
// so tests can assert on a distinct string per library without a real i18next
// instance.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: { name?: string }) => (opts?.name ? `${key}:${opts.name}` : key),
  }),
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

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

// --- PluginSlot mock -------------------------------------------------------------
vi.mock("../../../src/features/plugins/PluginSlot", () => ({
  PluginSlot: () => null,
}));

// --- ParentalPINModal mock -------------------------------------------------------
// Rendered as a stand-in "Verify" button so tests can simulate a correct PIN
// entry without exercising the real keypad/eveyFetch flow (covered by its
// own component).
vi.mock("../../../src/components/ParentalPINModal", () => ({
  ParentalPINModal: ({ visible, onVerified, onClose }: any) =>
    visible
      ? React.createElement(
          "div",
          { "data-testid": "pin-modal" },
          React.createElement("button", { onClick: onVerified, "aria-label": "verify-pin" }, "verify"),
          React.createElement("button", { onClick: onClose, "aria-label": "cancel-pin" }, "cancel"),
        )
      : null,
}));

// --- useParentalGate mock ---------------------------------------------------------
const needsPinMock = vi.fn(() => false);
vi.mock("../../../src/hooks/useParentalGate", () => ({
  useParentalGate: () => ({ needsPin: needsPinMock, profileId: "prof-1" }),
}));

// --- @tanstack/react-query mock ---------------------------------------------------
// libraryRowResults maps a libraryId to the query result its row should see;
// tests populate it per-library instead of relying on real react-query
// machinery. getLibraryServiceListMediaItemsQueryOptionsMock tags each query
// with the libraryId it was built for (via queryKey) so the useQueries mock
// can look the right result up.
const libraryRowResults = new Map<string, { data?: unknown; isLoading: boolean }>();
const useQueriesMock = vi.fn((opts: { queries: Array<{ queryKey: readonly unknown[] }> }) =>
  opts.queries.map(
    (q) => libraryRowResults.get(q.queryKey[0] as string) ?? { data: { items: [] }, isLoading: false },
  ),
);
vi.mock("@tanstack/react-query", () => ({
  useQueries: (...args: unknown[]) => useQueriesMock(...(args as [{ queries: Array<{ queryKey: readonly unknown[] }> }])),
  // The home lists installed home.section plugins so the customize panel can
  // name them; these tests are about the shelves, not the plugin catalogue.
  useQuery: () => ({ data: { plugins: [] } }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

// --- @evey/api mock --------------------------------------------------------------
const resumeMock = vi.fn();
const nextUpMock = vi.fn();
const listMediaItemsMock = vi.fn();
const librariesMock = vi.fn();
const feedConfigMock = vi.fn();
const recommendedMock = vi.fn();
const getLibraryServiceListMediaItemsQueryOptionsMock = vi.fn((params: { libraryId: string }) => ({
  queryKey: [params.libraryId],
}));
const getMusicAlbumsQueryOptionsMock = vi.fn((params: { libraryId: string }) => ({
  queryKey: [params.libraryId],
}));
const getPhotoAlbumsQueryOptionsMock = vi.fn((params: { libraryId: string }) => ({
  queryKey: [params.libraryId],
}));
vi.mock("@evey/api", () => ({
  useFeedServiceUpdateProfileFeedConfig: () => ({ mutateAsync: vi.fn() }),
  eveyFetch: vi.fn().mockResolvedValue({ plugins: [] }),
  usePlaybackServiceGetResumeItems: (...args: unknown[]) => resumeMock(...args),
  usePlaybackServiceGetNextUp: (...args: unknown[]) => nextUpMock(...args),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
  useLibraryServiceListLibraries: (...args: unknown[]) => librariesMock(...args),
  useFeedServiceGetFeedConfig: (...args: unknown[]) => feedConfigMock(...args),
  useFeedServiceRecommended: (...args: unknown[]) => recommendedMock(...args),
  getLibraryServiceListMediaItemsQueryOptions: (...args: unknown[]) =>
    getLibraryServiceListMediaItemsQueryOptionsMock(
      ...(args as [{ libraryId: string }]),
    ),
  getMusicServiceGetAlbumsQueryOptions: (...args: unknown[]) =>
    getMusicAlbumsQueryOptionsMock(...(args as [{ libraryId: string }])),
  getPhotoServiceListPhotoAlbumsQueryOptions: (...args: unknown[]) =>
    getPhotoAlbumsQueryOptionsMock(...(args as [{ libraryId: string }])),
}));

// --- store / collections / popular mocks -------------------------------------
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: unknown) => unknown) => sel({ profileId: "prof-1", accessToken: "TOKEN" }),
}));
const collectionsMock = vi.fn();
vi.mock("../../../src/api/collections", () => ({
  useCollections: () => collectionsMock(),
}));
const popularMock = vi.fn();
vi.mock("../../../src/hooks/usePopular", () => ({
  useMostPopular: (...args: unknown[]) => popularMock(...args),
}));

import HomeScreen, { selectHomeFocusTarget } from "./index";

const HERO_ITEM = {
  id: "mi-hero-1",
  title: "Restricted Movie",
  year: 2020,
  genres: ["Drama"],
  officialRating: "R",
};

beforeEach(() => {
  vi.clearAllMocks();
  libraryRowResults.clear();
  needsPinMock.mockReturnValue(false);
  resumeMock.mockReturnValue({ data: { items: [], progress: [] }, isLoading: false });
  nextUpMock.mockReturnValue({ data: { items: [] }, isLoading: false });
  listMediaItemsMock.mockReturnValue({ data: { items: [HERO_ITEM] }, isLoading: false });
  // No libraries by default: these tests exercise the transverse rows only.
  librariesMock.mockReturnValue({ data: { libraries: [] }, isLoading: false });
  // No stored config: the screen falls back to the shared default order, which
  // is what a fresh install shows.
  feedConfigMock.mockReturnValue({ data: { configs: [] } });
  recommendedMock.mockReturnValue({ data: { items: [] }, isLoading: false });
  popularMock.mockReturnValue({ data: [], isLoading: false });
  collectionsMock.mockReturnValue({ data: { collections: [] }, isLoading: false });
});

describe("selectHomeFocusTarget (initial TV remote focus)", () => {
  it("picks the hero Play button whenever the hero is visible, even if rows also have items", () => {
    expect(selectHomeFocusTarget(true, [true, true, true])).toBe("hero");
  });

  it("falls back to the first non-empty row, in on-screen order, when there is no hero", () => {
    expect(selectHomeFocusTarget(false, [false, true, true])).toBe(1);
    expect(selectHomeFocusTarget(false, [true, false, false])).toBe(0);
  });

  it("returns null when nothing is visible so no element claims initial focus", () => {
    expect(selectHomeFocusTarget(false, [false, false, false])).toBeNull();
  });
});

describe("HomeScreen hero play button (TV parental gate)", () => {
  it("plays directly when the gate does not require a PIN", () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    expect(pushMock).toHaveBeenCalledWith("/player/mi-hero-1");
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });

  it("shows the PIN modal instead of navigating when the gate requires a PIN", () => {
    needsPinMock.mockReturnValue(true);
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    expect(pushMock).not.toHaveBeenCalledWith("/player/mi-hero-1");
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
  });

  it("navigates to the player only after the PIN is verified", () => {
    needsPinMock.mockReturnValue(true);
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    expect(pushMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByLabelText("verify-pin"));
    expect(pushMock).toHaveBeenCalledWith("/player/mi-hero-1");
  });

  it("does not navigate if the PIN modal is cancelled", () => {
    needsPinMock.mockReturnValue(true);
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    fireEvent.click(screen.getByLabelText("cancel-pin"));
    expect(pushMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });
});

describe("HomeScreen per-library rows", () => {
  beforeEach(() => {
    librariesMock.mockReturnValue({
      data: {
        libraries: [
          { id: "lib-1", name: "Movies", type: "LIBRARY_TYPE_MOVIES" },
          { id: "lib-2", name: "Empty Library", type: "LIBRARY_TYPE_SHOWS" },
        ],
      },
      isLoading: false,
    });
    libraryRowResults.set("lib-1", {
      data: { items: [{ id: "mi-lib1-1", title: "Lib1 Movie" }] },
      isLoading: false,
    });
    libraryRowResults.set("lib-2", { data: { items: [] }, isLoading: false });
  });

  it("renders a row titled after the library, for a library with recent items", () => {
    render(<HomeScreen />);
    expect(screen.getByText("media.home.recentlyAddedInLibrary:Movies")).toBeTruthy();
  });

  it("renders no row (no orphaned title) for a library with no recent items", () => {
    render(<HomeScreen />);
    expect(screen.queryByText("media.home.recentlyAddedInLibrary:Empty Library")).toBeNull();
  });

  it("queries each library's rows scoped to its own id, sorted by date added", () => {
    render(<HomeScreen />);
    expect(getLibraryServiceListMediaItemsQueryOptionsMock).toHaveBeenCalledWith(
      expect.objectContaining({
        libraryId: "lib-1",
        sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
        sortOrder: "SORT_ORDER_DESC",
      }),
    );
    expect(getLibraryServiceListMediaItemsQueryOptionsMock).toHaveBeenCalledWith(
      expect.objectContaining({ libraryId: "lib-2" }),
    );
  });

  it("keeps the initial D-pad focus coherent: falls through past the static rows to the first non-empty library row", () => {
    resumeMock.mockReturnValue({ data: { items: [], progress: [] }, isLoading: false });
    nextUpMock.mockReturnValue({ data: { items: [] }, isLoading: false });
    listMediaItemsMock.mockReturnValue({ data: { items: [] }, isLoading: false }); // hides the hero too
    render(<HomeScreen />);
    const focused = document.querySelector('[data-tv-preferred-focus="true"]');
    expect(focused?.textContent).toContain("Lib1 Movie");
  });
});

// The home used to hardcode three rows plus one per library; the owner asked
// (2026-08-02) for it to match web/mobile, which means the SERVER decides what
// is on screen and in what order.
describe("HomeScreen feed-config driven rows", () => {
  beforeEach(() => {
    listMediaItemsMock.mockReturnValue({ data: { items: [] }, isLoading: false }); // no hero
    collectionsMock.mockReturnValue({
      data: { collections: [{ id: "col-1", name: "Marvel", posterUrl: "http://srv.test/c.jpg" }] },
      isLoading: false,
    });
    recommendedMock.mockReturnValue({
      data: { items: [{ mediaItemId: "mi-rec", title: "Recommended Film" }] },
      isLoading: false,
    });
    popularMock.mockReturnValue({ data: [{ mediaItemId: "mi-pop", title: "Popular Film" }], isLoading: false });
  });

  it("renders the shelves web and mobile have, which the TV never had", () => {
    render(<HomeScreen />);
    expect(screen.getByText("media.home.collections")).toBeTruthy();
    expect(screen.getByText("media.home.recommendedForYou")).toBeTruthy();
    expect(screen.getByText("media.home.mostPopular")).toBeTruthy();
  });

  it("renders the rows in the order the server configured, not a hardcoded one", () => {
    feedConfigMock.mockReturnValue({
      data: {
        configs: [
          { feedType: "most_popular", enabled: true, displayOrder: 0 },
          { feedType: "collections", enabled: true, displayOrder: 1 },
        ],
      },
    });
    render(<HomeScreen />);
    const titles = Array.from(document.querySelectorAll("span"))
      .map((n) => n.textContent)
      .filter((txt) => txt === "media.home.mostPopular" || txt === "media.home.collections");
    expect(titles).toEqual(["media.home.mostPopular", "media.home.collections"]);
  });

  it("hides a row the server disabled", () => {
    feedConfigMock.mockReturnValue({
      data: {
        configs: [
          { feedType: "collections", enabled: false, displayOrder: 0 },
          { feedType: "most_popular", enabled: true, displayOrder: 1 },
        ],
      },
    });
    render(<HomeScreen />);
    expect(screen.queryByText("media.home.collections")).toBeNull();
    expect(screen.getByText("media.home.mostPopular")).toBeTruthy();
  });

  it("sends a collection card to the collection, not to /media/<id>", () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("Marvel"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/collections/col-1");
  });
});

// A music or photo card must not land on the video detail screen - that was
// the reason those libraries had no row at all until the TV grew screens that
// suit them.
describe("HomeScreen music and photo library rows", () => {
  beforeEach(() => {
    listMediaItemsMock.mockReturnValue({ data: { items: [] }, isLoading: false });
    librariesMock.mockReturnValue({
      data: {
        libraries: [
          { id: "lib-mus", name: "Music", type: "LIBRARY_TYPE_MUSIC" },
          { id: "lib-pho", name: "Photos", type: "LIBRARY_TYPE_PHOTOS" },
        ],
      },
      isLoading: false,
    });
    libraryRowResults.set("lib-mus", {
      data: { albums: [{ id: "alb-1", title: "Kind of Blue", artist: "Miles Davis" }] },
      isLoading: false,
    });
    libraryRowResults.set("lib-pho", {
      data: { albums: [{ name: "2024/Holiday", photoCount: 12 }] },
      isLoading: false,
    });
  });

  it("queries albums, not media items, for those libraries", () => {
    render(<HomeScreen />);
    expect(getMusicAlbumsQueryOptionsMock).toHaveBeenCalledWith({ libraryId: "lib-mus" });
    expect(getPhotoAlbumsQueryOptionsMock).toHaveBeenCalledWith({ libraryId: "lib-pho" });
    expect(getLibraryServiceListMediaItemsQueryOptionsMock).not.toHaveBeenCalledWith(
      expect.objectContaining({ libraryId: "lib-mus" }),
    );
  });

  it("sends an album card to the album screen and a photo card to the photos screen", () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("Kind of Blue"));
    // With the library id: the album screen needs both, and a bare id left its
    // query disabled - raw id as the title, "no track found".
    expect(pushMock).toHaveBeenCalledWith("/(auth)/music/album/alb-1?libraryId=lib-mus");
    fireEvent.click(screen.getByLabelText("2024/Holiday"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/photos");
  });
});
