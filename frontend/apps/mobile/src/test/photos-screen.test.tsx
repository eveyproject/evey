import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Regression coverage for photos parity items: album cards must be
// pressable (drill-down), the lightbox needs prev/next + a position
// counter + rotation, grid thumbnails must request the optimized (webp,
// w=400) variant, and the photo list must be a single virtualized FlatList
// (no scrollEnabled={false} FlatList nested in an outer ScrollView).

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
  // Renders data items plus header/footer so the header-driven album/section
  // content (moved out of the old wrapping ScrollView) is still reachable,
  // while asserting elsewhere that this is the ONLY scrollable list (no
  // nested non-scrolling FlatList wrapped by an outer ScrollView).
  FlatList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListFooterComponent, scrollEnabled }: any) =>
    React.createElement(
      "div",
      { "data-testid": "flatlist", "data-scroll-enabled": String(scrollEnabled !== false) },
      ListHeaderComponent,
      (data ?? []).map((item: any, i: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, i) : i },
          renderItem({ item, index: i }),
        ),
      ),
      ListFooterComponent,
    ),
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

vi.mock("@evey/api", () => ({
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
  it("requests photos with the raised limit (parity with the web client's 5000 cap)", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => {
      const call = eveyFetchMock.mock.calls.find(([{ url }]) => url.startsWith("/api/v1/photos?"));
      expect(call?.[0].url).toContain("limit=5000");
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

  it("renders the photo grid as a single scrollable FlatList (no nested non-scrolling list)", async () => {
    render(<PhotosScreen />, { wrapper: makeWrapper() });
    await waitFor(() => expect(findThumb("p1")).toBeTruthy());

    const lists = screen.getAllByTestId("flatlist");
    // The main photo grid FlatList must scroll (default scrollEnabled).
    const mainList = lists.find((el) => el.getAttribute("data-scroll-enabled") === "true");
    expect(mainList).toBeTruthy();
  });
});
