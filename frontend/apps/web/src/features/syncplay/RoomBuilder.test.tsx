import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { RoomBuilder, type QueueItem } from "./RoomBuilder";

// onCreate now receives the full queue items (id + title + artist); these
// helpers read the last call's items / their ids.
function lastItems(onCreate: ReturnType<typeof vi.fn>): QueueItem[] {
  return onCreate.mock.calls.at(-1)?.[0] as QueueItem[];
}
function createdIds(onCreate: ReturnType<typeof vi.fn>): string[] {
  return lastItems(onCreate).map((i) => i.id);
}

// Hook/imperative mocks - the component applies a `select` transform, so the
// mocked hooks return data already in the shape the component consumes.
const mockGetAlbums = vi.fn();
const mockListTracks = vi.fn();
const mockListTracksImperative = vi.fn();
const mockGetPlaylistItems = vi.fn();
const mockPlaylistList = vi.fn();

// Spreads the real module: RoomBuilder also imports collectAllPages and
// EXPORT_PAGE_SIZE from @evey/api, and a mock listing only the endpoints leaves
// them undefined - the component then throws on the first album click.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useMusicServiceGetAlbums: (...a: unknown[]) => mockGetAlbums(...a),
  useMusicServiceListTracks: (...a: unknown[]) => mockListTracks(...a),
  musicServiceListTracks: (...a: unknown[]) => mockListTracksImperative(...a),
  playlistServiceGetPlaylistItems: (...a: unknown[]) => mockGetPlaylistItems(...a),
}));
const mockToastError = vi.fn();

vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok" }),
  toast: { error: (...a: unknown[]) => mockToastError(...a) },
}));
vi.mock("@/features/playlist/api", () => ({
  usePlaylistList: (...a: unknown[]) => mockPlaylistList(...a),
}));

describe("RoomBuilder", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAlbums.mockReturnValue({
      data: [{ id: "al1", title: "Album One", artist: "Artist", coverArtUrl: "", trackCount: 2 }],
    });
    mockListTracks.mockReturnValue({
      data: [
        { id: "t1", title: "Track One", artist: "A" },
        { id: "t2", title: "Track Two", artist: "A" },
      ],
    });
    mockListTracksImperative.mockResolvedValue({
      tracks: [
        { id: "t1", title: "Track One", artist: "A", album: "Album One" },
        { id: "t2", title: "Track Two", artist: "A", album: "Album One" },
      ],
    });
    mockGetPlaylistItems.mockResolvedValue({ items: [{ mediaItemId: "t3", title: "Track Three" }] });
    mockPlaylistList.mockReturnValue({ data: [{ id: "pl1", name: "My Playlist" }] });
  });

  it("adds a single track and creates the room with its id", async () => {
    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={onCreate} />);

    fireEvent.click(screen.getByRole("tab", { name: /tracks/i }));
    const adds = await screen.findAllByLabelText("Add to queue");
    fireEvent.click(adds[0]); // Track One

    // Queue now holds 1 item.
    expect(screen.getByText("Queue (1)")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /create room/i }));
    expect(createdIds(onCreate)).toEqual(["t1"]);
  });

  it("adds a whole album (scoped server-side by album id) and creates the room", async () => {
    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={onCreate} />);

    // Albums tab is the default; click the album's "Add album" action.
    fireEvent.click(screen.getByLabelText("Add album"));
    await waitFor(() => expect(screen.getByText("Queue (2)")).toBeInTheDocument());

    // The album's tracks are resolved by its opaque id, not a fuzzy title
    // search + client-side filter.
    expect(mockListTracksImperative).toHaveBeenCalledWith({
      libraryId: "lib-1",
      albumId: "al1",
      limit: 500,
      offset: 0,
    });

    fireEvent.click(screen.getByRole("button", { name: /create room/i }));
    expect(createdIds(onCreate)).toEqual(["t1", "t2"]);
  });

  it("reorders the queue with the move controls", async () => {
    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={onCreate} />);

    fireEvent.click(screen.getByRole("tab", { name: /tracks/i }));
    const adds = await screen.findAllByLabelText("Add to queue");
    fireEvent.click(adds[0]); // t1
    fireEvent.click((await screen.findAllByLabelText("Add to queue"))[1]); // t2

    // Move the 2nd item up → t2 before t1.
    fireEvent.click(screen.getAllByLabelText("Move up")[1]);
    fireEvent.click(screen.getByRole("button", { name: /create room/i }));
    expect(createdIds(onCreate)).toEqual(["t2", "t1"]);
  });

  it("passes full track metadata (not just ids) to onCreate", async () => {
    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={onCreate} />);

    fireEvent.click(screen.getByRole("tab", { name: /tracks/i }));
    const adds = await screen.findAllByLabelText("Add to queue");
    fireEvent.click(adds[0]); // Track One
    fireEvent.click(screen.getByRole("button", { name: /create room/i }));

    expect(lastItems(onCreate)).toEqual([{ id: "t1", title: "Track One", artist: "A" }]);
  });

  it("does not create a room while the queue is empty", () => {
    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={onCreate} />);
    const btn = screen.getByRole("button", { name: /create room/i });
    expect(btn).toBeDisabled();
    fireEvent.click(btn);
    expect(onCreate).not.toHaveBeenCalled();
  });

  // ListTracks answers with 50 rows when the caller sends no pagination, so a
  // box set used to land in the room as its first 50 tracks with nothing to
  // say the rest had been dropped. The queue has its own deliberate ceiling
  // (MAX_QUEUE = 500), so the visible outcome here is 50 -> 500, and the point
  // of the assertion is that the second page is actually requested.
  it("pulls every page of a long album into the queue", async () => {
    const page = (n: number, from: number) =>
      Array.from({ length: n }, (_, i) => ({ id: `t${from + i}`, title: `Track ${from + i}`, artist: "A" }));
    mockListTracksImperative.mockReset();
    mockListTracksImperative
      .mockResolvedValueOnce({ tracks: page(500, 0) })
      .mockResolvedValueOnce({ tracks: page(12, 500) });

    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={onCreate} />);
    fireEvent.click(screen.getByLabelText("Add album"));

    await waitFor(() => expect(screen.getByText("Queue (500)")).toBeInTheDocument());
    expect(mockListTracksImperative).toHaveBeenCalledTimes(2);
    expect(mockListTracksImperative.mock.calls[1][0]).toMatchObject({ limit: 500, offset: 500 });
  });
});

// The three swallowed failures (backlog 08-08): a failed request used to render
// as an EMPTY album / a silent no-op, indistinguishable from real emptiness.
// Same error surface as the rest of the feature: toast.error (SyncPlayBar).
describe("RoomBuilder failure surfacing", () => {
  it("surfaces an album whose tracks failed to load instead of showing it empty", async () => {
    mockListTracksImperative.mockRejectedValueOnce(new Error("network"));
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={vi.fn()} />);

    fireEvent.click(screen.getByText("Album One"));
    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith("media.syncplay.albumLoadFailed"));
    // The album must NOT stay expanded as a fake empty album.
    expect(screen.queryByText("Queue (0)")).toBeInTheDocument();
  });

  it("surfaces a failed add-album instead of silently leaving the queue unchanged", async () => {
    mockListTracksImperative.mockRejectedValueOnce(new Error("network"));
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={vi.fn()} />);

    fireEvent.click(screen.getByLabelText("Add album"));
    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith("media.syncplay.addAlbumFailed"));
  });

  it("surfaces a failed add-playlist instead of silently doing nothing", async () => {
    mockGetPlaylistItems.mockRejectedValueOnce(new Error("network"));
    render(<RoomBuilder libraryId="lib-1" creating={false} onCreate={vi.fn()} />);

    fireEvent.click(screen.getByRole("tab", { name: /playlists/i }));
    fireEvent.click(await screen.findByLabelText("Add to queue"));
    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith("media.syncplay.addPlaylistFailed"));
  });
});
