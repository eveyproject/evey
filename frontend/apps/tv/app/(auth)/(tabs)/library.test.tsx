import { describe, it, expect, vi, beforeEach } from "vitest";
vi.mock("../../../src/hooks/useMediaPrefetch", () => ({ useMediaPrefetch: () => ({ prefetchQueriesOnFocus: () => {}, cancelPrefetch: () => {} }) }));
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops boolean values passed through an unrecognized DOM prop, so passing
  // it through the ...p spread as-is (as other test files do) makes it
  // unobservable in jsdom - a real gap this suite needs to assert on.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor }: any) => {
    if (!data || data.length === 0) return null;
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

// --- expo-router mock --------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

// --- react-i18next mock -------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

// --- ThemeProvider mock --------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333" },
    radii: { sm: 4, md: 8 },
  }),
}));

// --- @evey/api mock --------------------------------------------------------------
const listLibrariesMock = vi.fn();
vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: (...args: unknown[]) => listLibrariesMock(...args),
}));

import LibraryScreen from "./library";

const LIBRARIES = [
  { id: "lib-1", name: "Movies", type: "LIBRARY_TYPE_MOVIES" },
  { id: "lib-2", name: "Shows", type: "LIBRARY_TYPE_SHOWS" },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("LibraryScreen (TV) error state", () => {
  it("gives the remote a focusable retry action instead of leaving it with nowhere to go", () => {
    const refetchMock = vi.fn();
    listLibrariesMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch: refetchMock });
    render(<LibraryScreen />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
    fireEvent.click(screen.getByText("common.retry"));
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("LibraryScreen (TV) empty state", () => {
  it("gives the remote a focusable target when there are zero libraries yet", () => {
    // The FlatList renders nothing and never fires its own hasTVPreferredFocus
    // (that only triggers on its first item) - without a fallback, this
    // screen used to strand the remote with no initial focus target.
    listLibrariesMock.mockReturnValue({ data: { libraries: [] }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<LibraryScreen />);
    const collectionsBtn = screen.getByText("common.collections").closest("button");
    expect(collectionsBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });
});

describe("LibraryScreen (TV) library type labels", () => {
  it("translates library types via the shared media.library.* keys instead of hardcoded English", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: LIBRARIES }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<LibraryScreen />);
    expect(screen.getByText("media.library.movies")).toBeTruthy();
    expect(screen.getByText("media.library.tvShows")).toBeTruthy();
  });
});
