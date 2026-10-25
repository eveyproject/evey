import { describe, it, expect, vi, beforeEach } from "vitest";
vi.mock("../../../src/hooks/useMediaPrefetch", () => ({ useMediaPrefetch: () => ({ prefetchOnFocus: () => {}, cancelPrefetch: () => {} }) }));
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
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
  StyleSheet: { create: (s: any) => s, absoluteFill: {} },
}));

// --- expo-image mock ---------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- @expo/vector-icons mock ---------------------------------------------------
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- expo-router mock ---------------------------------------------------------
const { pushMock, GENRE } = vi.hoisted(() => ({ pushMock: vi.fn(), GENRE: "Sci-Fi" }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ genre: GENRE }),
  useRouter: () => ({ push: pushMock }),
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
      onAccent: "#fff",
      surface: "#111",
      surfaceRaised: "#222",
      border: "#333",
      warning: "#ff0",
    },
    radii: { sm: 4, md: 8 },
  }),
}));

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv.test" }));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

// --- @evey/api mock: importOriginal keeps IN_LIBRARY_SEARCH_LIMIT and every
// other pure export real; only the network hook is faked. --------------------
const { searchMediaMock } = vi.hoisted(() => ({ searchMediaMock: vi.fn() }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceSearchMedia: (...args: unknown[]) => searchMediaMock(...args),
}));

import GenreDetailScreen from "./[genre]";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GenreDetailScreen (TV) error state", () => {
  it("gives the remote a focusable retry action", () => {
    const refetchMock = vi.fn();
    searchMediaMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch: refetchMock });
    render(<GenreDetailScreen />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
    fireEvent.click(screen.getByText("common.retry"));
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("GenreDetailScreen (TV) empty state", () => {
  it("stays focusable when the genre has no titles anywhere on the server", () => {
    searchMediaMock.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<GenreDetailScreen />);
    expect(screen.getByText("media.library.noItemsFiltered")).toBeTruthy();
    const backBtn = screen.getByText("common.nav.genres").closest("button");
    expect(backBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });
});

describe("GenreDetailScreen (TV) grid", () => {
  const ITEMS = [
    { id: "mi-1", title: "Alpha", communityRating: 8.4 },
    { id: "mi-2", title: "Beta" },
  ];

  it("puts the initial remote focus on the first poster", () => {
    searchMediaMock.mockReturnValue({ data: { items: ITEMS }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<GenreDetailScreen />);
    const firstCard = screen.getByText("Alpha").closest("button");
    expect(firstCard?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });

  it("navigates to the media detail screen on press", () => {
    searchMediaMock.mockReturnValue({ data: { items: ITEMS }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<GenreDetailScreen />);
    fireEvent.click(screen.getByText("Beta"));
    expect(pushMock).toHaveBeenCalledWith("/media/mi-2");
  });

  it("queries SearchMedia globally by genre only, with no library_id", () => {
    searchMediaMock.mockReturnValue({ data: { items: ITEMS }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<GenreDetailScreen />);
    const params = searchMediaMock.mock.calls[0][0];
    expect(params.genre).toBe(GENRE);
    expect(params.libraryId).toBeUndefined();
  });

  it("shows the translated genre as the header, falling back to the raw genre text", () => {
    searchMediaMock.mockReturnValue({ data: { items: ITEMS }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<GenreDetailScreen />);
    expect(screen.getByText(GENRE)).toBeTruthy();
  });
});
