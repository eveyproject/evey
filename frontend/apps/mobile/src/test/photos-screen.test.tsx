import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Regression coverage for photos parity items: album cards must be pressable
// (drill-down), the lightbox needs prev/next + a position counter + rotation,
// grid thumbnails must request the optimized (webp, w=400) variant, the photo
// grid must be a single virtualized LegendList, and the albums must be a
// horizontal carousel (no "show more" wall).

// The grid's onEndReached, captured from the LegendList mock so a test can
// simulate the scroll that drives pagination.
const { gridState } = vi.hoisted(() => ({ gridState: { onEndReached: null as (() => void) | null } }));

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  TouchableOpacity: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Dimensions: {
    get: () => ({ width: 390, height: 844 }),
  },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// LegendList mock: renders header + every data item + footer, and captures the
// grid's onEndReached. Only the vertical photo grid passes one; the horizontal
// album carousel does not, so an unconditional assignment would overwrite the
// grid's callback with the carousel's undefined. A mock that ignores
// onEndReached cannot see a pagination defect at all.
vi.mock("@legendapp/list/react-native", () => ({
  LegendList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListFooterComponent, onEndReached, horizontal }: any) => {
    if (onEndReached) gridState.onEndReached = onEndReached;
    return React.createElement(
      "div",
      { "data-testid": "legendlist", "data-horizontal": String(!!horizontal) },
      ListHeaderComponent,
      (data ?? []).map((item: any, i: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, i) : i },
          renderItem({ item, index: i }),
        ),
      ),
      ListFooterComponent,
    );
  },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: ({ source, style, ...p }: any) => React.createElement("img", { src: source?.uri, style, alt: "", ...p }),
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallbackOrOpts?: unknown, maybeOpts?: Record<string, unknown>) => {
      const fallback = typeof fallbackOrOpts === "string" ? fallbackOrOpts : key;
      const opts = (typeof fallbackOrOpts === "object" ? fallbackOrOpts : maybeOpts) as Record<string, unknown> | undefined;
      if (!opts) return fallback;
      return fallback.replace(/\{\{(\w+)\}\}/g, (_, k) => String(opts[k] ?? ""));
    },
  }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", onAccent: "#fff", border: "#333",
    },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

// --- misc hooks --------------------------------------------------------------------
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => undefined,
}));

// --- @evey/api -------------------------------------------------------------------------
const LIB_ID = "lib-1";
const ALBUM_NAME = "Vacation";

const libraries = [{ id: LIB_ID, name: "My Photos", type: "LIBRARY_TYPE_PHOTOS" }];
// coverPhotoId deliberately distinct from any photo id below so its cover
// thumbnail (which also matches "serve?w=400&fmt=webp") can't be confused
// with a grid tile when locating an image by photo id in tests.
const albums = [{ name: ALBUM_NAME, libraryId: LIB_ID, photoCount: 2, coverPhotoId: "cover-x" }];
const allPhotos = [
  { id: "p1", title: "Beach", album: ALBUM_NAME },
  { id: "p2", title: "Sunset", album: ALBUM_NAME },
  { id: "p3", title: "Home", album: "" },
];

const eveyFetchMock = vi.fn(({ url }: { url: string }) => {
  if (url.startsWith("/api/v1/libraries")) return Promise.resolve({ libraries });
  if (url.startsWith("/api/v1/photos/albums")) return Promise.resolve({ albums });
  if (url.startsWith("/api/v1/photos?")) {
    const albumParam = new URLSearchParams(url.split("?")[1]).get("album");
    const photos = albumParam ? allPhotos.filter((p) => p.album === albumParam) : allPhotos;
    return Promise.resolve({ photos, total: photos.length });
  }
  if (url.match(/\/api\/v1\/photos\/[^/]+$/)) {
    return Promise.resolve({ id: "p1", title: "Beach", filePath: "", fileSize: 0, mimeType: "", width: 0, height: 0, takenAt: "", cameraModel: "", album: ALBUM_NAME });
  }
  return Promise.reject(new Error(`unexpected url ${url}`));
});

// Spreading importOriginal is required, not cosmetic: the screen also imports
// the shared hasMoreFromCursor from this package, and a mock that lists only
// eveyFetch hands it back undefined.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: [{ url: string }]) => eveyFetchMock(...args),
}));

// --- Skeleton ---------------------------------------------------------------------------
vi.mock("../../src/components/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

// -------------------------------------------------------------------------------------
import PhotosScreen from "../../app/(auth)/(tabs)/photos";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

// jsdom's CSS selector engine (nwsapi) fails to match attribute selectors
// whose value contains an unescaped "&", even inside quotes - so thumbnail
// URLs (which carry "&fmt=webp") can't be found via querySelector directly.
// Filter the raw `src` attribute in JS instead.
function findThumb(id: string): HTMLImageElement | undefined {
  return Array.from(document.querySelectorAll("img")).find((img) => img.getAttribute("src")?.includes(`${id}/serve?w=400&fmt=webp`)) as HTMLImageElement | undefined;
}

beforeEach(() => {
  vi.clearAllMocks();
  gridState.onEndReached = null;
  eveyFetchMock.mockImplementation(({ url }: { url: string }) => {
    if (url.startsWith("/api/v1/libraries")) return Promise.resolve({ libraries });
    if (url.startsWith("/api/v1/photos/albums")) return Promise.resolve({ albums });
    if (url.startsWith("/api/v1/photos?")) {
      const albumParam = new URLSearchParams(url.split("?")[1]).get("album");
      const photos = albumParam ? allPhotos.filter((p) => p.album === albumParam) : allPhotos;
      return Promise.resolve({ photos, total: photos.length });
    }
    if (url.match(/\/api\/v1\/photos\/[^/]+$/)) {
      return Promise.resolve({ id: "p1", title: "Beach", filePath: "", fileSize: 0, mimeType: "", width: 0, height: 0, takenAt: "", cameraModel: "", album: ALBUM_NAME });
    }
    return Promise.reject(new Error(`unexpected url ${url}`));
  });
});

describe("PhotosScreen", () => {
  // This test used to assert `limit=5000`, i.e. it pinned the bug as the spec.
  // The server has never served that: any limit above maxPageSize (500) is
  // replaced by the photo handler's own default of 100 (clampLimitOffset,
  // grpcapi/helpers.go), so the grid held 100 tiles of a 7199-photo library
  // while its footer announced 7199. Asserting the VALUE is the point - a limit
  // the server refuses looks exactly like one it honours, at the UI level.
  it("requests a page size the server will honour, not an unbounded one", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => {
      const call = eveyFetchMock.mock.calls.find(([{ url }]) => url.startsWith("/api/v1/photos?"));
      expect(call).toBeTruthy();
      const limit = Number(new URLSearchParams(call![0].url.split("?")[1]).get("limit"));
      expect(limit).toBeGreaterThan(0);
      expect(limit).toBeLessThanOrEqual(500);
    });
  });

  it("fetches the next page with the cursor the previous page returned", async () => {
    eveyFetchMock.mockImplementation(({ url }: { url: string }) => {
      if (url.startsWith("/api/v1/libraries")) return Promise.resolve({ libraries });
      if (url.startsWith("/api/v1/photos/albums")) return Promise.resolve({ albums });
      if (url.startsWith("/api/v1/photos?")) {
        const params = new URLSearchParams(url.split("?")[1]);
        // Page 1 comes back with a cursor; page 2 has none, which is how the
        // server says "that was the last one".
        if (!params.get("cursor")) {
          return Promise.resolve({ photos: allPhotos, total: 7199, nextCursor: "cur-1" });
        }
        return Promise.resolve({ photos: [], total: 0 });
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    });

    render(<PhotosScreen />, { wrapper: makeWrapper() });

    // Wait for page 1, then simulate the scroll that asks for the next one.
    await waitFor(() => expect(gridState.onEndReached).toBeTruthy());
    await waitFor(() =>
      expect(eveyFetchMock.mock.calls.some(([{ url }]) => url.startsWith("/api/v1/photos?"))).toBe(true),
    );
    await act(async () => {
      gridState.onEndReached?.();
    });

    await waitFor(() => {
      const withCursor = eveyFetchMock.mock.calls.some(
        ([{ url }]) => url.startsWith("/api/v1/photos?") && url.includes("cursor=cur-1"),
      );
      expect(withCursor).toBe(true);
    });
  });

  // The lightbox steps through `photos`, which only holds the pages the GRID has
  // pulled in. Opening the viewer without having scrolled to the bottom used to
  // dead-end: the Next arrow disappeared partway through the album and the
  // counter reported the loaded count as the total. The grid's onEndReached is
  // deliberately NOT fired here - the point is that stepping through the viewer
  // pages on its own.
  it("pages from inside the lightbox, without the grid being scrolled", async () => {
    eveyFetchMock.mockImplementation(({ url }: { url: string }) => {
      if (url.startsWith("/api/v1/libraries")) return Promise.resolve({ libraries });
      if (url.startsWith("/api/v1/photos/albums")) return Promise.resolve({ albums });
      if (url.startsWith("/api/v1/photos?")) {
        const params = new URLSearchParams(url.split("?")[1]);
        if (!params.get("cursor")) {
          return Promise.resolve({ photos: allPhotos, total: 7199, nextCursor: "cur-1" });
        }
        return Promise.resolve({ photos: [], total: 0 });
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    });

    render(<PhotosScreen />, { wrapper: makeWrapper() });

    await waitFor(() => expect(findThumb("p1")).toBeTruthy());
    fireEvent.click(findThumb("p1")!.closest("button")!);
    await waitFor(() => expect(screen.getByTestId("modal")).toBeTruthy());

    await waitFor(() => {
      const pagedFromViewer = eveyFetchMock.mock.calls.some(
        ([{ url }]) => url.startsWith("/api/v1/photos?") && url.includes("cursor=cur-1"),
      );
      expect(
        pagedFromViewer,
        "opening the lightbox near the end of the loaded photos must request the next page",
      ).toBe(true);
    });
  });

  it("uses the optimized webp thumbnail (not full resolution) in the grid", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => expect(findThumb("p1")).toBeTruthy());
  });

  it("drills down into an album when its (now pressable) card is tapped", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => expect(screen.getByText(ALBUM_NAME)).toBeTruthy());

    const albumButton = screen.getByText(ALBUM_NAME).closest("button");
    expect(albumButton).toBeTruthy();
    fireEvent.click(albumButton!);

    // Breadcrumb appears and the request is scoped to the album.
    await waitFor(() => {
      const call = eveyFetchMock.mock.calls.find(([{ url }]) => url.includes(`album=${encodeURIComponent(ALBUM_NAME)}`));
      expect(call).toBeTruthy();
    });
  });

  it("opens the lightbox with a position counter and prev/next controls", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => expect(findThumb("p1")).toBeTruthy());

    // Photo grid tiles have no visible text, only a thumbnail image.
    const beachThumb = findThumb("p1");
    fireEvent.click(beachThumb!.closest("button")!);

    await waitFor(() => expect(screen.getByTestId("modal")).toBeTruthy());
    expect(screen.getByText("1 / 3")).toBeTruthy();
    // First photo: no "previous" control, but "next" is present.
    expect(screen.queryByLabelText("common.previous")).toBeNull();
    expect(screen.getByLabelText("common.next")).toBeTruthy();
    expect(screen.getByLabelText("common.rotateLeft")).toBeTruthy();
    expect(screen.getByLabelText("common.rotateRight")).toBeTruthy();
  });

  it("navigates to the next photo and updates the position counter", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => expect(findThumb("p1")).toBeTruthy());

    const beachThumb = findThumb("p1");
    fireEvent.click(beachThumb!.closest("button")!);
    await waitFor(() => expect(screen.getByText("1 / 3")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("common.next"));
    await waitFor(() => expect(screen.getByText("2 / 3")).toBeTruthy());
    expect(screen.getByLabelText("common.previous")).toBeTruthy();
  });

  it("renders the photo grid and album carousel as virtualized LegendLists", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => expect(findThumb("p1")).toBeTruthy());

    const lists = screen.getAllByTestId("legendlist");
    // A horizontal album carousel and a vertical photo grid - both virtualized,
    // neither a non-scrolling nested list mounted whole.
    expect(lists.some((el) => el.getAttribute("data-horizontal") === "true")).toBe(true);
    expect(lists.some((el) => el.getAttribute("data-horizontal") === "false")).toBe(true);
  });

  // The albums used to sit in a 2-column grid inside the outer list's header
  // (scrollEnabled=false, never windowed by RN), so a large library mounted
  // every cover at once behind a "show more" wall. They are now a horizontal
  // LegendList carousel - virtualized on its own axis, streaming the full set.
  describe("albums carousel", () => {
    const manyAlbums = Array.from({ length: 20 }, (_, i) => ({
      name: `Album ${i + 1}`,
      libraryId: LIB_ID,
      photoCount: 1,
      coverPhotoId: `cover-${i + 1}`,
    }));

    beforeEach(() => {
      eveyFetchMock.mockImplementation(({ url }: { url: string }) => {
        if (url.startsWith("/api/v1/libraries")) return Promise.resolve({ libraries });
        if (url.startsWith("/api/v1/photos/albums")) return Promise.resolve({ albums: manyAlbums });
        if (url.startsWith("/api/v1/photos?")) return Promise.resolve({ photos: allPhotos, total: allPhotos.length });
        if (url.match(/\/api\/v1\/photos\/[^/]+$/)) {
          return Promise.resolve({ id: "p1", title: "Beach", filePath: "", fileSize: 0, mimeType: "", width: 0, height: 0, takenAt: "", cameraModel: "", album: ALBUM_NAME });
        }
        return Promise.reject(new Error(`unexpected url ${url}`));
      });
    });

    it("streams the whole album set in a horizontal carousel with no 'show more' wall", async () => {
      render(<PhotosScreen />, { wrapper: makeWrapper() });

      expect(await screen.findByText("Album 1")).toBeTruthy();
      // The full set is fed to the carousel (LegendList windows it at runtime);
      // there is no client-side slice and no "Show more" button.
      expect(screen.getByText("Album 20")).toBeTruthy();
      expect(screen.queryByText("Show more")).toBeNull();

      const lists = screen.getAllByTestId("legendlist");
      expect(lists.some((el) => el.getAttribute("data-horizontal") === "true")).toBe(true);
    });
  });
});
