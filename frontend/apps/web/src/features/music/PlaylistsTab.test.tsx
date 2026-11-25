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
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok" }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const generate = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
const reorder = vi.hoisted(() => vi.fn());
const addItem = vi.hoisted(() => vi.fn());
const update = vi.hoisted(() => vi.fn());
vi.mock("@/features/playlist/api", () => ({
  useGeneratePlaylist: () => generate,
  useReorderPlaylistItems: () => ({ mutateAsync: reorder }),
  useAddToPlaylist: () => ({ mutateAsync: addItem }),
  useUpdatePlaylist: () => ({ mutateAsync: update, isPending: false }),
}));
vi.mock("./MusicPlaylistModal", () => ({
  MusicPlaylistModal: ({ open, onCreated }: { open: boolean; onCreated: (id: string) => void }) =>
    open ? React.createElement("button", { onClick: () => onCreated("p1") }, "modal-create") : null,
}));
vi.mock("./AddToQueueButton", () => ({ AddToQueueButton: () => null }));
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
  reorder.mockResolvedValue(undefined);
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

  it("shows a member beyond the loaded track pages from what the server sends with it", () => {
    api.items = {
      data: { items: [{ mediaItemId: "t1" }, { mediaItemId: "far", title: "Far Away", artist: "B", mediaType: "music", duration: 30 }] },
      isLoading: false,
    };
    const onPlay = mount();
    fireEvent.click(screen.getByText("Soirée"));
    expect(screen.getByText("Far Away")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Far Away"));
    expect(onPlay).toHaveBeenCalledWith(expect.objectContaining({ ID: "far", Title: "Far Away", Artist: "B" }), expect.any(Array), expect.anything());
  });

  it("leaves the videos of a mixed playlist out of the list", () => {
    api.items = { data: { items: [{ mediaItemId: "v1", title: "A film", mediaType: "video" }, { mediaItemId: "t1" }] }, isLoading: false };
    mount();
    fireEvent.click(screen.getByText("Soirée"));
    expect(screen.queryByText("A film")).toBeNull();
    expect(screen.getByText("One")).toBeInTheDocument();
  });

  it("reorders with the arrow keys and sends every member, the videos in their slots", async () => {
    api.items = {
      data: { items: [{ mediaItemId: "t1" }, { mediaItemId: "v1", title: "A film", mediaType: "video" }, { mediaItemId: "t2" }] },
      isLoading: false,
    };
    mount();
    fireEvent.click(screen.getByText("Soirée"));
    fireEvent.keyDown(screen.getAllByRole("button", { name: "media.music.moveInQueue" })[0], { key: "ArrowDown" });
    await waitFor(() =>
      expect(reorder).toHaveBeenCalledWith({
        playlistId: "p1",
        items: [
          { mediaItemId: "t2", sortOrder: 0 },
          { mediaItemId: "v1", sortOrder: 1 },
          { mediaItemId: "t1", sortOrder: 2 },
        ],
      }),
    );
    // Shown at once, before the refetch.
    const titles = screen.getAllByText(/^(One|Two)$/).map((el) => el.textContent);
    expect(titles).toEqual(["Two", "One"]);
  });

  it("creates a playlist by hand and opens it", () => {
    mount();
    fireEvent.click(screen.getByTestId("new-playlist-btn"));
    fireEvent.click(screen.getByText("modal-create"));
    expect(api.itemsArgs).toHaveBeenLastCalledWith("p1", undefined, expect.anything());
  });

  it("renames the open playlist in place; a taken name is said so", async () => {
    update.mockResolvedValueOnce({});
    mount();
    fireEvent.click(screen.getByText("Soirée"));
    fireEvent.click(screen.getByTestId("playlist-rename-btn"));
    const field = screen.getByRole("textbox", { name: "media.playlist.name" });
    fireEvent.change(field, { target: { value: "Dimanche" } });
    fireEvent.submit(field.closest("form")!);
    await waitFor(() => expect(update).toHaveBeenCalledWith({ id: "p1", name: "Dimanche" }));
    expect(await screen.findByText("Dimanche")).toBeInTheDocument();

    update.mockRejectedValueOnce(Object.assign(new Error("x"), { httpStatus: 409 }));
    fireEvent.click(screen.getByTestId("playlist-rename-btn"));
    const again = screen.getByRole("textbox", { name: "media.playlist.name" });
    fireEvent.change(again, { target: { value: "Soirée bis" } });
    fireEvent.submit(again.closest("form")!);
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("media.music.mixNameTaken"));
  });

  it("Escape leaves the name as it was", () => {
    mount();
    fireEvent.click(screen.getByText("Soirée"));
    fireEvent.click(screen.getByTestId("playlist-rename-btn"));
    const field = screen.getByRole("textbox", { name: "media.playlist.name" });
    fireEvent.change(field, { target: { value: "Autre" } });
    fireEvent.keyDown(field, { key: "Escape" });
    expect(update).not.toHaveBeenCalled();
    expect(screen.getByText("Soirée")).toBeInTheDocument();
  });
});
