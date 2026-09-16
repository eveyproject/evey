import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TracksTab } from "./TracksTab";
import type { Track } from "./musicQueries";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, opts?: unknown) => (opts && typeof opts === "object" && "count" in (opts as object) ? `${(opts as { count: number }).count} tracks` : k),
  }),
}));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => `${s}s` }));
vi.mock("./TrackArt", () => ({ TrackArt: () => null }));
vi.mock("./DownloadTrackButton", () => ({ DownloadTrackButton: () => null }));
vi.mock("./AddToPlaylistButton", () => ({ AddToPlaylistButton: () => null }));
vi.mock("@/components/ui/FetchErrorState", () => ({ FetchErrorState: ({ onRetry }: { onRetry?: () => void }) => <button onClick={onRetry}>retry</button> }));

function track(n: number, extra: Partial<Track> = {}): Track {
  return { ID: `t${n}`, Title: `Track ${n}`, Artist: "Band A", Album: "Album X", AlbumArtist: "Band A", Year: 2001, TrackNumber: n, DiskNumber: 1, Duration: 100 + n, FilePath: "", MimeType: "", Genre: "Rock", CoverArtURL: "", ...extra };
}

const noop = () => {};
function mount(props: Partial<Parameters<typeof TracksTab>[0]> & { allTracks: Track[] }) {
  const onPlay = vi.fn();
  const utils = render(
    <TracksTab
      loading={false}
      playing={null}
      onPlay={onPlay}
      search=""
      onSearchChange={noop}
      albumFilter={null}
      albumLabel=""
      artistFilter={null}
      onClearAlbumFilter={noop}
      onClearArtistFilter={noop}
      genreFilter=""
      yearFilter={0}
      artistSelectFilter=""
      onGenreChange={noop}
      onYearChange={noop}
      onArtistSelectChange={noop}
      {...props}
    />,
  );
  return { ...utils, onPlay };
}

// The tab pages the loaded rows by 50, keeps Next alive on the last loaded
// page while the server still has rows (and turns the page once they land),
// filters client-side by genre/year/artist and, in album mode, by the search
// box, and hands the FILTERED list to the player so "next track" follows
// what the user sees. A failed fetch is an error, never "no tracks".
describe("TracksTab", () => {
  it("pages by 50 and asks the server for more on the last loaded page", async () => {
    const onLoadMore = vi.fn();
    const first = Array.from({ length: 50 }, (_, i) => track(i + 1));
    const { rerender, onPlay } = mount({ allTracks: first, hasMore: true, onLoadMore });
    expect(screen.getByText("1 / 1+ · 50 tracks+")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "common.previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "common.next" })).toBeEnabled();

    // Last loaded page, server has more: Next asks for the next server page.
    fireEvent.click(screen.getByRole("button", { name: "common.next" }));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
    expect(screen.getByText("1 / 1+ · 50 tracks+")).toBeInTheDocument();

    // The rows land: the page turns on its own, and Next dies once nothing is left.
    const more = [...first, ...Array.from({ length: 10 }, (_, i) => track(51 + i))];
    rerender(
      <TracksTab loading={false} playing={null} onPlay={onPlay} search="" onSearchChange={noop} albumFilter={null} albumLabel="" artistFilter={null} onClearAlbumFilter={noop} onClearArtistFilter={noop} genreFilter="" yearFilter={0} artistSelectFilter="" onGenreChange={noop} onYearChange={noop} onArtistSelectChange={noop} allTracks={more} hasMore={false} onLoadMore={onLoadMore} />,
    );
    expect(await screen.findByText("2 / 2 · 60 tracks")).toBeInTheDocument();
    expect(screen.getByText("Track 51")).toBeInTheDocument();
    expect(screen.queryByText("Track 1")).toBeNull();
    expect(screen.getByRole("button", { name: "common.next" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "common.previous" }));
    expect(screen.getByText("Track 1")).toBeInTheDocument();
  });

  it("filters client-side and plays within the filtered list", () => {
    const all = [track(1), track(2, { Genre: "Jazz", Year: 1999, Artist: "Trio B", AlbumArtist: "Trio B" }), track(3, { Genre: "Jazz" })];
    const { onPlay } = mount({ allTracks: all, genreFilter: "Jazz" });
    expect(screen.queryByText("Track 1")).toBeNull();
    fireEvent.click(screen.getByText("Track 3"));
    expect(onPlay).toHaveBeenCalledWith(all[2], [all[1], all[2]]);
    expect(screen.getByRole("button", { name: "media.music.resetFilters" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "1999" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Trio B" })).toBeInTheDocument();
  });

  it("in album mode the search box filters the already-scoped rows", () => {
    const all = [track(1, { Title: "Opening" }), track(2, { Title: "Finale" })];
    mount({ allTracks: all, albumFilter: "alb-1", albumLabel: "Album X", search: "fin" });
    expect(screen.queryByText("Opening")).toBeNull();
    expect(screen.getByText("Finale")).toBeInTheDocument();
    expect(screen.getByText(/media.music.albums: Album X/)).toBeInTheDocument();
  });

  it("a failed fetch is an error state, an empty result says no tracks", () => {
    const onRetry = vi.fn();
    mount({ allTracks: [], isError: true, onRetry });
    fireEvent.click(screen.getByText("retry"));
    expect(onRetry).toHaveBeenCalled();
    expect(screen.queryByText("media.music.noTracks")).toBeNull();
    mount({ allTracks: [], search: "zzz" });
    expect(screen.getByText("media.music.noTracksSearch")).toBeInTheDocument();
  });
});
