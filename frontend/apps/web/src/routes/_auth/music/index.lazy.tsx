import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useMusicPlayerStore, type MusicSource } from "@evey/store";
import { useTranslation } from "react-i18next";
import { useDebounce } from "@/hooks/useDebounce";
import { Shuffle, TriangleAlert } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { EmptyState } from "@/components/ui/EmptyState";
import { classifyError } from "@/utils/errorClassification";
import { shuffleTracks } from "@/features/music/musicFormat";
import { useMusicLibraries, useMusicTracks, useMusicAlbums, useMusicArtists, type Track } from "@/features/music/musicQueries";
import { TracksTab } from "@/features/music/TracksTab";
import { AlbumsTab } from "@/features/music/AlbumsTab";
import { ArtistsTab } from "@/features/music/ArtistsTab";
import { PlaylistsTab } from "@/features/music/PlaylistsTab";

export const Route = createLazyFileRoute("/_auth/music/")({
  component: MusicPage,
});

type Tab = "albums" | "tracks" | "artists" | "playlists";

function MusicPage() {
  const { t } = useTranslation();
  const urlSearch = Route.useSearch();
  const [tab, setTab] = useState<Tab>(urlSearch.tab ?? "albums");
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(urlSearch.libraryId);
  const playing = useMusicPlayerStore((s) => s.playing);
  const playInStore = useMusicPlayerStore((s) => s.play);
  const [searchInput, setSearchInput] = useState("");
  const [albumFilter, setAlbumFilter] = useState<string | null>(urlSearch.album ?? null);
  const [artistFilter, setArtistFilter] = useState<string | null>(urlSearch.artist ?? null);
  const [genreFilter, setGenreFilter] = useState<string>("");
  const [yearFilter, setYearFilter] = useState<number>(0);
  const [artistSelectFilter, setArtistSelectFilter] = useState<string>("");
  const search = useDebounce(searchInput, 300);

  const { data: libraries, isLoading: libLoading, isError: libError, error: libErrorObj, refetch: refetchLibraries } = useMusicLibraries();

  const firstLib = libraries?.[0];
  const activeLibraryID = selectedLibraryID ?? firstLib?.id;

  const { data: tracks, isLoading: tracksLoading, isError: tracksError, error: tracksErrorObj, refetch: refetchTracks } = useMusicTracks(activeLibraryID, search, albumFilter ?? undefined);
  const { data: albums, isLoading: albumsLoading, isError: albumsError, error: albumsErrorObj, refetch: refetchAlbums } = useMusicAlbums(activeLibraryID);
  const { data: artists, isLoading: artistsLoading, isError: artistsError, error: artistsErrorObj, refetch: refetchArtists } = useMusicArtists(activeLibraryID);

  // albumFilter holds the opaque album id; resolve a human label for the chip /
  // play-source from the loaded album list, falling back to a loaded track's
  // album name (covers a bookmarked URL before the album list arrives).
  const albumLabel = albumFilter
    ? (albums?.find((a) => a.ID === albumFilter)?.Title || tracks?.[0]?.Album || "")
    : "";

  function handlePlay(t: Track, allTracks: Track[], source?: MusicSource) {
    playInStore(t, allTracks, source);
  }

  // Ephemeral shuffle: randomize the loaded tracks and play - no saved playlist.
  function playRandom() {
    const pool = tracks ?? [];
    if (pool.length === 0) return;
    const shuffled = shuffleTracks(pool);
    handlePlay(shuffled[0], shuffled);
  }

  const tabClass = (active: boolean) =>
    `px-4 py-1.5 rounded-[var(--radius-sm)] border-none cursor-pointer text-sm transition-colors-fast ${
      active
        ? "font-semibold bg-[var(--accent)] text-[var(--filter-active-text)]"
        : "font-normal bg-transparent text-[var(--text-secondary)]"
    }`;

  if (libLoading) {
    return (
      <div className="flex-1 p-6">
        <div className="animate-pulse h-8 w-48 rounded mb-6 bg-[var(--surface-raised)]" />
      </div>
    );
  }

  if (libError) {
    // Without this branch a failed fetch fell through to "no libraries",
    // telling the user their music collection vanished instead of the server
    // being unreachable.
    return (
      <div className="flex-1 p-6">
        <EmptyState
          icon={<TriangleAlert size={48} strokeWidth={1.5} />}
          title={classifyError(libErrorObj) === "offline" ? t("common.offline.title", "Offline") : t("common.errorBoundary.title")}
          description={
            classifyError(libErrorObj) === "offline"
              ? t("common.offline.description", "Check your internet connection and try again.")
              : t("common.errorBoundary.description")
          }
          action={{ label: t("common.retry", "Retry"), onClick: () => void refetchLibraries() }}
        />
      </div>
    );
  }

  if (!libraries || libraries.length === 0) {
    return (
      <div className="flex-1 p-6">
        <p className="text-[var(--text-muted)]">{t("media.music.noLibraries")}</p>
      </div>
    );
  }

  return (
    <div className={`flex-1 p-6 ${playing ? "pb-[120px] sm:pb-[80px]" : ""}`}>
      <div className="flex flex-col sm:flex-row sm:items-center gap-4 mb-4">
        <h1 className="text-2xl font-bold text-[var(--text)]">
          {t("media.music.title")}
        </h1>

        {libraries.length > 1 && (
          <select
            value={activeLibraryID ?? ""}
            aria-label={t("media.library.selectLibrary")}
            onChange={(e) => setSelectedLibraryID(e.target.value)}
            className="text-sm px-3 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer"
          >
            {libraries.map((lib) => (
              <option key={lib.id} value={lib.id}>{lib.name}</option>
            ))}
          </select>
        )}

        {(tracks?.length ?? 0) > 0 && (
          <button
            type="button"
            onClick={playRandom}
            data-testid="music-shuffle-play"
            className="sm:ml-auto inline-flex items-center gap-2 px-4 py-1.5 rounded-[var(--radius-sm)] text-sm font-medium bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer"
          >
            <Shuffle size={ICON.md} aria-hidden="true" />
            {t("media.music.playRandom")}
          </button>
        )}
      </div>

      <div className="flex flex-wrap gap-2 mb-6">
        {(["albums", "tracks", "artists", "playlists"] as Tab[]).map((tabKey) => (
          <button key={tabKey} className={tabClass(tab === tabKey)} onClick={() => setTab(tabKey)}>
            {t(`media.music.${tabKey}`)}
          </button>
        ))}
      </div>

      {tab === "tracks" && (
        <TracksTab
          allTracks={tracks ?? []}
          loading={tracksLoading}
          isError={tracksError}
          error={tracksErrorObj}
          onRetry={() => void refetchTracks()}
          playing={playing}
          onPlay={(tr, filtered) =>
            handlePlay(
              tr,
              filtered,
              albumFilter
                ? { type: "album", id: albumFilter, label: albumLabel || albumFilter }
                : artistFilter
                  ? { type: "artist", id: artistFilter, label: artistFilter }
                  : undefined,
            )
          }
          search={searchInput}
          onSearchChange={setSearchInput}
          albumFilter={albumFilter}
          albumLabel={albumLabel}
          artistFilter={artistFilter}
          onClearAlbumFilter={() => setAlbumFilter(null)}
          onClearArtistFilter={() => setArtistFilter(null)}
          genreFilter={genreFilter}
          yearFilter={yearFilter}
          artistSelectFilter={artistSelectFilter}
          onGenreChange={setGenreFilter}
          onYearChange={setYearFilter}
          onArtistSelectChange={setArtistSelectFilter}
        />
      )}
      {tab === "albums" && (
        <AlbumsTab
          albums={albums ?? []}
          loading={albumsLoading}
          isError={albumsError}
          error={albumsErrorObj}
          onRetry={() => void refetchAlbums()}
          libraryID={activeLibraryID}
          onAlbumClick={(a) => { setAlbumFilter(a.ID); setArtistFilter(null); setTab("tracks"); }}
        />
      )}
      {tab === "artists" && (
        <ArtistsTab
          artists={artists ?? []}
          loading={artistsLoading}
          isError={artistsError}
          error={artistsErrorObj}
          onRetry={() => void refetchArtists()}
          onArtistClick={(a) => { setArtistFilter(a.Name); setAlbumFilter(null); setTab("tracks"); }}
        />
      )}
      {tab === "playlists" && (
        <PlaylistsTab allTracks={tracks ?? []} onPlay={(tr, all, source) => handlePlay(tr, all, source)} libraryID={activeLibraryID} />
      )}

      {/* NowPlayingBar is now rendered globally via AuthLayout's GlobalAudioPlayer */}
    </div>
  );
}
