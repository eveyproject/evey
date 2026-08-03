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
  // it through the ...p spread as-is would make it unobservable in jsdom -
  // a real gap this suite needs to assert on.
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
    // ListEmptyComponent is passed here as an already-built element (not a
    // component reference), matching how the screen under test uses it.
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
// ScreenState's "Back" button imports router.back from expo-router directly -
// the real package's build output isn't transformable by this test's JSX pipeline.
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
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
const collectionsMock = vi.fn();
vi.mock("../../../src/api/collections", () => ({
  useCollections: () => collectionsMock(),
}));

import CollectionsScreen, { collectionSubtitle } from "./index";

const COLLECTIONS = [
  { id: "col-1", name: "Marvel" },
  { id: "col-2", name: "Star Wars" },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CollectionsScreen (TV) error state", () => {
  it("tells a failed load apart from an empty library, with a focusable retry", () => {
    const refetchMock = vi.fn();
    collectionsMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch: refetchMock });
    render(<CollectionsScreen />);

    expect(screen.getByText("Something went wrong")).toBeTruthy();
    const retryBtn = screen.getByText("Retry").closest("button");
    expect(retryBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
    fireEvent.click(retryBtn!);
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("CollectionsScreen (TV) loaded state", () => {
  it("renders every collection returned by the server", () => {
    collectionsMock.mockReturnValue({
      data: { collections: COLLECTIONS },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<CollectionsScreen />);

    expect(screen.getByText("Marvel")).toBeTruthy();
    expect(screen.getByText("Star Wars")).toBeTruthy();
  });
});

describe("CollectionsScreen (TV) empty state", () => {
  it("renders a distinct message from the error state when there are legitimately zero collections", () => {
    collectionsMock.mockReturnValue({
      data: { collections: [] },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<CollectionsScreen />);

    expect(screen.getByText("common.noCollections")).toBeTruthy();
    expect(screen.queryByText("Something went wrong")).toBeNull();
  });
});

describe("collectionSubtitle (TV)", () => {
  const t = (key: string, options?: Record<string, unknown>) =>
    options?.count !== undefined ? `${options.count} titles` : key;

  it("returns no parts for a manual collection with no known count", () => {
    expect(collectionSubtitle(undefined, undefined, t)).toEqual({});
  });

  it("returns a badge for a saga collection", () => {
    expect(collectionSubtitle("saga", undefined, t)).toEqual({ badge: "media.collections.saga" });
  });

  it("returns a count when itemCount is positive, regardless of kind", () => {
    expect(collectionSubtitle("manual", 7, t)).toEqual({ count: "7 titles" });
  });

  it("omits the count when itemCount is zero", () => {
    expect(collectionSubtitle(undefined, 0, t)).toEqual({});
  });

  it("combines badge and count for a saga with visible titles", () => {
    expect(collectionSubtitle("saga", 3, t)).toEqual({ badge: "media.collections.saga", count: "3 titles" });
  });
});
