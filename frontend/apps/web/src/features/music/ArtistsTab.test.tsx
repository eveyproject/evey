import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { ArtistsTab } from "./ArtistsTab";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { count?: number }) => (o ? `${k}:${o.count}` : k) }),
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok en" }) }));
vi.mock("@/components/ui/FetchErrorState", () => ({ FetchErrorState: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));
vi.mock("../../lib/coverArtSrc", () => ({ coverArtSrc: (url: string, token: string, w: number) => `${url}${token}&w=${w}` }));

const artists = [
  { Name: "Band A", AlbumCount: 2, TrackCount: 20, CoverArtURL: "/api/v1/music/album-art/a" },
  { Name: "Band B", AlbumCount: 1, TrackCount: 5, CoverArtURL: "" },
] as any[];

// Each row names the artist with its album and track counts, paints the
// cover through the token-carrying source, opens on click or Enter, and a
// failed fetch is an error state, never "no artists".
describe("ArtistsTab", () => {
  it("lists artists with counts and covers, and opens one on click or keyboard", () => {
    const onArtistClick = vi.fn();
    render(<ArtistsTab artists={artists} loading={false} onArtistClick={onArtistClick} />);
    expect(screen.getByText("Band A")).toBeInTheDocument();
    expect(screen.getByText("media.music.albumCount:2 · media.music.trackCount:20")).toBeInTheDocument();
    const rows = screen.getAllByRole("button");
    expect(rows).toHaveLength(2);
    const avatar = rows[0].querySelector("[style*='background-image']") as HTMLElement;
    expect(avatar.style.backgroundImage).toContain("/api/v1/music/album-art/a?token=tok%20en&w=96");
    expect(rows[1].querySelector("[style*='background-image']")).toBeNull();
    fireEvent.click(rows[0]);
    expect(onArtistClick).toHaveBeenCalledWith(artists[0]);
    fireEvent.keyDown(rows[1], { key: "Enter" });
    expect(onArtistClick).toHaveBeenLastCalledWith(artists[1]);
    fireEvent.keyDown(rows[1], { key: " " });
    expect(onArtistClick).toHaveBeenCalledTimes(3);
  });

  it("loading, error and empty states are distinct", () => {
    const onRetry = vi.fn();
    const { rerender } = render(<ArtistsTab artists={[]} loading onArtistClick={vi.fn()} />);
    expect(screen.getByText("media.music.loading")).toBeInTheDocument();
    rerender(<ArtistsTab artists={[]} loading={false} isError onRetry={onRetry} onArtistClick={vi.fn()} />);
    expect(screen.queryByText("media.music.noArtists")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(onRetry).toHaveBeenCalled();
    rerender(<ArtistsTab artists={[]} loading={false} onArtistClick={vi.fn()} />);
    expect(screen.getByText("media.music.noArtists")).toBeInTheDocument();
  });
});
