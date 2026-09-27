import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The favourites grid: loading and failure go through the shared screen
// state (with a retry), the first card takes the remote's initial focus, a
// card opens its title, an untitled one still has an accessible name, and
// an empty list says so instead of showing a blank wall.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "grid" },
      (data ?? []).length === 0
        ? ListEmptyComponent
        : (data as any[]).map((item, index) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item, index }))),
    ),
  StyleSheet: { create: (s: any) => s },
}));
const push = vi.fn();
vi.mock("expo-router", () => ({ useRouter: () => ({ push }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888" }, radii: { sm: 4 } }),
}));
vi.mock("../../../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, hasTVPreferredFocus, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-focus": String(hasTVPreferredFocus) }, children),
}));
vi.mock("../../../src/components/ScreenState", () => ({
  ScreenState: ({ loading, error, onRetry }: any) =>
    React.createElement("div", { "data-testid": "state", "data-loading": String(loading), "data-error": String(error) },
      React.createElement("button", { onClick: onRetry }, "retry")),
}));
const favs = { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() };
vi.mock("../../../src/hooks/useFavorite", () => ({ useFavoritesList: () => favs }));

import FavoritesScreen from "./favorites";

beforeEach(() => {
  vi.clearAllMocks();
  favs.data = undefined;
  favs.isLoading = false;
  favs.isError = false;
});

describe("tv favourites screen", () => {
  it("shows the grid with the first card focused, opens a title, names an untitled one", () => {
    favs.data = [{ mediaItemId: "m1", title: "Some Movie", year: 1995 }, { mediaItemId: "m2" }];
    render(<FavoritesScreen />);
    const cards = screen.getAllByRole("button");
    expect(cards).toHaveLength(2);
    expect(cards[0].getAttribute("data-focus")).toBe("true");
    expect(cards[1].getAttribute("data-focus")).toBe("false");
    expect(cards[0].getAttribute("aria-label")).toBe("Some Movie");
    expect(cards[1].getAttribute("aria-label")).toBe("Untitled");
    expect(screen.getByText("1995")).toBeTruthy();
    fireEvent.click(cards[1]);
    expect(push).toHaveBeenCalledWith("/media/m2");
  });

  // The empty state used to be a sentence and nothing else: the remote had
  // nowhere to go from here and the viewer had to guess the rail.
  it("says when there is nothing yet, and offers the way to the libraries", () => {
    favs.data = [];
    render(<FavoritesScreen />);
    expect(screen.getByText("media.favorites.empty")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("media.library.libraries"));
    expect(push).toHaveBeenCalledWith("/(auth)/(tabs)/library");
  });

  it("routes loading and failure through the screen state with a retry", () => {
    favs.isLoading = true;
    const { unmount } = render(<FavoritesScreen />);
    expect(screen.getByTestId("state").getAttribute("data-loading")).toBe("true");
    unmount();

    favs.isLoading = false;
    favs.isError = true;
    render(<FavoritesScreen />);
    expect(screen.getByTestId("state").getAttribute("data-error")).toBe("true");
    fireEvent.click(screen.getByText("retry"));
    expect(favs.refetch).toHaveBeenCalledTimes(1);
  });
});
