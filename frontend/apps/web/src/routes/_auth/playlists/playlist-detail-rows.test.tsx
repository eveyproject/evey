import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const playlistDetail = vi.fn();
const remove = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
const startTokenDownload = vi.hoisted(() => vi.fn());
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const auth = vi.hoisted(() => ({ accessToken: "tok" as string | null }));
const play = vi.hoisted(() => vi.fn());

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => ({ ...def, useParams: () => ({ playlistId: "pl-1" }) }),
  Link: ({ children, params }: any) => React.createElement("a", { href: `/media/${params?.mediaId}` }, children),
  useRouter: () => ({ history: { back: vi.fn() }, navigate: vi.fn() }),
  useCanGoBack: () => false,
}));
vi.mock("@/features/playlist/api", () => ({
  usePlaylistDetail: (...args: unknown[]) => playlistDetail(...args),
  useRemoveFromPlaylist: () => remove,
}));
vi.mock("@/features/playlist/SmartPlaylistRulesEditor", () => ({
  SmartPlaylistRulesEditor: ({ initialRules }: any) => React.createElement("div", { "data-testid": "rules" }, String(initialRules.length)),
}));
vi.mock("@/utils/downloadToken", () => ({ startTokenDownload: (...a: unknown[]) => startTokenDownload(...a) }));
vi.mock("@evey/store", () => ({
  toast,
  useAuthStore: (sel: (s: { accessToken: string | null }) => unknown) => sel(auth),
  useMusicPlayerStore: (sel: (s: { play: typeof play }) => unknown) => sel({ play }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { count?: number }) => (o ? `${k}:${o.count}` : k) }),
}));

async function renderPage() {
  const mod = await import("./$playlistId.lazy");
  const PlaylistDetail = (mod.Route as unknown as { component: React.ComponentType }).component;
  render(React.createElement(PlaylistDetail));
}

const loaded = (data: Record<string, unknown>) =>
  playlistDetail.mockReturnValue({ data, isLoading: false, isError: false, error: null, refetch: vi.fn() });

beforeEach(() => {
  vi.clearAllMocks();
  auth.accessToken = "tok";
  startTokenDownload.mockResolvedValue(undefined);
});

// Rows are numbered from their sort order, link to the title, and only a
// MANUAL playlist lets a row be removed (a smart one is defined by its rules,
// which get their editor instead). The ZIP download mints a click-time token
// and exists only when there is something to download and someone signed in.
describe("PlaylistDetail rows and actions", () => {
  it("a manual playlist: numbered rows, remove per row, download with a click-time token", async () => {
    loaded({
      name: "Mix", type: "manual", description: "Evening",
      items: [
        { media_item_id: "m1", title: "First", sort_order: 0, type: "MEDIA_ITEM_TYPE_MOVIE", year: 1999 },
        { media_item_id: "m2", sort_order: 4 },
      ],
    });
    await renderPage();
    expect(screen.getByText("Evening")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("movie")).toBeInTheDocument();
    expect(screen.getByText("1999")).toBeInTheDocument();
    expect(screen.getByText("m2").closest("a")?.getAttribute("href")).toBe("/media/m2");
    expect(screen.getByText("common.item:2")).toBeInTheDocument();
    expect(screen.queryByTestId("rules")).toBeNull();

    const removes = screen.getAllByLabelText("common.remove");
    expect(removes).toHaveLength(2);
    fireEvent.click(removes[1]);
    expect(remove.mutate).toHaveBeenCalledWith({ playlistId: "pl-1", mediaItemId: "m2" }, expect.any(Object));
    remove.mutate.mock.calls[0][1].onSuccess();
    expect(toast.success).toHaveBeenCalledWith("media.playlist.removedFromPlaylist");
    remove.mutate.mock.calls[0][1].onError();
    expect(toast.error).toHaveBeenCalledWith("media.playlist.failedRemove");

    fireEvent.click(screen.getByTestId("playlist-download-btn"));
    await waitFor(() => expect(startTokenDownload).toHaveBeenCalledWith("/api/v1/playlists/pl-1/download", "pl-1", "Mix.zip"));
  });

  it("a smart playlist shows its rules editor and no per-row removal; a refused download is a toast", async () => {
    loaded({ name: "Auto", type: "smart", rules: [{ f: 1 }, { f: 2 }], items: [{ media_item_id: "m1", title: "Only" }] });
    await renderPage();
    expect(screen.getByTestId("rules")).toHaveTextContent("2");
    expect(screen.queryByLabelText("common.remove")).toBeNull();
    expect(screen.getByText("-")).toBeInTheDocument();
    startTokenDownload.mockRejectedValueOnce(new Error("403"));
    fireEvent.click(screen.getByTestId("playlist-download-btn"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("common.error"));
  });

  it("offers no download for an empty playlist or without a session, and words the empty state by type", async () => {
    loaded({ name: "Empty", type: "smart", rules: [], items: [] });
    await renderPage();
    expect(screen.queryByTestId("playlist-download-btn")).toBeNull();
    expect(screen.getByText("media.playlist.noItemsSmart")).toBeInTheDocument();

    auth.accessToken = null;
    loaded({ name: "Full", type: "manual", items: [{ media_item_id: "m1" }] });
    await renderPage();
    expect(screen.queryByTestId("playlist-download-btn")).toBeNull();
  });

  // A track of a mixed playlist linked to /media/<track id>, which answers
  // 404: it plays in the music player with the playlist's other tracks, and
  // its row names the artist.
  it("plays a music track in the music player instead of linking to the video page", async () => {
    loaded({
      name: "Mixed", type: "manual",
      items: [
        { media_item_id: "t-1", media_type: "music", title: "First song", artist: "An artist", sort_order: 0 },
        { media_item_id: "m-1", media_type: "video", title: "A film", sort_order: 1 },
        { media_item_id: "t-2", media_type: "music", title: "Second song", artist: "Another artist", cover_art_url: "/art/t-2", duration: 180, sort_order: 2 },
      ],
    });
    await renderPage();

    expect(screen.getByText("An artist")).toBeInTheDocument();
    expect(screen.getByText("Second song").closest("a")).toBeNull();
    expect(screen.getByText("A film").closest("a")?.getAttribute("href")).toBe("/media/m-1");

    fireEvent.click(screen.getByRole("button", { name: "Second song" }));
    expect(play).toHaveBeenCalledTimes(1);
    const [track, queue] = play.mock.calls[0];
    expect(track).toMatchObject({ ID: "t-2", Title: "Second song", Artist: "Another artist", CoverArtURL: "/art/t-2", Duration: 180 });
    expect(queue.map((q: { ID: string }) => q.ID)).toEqual(["t-1", "t-2"]);
  });
});
