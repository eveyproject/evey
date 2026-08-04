import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
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
  useLocalSearchParams: () => ({ id: "pl-1" }),
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

// --- local playlists api mock --------------------------------------------------
const playlistDetailMock = vi.fn();
vi.mock("../../../src/api/playlists", () => ({
  usePlaylistDetail: () => playlistDetailMock(),
}));

import PlaylistDetailScreen from "./[id]";

const PLAYLIST = {
  id: "pl-1",
  name: "Road Trip",
  description: "Long drives",
  items: [
    { media_item_id: "t-1", title: "Song A", sort_order: 0 },
    { media_item_id: "t-2", title: "Song B", sort_order: 1 },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("PlaylistDetailScreen (TV) loading state", () => {
  it("shows a spinner and neither the error nor the not-found message", () => {
    playlistDetailMock.mockReturnValue({ data: undefined, isLoading: true, isError: false, refetch: vi.fn() });
    render(<PlaylistDetailScreen />);

    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("Something went wrong")).toBeNull();
    expect(screen.queryByText("media.playlists.notFound")).toBeNull();
  });
});

// The bug this screen had: a fetch that FAILED (isError, data undefined) took
// the exact same branch as a playlist that genuinely does not exist
// (isError false, data undefined), so a dead network read as "not found" -
// permanent-sounding wording for a transient, retryable failure.
describe("PlaylistDetailScreen (TV) error state", () => {
  it("tells a failed load apart from a missing playlist, with a focusable retry", () => {
    const refetchMock = vi.fn();
    playlistDetailMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch: refetchMock });
    render(<PlaylistDetailScreen />);

    expect(screen.getByText("Something went wrong")).toBeTruthy();
    expect(screen.queryByText("media.playlists.notFound")).toBeNull();
    const retryBtn = screen.getByText("Retry").closest("button");
    expect(retryBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
    fireEvent.click(retryBtn!);
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("PlaylistDetailScreen (TV) not-found state", () => {
  it("renders a distinct message from the error state when the request succeeded with nothing", () => {
    playlistDetailMock.mockReturnValue({ data: undefined, isLoading: false, isError: false, refetch: vi.fn() });
    render(<PlaylistDetailScreen />);

    expect(screen.getByText("media.playlists.notFound")).toBeTruthy();
    expect(screen.queryByText("Something went wrong")).toBeNull();
  });
});

describe("PlaylistDetailScreen (TV) loaded state", () => {
  it("renders every item returned by the server", () => {
    playlistDetailMock.mockReturnValue({ data: PLAYLIST, isLoading: false, isError: false, refetch: vi.fn() });
    render(<PlaylistDetailScreen />);

    expect(screen.getByText("Song A")).toBeTruthy();
    expect(screen.getByText("Song B")).toBeTruthy();
  });
});
