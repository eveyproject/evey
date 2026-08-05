import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListEmptyComponent }: any) => {
    const header = typeof ListHeaderComponent === "function" ? React.createElement(ListHeaderComponent) : ListHeaderComponent;
    const empty = typeof ListEmptyComponent === "function" ? React.createElement(ListEmptyComponent) : ListEmptyComponent;
    if (!data || data.length === 0) {
      return React.createElement("div", null, header, empty ?? null);
    }
    return React.createElement(
      "div",
      null,
      header,
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

const localSearchParamsMock = vi.fn();
vi.mock("expo-router", () => ({
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

const listTracksMock = vi.fn();
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useMusicServiceListTracks: (...args: unknown[]) => listTracksMock(...args),
}));

import { useMusicQueueStore } from "../../../../src/stores/musicQueue";
import AlbumDetailScreen from "./[id]";

const ALBUM_TRACKS = [
  { id: "tr-1", title: "One More Time", artist: "Daft Punk", album: "Discovery", trackNumber: 1, duration: 320, coverArtUrl: "" },
  { id: "tr-2", title: "Aerodynamic", artist: "Daft Punk", album: "Discovery", trackNumber: 2, duration: 210, coverArtUrl: "" },
];

beforeEach(() => {
  vi.clearAllMocks();
  useMusicQueueStore.setState({ queue: [], currentIndex: -1, isPlaying: false });
  localSearchParamsMock.mockReturnValue({ id: "album-1", libraryId: "lib-music-1" });
  listTracksMock.mockReturnValue({ data: ALBUM_TRACKS, isLoading: false });
});

describe("AlbumDetailScreen (TV)", () => {
  it("queries the album's tracks scoped to the source library", () => {
    render(<AlbumDetailScreen />);
    expect(listTracksMock).toHaveBeenCalledWith(
      { libraryId: "lib-music-1", albumId: "album-1" },
      expect.objectContaining({ query: expect.objectContaining({ enabled: true }) }),
    );
  });

  it("renders every track of the album in order", () => {
    render(<AlbumDetailScreen />);
    expect(screen.getByLabelText("One More Time")).toBeTruthy();
    expect(screen.getByLabelText("Aerodynamic")).toBeTruthy();
  });

  it("loads the whole album into the queue, anchored at the pressed track", () => {
    render(<AlbumDetailScreen />);
    fireEvent.click(screen.getByLabelText("Aerodynamic"));

    const state = useMusicQueueStore.getState();
    expect(state.currentIndex).toBe(1);
    expect(state.isPlaying).toBe(true);
    expect(state.queue.map((t) => t.id)).toEqual(["tr-1", "tr-2"]);
  });

  it("shows an empty state when the album has no tracks", () => {
    listTracksMock.mockReturnValue({ data: [], isLoading: false });
    render(<AlbumDetailScreen />);
    expect(screen.getByText("media.music.noTracks")).toBeTruthy();
  });
});
