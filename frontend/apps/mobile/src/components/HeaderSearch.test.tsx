import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The header field lives inside the native toolbar, so the results panel now
// renders in a Modal (its own native surface) as a full-screen safe-area
// overlay - see the header comment in HeaderSearch.tsx for the rationale.
// The Modal mock always mounts its children (like BottomSheet.test.tsx);
// the real gating is the `open` guard inside the Modal.

vi.mock("react-native", () => ({
  View: React.forwardRef(({ children, ...p }: any, ref: any) =>
    React.createElement("div", { ...p, ref }, children)),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, onFocus, onSubmitEditing, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
      onFocus,
      onKeyDown: (e: React.KeyboardEvent) => { if (e.key === "Enter") onSubmitEditing?.(); },
    }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "search-loading", ...p }),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Modal: ({ children }: any) => React.createElement("div", null, children),
  Keyboard: { addListener: () => ({ remove: () => {} }), dismiss: () => {} },
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));

vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));
vi.mock("expo-router", () => ({ router: { push: vi.fn() } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, opts?: unknown) => (typeof opts === "string" ? opts : key) }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      text: "#fff", textMuted: "#888", accent: "#f00", surfaceRaised: "#222",
      border: "#333", onAccent: "#fff", shadow: "#000", surface: "#111", bg: "#000", danger: "#f00",
    },
  }),
}));

vi.mock("../hooks/useDebounce", () => ({ useDebounce: (v: unknown) => v }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../lib/artwork", () => ({ artworkUrl: (serverUrl: string, id: string) => `${serverUrl}/poster/${id}` }));
vi.mock("./music/musicSource", () => ({
  searchTrackMusicSource: (albumId?: string, album?: string) => ({ albumId, album }),
}));

const { setQueueMock } = vi.hoisted(() => ({ setQueueMock: vi.fn() }));
vi.mock("../stores/musicQueue", () => ({
  useMusicQueueStore: (selector: (s: { setQueue: typeof setQueueMock }) => unknown) => selector({ setQueue: setQueueMock }),
}));

let secureStoreValue: string | null = null;
vi.mock("expo-secure-store", () => ({
  getItem: (_key: string) => secureStoreValue,
  setItem: vi.fn(),
}));

// --- @evey/api -------------------------------------------------------------------------
let mediaState: { data?: { items: unknown[]; totalHits?: number }; isLoading: boolean; isError: boolean; refetch: () => void };
let musicState: { data?: { tracks: unknown[] }; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("@evey/api", () => ({
  useLibraryServiceSearchMedia: () => mediaState,
  useMusicServiceSearchMusic: () => musicState,
  displayTitle: (item: { title?: string }) => item.title ?? "",
}));

import { router } from "expo-router";
import { HeaderSearch } from "./HeaderSearch";

function settled<T>(data: T): { data: T; isLoading: boolean; isError: boolean; refetch: () => void } {
  return { data, isLoading: false, isError: false, refetch: vi.fn() };
}

beforeEach(() => {
  vi.clearAllMocks();
  secureStoreValue = null;
  mediaState = settled({ items: [] });
  musicState = settled({ tracks: [] });
});

// The header field is now only a trigger (a Pressable showing the placeholder
// TEXT); tapping it opens the Modal whose own TextInput (aria-label = the
// placeholder) is the real, keyboard-owning input. So: click the trigger text,
// then type into the modal input.
function typeAndFocus(query: string) {
  fireEvent.click(screen.getByText("media.search.placeholder"));
  const input = screen.getByLabelText("media.search.placeholder");
  fireEvent.change(input, { target: { value: query } });
}

describe("HeaderSearch - media scope, loading vs failure vs no results", () => {
  it("shows a loading indicator while the query is in flight, not 'no results'", () => {
    mediaState = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(<HeaderSearch />);
    typeAndFocus("batman");

    expect(screen.getByTestId("search-loading")).toBeTruthy();
    expect(screen.queryByText("media.search.noResults")).toBeNull();
    expect(screen.queryByText("media.search.searchFailed")).toBeNull();
  });

  it("shows a retry option, not 'no results', when the search fails", () => {
    const refetch = vi.fn();
    mediaState = { data: undefined, isLoading: false, isError: true, refetch };
    render(<HeaderSearch />);
    typeAndFocus("batman");

    expect(screen.getByText("media.search.searchFailed")).toBeTruthy();
    expect(screen.queryByText("media.search.noResults")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("shows 'no results' once the search has actually completed empty", () => {
    mediaState = settled({ items: [] });
    render(<HeaderSearch />);
    typeAndFocus("zzzznotfound");

    expect(screen.getByText("media.search.noResults")).toBeTruthy();
    expect(screen.queryByTestId("search-loading")).toBeNull();
    expect(screen.queryByText("media.search.searchFailed")).toBeNull();
  });
});

describe("HeaderSearch - selecting a media result", () => {
  it("navigates to the item and closes the dropdown", () => {
    mediaState = settled({ items: [{ id: "m1", title: "Movie One", type: "MEDIA_ITEM_TYPE_MOVIE", year: 2020 }], totalHits: 1 });
    render(<HeaderSearch />);
    typeAndFocus("movie");

    fireEvent.click(screen.getByLabelText("Movie One"));

    expect(router.push).toHaveBeenCalledWith("/media/m1");
    expect(screen.queryByLabelText("Movie One")).toBeNull();
  });

  it("shows a 'view all' link and navigates to the full search screen when there are more hits than shown", () => {
    mediaState = settled({ items: [{ id: "m1", title: "Movie One" }], totalHits: 5 });
    render(<HeaderSearch />);
    typeAndFocus("movie");

    fireEvent.click(screen.getByText("media.search.viewAllResults"));

    expect(router.push).toHaveBeenCalledWith({
      pathname: "/(auth)/(tabs)/search",
      params: { q: "movie" },
    });
  });
});

describe("HeaderSearch - music scope", () => {
  it("only queries the active scope and enqueues the picked track", () => {
    mediaState = settled({ items: [] });
    musicState = settled({ tracks: [{ id: "t1", title: "Song", artist: "Artist", album: "Album" }] });
    render(<HeaderSearch />);
    typeAndFocus("queen");

    fireEvent.click(screen.getByText("media.search.sectionMusic"));
    fireEvent.click(screen.getByLabelText("Song"));

    expect(setQueueMock).toHaveBeenCalledWith(
      [{ id: "t1", title: "Song", artist: "Artist", album: "Album", duration: 0 }],
      0,
      { albumId: undefined, album: "Album" },
    );
  });
});

describe("HeaderSearch - recent searches", () => {
  it("shows recents on focus when the query is empty, and picking one opens the full search screen", () => {
    secureStoreValue = JSON.stringify(["old query"]);
    render(<HeaderSearch />);

    fireEvent.click(screen.getByText("media.search.placeholder"));

    expect(screen.getByLabelText("old query")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("old query"));

    expect(router.push).toHaveBeenCalledWith({
      pathname: "/(auth)/(tabs)/search",
      params: { q: "old query" },
    });
  });
});

describe("HeaderSearch - submitting from the keyboard", () => {
  it("opens the full search screen with the typed query", () => {
    render(<HeaderSearch />);
    fireEvent.click(screen.getByText("media.search.placeholder"));
    const input = screen.getByLabelText("media.search.placeholder");
    fireEvent.change(input, { target: { value: "batman" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(router.push).toHaveBeenCalledWith({
      pathname: "/(auth)/(tabs)/search",
      params: { q: "batman" },
    });
  });
});
