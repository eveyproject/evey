import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { AlbumsTab } from "./AlbumsTab";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { count?: number }) => (o ? `${k}:${o.count}` : k) }),
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok en" }),
  toast: { error: (...a: unknown[]) => toastError(...a) },
}));
const toastError = vi.hoisted(() => vi.fn());
const startTokenDownload = vi.hoisted(() => vi.fn());
vi.mock("@/utils/downloadToken", () => ({ startTokenDownload: (...a: unknown[]) => startTokenDownload(...a) }));
vi.mock("@/components/ui/FetchErrorState", () => ({ FetchErrorState: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));
vi.mock("@/features/library/MediaGrid", () => ({
  MediaGrid: ({ children, isLoading }: any) => React.createElement("div", { "data-testid": "grid", "data-loading": String(isLoading) }, children),
}));
vi.mock("../../lib/coverArtSrc", () => ({ coverArtSrc: (url: string, token: string, w: number) => `${url}${token}&w=${w}` }));

const albums = [
  { ID: "a1", Title: "First", Artist: "Band A", TrackCount: 12, Year: 1999, CoverArtURL: "/api/v1/music/album-art/a1" },
  { ID: "a2", Title: "", Artist: "", TrackCount: 3, Year: 0, CoverArtURL: "" },
] as any[];

beforeEach(() => {
  startTokenDownload.mockReset().mockResolvedValue(undefined);
  toastError.mockReset();
});

// A card paints its cover through the token-carrying source at 400 px,
// prints the year only when known and a dash for missing text, opens on click
// or keyboard, and its download button mints a click-time token bound to the
// album without opening the album. A failed fetch is an error state, never
// "no albums", and a refused download is a toast.
describe("AlbumsTab", () => {
  it("renders cards with cover, counts and year, and opens an album", () => {
    const onAlbumClick = vi.fn();
    render(<AlbumsTab albums={albums} loading={false} libraryID="lib-1" onAlbumClick={onAlbumClick} />);
    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.getByText("media.music.trackCount:12 · 1999")).toBeInTheDocument();
    expect(screen.getByText("media.music.trackCount:3")).toBeInTheDocument();
    expect(screen.getAllByText("-")).toHaveLength(2);
    const cards = screen.getAllByRole("button").filter((b) => b.tagName === "DIV");
    expect(cards).toHaveLength(2);
    const cover = cards[0].querySelector("[style*='background-image']") as HTMLElement;
    expect(cover.style.backgroundImage).toContain("/api/v1/music/album-art/a1?token=tok%20en&w=400");
    expect(cards[1].querySelector("[style*='background-image']")).toBeNull();

    fireEvent.click(cards[0]);
    expect(onAlbumClick).toHaveBeenCalledWith(albums[0]);
    fireEvent.keyDown(cards[1], { key: " " });
    expect(onAlbumClick).toHaveBeenLastCalledWith(albums[1]);
  });

  it("downloads an album with a click-time token without opening it, and toasts a refusal", async () => {
    const onAlbumClick = vi.fn();
    render(<AlbumsTab albums={albums} loading={false} libraryID="lib 1" onAlbumClick={onAlbumClick} />);
    const downloads = screen.getAllByLabelText("media.music.downloadAlbum");
    fireEvent.click(downloads[0]);
    await waitFor(() => expect(startTokenDownload).toHaveBeenCalledTimes(1));
    expect(startTokenDownload).toHaveBeenCalledWith("/api/v1/music/albums/download?albumId=a1&libraryId=lib%201", "a1");
    expect(onAlbumClick).not.toHaveBeenCalled();

    startTokenDownload.mockRejectedValueOnce(new Error("denied"));
    fireEvent.click(downloads[1]);
    await waitFor(() => expect(toastError).toHaveBeenCalledWith("common.error"));
  });

  it("does nothing without a library, and keeps loading, error and empty states apart", () => {
    const { unmount } = render(<AlbumsTab albums={albums} loading={false} libraryID={undefined} onAlbumClick={() => {}} />);
    fireEvent.click(screen.getAllByLabelText("media.music.downloadAlbum")[0]);
    expect(startTokenDownload).not.toHaveBeenCalled();
    unmount();

    const onRetry = vi.fn();
    const err = render(<AlbumsTab albums={[]} loading={false} isError error={new Error("x")} onRetry={onRetry} libraryID="l" onAlbumClick={() => {}} />);
    fireEvent.click(screen.getByText("retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("media.music.noAlbums")).toBeNull();
    err.unmount();

    const empty = render(<AlbumsTab albums={[]} loading={false} libraryID="l" onAlbumClick={() => {}} />);
    expect(screen.getByText("media.music.noAlbums")).toBeInTheDocument();
    empty.unmount();

    render(<AlbumsTab albums={[]} loading isError libraryID="l" onAlbumClick={() => {}} />);
    expect(screen.getByTestId("grid").getAttribute("data-loading")).toBe("true");
    expect(screen.queryByText("retry")).toBeNull();
  });
});
