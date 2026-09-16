import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { PlaylistsTab } from "./PlaylistsTab";
import type { Track } from "./musicQueries";

const api = vi.hoisted(() => ({
  lists: { data: undefined as unknown, isLoading: false },
  items: { data: undefined as unknown, isLoading: false },
  itemsArgs: vi.fn(),
  removeItem: vi.fn(),
  deletePlaylist: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  usePlaylistServiceListPlaylists: () => api.lists,
  usePlaylistServiceGetPlaylistItems: (...a: unknown[]) => {
    api.itemsArgs(...a);
    return api.items;
  },
  usePlaylistServiceRemovePlaylistItem: () => ({ mutateAsync: api.removeItem }),
  usePlaylistServiceDeletePlaylist: () => ({ mutateAsync: api.deletePlaylist }),
  getPlaylistServiceListPlaylistsQueryKey: () => ["playlists"],
  getPlaylistServiceGetPlaylistItemsQueryKey: (id: string) => ["playlist-items", id],
  V1PlaylistType: { PLAYLIST_TYPE_SMART: "PLAYLIST_TYPE_SMART", PLAYLIST_TYPE_MANUAL: "PLAYLIST_TYPE_MANUAL" },
  formatTime: (s: number) => `${s}s`,
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok" }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const generate = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
vi.mock("@/features/playlist/api", () => ({ useGeneratePlaylist: () => generate }));
vi.mock("@/utils/downloadToken", () => ({ startTokenDownload: vi.fn() }));
vi.mock("./musicFormat", () => ({ shuffleTracks: (t: unknown[]) => [...t].reverse() }));
vi.mock("./TrackArt", () => ({ TrackArt: () => null }));

const tracks: Track[] = [
  { ID: "t1", Title: "One", Artist: "A", Album: "X", AlbumArtist: "A", Year: 0, TrackNumber: 1, DiskNumber: 1, Duration: 10, FilePath: "", MimeType: "", Genre: "", CoverArtURL: "" },
  { ID: "t2", Title: "Two", Artist: "A", Album: "X", AlbumArtist: "A", Year: 0, TrackNumber: 2, DiskNumber: 1, Duration: 20, FilePath: "", MimeType: "", Genre: "", CoverArtURL: "" },
];

function mount(over: Partial<React.ComponentProps<typeof PlaylistsTab>> = {}) {
  const onPlay = vi.fn();
  render(<PlaylistsTab allTracks={tracks} onPlay={onPlay} libraryID="lib-1" {...over} />);
  return onPlay;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(console, "warn").mockImplementation(() => {});
  api.lists = { data: { playlists: [{ id: "p1", name: "Soirée", type: "PLAYLIST_TYPE_MANUAL" }, { id: "p2", name: "Auto", type: "PLAYLIST_TYPE_SMART" }] }, isLoading: false };
  api.items = { data: { items: [{ mediaItemId: "t2" }, { mediaItemId: "ghost" }, { mediaItemId: "t1" }] }, isLoading: false };
  api.removeItem.mockResolvedValue({});
  api.deletePlaylist.mockResolvedValue({});
});

// The tab lists manual playlists only (smart ones live elsewhere), opens one
// into its tracks in playlist order resolved against the loaded library,
// plays and shuffles with the playlist as the source, removes a track and
// refreshes, and deletes a playlist after a confirm.
describe("PlaylistsTab", () => {
  it("lists manual playlists, opens one, plays in order and shuffles with the playlist as source", () => {
    const onPlay = mount();
    expect(screen.getByText("Soirée")).toBeInTheDocument();
    expect(screen.queryByText("Auto")).toBeNull();
    fireEvent.click(screen.getByText("Soirée"));
    expect(api.itemsArgs).toHaveBeenLastCalledWith("p1", undefined, expect.objectContaining({ query: expect.objectContaining({ enabled: true }) }));
    expect(screen.getByText("Two")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Two"));
    expect(onPlay).toHaveBeenCalledWith(tracks[1], [tracks[1], tracks[0]], { type: "playlist", id: "p1", label: "Soirée" });
    fireEvent.click(screen.getByText("media.music.shuffle"));
    expect(onPlay).toHaveBeenLastCalledWith(tracks[0], [tracks[0], tracks[1]], { type: "playlist", id: "p1", label: "Soirée" });
  });

  it("removes a track and refreshes the playlist; a failure is reported", async () => {
    mount();
    fireEvent.click(screen.getByText("Soirée"));
    fireEvent.click(screen.getAllByRole("button", { name: "common.delete" })[0]);
    await waitFor(() => expect(api.removeItem).toHaveBeenCalledWith({ id: "p1", mediaItemId: "t2" }));
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ["playlist-items", "p1"] }));
    api.removeItem.mockRejectedValueOnce(new Error("nope"));
    fireEvent.click(screen.getAllByRole("button", { name: "common.delete" })[1]);
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("media.playlist.failedRemove"));
  });

  it("deletes a playlist only after the confirm, and generates one for the library", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    mount();
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    expect(api.deletePlaylist).not.toHaveBeenCalled();
    confirm.mockReturnValue(true);
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    await waitFor(() => expect(api.deletePlaylist).toHaveBeenCalledWith({ id: "p1" }));
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ["playlists"] }));

    fireEvent.click(screen.getByTestId("generate-playlist-btn"));
    expect(generate.mutate).toHaveBeenCalledWith({ libraryId: "lib-1", name: "media.playlist.generatedName" }, expect.anything());
    confirm.mockRestore();
  });
});
