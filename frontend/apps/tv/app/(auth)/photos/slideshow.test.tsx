import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
// useTVEventHandler captures the screen's own callback so tests can simulate a
// remote keypress directly - same convention as player/[id].test.tsx.
let capturedTVHandler: ((evt: { eventType: string } | undefined) => void) | null = null;
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {}, absoluteFill: {} },
  useTVEventHandler: (cb: (evt: { eventType: string } | undefined) => void) => {
    capturedTVHandler = cb;
  },
}));

// --- expo-image mock ---------------------------------------------------------
// Image.prefetch is called by the screen's preload effect - a bare functional
// mock (as other screens use) leaves it undefined and throws. The Image
// function is built entirely inside the factory (not a module-level const):
// this factory runs while resolving the real slideshow.tsx's own imports,
// which happens before this file's own top-level statements do, so a
// module-level const referenced here hits its temporal dead zone.
const { imagePrefetchMock } = vi.hoisted(() => ({
  imagePrefetchMock: vi.fn((..._args: unknown[]) => Promise.resolve()),
}));
vi.mock("expo-image", () => {
  const Image: any = (props: any) => React.createElement("img", { alt: "", src: props.source?.uri });
  Image.prefetch = (...args: unknown[]) => imagePrefetchMock(...args);
  return { Image };
});

// --- expo-router mock ---------------------------------------------------------
const { backMock, replaceMock, canGoBackMock, localSearchParamsMock } = vi.hoisted(() => ({
  backMock: vi.fn(),
  replaceMock: vi.fn(),
  canGoBackMock: vi.fn(() => true),
  localSearchParamsMock: vi.fn(),
}));
vi.mock("expo-router", () => ({
  router: {
    back: (...args: unknown[]) => backMock(...args),
    replace: (...args: unknown[]) => replaceMock(...args),
    canGoBack: () => canGoBackMock(),
  },
  useLocalSearchParams: () => localSearchParamsMock(),
}));

// --- react-i18next mock --------------------------------------------------------
// t still returns the key (so text assertions read as before), but records its
// arguments: the photo counter's interpolated total is only observable there.
const { translateMock } = vi.hoisted(() => ({
  translateMock: vi.fn((key: string, _opts?: unknown) => key),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, opts?: unknown) => translateMock(key, opts) }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
// Not read directly by this screen, but FocusableCard (used by its error/empty
// state) imports it statically - same mock as photos/index.test.tsx.
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
const { listPhotosMock } = vi.hoisted(() => ({ listPhotosMock: vi.fn() }));
// Spreading importOriginal is required, not cosmetic: the screen also imports
// the shared hasMoreFromCursor from this package, and a mock that lists only
// hooks hands it back undefined.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePhotoServiceListPhotos: (...args: unknown[]) => listPhotosMock(...args),
}));

// -------------------------------------------------------------------------------------
import PhotoSlideshowScreen from "./slideshow";

const PHOTOS = [
  { id: "p0", title: "Zero", blurhash: "" },
  { id: "p1", title: "One", blurhash: "" },
  { id: "p2", title: "Two", blurhash: "" },
];

beforeEach(() => {
  vi.clearAllMocks();
  capturedTVHandler = null;
  canGoBackMock.mockReturnValue(true);
  localSearchParamsMock.mockReturnValue({ libraryId: "lib-1", startIndex: "1" });
  listPhotosMock.mockReturnValue({
    data: { photos: PHOTOS, total: PHOTOS.length },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  });
});

describe("PhotoSlideshowScreen (TV) - initial position", () => {
  it("shows the photo at the startIndex passed in the route params", () => {
    const { container } = render(<PhotoSlideshowScreen />);

    expect(screen.getByText("One")).toBeTruthy();
    expect(container.querySelector("img")?.getAttribute("src")).toContain("/api/v1/photos/p1/serve");
  });
});

describe("PhotoSlideshowScreen (TV) - D-pad navigation", () => {
  it("steps to the next/previous photo on right/left remote events", () => {
    render(<PhotoSlideshowScreen />);
    expect(screen.getByText("One")).toBeTruthy();

    act(() => {
      capturedTVHandler?.({ eventType: "right" });
    });
    expect(screen.getByText("Two")).toBeTruthy();

    act(() => {
      capturedTVHandler?.({ eventType: "left" });
    });
    act(() => {
      capturedTVHandler?.({ eventType: "left" });
    });
    expect(screen.getByText("Zero")).toBeTruthy();
  });
});

describe("PhotoSlideshowScreen (TV) - edges", () => {
  // src/lib/photoSlideshow.ts's manual prevSlideshowIndex/nextSlideshowIndex
  // clamp at both ends rather than wrapping - only the autoplay tick (not
  // under test here) loops back to the start.
  it("clamps at the first photo instead of wrapping to the last on left", () => {
    localSearchParamsMock.mockReturnValue({ libraryId: "lib-1", startIndex: "0" });
    render(<PhotoSlideshowScreen />);
    expect(screen.getByText("Zero")).toBeTruthy();

    act(() => {
      capturedTVHandler?.({ eventType: "left" });
    });
    expect(screen.getByText("Zero")).toBeTruthy();
  });

  it("clamps at the last photo instead of wrapping to the first on right", () => {
    localSearchParamsMock.mockReturnValue({ libraryId: "lib-1", startIndex: "2" });
    render(<PhotoSlideshowScreen />);
    expect(screen.getByText("Two")).toBeTruthy();

    act(() => {
      capturedTVHandler?.({ eventType: "right" });
    });
    expect(screen.getByText("Two")).toBeTruthy();
  });
});

describe("PhotoSlideshowScreen (TV) - paged listing", () => {
  // The screen used to ask for 1000 photos in one call. The server replaces any
  // limit above maxPageSize (500) with the photo handler's default of 100, so
  // the slideshow silently held 100 photos of any album - and a start index the
  // grid had paged past was clamped onto whichever photo happened to be loaded,
  // i.e. the WRONG picture.
  it("asks for a page size the server will honour", () => {
    render(<PhotoSlideshowScreen />);

    const params = listPhotosMock.mock.calls[0][0] as { limit?: number };
    expect(params.limit).toBeGreaterThan(0);
    expect(params.limit).toBeLessThanOrEqual(500);
  });

  it("asks for the next page with the cursor the previous page returned", () => {
    listPhotosMock.mockReturnValue({
      data: { photos: PHOTOS, total: 300, nextCursor: "cur-1" },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    });

    render(<PhotoSlideshowScreen />);

    const withCursor = listPhotosMock.mock.calls.some(
      (c) => (c[0] as { cursor?: string })?.cursor === "cur-1",
    );
    expect(withCursor).toBe(true);
  });

  it("counts against the album's real size, not how much of it has been fetched", () => {
    listPhotosMock.mockReturnValue({
      data: { photos: PHOTOS, total: 228, nextCursor: "cur-1" },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    });

    render(<PhotoSlideshowScreen />);

    // The i18n stub returns the key, so assert on what was handed to it.
    expect(translateMock).toHaveBeenCalledWith(
      "media.photos.positionOf",
      expect.objectContaining({ total: 228 }),
    );
  });
});
