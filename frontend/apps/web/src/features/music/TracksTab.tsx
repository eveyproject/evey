import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { X, TriangleAlert } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { EmptyState } from "@/components/ui/EmptyState";
import { classifyError } from "@/utils/errorClassification";
import { formatDuration } from "./musicFormat";
import type { Track } from "./musicQueries";
import { TrackArt } from "./TrackArt";
import { DownloadTrackButton } from "./DownloadTrackButton";
import { AddToPlaylistButton } from "./AddToPlaylistButton";

const TRACKS_PAGE_SIZE = 50;

export function TracksTab({
  allTracks,
  loading,
  isError,
  error,
  onRetry,
  playing,
  onPlay,
  search,
  onSearchChange,
  albumFilter,
  albumLabel,
  artistFilter,
  onClearAlbumFilter,
  onClearArtistFilter,
  genreFilter,
  yearFilter,
  artistSelectFilter,
  onGenreChange,
  onYearChange,
  onArtistSelectChange,
}: {
  allTracks: Track[];
  loading: boolean;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
  playing: Track | null;
  onPlay: (t: Track, filtered: Track[]) => void;
  search: string;
  onSearchChange: (v: string) => void;
  albumFilter: string | null;
  albumLabel: string;
  artistFilter: string | null;
  onClearAlbumFilter: () => void;
  onClearArtistFilter: () => void;
  genreFilter: string;
  yearFilter: number;
  artistSelectFilter: string;
  onGenreChange: (v: string) => void;
  onYearChange: (v: number) => void;
  onArtistSelectChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(0);

  const genreOptions = useMemo(
    () => Array.from(new Set(allTracks.map((x) => x.Genre).filter((g) => !!g))).sort((a, b) => a.localeCompare(b)),
    [allTracks],
  );
  const yearOptions = useMemo(
    () => Array.from(new Set(allTracks.map((x) => x.Year).filter((y) => y > 0))).sort((a, b) => b - a),
    [allTracks],
  );
  const artistOptions = useMemo(() => {
    const set = new Set<string>();
    allTracks.forEach((x) => { if (x.Artist) set.add(x.Artist); if (x.AlbumArtist) set.add(x.AlbumArtist); });
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [allTracks]);

  // In album mode the server already scopes allTracks to the album and ignores
  // the search box, so apply the free-text query client-side here instead.
  const albumQuery = albumFilter ? search.trim().toLowerCase() : "";
  const tracks = useMemo(
    () => allTracks.filter((x) => {
      if (albumQuery && !(
        x.Title.toLowerCase().includes(albumQuery) ||
        x.Artist.toLowerCase().includes(albumQuery) ||
        x.Album.toLowerCase().includes(albumQuery)
      )) return false;
      if (artistFilter && x.Artist !== artistFilter && x.AlbumArtist !== artistFilter) return false;
      if (genreFilter && x.Genre !== genreFilter) return false;
      if (yearFilter && x.Year !== yearFilter) return false;
      if (artistSelectFilter && x.Artist !== artistSelectFilter && x.AlbumArtist !== artistSelectFilter) return false;
      return true;
    }),
    [allTracks, albumQuery, artistFilter, genreFilter, yearFilter, artistSelectFilter],
  );

  const hasExtraFilter = !!genreFilter || yearFilter > 0 || !!artistSelectFilter;

  const totalPages = Math.ceil(tracks.length / TRACKS_PAGE_SIZE);
  const pageTrack = useMemo(
    () => tracks.slice(page * TRACKS_PAGE_SIZE, (page + 1) * TRACKS_PAGE_SIZE),
    [tracks, page],
  );

  useEffect(() => { setPage(0); }, [genreFilter, yearFilter, artistSelectFilter, albumFilter, artistFilter]);

  const selectClass = "text-sm px-2 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer";

  return (
    <div>
      <div className="mb-4 flex items-center gap-2 flex-wrap">
        <input
          type="search"
          placeholder={t("media.music.searchPlaceholder")}
          aria-label={t("media.music.searchPlaceholder")}
          value={search}
          onChange={(e) => { onSearchChange(e.target.value); setPage(0); }}
          className="w-full max-w-[400px] px-3 py-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] text-base sm:text-sm"
        />
        {albumFilter && (
          <button
            onClick={onClearAlbumFilter}
            className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs bg-[var(--accent)] text-[var(--filter-active-text)] cursor-pointer border-none"
            title={t("common.clear")}
          >
            {t("media.music.albums")}: {albumLabel || albumFilter} <X size={ICON.xs} />
          </button>
        )}
        {artistFilter && (
          <button
            onClick={onClearArtistFilter}
            className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs bg-[var(--accent)] text-[var(--filter-active-text)] cursor-pointer border-none"
            title={t("common.clear")}
          >
            {t("media.music.artists")}: {artistFilter} <X size={ICON.xs} />
          </button>
        )}
      </div>
      <div className="mb-4 flex items-center gap-2 flex-wrap">
        <select
          value={genreFilter}
          onChange={(e) => onGenreChange(e.target.value)}
          className={selectClass}
          aria-label={t("media.music.genre")}
        >
          <option value="">{t("media.music.genre") || "Genre"}</option>
          {genreOptions.map((g) => <option key={g} value={g}>{g}</option>)}
        </select>
        <select
          value={yearFilter || ""}
          onChange={(e) => onYearChange(Number(e.target.value) || 0)}
          className={selectClass}
          aria-label={t("media.music.year")}
        >
          <option value="">{t("media.music.year") || "Year"}</option>
          {yearOptions.map((y) => <option key={y} value={y}>{y}</option>)}
        </select>
        <select
          value={artistSelectFilter}
          onChange={(e) => onArtistSelectChange(e.target.value)}
          className={selectClass}
          aria-label={t("media.music.artists")}
        >
          <option value="">{t("media.music.artists")}</option>
          {artistOptions.map((a) => <option key={a} value={a}>{a}</option>)}
        </select>
        {hasExtraFilter && (
          <button
            onClick={() => { onGenreChange(""); onYearChange(0); onArtistSelectChange(""); }}
            className="px-3 py-1.5 rounded-[var(--radius-sm)] text-xs border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer"
          >
            {t("media.music.resetFilters")}
          </button>
        )}
      </div>
      {loading && <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>}
      {!loading && isError && (
        // A failed track fetch must not collapse into "no tracks found" - that
        // reads as an empty library when the server actually errored.
        <EmptyState
          icon={<TriangleAlert size={48} strokeWidth={1.5} />}
          title={classifyError(error) === "offline" ? t("common.offline.title", "Offline") : t("common.errorBoundary.title")}
          description={
            classifyError(error) === "offline"
              ? t("common.offline.description", "Check your internet connection and try again.")
              : t("common.errorBoundary.description")
          }
          action={onRetry ? { label: t("common.retry", "Retry"), onClick: onRetry } : undefined}
        />
      )}
      {!loading && !isError && !tracks.length && <p className="text-[var(--text-muted)]">{search ? t("media.music.noTracksSearch") : t("media.music.noTracks")}</p>}
      {!loading && !isError && tracks.length > 0 && <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="text-[var(--text-muted)] text-left border-b border-[var(--border)]">
            <th className="px-3 py-2">#</th>
            <th className="px-3 py-2"></th>
            <th className="px-3 py-2">{t("media.library.titleSort")}</th>
            <th className="px-3 py-2">{t("media.music.artists")}</th>
            <th className="px-3 py-2">{t("media.music.albums")}</th>
            <th className="px-3 py-2 text-right">{t("media.library.duration")}</th>
            <th className="px-3 py-2"></th>
          </tr>
        </thead>
        <tbody>
          {pageTrack.map((track, i) => {
            const isActive = playing?.ID === track.ID;
            return (
              <tr
                key={track.ID}
                onClick={() => onPlay(track, tracks)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onPlay(track, tracks);
                  }
                }}
                role="button"
                tabIndex={0}
                aria-label={`${track.Title || t("media.music.trackCount", { count: 1 })}${track.Artist ? " - " + track.Artist : ""}`}
                className={`cursor-pointer border-b border-[var(--border)] hover:bg-[var(--surface-raised)] focus:outline focus:outline-2 focus:outline-[var(--accent)] ${isActive ? "bg-[var(--surface-raised)]" : ""}`}
              >
                <td className="px-3 py-2 text-[var(--text-muted)]">{page * TRACKS_PAGE_SIZE + i + 1}</td>
                <td className="px-1 py-2">
                  <TrackArt trackID={track.ID} coverArtURL={track.CoverArtURL} size={36} />
                </td>
                <td className={`px-3 py-2 font-medium ${isActive ? "text-[var(--accent)]" : "text-[var(--text)]"}`}>{track.Title || "-"}</td>
                <td className="px-3 py-2 text-[var(--text-secondary)]">{track.Artist || "-"}</td>
                <td className="px-3 py-2 text-[var(--text-secondary)]">{track.Album || "-"}</td>
                <td className="px-3 py-2 text-[var(--text-muted)] text-right">
                  {track.Duration > 0 ? formatDuration(track.Duration) : "-"}
                </td>
                <td className="px-3 py-2 text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                  <DownloadTrackButton trackID={track.ID} />
                  <AddToPlaylistButton trackID={track.ID} />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>}
      {!loading && tracks.length > 0 && totalPages > 1 && (
        <div className="flex items-center justify-between mt-4 text-[var(--text-muted)] text-sm">
          <button
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0}
            className="px-4 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] disabled:text-[var(--text-muted)] disabled:cursor-default disabled:opacity-50 cursor-pointer"
          >
            {t("common.previous")}
          </button>
          <span>
            {page + 1} / {totalPages} · {t("media.music.trackCount", { count: tracks.length })}
          </span>
          <button
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={page === totalPages - 1}
            className="px-4 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] disabled:text-[var(--text-muted)] disabled:cursor-default disabled:opacity-50 cursor-pointer"
          >
            {t("common.next")}
          </button>
        </div>
      )}
    </div>
  );
}
