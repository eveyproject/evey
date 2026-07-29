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
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent, ListFooterComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(React.Fragment, null, ListEmptyComponent) : null;
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
