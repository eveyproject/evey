import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Shuffle play parity with web: with no local (client-only) filter active,
// draw a random item from the WHOLE library via a random page offset
// (pageSize 1) rather than just the loaded slice, then jump straight to the
// player. With a text/year/rating filter active - since those have no
// server equivalent here - shuffle within what's actually displayed instead.

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  TouchableOpacity: ({ children, onPress, disabled, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: disabled ? undefined : onPress, disabled, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  Pressable: ({ children, onPress, ...p }: any) => React.createElement("button", { onClick: onPress, ...p }, children),
  TextInput: ({ value, onChangeText, placeholder, ...p }: any) =>
    React.createElement("input", {
      value,
      placeholder,
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(React.Fragment, null, ListEmptyComponent) : null;
    }
    return React.createElement(
      "div",
      null,
      data.map((item: any, i: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, i) : i },
          renderItem({ item, index: i }),
        ),
      ),
    );
  },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Dimensions: {
    get: () => ({ width: 400, height: 800 }),
    addEventListener: () => ({ remove: vi.fn() }),
  },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

// --- expo-router ---------------------------------------------------------------------
const { pushMock, LIBRARY_ID } = vi.hoisted(() => ({ pushMock: vi.fn(), LIBRARY_ID: "lib-1" }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ libraryId: LIBRARY_ID }),
  useRouter: () => ({ push: pushMock }),
  Stack: { Screen: () => null },
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
      warning: "#fa0",
      success: "#0f0",
    },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

// --- misc hooks/components the screen depends on, stubbed to keep this a
// --- focused render test of the shuffle behavior --------------------------------------
vi.mock("../hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => undefined,
}));
// useServerUrl pulls in server-config -> expo-secure-store, which has no native
// module under jsdom; mock it like the other screen tests do.
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useDebounce", () => ({
  useDebounce: (v: unknown) => v,
}));
vi.mock("../hooks/useFavorite", () => ({
  useBulkFavorite: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("../components/AddToPlaylistSheet", () => ({ AddToPlaylistSheet: () => null }));
vi.mock("../components/Skeleton", () => ({ Skeleton: () => null }));
// The grid now renders the shared MediaCard (pulls in image/watch hooks); the
// shuffle tests only exercise the shuffle button, so stub it out.
vi.mock("../components/MediaCard", () => ({ MediaCard: () => null }));

// --- @evey/store -----------------------------------------------------------------------
const { toastErrorMock, toastInfoMock } = vi.hoisted(() => ({
  toastErrorMock: vi.fn(),
  toastInfoMock: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ profileId: "profile-1" }),
  toast: { success: vi.fn(), error: toastErrorMock, info: toastInfoMock },
}));

// --- @evey/api -------------------------------------------------------------------------
const ITEMS = [
  { id: "mi-1", title: "Alpha", genres: ["Action"], year: 2001 },
  { id: "mi-2", title: "Beta", genres: ["Drama"], year: 2002 },
  { id: "mi-3", title: "Gamma", genres: ["Action"], year: 2003 },
];

const { listMediaItemsMock, rawListMediaItemsMock, searchMediaMock } = vi.hoisted(() => ({
  listMediaItemsMock: vi.fn(),
  rawListMediaItemsMock: vi.fn(),
  searchMediaMock: vi.fn(),
}));

vi.mock("@evey/api", () => ({
  useLibraryServiceGetLibrary: () => ({ data: { id: LIBRARY_ID, name: "My Library" } }),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
  libraryServiceListMediaItems: (...args: unknown[]) => rawListMediaItemsMock(...args),
  useLibraryServiceSearchMedia: (...args: unknown[]) => searchMediaMock(...args),
  usePlaybackServiceGetWatchHistory: () => ({ data: undefined }),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}));

// -------------------------------------------------------------------------------------
import LibraryDetailScreen from "../../app/(auth)/library/[libraryId]";

beforeEach(() => {
  vi.clearAllMocks();
  // A fresh object per call (rather than mockReturnValue's single shared
  // reference) mirrors react-query returning a new result object per fetch,
  // so the screen's data-changed effect (keyed on referential identity)
  // re-syncs allItems on every render instead of only the first.
  listMediaItemsMock.mockImplementation(() => ({
    data: { items: ITEMS, pagination: { totalItems: ITEMS.length, totalPages: 1 } },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }));
  // Server search is dormant by default (no text filter); tests that exercise
  // the filter path override this to return matching items.
  searchMediaMock.mockImplementation(() => ({
    data: undefined,
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }));
});

describe("LibraryDetailScreen shuffle", () => {
  it("shuffles across the whole library via a random page offset and navigates to the player", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0); // page = floor(0 * total) + 1 = 1
    rawListMediaItemsMock.mockResolvedValue({
      items: [{ id: "mi-2" }],
      pagination: { totalItems: 5 },
    });

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByTestId("library-shuffle-play"));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/(auth)/player/mi-2"));
    expect(rawListMediaItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ libraryId: LIBRARY_ID, "pagination.page": 1, "pagination.pageSize": 1 }),
    );
  });

  it("shuffles within the displayed items when a text filter is active, without hitting the whole-library probe", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    // With a text filter active, the screen now hits the server search
    // (useLibraryServiceSearchMedia) and shuffles within those results - never
    // the whole-library probe.
    searchMediaMock.mockImplementation(() => ({
      data: { items: [{ id: "mi-2", title: "Beta" }], totalHits: 1 },
      isLoading: false,
      isError: false,
      isFetching: false,
      refetch: vi.fn(),
    }));
    render(<LibraryDetailScreen />);

    fireEvent.change(screen.getByPlaceholderText("media.library.filterByName"), {
      target: { value: "Beta" },
    });

    fireEvent.click(screen.getByTestId("library-shuffle-play"));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/(auth)/player/mi-2"));
    expect(rawListMediaItemsMock).not.toHaveBeenCalled();
  });

  it("toasts a failure when the whole-library probe rejects", async () => {
    rawListMediaItemsMock.mockRejectedValue(new Error("network down"));

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByTestId("library-shuffle-play"));

    await waitFor(() => expect(toastErrorMock).toHaveBeenCalledWith("common.shuffleFailed"));
    expect(pushMock).not.toHaveBeenCalled();
  });
});
