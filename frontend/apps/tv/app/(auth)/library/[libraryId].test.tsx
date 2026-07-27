import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Genre filter parity with mobile (a param on the list request) and shuffle
// parity with web: this screen has no local-only facet (the genre filter is
// server-side), so a shuffle always draws from the WHOLE library via a
// random page offset (pageSize 1) rather than the loaded slice, with the
// genre filter, if any, baked into the probe request.

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  TextInput: ({ onChangeText, value, accessibilityLabel, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      value,
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
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
const { pushMock, LIBRARY_ID } = vi.hoisted(() => ({ pushMock: vi.fn(), LIBRARY_ID: "lib-1" }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ libraryId: LIBRARY_ID }),
  useRouter: () => ({ push: pushMock }),
  Stack: { Screen: () => null },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
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

// --- @evey/store mock -----------------------------------------------------------------
const { toastErrorMock, toastInfoMock } = vi.hoisted(() => ({
  toastErrorMock: vi.fn(),
  toastInfoMock: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: toastErrorMock, info: toastInfoMock },
}));

// --- @evey/api mock --------------------------------------------------------------------
const { listMediaItemsMock, listGenresMock, rawListMediaItemsMock, searchMediaMock, watchHistoryMock } = vi.hoisted(() => ({
  listMediaItemsMock: vi.fn(),
  listGenresMock: vi.fn(),
  rawListMediaItemsMock: vi.fn(),
  searchMediaMock: vi.fn(),
  watchHistoryMock: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  useLibraryServiceGetLibrary: () => ({ data: { id: LIBRARY_ID, name: "My Library" } }),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
  useLibraryServiceListGenres: (...args: unknown[]) => listGenresMock(...args),
  useLibraryServiceSearchMedia: (...args: unknown[]) => searchMediaMock(...args),
  usePlaybackServiceGetWatchHistory: (...args: unknown[]) => watchHistoryMock(...args),
  libraryServiceListMediaItems: (...args: unknown[]) => rawListMediaItemsMock(...args),
}));

// -------------------------------------------------------------------------------------
import LibraryDetailScreen from "./[libraryId]";

const ITEMS = [
  { id: "mi-1", title: "Alpha", genres: ["Action"] },
  { id: "mi-2", title: "Beta", genres: ["Drama"] },
];

beforeEach(() => {
  vi.clearAllMocks();
  listMediaItemsMock.mockImplementation(() => ({
    data: { items: ITEMS, pagination: { totalItems: ITEMS.length, totalPages: 1 } },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }));
  listGenresMock.mockReturnValue({ data: { genres: ["Action", "Drama"] } });
  searchMediaMock.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() });
  watchHistoryMock.mockReturnValue({ data: { items: [] } });
});

describe("TV LibraryDetailScreen genre filter", () => {
  it("sends the selected genre as a param on the media list request", () => {
    render(<LibraryDetailScreen />);

    fireEvent.click(screen.getByLabelText("Action"));

    expect(listMediaItemsMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ libraryId: LIBRARY_ID, genre: "Action" }),
      expect.anything(),
    );
  });

  it("omits the genre param again once 'All' is re-selected", () => {
    render(<LibraryDetailScreen />);

    fireEvent.click(screen.getByLabelText("Action"));
    fireEvent.click(screen.getByLabelText("media.search.allGenres"));

    const lastCallParams = listMediaItemsMock.mock.calls.at(-1)?.[0];
    expect(lastCallParams).not.toHaveProperty("genre");
  });
});

describe("TV LibraryDetailScreen shuffle", () => {
  it("shuffles across the whole library via a random page offset and navigates to the player", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0); // page = floor(0 * total) + 1 = 1
    rawListMediaItemsMock.mockResolvedValue({ items: [{ id: "mi-9" }], pagination: { totalItems: 7 } });

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("common.shufflePlay"));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/player/mi-9"));
    expect(rawListMediaItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ libraryId: LIBRARY_ID, "pagination.page": 1, "pagination.pageSize": 1 }),
    );
  });

  it("bakes the active genre filter into the shuffle probe", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    rawListMediaItemsMock.mockResolvedValue({ items: [{ id: "mi-9" }], pagination: { totalItems: 1 } });

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("Action"));
    fireEvent.click(screen.getByLabelText("common.shufflePlay"));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/player/mi-9"));
    expect(rawListMediaItemsMock).toHaveBeenCalledWith(
      expect.objectContaining({ genre: "Action" }),
    );
  });

  it("toasts a failure when the probe rejects", async () => {
    rawListMediaItemsMock.mockRejectedValue(new Error("network down"));

    render(<LibraryDetailScreen />);
    fireEvent.click(screen.getByLabelText("common.shufflePlay"));

    await waitFor(() => expect(toastErrorMock).toHaveBeenCalledWith("common.shuffleFailed"));
    expect(pushMock).not.toHaveBeenCalled();
  });
});
