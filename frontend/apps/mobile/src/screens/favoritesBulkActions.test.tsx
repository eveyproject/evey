import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// Parity gap: web's favorites BulkActionBar offers "add to playlist" and "add
// to collection" on top of watched/unwatched - mobile's favorites screen only
// had watched/unwatched. These tests prove the bulk bar now exposes both new
// actions and wires them to the shared BulkAddToPlaylistSheet/
// BulkAddToCollectionSheet (extracted from library/[libraryId].tsx).

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  TouchableOpacity: ({ children, onPress, onLongPress, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, onDoubleClick: onLongPress, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
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
  Dimensions: { get: () => ({ width: 400, height: 800 }), addEventListener: () => ({ remove: vi.fn() }) },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
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
const { pushMock } = vi.hoisted(() => ({ pushMock: vi.fn() }));
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

// --- ThemeProvider -------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      surfaceRaised: "#222",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      border: "#333",
      error: "#e00",
      overlay: "#00000080",
    },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useDebounce", () => ({ useDebounce: (v: unknown) => v }));
vi.mock("../components/Skeleton", () => ({ Skeleton: () => null }));

const invalidateQueries = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", async (orig) => ({
  ...(await orig<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries }),
}));

const { markWatchedMock, markUnwatchedMock } = vi.hoisted(() => ({
  markWatchedMock: vi.fn(),
  markUnwatchedMock: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: () => ({ mutateAsync: markWatchedMock, isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutateAsync: markUnwatchedMock, isPending: false }),
}));

// Shape copied from packages/store/src/auth.ts: `isAdmin` is a plain boolean
// field on the store state, read through the selector like the screen does.
const { isAdminRef } = vi.hoisted(() => ({ isAdminRef: { current: true } }));
vi.mock("@evey/store", () => ({ useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }), isDemoMode: () => false,
  useAuthStore: (selector: (s: any) => unknown) => selector({ profileId: "profile-1", isAdmin: isAdminRef.current }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

const FAVORITES = [
  { mediaItemId: "mi-1", title: "Alpha", createdAt: "2026-01-01", year: 2001 },
  { mediaItemId: "mi-2", title: "Beta", createdAt: "2026-01-02", year: 2002 },
];
vi.mock("../hooks/useFavorite", () => ({
  useFavoritesList: () => ({ data: FAVORITES, isLoading: false, isError: false, isRefetching: false, refetch: vi.fn() }),
}));

// The two bulk sheets are extracted into a shared module (BulkAddSheets.tsx)
// and covered by their own behavior elsewhere (library screen); here we only
// assert the favorites bulk bar renders them with the full selection once
// tapped, not their internals (playlist/collection fetch + Promise.all add).
const { playlistSheetProps, collectionSheetProps } = vi.hoisted(() => ({
  playlistSheetProps: vi.fn(),
  collectionSheetProps: vi.fn(),
}));
vi.mock("../components/library/BulkAddSheets", () => ({
  BulkAddToPlaylistSheet: (props: any) => {
    playlistSheetProps(props);
    return props.visible ? React.createElement("div", { "data-testid": "bulk-playlist-sheet" }) : null;
  },
  BulkAddToCollectionSheet: (props: any) => {
    collectionSheetProps(props);
    return props.visible ? React.createElement("div", { "data-testid": "bulk-collection-sheet" }) : null;
  },
}));

import FavoritesScreen from "../../app/(auth)/(tabs)/favorites";

function selectFirstItem() {
  // Long press (mapped to onDoubleClick by the RN mock above) enters bulk
  // mode and selects the item under it, mirroring the grid's real behavior.
  fireEvent.doubleClick(screen.getByLabelText("Alpha"));
}

beforeEach(() => {
  vi.clearAllMocks();
  isAdminRef.current = true;
});

describe("FavoritesScreen bulk actions", () => {
  it("offers add-to-playlist and add-to-collection in the bulk action bar", () => {
    render(<FavoritesScreen />);
    selectFirstItem();

    expect(screen.getByLabelText("Playlist")).toBeTruthy();
    expect(screen.getByLabelText("Add to collection")).toBeTruthy();
  });

  it("opens the bulk playlist sheet with the full selection", () => {
    render(<FavoritesScreen />);
    selectFirstItem();

    fireEvent.click(screen.getByLabelText("Playlist"));

    expect(screen.getByTestId("bulk-playlist-sheet")).toBeTruthy();
    expect(playlistSheetProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: true, mediaItemIds: ["mi-1"] }),
    );
  });

  it("opens the bulk collection sheet with the full selection", () => {
    render(<FavoritesScreen />);
    selectFirstItem();

    fireEvent.click(screen.getByLabelText("Add to collection"));

    expect(screen.getByTestId("bulk-collection-sheet")).toBeTruthy();
    expect(collectionSheetProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: true, mediaItemIds: ["mi-1"] }),
    );
  });
});

// AddCollectionItem RPC requires RoleAdmin server-side (interceptors/rbac.go).
describe("FavoritesScreen bulk 'add to collection' admin gating", () => {
  it("hides only the 'add to collection' button for a non-admin, keeping the rest of the bar", () => {
    isAdminRef.current = false;
    render(<FavoritesScreen />);
    selectFirstItem();

    expect(screen.queryByLabelText("Add to collection")).toBeNull();
    expect(screen.getByLabelText("Mark watched")).toBeTruthy();
    expect(screen.getByLabelText("Mark unwatched")).toBeTruthy();
    expect(screen.getByLabelText("Playlist")).toBeTruthy();
  });
});

// The audit repro (2026-08-25): select items, tap "Watched" (a sequential
// per-item loop with no visual feedback), then tap "Unwatched" two seconds
// later - the two loops interleave request by request and the selection ends
// half watched / half unwatched, a state neither button asked for.
describe("FavoritesScreen bulk actions run one at a time", () => {
  it("ignores a second bulk action while the first one's loop is still running", async () => {
    let releaseFirst: () => void = () => {};
    markWatchedMock.mockImplementation(
      () => new Promise<void>((resolve) => { releaseFirst = () => resolve(); }),
    );
    render(<FavoritesScreen />);
    selectFirstItem();

    fireEvent.click(screen.getByLabelText("Mark watched"));
    // The first loop is parked on its in-flight request; a competing action
    // must be refused, not queued and not interleaved.
    fireEvent.click(screen.getByLabelText("Mark unwatched"));
    expect(markUnwatchedMock).not.toHaveBeenCalled();

    releaseFirst();
    await screen.findByLabelText("Alpha");
    expect(markUnwatchedMock).not.toHaveBeenCalled();
    expect(markWatchedMock).toHaveBeenCalledTimes(1);
    // The batch refreshes the watched badges and the home rows once it ends.
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["/api/v1/playback/history"] });
  });
});
