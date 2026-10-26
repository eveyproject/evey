import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  useWindowDimensions: () => ({ width: 1200, height: 675, scale: 1.6, fontScale: 1 }),
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    const empty = typeof ListEmptyComponent === "function" ? React.createElement(ListEmptyComponent) : ListEmptyComponent;
    if (!data || data.length === 0) {
      return React.createElement("div", null, empty ?? null);
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

vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

const pushMock = vi.fn();
const localSearchParamsMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args) },
  useLocalSearchParams: () => localSearchParamsMock(),
  Stack: { Screen: () => null },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333" },
    radii: { sm: 4, md: 8 },
  }),
}));

vi.mock("../../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));
vi.mock("../../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

const getAlbumsMock = vi.fn();
vi.mock("@evey/api", () => ({
  useMusicServiceGetAlbums: (...args: unknown[]) => getAlbumsMock(...args),
}));

import ArtistDetailScreen from "./[id]";

// The library's whole album catalog - GetAlbums has no artist filter, so the
// screen must keep only the ones matching this artist itself.
const ALL_ALBUMS = [
  { id: "album-1", title: "Voyage", artist: "Chrome Duo", year: 2001, coverArtUrl: "" },
  { id: "album-2", title: "Random Access Memories", artist: "Chrome Duo", year: 2013, coverArtUrl: "" },
  { id: "album-3", title: "Currents", artist: "Calm Lagoon", year: 2015, coverArtUrl: "" },
];

beforeEach(() => {
  vi.clearAllMocks();
  localSearchParamsMock.mockReturnValue({ id: "Chrome Duo", libraryId: "lib-music-1" });
  getAlbumsMock.mockImplementation((_params, options) => {
    const select = options?.query?.select;
    return { data: select ? select({ albums: ALL_ALBUMS }) : ALL_ALBUMS, isLoading: false };
  });
});

describe("ArtistDetailScreen (TV)", () => {
  it("only lists albums that belong to this artist", () => {
    render(<ArtistDetailScreen />);
    expect(screen.getByLabelText("Voyage")).toBeTruthy();
    expect(screen.getByLabelText("Random Access Memories")).toBeTruthy();
    expect(screen.queryByLabelText("Currents")).toBeNull();
  });

  it("drills further down into an album on press, carrying the library id", () => {
    render(<ArtistDetailScreen />);
    fireEvent.click(screen.getByLabelText("Voyage"));
    expect(pushMock).toHaveBeenCalledWith(
      expect.stringContaining("/(auth)/music/album/album-1"),
    );
    expect(pushMock).toHaveBeenCalledWith(expect.stringContaining("libraryId=lib-music-1"));
  });

  it("shows an empty state when the artist has no albums in this library", () => {
    getAlbumsMock.mockReturnValue({ data: [], isLoading: false });
    render(<ArtistDetailScreen />);
    expect(screen.getByText("media.music.noAlbums")).toBeTruthy();
  });
});
