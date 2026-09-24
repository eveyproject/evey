import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TouchableOpacity: ({ children, onPress, testID, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "data-testid": testID, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent, ListHeaderComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "list" },
      ListHeaderComponent ?? null,
      data.length === 0 && ListEmptyComponent
        ? ListEmptyComponent
        : data.map((item: unknown, index: number) =>
            React.createElement(React.Fragment, { key: keyExtractor(item, index) }, renderItem({ item, index })),
          ),
    ),
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) =>
      typeof opts === "object" && opts && "name" in opts ? `${key}:${opts.name}` : key,
  }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      text: "#fff", textMuted: "#888", surface: "#111", accent: "#f00", onAccent: "#000",
    },
    radii: { sm: 4, md: 8 },
    spacing: { md: 12 },
  }),
}));

vi.mock("../../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
vi.mock("../../hooks/useServerUrl", () => ({ useServerUrl: () => "http://server" }));

vi.mock("./TrackArt", () => ({ TrackArt: () => React.createElement("div", { "data-testid": "track-art" }) }));

const usePlaylistListMock = vi.fn();
const usePlaylistDetailMock = vi.fn();
const removeMutateMock = vi.fn();
const deleteMutateMock = vi.fn();
const generateMutateMock = vi.fn();
vi.mock("../../api/playlists", () => ({
  usePlaylistList: (...a: unknown[]) => usePlaylistListMock(...a),
  usePlaylistDetail: (...a: unknown[]) => usePlaylistDetailMock(...a),
  useRemoveFromPlaylist: () => ({ mutate: removeMutateMock }),
  useDeletePlaylist: () => ({ mutate: deleteMutateMock }),
  useGeneratePlaylist: () => ({ mutate: generateMutateMock, isPending: false }),
}));

const setQueueMock = vi.fn();
vi.mock("../../stores/musicQueue", () => ({
  useMusicQueueStore: (selector: (s: { setQueue: typeof setQueueMock }) => unknown) =>
    selector({ setQueue: setQueueMock }),
}));

import { MusicPlaylistsTab } from "./MusicPlaylistsTab";

const PLAYLISTS = [
  { id: "p1", name: "Road Trip", description: "Driving tunes", type: "manual" as const },
  { id: "p2", name: "Auto Mix", description: "", type: "smart" as const },
];

const LIBRARY_TRACKS = [
  { id: "t1", title: "Song A", artist: "Artist A", album: "Album A", duration: 200, trackNumber: 1 },
  { id: "t2", title: "Song B", artist: "Artist B", album: "Album B", duration: 150, trackNumber: 2 },
];

beforeEach(() => {
  vi.clearAllMocks();
  usePlaylistListMock.mockReturnValue({ data: PLAYLISTS, isLoading: false });
  usePlaylistDetailMock.mockReturnValue({ data: undefined, isLoading: false });
});

describe("MusicPlaylistsTab", () => {
  it("lists manual playlists only, hiding smart ones", () => {
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    expect(screen.getByText("Road Trip")).toBeTruthy();
    expect(screen.queryByText("Auto Mix")).toBeNull();
  });

  it("shows the empty state when there are no playlists", () => {
    usePlaylistListMock.mockReturnValue({ data: [], isLoading: false });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    expect(screen.getByText("media.music.noPlaylists")).toBeTruthy();
  });

  // "You have no playlists" is a statement about the account. A failed fetch
  // does not license it - and this tab was the one place left in the app still
  // making it: AddToPlaylistSheet already renders LoadFailed here, and its own
  // test is named after exactly this distinction.
  it("shows LoadFailed with a retry - not 'no playlists' - when the fetch fails", () => {
    const refetch = vi.fn();
    usePlaylistListMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.music.noPlaylists")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  // Same distinction one level down: "this playlist is empty" is a statement
  // about the playlist, and a failed detail fetch has not earned it either.
  it("shows LoadFailed - not 'empty playlist' - when the opened playlist fails to load", () => {
    const refetch = vi.fn();
    usePlaylistDetailMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByText("Road Trip"));

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.music.emptyPlaylist")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("opens a playlist and joins its items against the loaded library tracks", () => {
    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t1" }, { media_item_id: "t2" }] },
      isLoading: false,
    });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByText("Road Trip"));
    expect(screen.getByText("Song A")).toBeTruthy();
    expect(screen.getByText("Song B")).toBeTruthy();
  });

  it("drops playlist items that aren't in the loaded library track list", () => {
    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t1" }, { media_item_id: "unknown" }] },
      isLoading: false,
    });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByText("Road Trip"));
    expect(screen.getByText("Song A")).toBeTruthy();
    expect(screen.queryByText("Song B")).toBeNull();
  });

  it("plays a track through the shared queue store, tagging the playlist as source", () => {
    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t1" }, { media_item_id: "t2" }] },
      isLoading: false,
    });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByText("Road Trip"));
    fireEvent.click(screen.getByText("Song B"));
    expect(setQueueMock).toHaveBeenCalledWith(
      [
        { id: "t1", title: "Song A", artist: "Artist A", album: "Album A", duration: 200 },
        { id: "t2", title: "Song B", artist: "Artist B", album: "Album B", duration: 150 },
      ],
      1,
      { type: "playlist", id: "p1", label: "Road Trip" },
    );
  });

  it("shuffles the playlist's tracks into the queue starting at index 0", () => {
    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t1" }, { media_item_id: "t2" }] },
      isLoading: false,
    });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByText("Road Trip"));
    fireEvent.click(screen.getByTestId("playlist-shuffle-play"));
    expect(setQueueMock).toHaveBeenCalledTimes(1);
    const [tracks, startIndex, source] = setQueueMock.mock.calls[0];
    expect(startIndex).toBe(0);
    expect(source).toEqual({ type: "playlist", id: "p1", label: "Road Trip" });
    expect(tracks).toHaveLength(2);
    expect(tracks.map((tr: { id: string }) => tr.id).sort()).toEqual(["t1", "t2"]);
  });

  it("removes a track from the playlist", () => {
    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t1" }] },
      isLoading: false,
    });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByText("Road Trip"));
    fireEvent.click(screen.getByLabelText("common.delete"));
    expect(removeMutateMock).toHaveBeenCalledWith({ playlistId: "p1", mediaItemId: "t1" }, expect.objectContaining({ onError: expect.any(Function) }));
  });

  it("opens directly into the requested playlist via initialSelectedId", () => {
    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t1" }] },
      isLoading: false,
    });
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} initialSelectedId="p1" />);
    expect(screen.getByText("Song A")).toBeTruthy();
  });

  it("re-opens a new target playlist when initialSelectedId changes on an already-mounted tab", () => {
    usePlaylistDetailMock.mockReturnValue({ data: undefined, isLoading: false });
    const { rerender } = render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} initialSelectedId={null} />);
    expect(screen.getByText("Road Trip")).toBeTruthy();

    usePlaylistDetailMock.mockReturnValue({
      data: { id: "p1", name: "Road Trip", items: [{ media_item_id: "t2" }] },
      isLoading: false,
    });
    rerender(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} initialSelectedId="p1" />);
    expect(screen.getByText("Song B")).toBeTruthy();
  });

  it("deletes a playlist after confirming the destructive alert action", async () => {
    const { Alert } = await import("react-native");
    (Alert.alert as unknown as ReturnType<typeof vi.fn>).mockImplementation(
      (_title: string, _msg: string, buttons: { text: string; onPress?: () => void }[]) => {
        buttons.find((b) => b.text === "common.delete")?.onPress?.();
      },
    );
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    fireEvent.click(screen.getByTestId("icon-trash-outline").parentElement!);
    await waitFor(() => expect(deleteMutateMock).toHaveBeenCalledWith("p1", expect.objectContaining({ onError: expect.any(Function) })));
  });

  // Web parity: the server-side taste-weighted generator (POST
  // /playlists/generate) is reachable from the playlists tab.
  it("generates a playlist for the active library from the header button", () => {
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} libraryId="lib-1" />);
    fireEvent.click(screen.getByTestId("generate-playlist-btn"));
    expect(generateMutateMock).toHaveBeenCalledWith(
      { libraryId: "lib-1", name: "media.playlist.generatedName" },
      expect.anything(),
    );
  });

  it("hides the generate button when no library is known", () => {
    render(<MusicPlaylistsTab libraryTracks={LIBRARY_TRACKS} />);
    expect(screen.queryByTestId("generate-playlist-btn")).toBeNull();
  });
});
