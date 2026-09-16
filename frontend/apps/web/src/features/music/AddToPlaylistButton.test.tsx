import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AddToPlaylistButton } from "./AddToPlaylistButton";

const api = vi.hoisted(() => ({ listArgs: vi.fn(), lists: { data: undefined as unknown }, add: vi.fn(), create: vi.fn() }));
vi.mock("@evey/api", () => ({
  usePlaylistServiceListPlaylists: (...a: unknown[]) => {
    api.listArgs(...a);
    return api.lists;
  },
  usePlaylistServiceAddPlaylistItem: () => ({ mutateAsync: api.add }),
  usePlaylistServiceCreatePlaylist: () => ({ mutateAsync: api.create }),
  getPlaylistServiceListPlaylistsQueryKey: () => ["playlists"],
  V1PlaylistType: { PLAYLIST_TYPE_SMART: "PLAYLIST_TYPE_SMART", PLAYLIST_TYPE_MANUAL: "PLAYLIST_TYPE_MANUAL" },
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(console, "warn").mockImplementation(() => {});
  api.lists = { data: { playlists: [{ id: "p1", name: "Soirée", type: "PLAYLIST_TYPE_MANUAL" }, { id: "p2", name: "Auto", type: "PLAYLIST_TYPE_SMART" }] } };
  api.add.mockResolvedValue({});
  api.create.mockResolvedValue({ id: "p9" });
});

// The menu lists manual playlists only and fetches them only once opened;
// adding sends the track as music and closes the menu; "new playlist" asks
// for a name, creates, refreshes the list and adds the track in one go, and
// a blank name is a no-op.
describe("AddToPlaylistButton", () => {
  it("opens on demand, lists manual playlists, and adds the track", async () => {
    render(<AddToPlaylistButton trackID="t1" />);
    expect(api.listArgs).toHaveBeenLastCalledWith(undefined, { query: { enabled: false, staleTime: 30_000 } });
    fireEvent.click(screen.getByRole("button", { name: "media.music.addToPlaylist" }));
    expect(api.listArgs).toHaveBeenLastCalledWith(undefined, { query: { enabled: true, staleTime: 30_000 } });
    expect(screen.getByRole("menuitem", { name: "Soirée" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Auto" })).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: "Soirée" }));
    await waitFor(() => expect(api.add).toHaveBeenCalledWith({ id: "p1", data: { mediaItemId: "t1", mediaType: "music" } }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("media.music.addedToPlaylist"));
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("creates a named playlist and adds the track; a blank name does nothing; a failed add is reported", async () => {
    const prompt = vi.spyOn(window, "prompt").mockReturnValue("  Road trip ");
    render(<AddToPlaylistButton trackID="t1" />);
    fireEvent.click(screen.getByRole("button", { name: "media.music.addToPlaylist" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "media.music.newPlaylist" }));
    await waitFor(() => expect(api.create).toHaveBeenCalledWith({ data: { name: "Road trip", type: "PLAYLIST_TYPE_MANUAL" } }));
    await waitFor(() => expect(api.add).toHaveBeenCalledWith({ id: "p9", data: { mediaItemId: "t1", mediaType: "music" } }));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["playlists"] });

    prompt.mockReturnValue("   ");
    fireEvent.click(screen.getByRole("button", { name: "media.music.addToPlaylist" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "media.music.newPlaylist" }));
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(api.create).toHaveBeenCalledTimes(1);

    api.add.mockRejectedValueOnce(new Error("nope"));
    fireEvent.click(screen.getByRole("button", { name: "media.music.addToPlaylist" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Soirée" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("media.playlist.addError"));
    prompt.mockRestore();
  });

  it("closes on a click outside", () => {
    render(<AddToPlaylistButton trackID="t1" />);
    fireEvent.click(screen.getByRole("button", { name: "media.music.addToPlaylist" }));
    expect(screen.getByRole("menu")).toBeInTheDocument();
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
