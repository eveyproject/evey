import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops boolean values passed through an unrecognized DOM prop, so passing
  // it through the ...p spread as-is would make it unobservable in jsdom.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ?? null;
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
  StyleSheet: { create: (s: any) => s },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- expo-router mock -------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
  useLocalSearchParams: () => ({ id: "col-1" }),
  router: { back: vi.fn() },
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
    radii: { sm: 4, md: 8 },
  }),
}));

// --- local collections api mock --------------------------------------------------
const collectionMock = vi.fn();
vi.mock("../../../src/api/collections", () => ({
  useCollection: () => collectionMock(),
}));

import CollectionDetailScreen from "./[id]";

const ITEMS = [
  { id: "m-1", title: "Iron Man", type: "MEDIA_ITEM_TYPE_MOVIE", year: 2008 },
  { id: "m-2", title: "Iron Man 2", type: "MEDIA_ITEM_TYPE_MOVIE", year: 2010 },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CollectionDetailScreen (TV) loading state", () => {
  it("shows a spinner and neither the error nor the empty message", () => {
    collectionMock.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    });
    render(<CollectionDetailScreen />);

    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("Something went wrong")).toBeNull();
    expect(screen.queryByText("common.noItems")).toBeNull();
  });
});

describe("CollectionDetailScreen (TV) error state", () => {
  it("tells a failed load apart from an empty collection, with a focusable retry", () => {
    const refetchMock = vi.fn();
    collectionMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      refetch: refetchMock,
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    });
    render(<CollectionDetailScreen />);

    expect(screen.getByText("Something went wrong")).toBeTruthy();
    expect(screen.queryByText("common.noItems")).toBeNull();
    const retryBtn = screen.getByText("Retry").closest("button");
    expect(retryBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
    fireEvent.click(retryBtn!);
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("CollectionDetailScreen (TV) empty state", () => {
  it("renders a distinct message from the error state when the collection legitimately has zero members", () => {
    collectionMock.mockReturnValue({
      data: { collection: { id: "col-1", name: "Empty Saga" }, items: [], total: 0 },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    });
    render(<CollectionDetailScreen />);

    expect(screen.getByText("common.noItems")).toBeTruthy();
    expect(screen.queryByText("Something went wrong")).toBeNull();
  });
});

describe("CollectionDetailScreen (TV) loaded state", () => {
  it("renders every member returned by the server", () => {
    collectionMock.mockReturnValue({
      data: { collection: { id: "col-1", name: "Iron Man Saga" }, items: ITEMS, total: 2 },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    });
    render(<CollectionDetailScreen />);

    expect(screen.getByText("Iron Man")).toBeTruthy();
    expect(screen.getByText("Iron Man 2")).toBeTruthy();
  });
});
