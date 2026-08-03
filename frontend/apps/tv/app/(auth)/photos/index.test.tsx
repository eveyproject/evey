import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops a boolean value passed through an unrecognized DOM prop, so
  // spreading it through as-is would make it unobservable in jsdom.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent, ListFooterComponent, onEndReached }: any) => {
    // No real scroll geometry in jsdom - the test simulates "reached the
    // bottom" by clicking this hidden trigger, exactly like a TV remote
    // driving the FlatList past onEndReachedThreshold would.
    const trigger = React.createElement(
      "button",
      { "data-testid": "end-reached-trigger", onClick: () => onEndReached?.(), style: { display: "none" } },
      "trigger",
    );
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(React.Fragment, null, ListEmptyComponent, trigger) : trigger;
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
      ListFooterComponent ?? null,
      trigger,
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
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- expo-router mock ---------------------------------------------------------
const { pushMock } = vi.hoisted(() => ({ pushMock: vi.fn() }));
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args) },
  Stack: { Screen: () => null },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
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
      surfaceRaised: "#222",
      border: "#333",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv.test" }));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

// --- @evey/api mock --------------------------------------------------------------
const { listLibrariesMock, listAlbumsMock, listPhotosMock } = vi.hoisted(() => ({
  listLibrariesMock: vi.fn(),
  listAlbumsMock: vi.fn(),
  listPhotosMock: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: (...args: unknown[]) => listLibrariesMock(...args),
  usePhotoServiceListPhotoAlbums: (...args: unknown[]) => listAlbumsMock(...args),
  usePhotoServiceListPhotos: (...args: unknown[]) => listPhotosMock(...args),
}));

// -------------------------------------------------------------------------------------
import PhotosScreen from "./index";

const LIBRARIES = [{ id: "lib-1", name: "My Photos", type: "LIBRARY_TYPE_PHOTOS" }];
const PHOTOS = [
  { id: "p1", title: "Beach", blurhash: "" },
  { id: "p2", title: "Mountains", blurhash: "" },
];

beforeEach(() => {
  vi.clearAllMocks();
  listLibrariesMock.mockReturnValue({ data: LIBRARIES, isLoading: false });
  listAlbumsMock.mockReturnValue({ data: [], isLoading: false });
  listPhotosMock.mockReturnValue({
    data: { photos: PHOTOS, total: PHOTOS.length },
    isLoading: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
  });
});

describe("PhotosScreen (TV) - no photo libraries", () => {
  it("keeps a focusable escape target so the remote never gets stuck", () => {
    listLibrariesMock.mockReturnValue({ data: [], isLoading: false });
    render(<PhotosScreen />);

    expect(screen.getByText("media.photos.noPhotoLibraries")).toBeTruthy();
    const escapeBtn = screen.getByText("media.library.libraries").closest("button");
    expect(escapeBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });
});

describe("PhotosScreen (TV) - empty album", () => {
  it("keeps a focusable escape target when a library has no photos", () => {
    listPhotosMock.mockReturnValue({
      data: { photos: [], total: 0 },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<PhotosScreen />);

    expect(screen.getByText("media.photos.noPhotos")).toBeTruthy();
    const escapeBtn = screen.getByText("media.library.libraries").closest("button");
    expect(escapeBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });
});

describe("PhotosScreen (TV) - grid thumbnails", () => {
  it("requests a server-whitelisted thumbnail width, never full resolution", () => {
    render(<PhotosScreen />);

    const images = screen.getAllByAltText("") as HTMLImageElement[];
    const beachImg = images.find((img) => img.src.includes("/api/v1/photos/p1/serve"));
    expect(beachImg?.src).toContain("w=400");
  });

  it("opens the slideshow at the pressed photo's index", () => {
    render(<PhotosScreen />);

    fireEvent.click(screen.getByLabelText("Mountains"));

    expect(pushMock).toHaveBeenCalledWith(
      expect.stringContaining("/(auth)/photos/slideshow?libraryId=lib-1&startIndex=1"),
    );
  });
});

describe("PhotosScreen (TV) - loading state", () => {
  it("shows a spinner instead of an empty grid while libraries are loading", () => {
    listLibrariesMock.mockReturnValue({ data: undefined, isLoading: true });
    render(<PhotosScreen />);

    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("media.photos.noPhotoLibraries")).toBeNull();
    expect(screen.queryByLabelText("Beach")).toBeNull();
  });
});

describe("PhotosScreen (TV) - request failure", () => {
  it("shows a distinct error with a focusable retry, not the empty-library or empty-album state", () => {
    const refetchMock = vi.fn();
    listPhotosMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      isError: true,
      refetch: refetchMock,
    });
    render(<PhotosScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.photos.noPhotos")).toBeNull();
    expect(screen.queryByText("media.photos.noPhotoLibraries")).toBeNull();

    const retryBtn = screen.getByText("common.retry").closest("button");
    expect(retryBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");

    fireEvent.click(retryBtn!);
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("PhotosScreen (TV) - tile accessibility", () => {
  it("gives every titled tile a non-empty accessible label", () => {
    render(<PhotosScreen />);

    expect(screen.getByLabelText("Beach")).toBeTruthy();
    expect(screen.getByLabelText("Mountains")).toBeTruthy();
  });

  it("announces its position when a photo carries neither title nor album", () => {
    listPhotosMock.mockReturnValue({
      data: { photos: [...PHOTOS, { id: "p3", title: "", album: "", blurhash: "" }], total: 3 },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    });
    const { container } = render(<PhotosScreen />);

    // On a television the focused element is the only thing announced, so a
    // silent tile is unreachable information.
    expect(container.querySelectorAll('button[aria-label=""]').length).toBe(0);
    expect(screen.getByLabelText("media.photos.openPhoto")).toBeTruthy();
  });
});

describe("PhotosScreen (TV) - keyset pagination", () => {
  // A full page (== PAGE_SIZE, 60) with a non-empty nextCursor - the exact
  // shape a keyset page returns (see V1ListPhotosResponse.total's doc: total
  // is 0 on that path, never usable as the "more pages" signal).
  const FULL_PAGE = Array.from({ length: 60 }, (_, i) => ({ id: `p${i}`, title: `Photo ${i}`, blurhash: "" }));

  it("sends the previous page's nextCursor on the following request", () => {
    listPhotosMock.mockReturnValue({
      data: { photos: FULL_PAGE, total: 2, nextCursor: "cursor-page-2" },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<PhotosScreen />);

    // Page 1: no cursor param at all (the offset(0) path).
    const firstCallParams = listPhotosMock.mock.calls[0][0];
    expect(firstCallParams).not.toHaveProperty("cursor");

    fireEvent.click(screen.getByTestId("end-reached-trigger"));

    const lastCallParams = listPhotosMock.mock.calls.at(-1)?.[0];
    expect(lastCallParams).toMatchObject({ cursor: "cursor-page-2" });
  });

  it("stops paginating once nextCursor comes back empty", () => {
    listPhotosMock.mockReturnValue({
      data: { photos: PHOTOS, total: PHOTOS.length, nextCursor: "" },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<PhotosScreen />);

    const callsBefore = listPhotosMock.mock.calls.length;
    fireEvent.click(screen.getByTestId("end-reached-trigger"));

    // No new request means the cursor state never advanced - the empty
    // nextCursor correctly ended the list instead of looping or truncating.
    expect(listPhotosMock.mock.calls.length).toBe(callsBefore);
  });
});
