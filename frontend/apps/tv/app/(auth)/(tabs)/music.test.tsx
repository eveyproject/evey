import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(ListEmptyComponent) : null;
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

// --- expo-image mock ---------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args) },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333" },
    radii: { sm: 4, md: 8 },
  }),
}));

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

// --- @evey/api mock --------------------------------------------------------------
const listLibrariesMock = vi.fn();
const getArtistsMock = vi.fn();
const getAlbumsMock = vi.fn();
const listTracksMock = vi.fn();
vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: (...args: unknown[]) => listLibrariesMock(...args),
  useMusicServiceGetArtists: (...args: unknown[]) => getArtistsMock(...args),
  useMusicServiceGetAlbums: (...args: unknown[]) => getAlbumsMock(...args),
  useMusicServiceListTracks: (...args: unknown[]) => listTracksMock(...args),
}));

import { useMusicQueueStore } from "../../../src/stores/musicQueue";
import MusicScreen, { selectMusicFocusSection } from "./music";

const LIBRARIES = [{ id: "lib-music-1", name: "My Music", type: "LIBRARY_TYPE_MUSIC" }];
const ARTISTS = [
  { id: "Daft Punk", name: "Daft Punk", trackCount: 12, albumCount: 2, coverArtUrl: "" },
];
const ALBUMS = [
  { id: "album-1", title: "Discovery", artist: "Daft Punk", year: 2001, trackCount: 14, coverArtUrl: "" },
];
const TRACKS = [
  { id: "tr-1", title: "One More Time", artist: "Daft Punk", album: "Discovery", duration: 320 },
  { id: "tr-2", title: "Aerodynamic", artist: "Daft Punk", album: "Discovery", duration: 210 },
];

beforeEach(() => {
  vi.clearAllMocks();
  useMusicQueueStore.setState({ queue: [], currentIndex: -1, isPlaying: false });
  listLibrariesMock.mockReturnValue({ data: LIBRARIES, isLoading: false });
  getArtistsMock.mockReturnValue({ data: ARTISTS, isLoading: false, isError: false, refetch: vi.fn() });
  getAlbumsMock.mockReturnValue({ data: ALBUMS, isLoading: false, isError: false, refetch: vi.fn() });
  listTracksMock.mockReturnValue({ data: TRACKS, isLoading: false, isError: false, refetch: vi.fn() });
});

describe("selectMusicFocusSection (initial TV remote focus)", () => {
  it("picks the first section that actually has items", () => {
    expect(selectMusicFocusSection([true, true, true])).toBe(0);
    expect(selectMusicFocusSection([false, true, true])).toBe(1);
    expect(selectMusicFocusSection([false, false, true])).toBe(2);
  });

  it("returns null when every section is empty", () => {
    expect(selectMusicFocusSection([false, false, false])).toBeNull();
  });
});

describe("MusicScreen (TV)", () => {
  it("shows a spinner while libraries are loading", () => {
    listLibrariesMock.mockReturnValue({ data: undefined, isLoading: true });
    render(<MusicScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
  });

  it("shows an empty state when there are no music libraries", () => {
    listLibrariesMock.mockReturnValue({ data: [], isLoading: false });
    render(<MusicScreen />);
    expect(screen.getByText("media.music.noLibraries")).toBeTruthy();
  });

  it("renders the artists, albums and tracks rows with focusable cards", () => {
    render(<MusicScreen />);
    expect(screen.getByLabelText("Daft Punk")).toBeTruthy();
    expect(screen.getByLabelText("Discovery")).toBeTruthy();
    expect(screen.getByLabelText("One More Time")).toBeTruthy();
    expect(screen.getByLabelText("Aerodynamic")).toBeTruthy();
  });

  it("drills down into an artist on press", () => {
    render(<MusicScreen />);
    fireEvent.click(screen.getByLabelText("Daft Punk"));
    expect(pushMock).toHaveBeenCalledWith(
      expect.stringContaining("/(auth)/music/artist/Daft%20Punk"),
    );
    expect(pushMock).toHaveBeenCalledWith(expect.stringContaining("libraryId=lib-music-1"));
  });

  it("drills down into an album on press", () => {
    render(<MusicScreen />);
    fireEvent.click(screen.getByLabelText("Discovery"));
    expect(pushMock).toHaveBeenCalledWith(
      expect.stringContaining("/(auth)/music/album/album-1"),
    );
    expect(pushMock).toHaveBeenCalledWith(expect.stringContaining("libraryId=lib-music-1"));
  });

  it("selecting a track loads the full track list into the queue at that index", () => {
    render(<MusicScreen />);
    fireEvent.click(screen.getByLabelText("Aerodynamic"));

    const state = useMusicQueueStore.getState();
    expect(state.currentIndex).toBe(1);
    expect(state.isPlaying).toBe(true);
    expect(state.queue.map((t) => t.id)).toEqual(["tr-1", "tr-2"]);
  });

  // A row's own query can fail independently of the others (libraries loaded
  // fine, artists didn't) - it used to fall through to the same "no artists"
  // wording as a library that genuinely has none, and the other two rows
  // must keep rendering regardless.
  it("tells a failed artists row apart from an empty one, without taking down the albums/tracks rows", () => {
    const refetchArtists = vi.fn();
    getArtistsMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch: refetchArtists });
    render(<MusicScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.music.noArtists")).toBeNull();
    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetchArtists).toHaveBeenCalled();

    // Unaffected rows still render.
    expect(screen.getByLabelText("Discovery")).toBeTruthy();
    expect(screen.getByLabelText("One More Time")).toBeTruthy();
  });
});
