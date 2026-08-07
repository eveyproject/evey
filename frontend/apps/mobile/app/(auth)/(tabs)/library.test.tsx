import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A failed library fetch used to state "loading failed" and stop there: this
// screen carried its own inline copy of the failure block, and that copy had
// no retry at all. The whole library list was then unreachable until the app
// was killed and relaunched.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "library-list" },
      (data ?? []).length === 0
        ? ListEmptyComponent ?? null
        : data.map((item: any) =>
            React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
          ),
    ),
  TouchableOpacity: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("expo-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222" },
    radii: { sm: 4, md: 8 },
    spacing: { md: 8, xl: 16 },
  }),
}));

let librariesState: { data?: { libraries: unknown[] }; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: () => librariesState,
  useLibraryServiceListMediaItems: () => ({ data: undefined }),
  hasMediaItemCount: () => false,
}));

import LibraryScreen from "./library";

beforeEach(() => {
  vi.clearAllMocks();
  librariesState = { data: { libraries: [] }, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("LibraryScreen - a failed fetch must be recoverable", () => {
  it("offers a retry that re-runs the request when the fetch fails", () => {
    const refetch = vi.fn();
    librariesState = { data: undefined, isLoading: false, isError: true, refetch };

    render(<LibraryScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    // The point of the test: the message alone is a dead end.
    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("shows the spinner while loading, neither the failure nor the list", () => {
    librariesState = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };

    render(<LibraryScreen />);

    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });

  it("renders the libraries once they arrive", () => {
    librariesState = {
      data: { libraries: [{ id: "lib-1", name: "Some Library", type: "LIBRARY_TYPE_MOVIES" }] },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    };

    render(<LibraryScreen />);

    expect(screen.getByText("Some Library")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });
});
