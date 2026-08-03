import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// AddCollectionItem RPC requires RoleAdmin server-side (interceptors/rbac.go).
// The bulk action bar's "add to collection" button must be admin-only, like
// item removal in collections/[id].tsx, while the rest of the bar (watched,
// unwatched, favorite, playlist, cancel) stays available to everyone.

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  TouchableOpacity: ({ children, onPress, onLongPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, onDoubleClick: onLongPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  Pressable: ({ children, onPress, onLongPress, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, onDoubleClick: onLongPress, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  TextInput: ({ value, onChangeText, placeholder, ...p }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText?.(e.target.value), ...p }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(React.Fragment, null, ListEmptyComponent) : null;
    }
    return React.createElement(
      "div",
      null,
      data.map((item: any, i: number) =>
        React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, i) : i }, renderItem({ item, index: i })),
      ),
    );
  },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Dimensions: { get: () => ({ width: 400, height: 800 }), addEventListener: () => ({ remove: vi.fn() }) },
  StyleSheet: {
    create: (s: any) => s,
    absoluteFillObject: {},
    hairlineWidth: 1,
  },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

// --- expo-router ---------------------------------------------------------------------
const { LIBRARY_ID } = vi.hoisted(() => ({ LIBRARY_ID: "lib-1" }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ libraryId: LIBRARY_ID }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  Stack: { Screen: () => null },
}));

// --- ThemeProvider -------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", onAccent: "#fff", border: "#333",
      error: "#e00", warning: "#fa0", success: "#0f0",
    },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useDebounce", () => ({ useDebounce: (v: unknown) => v }));
vi.mock("../hooks/useFavorite", () => ({ useBulkFavorite: () => ({ mutateAsync: vi.fn(), isPending: false }) }));
vi.mock("../components/Skeleton", () => ({ Skeleton: () => null }));
vi.mock("../components/MediaCard", () => ({ MediaCard: () => null }));
vi.mock("../components/library/LibraryFiltersPanel", () => ({ LibraryFiltersPanel: () => null }));
vi.mock("../features/plugins/PluginSlot", () => ({ PluginSlot: () => null }));

const { playlistSheetProps, collectionSheetProps } = vi.hoisted(() => ({
  playlistSheetProps: vi.fn(),
  collectionSheetProps: vi.fn(),
}));
vi.mock("../components/library/BulkAddSheets", () => ({
  BulkAddToPlaylistSheet: (props: any) => {
    playlistSheetProps(props);
    return null;
  },
  BulkAddToCollectionSheet: (props: any) => {
    collectionSheetProps(props);
    return null;
  },
}));

// --- @evey/store -----------------------------------------------------------------------
// Shape copied from packages/store/src/auth.ts: `isAdmin` is a plain boolean
// field on the store state, read through the selector like the screen does.
const { isAdminRef } = vi.hoisted(() => ({ isAdminRef: { current: true } }));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ profileId: "profile-1", isAdmin: isAdminRef.current }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

// --- @evey/api -------------------------------------------------------------------------
const ITEMS = [{ id: "mi-1", title: "Alpha", genres: ["Action"], year: 2001 }];

const { getLibraryMock } = vi.hoisted(() => ({ getLibraryMock: vi.fn() }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetLibrary: () => getLibraryMock(),
  useLibraryServiceListMediaItems: () => ({
    data: { items: ITEMS, pagination: { totalItems: ITEMS.length, totalPages: 0, nextCursor: "" } },
    isLoading: false, isError: false, isFetching: false, refetch: vi.fn(),
  }),
  libraryServiceListMediaItems: vi.fn(),
  useLibraryServiceSearchMedia: () => ({ data: undefined, isLoading: false, isError: false, isFetching: false, refetch: vi.fn() }),
  useLibraryServiceFilterMediaItems: () => ({ data: undefined, isLoading: false, isError: false, isFetching: false, refetch: vi.fn() }),
  useLibraryServiceListGenres: () => ({ data: undefined }),
  usePlaybackServiceGetWatchHistory: () => ({ data: undefined }),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}));

// -------------------------------------------------------------------------------------
import LibraryDetailScreen from "../../app/(auth)/library/[libraryId]";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function renderScreen() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LibraryDetailScreen />
    </QueryClientProvider>,
  );
}

function enterBulkModeOnAlpha() {
  fireEvent.doubleClick(screen.getByLabelText("Alpha"));
}

beforeEach(() => {
  vi.clearAllMocks();
  isAdminRef.current = true;
  getLibraryMock.mockReturnValue({ data: { id: LIBRARY_ID, name: "My Library" } });
});

describe("LibraryDetailScreen bulk 'add to collection'", () => {
  it("shows the button for an admin alongside the rest of the bar", () => {
    renderScreen();
    enterBulkModeOnAlpha();

    expect(screen.getByLabelText("Add to collection")).toBeTruthy();
    expect(screen.getByLabelText("Mark watched")).toBeTruthy();
    expect(screen.getByLabelText("Mark unwatched")).toBeTruthy();
    expect(screen.getByLabelText("Favorite")).toBeTruthy();
    expect(screen.getByLabelText("Playlist")).toBeTruthy();
  });

  it("hides only the button for a non-admin, keeping the rest of the bar", () => {
    isAdminRef.current = false;
    renderScreen();
    enterBulkModeOnAlpha();

    expect(screen.queryByLabelText("Add to collection")).toBeNull();
    expect(screen.getByLabelText("Mark watched")).toBeTruthy();
    expect(screen.getByLabelText("Mark unwatched")).toBeTruthy();
    expect(screen.getByLabelText("Favorite")).toBeTruthy();
    expect(screen.getByLabelText("Playlist")).toBeTruthy();
  });
});
