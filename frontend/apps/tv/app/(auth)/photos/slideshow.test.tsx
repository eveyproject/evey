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
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
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
vi.mock("@evey/api", () => ({
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
