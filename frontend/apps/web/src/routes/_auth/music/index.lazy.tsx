import { useState, useMemo, useEffect, useRef } from "react";
import { Virtuoso } from "react-virtuoso";
import { MediaGrid } from "@/features/library/MediaGrid";
import { useDebounce } from "@/hooks/useDebounce";
import { createLazyFileRoute } from "@tanstack/react-router";
import {
  useLibraryServiceListLibraries,
  useMusicServiceListTracks,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
  usePlaylistServiceListPlaylists,
  usePlaylistServiceCreatePlaylist,
  usePlaylistServiceAddPlaylistItem,
  usePlaylistServiceGetPlaylistItems,
  usePlaylistServiceRemovePlaylistItem,
  usePlaylistServiceDeletePlaylist,
  getPlaylistServiceListPlaylistsQueryKey,
  getPlaylistServiceGetPlaylistItemsQueryKey,
  V1PlaylistType,
} from "@evey/api";
import { useAuthStore, useMusicPlayerStore, toast, type MusicSource } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { startTokenDownload } from "@/utils/downloadToken";
import { Music, X, Plus, Trash2, ArrowLeft, Download, Shuffle, Sparkles, TriangleAlert } from "lucide-react";
import { useGeneratePlaylist } from "@/features/playlist/api";
import { ICON } from "@/constants/iconSizes";
import { EmptyState } from "@/components/ui/EmptyState";
import { classifyError } from "@/utils/errorClassification";

export const Route = createLazyFileRoute("/_auth/music/")({
  component: MusicPage,
});

type Track = {
  ID: string;
  Title: string;
  Artist: string;
  Album: string;
  AlbumArtist: string;
  Year: number;
  TrackNumber: number;
  DiskNumber: number;
  Duration: number;
  FilePath: string;
  MimeType: string;
  Genre: string;
  CoverArtURL: string;
};

type Album = {
  ID: string;
  Title: string;
  Artist: string;
  Year: number;
  CoverArtURL: string;
  TrackCount: number;
  Duration: number;
};

type Artist = {
  ID: string;
  Name: string;
  TrackCount: number;
  AlbumCount: number;
  CoverArtURL: string;
};

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${String(s).padStart(2, "0")}`;
}

function TrackArt({ trackID, coverArtURL, size = 36 }: { trackID: string; coverArtURL?: string; size?: number }) {
  const [error, setError] = useState(false);
  // Pin token at trackID-change time: subscribing would reload the <img>
  // on every token refresh (15 min), cache-invalidating every visible cover.
  const src = useMemo(() => {
    const tok = useAuthStore.getState().accessToken;
    const tp = tok ? `?token=${encodeURIComponent(tok)}` : "";
    return `/api/v1/music/tracks/${trackID}/art${tp}`;
  }, [trackID]);
  if (error || !coverArtURL) {
    return (
      <div
        className="flex items-center justify-center bg-[var(--surface)] rounded-[var(--radius-sm)]"
        style={{
          width: size,
          height: size,
          minWidth: size,
        }}
      >
        <Music size={size * 0.5} className="text-[var(--text-muted)] opacity-50" />
      </div>
    );
  }
  return (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      width={size}
      height={size}
      className="object-cover rounded-[var(--radius-sm)] bg-[var(--surface)]"
      style={{
        width: size,
        height: size,
        minWidth: size,
      }}
      onError={() => setError(true)}
    />
  );
}

// shuffleTracks returns a new array in random order (Fisher-Yates). Used by the
// ephemeral "shuffle play" - builds a randomized queue without persisting it.
function shuffleTracks<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

function useMusicLibraries() {
  return useLibraryServiceListLibraries({
    query: {
      select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC").map<LibraryItem>((l) => ({ id: l.id ?? "", name: l.name ?? "", type: l.type ?? "" })),
      staleTime: 60_000,
    },
  });
}

function useMusicTracks(libraryID: string | undefined, search: string, albumId?: string) {
  return useMusicServiceListTracks(
    // When albumId is set the server returns that album's full track set and
    // ignores search/limit; the in-tab search box then filters it client-side.
    { libraryId: libraryID, limit: 200, search: albumId ? undefined : (search || undefined), albumId: albumId || undefined },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.tracks ?? []).map<Track>((t) => ({
          ID: t.id ?? "",
          Title: t.title ?? "",
          Artist: t.artist ?? "",
          Album: t.album ?? "",
          AlbumArtist: t.albumArtist ?? "",
          Year: t.year ?? 0,
          TrackNumber: t.trackNumber ?? 0,
          DiskNumber: t.diskNumber ?? 0,
          Duration: t.duration ?? 0,
          FilePath: t.filePath ?? "",
          MimeType: t.mimeType ?? "",
          Genre: t.genre ?? "",
          CoverArtURL: t.coverArtUrl ?? "",
        })),
        staleTime: 60_000,
      },
    },
  );
}

function useMusicAlbums(libraryID: string | undefined) {
  return useMusicServiceGetAlbums(
    { libraryId: libraryID },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.albums ?? []).map<Album>((a) => ({
          ID: a.id ?? "",
          Title: a.title ?? "",
          Artist: a.artist ?? "",
          Year: a.year ?? 0,
          CoverArtURL: a.coverArtUrl ?? "",
          TrackCount: a.trackCount ?? 0,
          Duration: a.duration ?? 0,
        })),
        staleTime: 60_000,
      },
    },
  );
}

function useMusicArtists(libraryID: string | undefined) {
  return useMusicServiceGetArtists(
    { libraryId: libraryID },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.artists ?? []).map<Artist>((a) => ({
          ID: a.id ?? "",
          Name: a.name ?? "",
          TrackCount: a.trackCount ?? 0,
          AlbumCount: a.albumCount ?? 0,
          CoverArtURL: a.coverArtUrl ?? "",
        })),
        staleTime: 60_000,
      },
    },
  );
}

type Tab = "albums" | "tracks" | "artists" | "playlists";


function DownloadTrackButton({ trackID }: { trackID: string }) {
  const { t } = useTranslation();
  async function download() {
    // Click-time mint: the account JWT must not ride in the URL (proxy logs,
    // browser history) - see utils/downloadToken.ts.
    try {
      await startTokenDownload(`/api/v1/music/tracks/${encodeURIComponent(trackID)}/download`, trackID);
    } catch {
      toast.error(t("common.error"));
    }
  }
  return (
    <button
      type="button"
      onClick={download}
      aria-label={t("media.music.downloadTrack")}
      title={t("media.music.downloadTrack")}
      className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded mr-1"
    >
      <Download size={ICON.lg} />
    </button>
  );
}

function AddToPlaylistButton({ trackID }: { trackID: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();
  const { data: playlistsData } = usePlaylistServiceListPlaylists(undefined, {
    query: { enabled: open, staleTime: 30_000 },
  });
  const addItem = usePlaylistServiceAddPlaylistItem();
  const createPlaylist = usePlaylistServiceCreatePlaylist();

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const playlists = (playlistsData?.playlists ?? []).filter(
    (p) => p.type !== V1PlaylistType.PLAYLIST_TYPE_SMART,
  );

  async function addToPlaylist(playlistId: string) {
    try {
      await addItem.mutateAsync({
        id: playlistId,
        data: { mediaItemId: trackID, mediaType: "music" },
      });
      toast.success(t("media.music.addedToPlaylist"));
    } catch (err) {
      console.warn("add to playlist failed", err);
      toast.error(t("media.playlist.addError"));
    }
    setOpen(false);
  }

  async function createAndAdd() {
    const name = window.prompt(t("media.music.createPlaylistPrompt"));
    if (!name || !name.trim()) {
      setOpen(false);
      return;
    }
    try {
      const created = await createPlaylist.mutateAsync({
        data: { name: name.trim(), type: V1PlaylistType.PLAYLIST_TYPE_MANUAL },
      });
      await queryClient.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() });
      toast.success(t("media.music.playlistCreated"));
      if (created?.id) {
        await addItem.mutateAsync({
          id: created.id,
          data: { mediaItemId: trackID, mediaType: "music" },
        });
        toast.success(t("media.music.addedToPlaylist"));
      }
    } catch (err) {
      // The success toast for the creation has already been shown by the time
      // the add can fail, so silence here read as "the track is in it".
      console.warn("create + add failed", err);
      toast.error(t("media.playlist.addError"));
    }
    setOpen(false);
  }

  return (
    <div className="relative inline-block" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={t("media.music.addToPlaylist")}
        title={t("media.music.addToPlaylist")}
        className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded"
      >
        <Plus size={ICON.lg} />
      </button>
      {open && (
        <div
          className="absolute right-0 mt-1 z-popover min-w-[200px] max-h-[300px] overflow-y-auto rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] shadow-lg"
          role="menu"
        >
          {playlists.length === 0 && (
            <div className="px-3 py-2 text-xs text-[var(--text-muted)]">
              {t("media.music.addToPlaylist")}
            </div>
          )}
          {playlists.map((p) => (
            <button
              key={p.id}
              type="button"
              role="menuitem"
              onClick={() => p.id && addToPlaylist(p.id)}
              className="w-full text-left px-3 py-2 text-sm bg-transparent border-none cursor-pointer text-[var(--text)] hover:bg-[var(--surface)]"
            >
              {p.name || "-"}
            </button>
          ))}
          <button
            type="button"
            role="menuitem"
            onClick={createAndAdd}
            className="w-full text-left px-3 py-2 text-sm border-t border-[var(--border)] bg-transparent border-l-0 border-r-0 border-b-0 cursor-pointer text-[var(--accent)] hover:bg-[var(--surface)]"
          >
            {t("media.music.newPlaylist")}
          </button>
        </div>
      )}
    </div>
  );
}

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

function PlaylistsTab({
  allTracks,
  onPlay,
  libraryID,
}: {
  allTracks: Track[];
  onPlay: (tr: Track, all: Track[], source?: MusicSource) => void;
  libraryID?: string;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const accessToken = useAuthStore((s) => s.accessToken);
  const [selected, setSelected] = useState<string | null>(null);
  const generate = useGeneratePlaylist();

  function handleGenerate() {
    if (!libraryID || generate.isPending) return;
    generate.mutate(
      { libraryId: libraryID, name: t("media.playlist.generatedName") },
      {
        onSuccess: (res) => {
          toast.success(t("media.playlist.generated"));
          if (res?.id) setSelected(res.id);
        },
        onError: () => toast.error(t("media.playlist.generateFailed")),
      },
    );
  }
  const { data: playlistsData, isLoading: listLoading } = usePlaylistServiceListPlaylists();
  const { data: itemsData, isLoading: itemsLoading } = usePlaylistServiceGetPlaylistItems(
    selected ?? "",
    undefined,
    { query: { enabled: !!selected, staleTime: 15_000 } },
  );
  const removeItem = usePlaylistServiceRemovePlaylistItem();
  const deletePlaylist = usePlaylistServiceDeletePlaylist();

  const playlists = (playlistsData?.playlists ?? []).filter(
    (p) => p.type !== V1PlaylistType.PLAYLIST_TYPE_SMART,
  );
  const selectedPlaylist = playlists.find((p) => p.id === selected);
  const playlistSource: MusicSource | undefined = selectedPlaylist?.id
    ? { type: "playlist", id: selectedPlaylist.id, label: selectedPlaylist.name ?? selectedPlaylist.id }
    : undefined;

  const tracksByID = useMemo(() => {
    const m = new Map<string, Track>();
    allTracks.forEach((tr) => m.set(tr.ID, tr));
    return m;
  }, [allTracks]);

  const playlistTracks: Track[] = (itemsData?.items ?? [])
    .map((it) => tracksByID.get(it.mediaItemId ?? ""))
    .filter((x): x is Track => !!x);

  async function handleRemove(trackID: string) {
    if (!selected) return;
    try {
      await removeItem.mutateAsync({ id: selected, mediaItemId: trackID });
      await queryClient.invalidateQueries({
        queryKey: getPlaylistServiceGetPlaylistItemsQueryKey(selected),
      });
    } catch (err) {
      console.warn("remove playlist item failed", err);
      toast.error(t("media.playlist.failedRemove"));
    }
  }

  async function handleDeletePlaylist(pid: string) {
    try {
      await deletePlaylist.mutateAsync({ id: pid });
      await queryClient.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() });
      setSelected(null);
    } catch (err) {
      console.warn("delete playlist failed", err);
      toast.error(t("media.playlist.deleteError"));
    }
  }

  if (selected) {
    return (
      <div>
        <div className="flex items-center gap-3 mb-4">
          <button
            type="button"
            onClick={() => setSelected(null)}
            className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded"
            aria-label={t("common.back")}
          >
            <ArrowLeft size={ICON.lg} />
          </button>
          <h2 className="text-lg font-semibold text-[var(--text)]">{selectedPlaylist?.name ?? ""}</h2>
          {playlistTracks.length > 0 && (
            <button
              type="button"
              onClick={() => { const shuffled = shuffleTracks(playlistTracks); onPlay(shuffled[0], shuffled, playlistSource); }}
              className="ml-auto flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer"
            >
              <Shuffle size={ICON.sm} />
              {t("media.music.shuffle")}
            </button>
          )}
          {selected && playlistTracks.length > 0 && accessToken && (
            <a
              href="#"
              onClick={async (e) => {
                e.preventDefault();
                // Click-time mint - the JWT must not ride in the URL.
                try {
                  await startTokenDownload(`/api/v1/playlists/${selected}/download`, selected, `${selectedPlaylist?.name ?? "playlist"}.zip`);
                } catch {
                  toast.error(t("common.error"));
                }
              }}
              data-testid="playlist-download-btn"
              className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-muted)] hover:bg-[var(--surface-raised)] transition-colors-fast ${playlistTracks.length > 0 ? "" : "ml-auto"}`}
              title={t("media.playlist.download")}
            >
              <Download size={ICON.sm} />
              {t("media.playlist.download")}
            </a>
          )}
        </div>
        {itemsLoading ? (
          <p className="text-[var(--text-muted)] text-sm">{t("media.music.loading")}</p>
        ) : playlistTracks.length === 0 ? (
          <p className="text-[var(--text-muted)] text-sm">{t("media.music.emptyPlaylist")}</p>
        ) : (
          <div className="space-y-1">
            {playlistTracks.map((tr, i) => (
              <div
                key={`${tr.ID}-${i}`}
                className="flex items-center gap-3 p-2 rounded-[var(--radius-sm)] hover:bg-[var(--surface)]"
              >
                <button
                  type="button"
                  onClick={() => onPlay(tr, playlistTracks, playlistSource)}
                  className="flex items-center gap-3 flex-1 min-w-0 bg-transparent border-none cursor-pointer text-left p-0"
                >
                  <TrackArt trackID={tr.ID} coverArtURL={tr.CoverArtURL} />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-[var(--text)] truncate">{tr.Title}</p>
                    <p className="text-xs text-[var(--text-muted)] truncate">{tr.Artist}</p>
                  </div>
                  <span className="text-xs text-[var(--text-muted)]">{formatDuration(tr.Duration)}</span>
                </button>
                <button
                  type="button"
                  onClick={() => handleRemove(tr.ID)}
                  aria-label={t("common.delete")}
                  className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--color-danger)] p-1 rounded"
                >
                  <Trash2 size={ICON.md} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  }

  if (listLoading) {
    return <p className="text-[var(--text-muted)] text-sm">{t("media.music.loading")}</p>;
  }
  return (
    <div className="space-y-2">
      {libraryID && (
        <div className="flex justify-end">
          <button
            type="button"
            onClick={handleGenerate}
            disabled={generate.isPending}
            data-testid="generate-playlist-btn"
            className="inline-flex items-center gap-2 px-4 py-1.5 rounded-[var(--radius-sm)] text-sm font-medium bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer disabled:opacity-50 disabled:cursor-default"
          >
            <Sparkles size={ICON.md} aria-hidden="true" />
            {generate.isPending ? t("media.playlist.generating") : t("media.playlist.generate")}
          </button>
        </div>
      )}
      {playlists.length === 0 && (
        <p className="text-[var(--text-muted)] text-sm">{t("media.music.noPlaylists")}</p>
      )}
      {playlists.map((p) => (
        <div
          key={p.id}
          className="flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] hover:bg-[var(--surface)]"
        >
          <button
            type="button"
            onClick={() => p.id && setSelected(p.id)}
            className="flex-1 text-left bg-transparent border-none cursor-pointer p-0"
          >
            <p className="font-medium text-[var(--text)]">{p.name || "-"}</p>
            {p.description && (
              <p className="text-xs text-[var(--text-muted)] truncate">{p.description}</p>
            )}
          </button>
          <button
            type="button"
            onClick={() => p.id && confirm(t("media.playlist.deleteConfirm", { name: p.name ?? "" })) && handleDeletePlaylist(p.id)}
            aria-label={t("common.delete")}
            className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--color-danger)] p-1 rounded"
          >
            <Trash2 size={ICON.md} />
          </button>
        </div>
      ))}
    </div>
  );
}

const TRACKS_PAGE_SIZE = 50;

function TracksTab({
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

function AlbumsTab({
  albums,
  loading,
  isError,
  error,
  onRetry,
  libraryID,
  onAlbumClick,
}: {
  albums: Album[];
  loading: boolean;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
  libraryID: string | undefined;
  onAlbumClick: (a: Album) => void;
}) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  // A failed album fetch must not collapse into "no albums found" - that reads
  // as an empty library when the server actually errored.
  if (!loading && isError) {
    return (
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
    );
  }
  if (!loading && !albums.length) return <p className="text-[var(--text-muted)]">{t("media.music.noAlbums")}</p>;

  async function downloadAlbum(a: Album) {
    if (!libraryID) return;
    // Click-time mint bound to the albumId (the server guard binds on
    // albumId+album, name empty here) - the JWT must not ride in the URL.
    try {
      await startTokenDownload(`/api/v1/music/albums/download?albumId=${encodeURIComponent(a.ID)}&libraryId=${encodeURIComponent(libraryID)}`, a.ID);
    } catch {
      toast.error(t("common.error"));
    }
  }

  return (
    <MediaGrid isLoading={loading}>
      {albums.map((a) => (
        <div
          key={a.ID}
          // Largeur FIXE, calée sur MediaCard (w-40 sm:w-48 = les minima des
          // pistes de MediaGrid). MediaGrid pose `justify-items-start` pour
          // aligner ces cartes à largeur fixe : sans largeur déclarée, la carte
          // se dimensionne sur son contenu, donc sur la longueur du titre, et la
          // pochette en aspect-square suivait - titre long = vignette géante,
          // titre court = vignette minuscule.
          className="evey-album-card relative w-40 sm:w-48 rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] border border-[var(--border)] hover:border-[var(--accent)] transition-colors-fast group"
        >
          <div
            role="button"
            tabIndex={0}
            onClick={() => onAlbumClick(a)}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onAlbumClick(a); } }}
            className="cursor-pointer"
          >
            <div
              className="w-full flex items-center justify-center bg-[var(--surface)] bg-cover bg-center aspect-square"
              style={{
                backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL}${tokenParam})` : undefined,
              }}
            >
              {!a.CoverArtURL && (
                <Music size={ICON["2xl"]} className="opacity-30 text-[var(--text-muted)]" />
              )}
            </div>
            <div className="p-2">
              <p className="font-medium text-sm truncate text-[var(--text)]">{a.Title || "-"}</p>
              <p className="text-xs truncate text-[var(--text-muted)]">{a.Artist || "-"}</p>
              <p className="text-xs text-[var(--text-muted)]">
                {t("media.music.trackCount", { count: a.TrackCount })}
                {a.Year > 0 ? ` · ${a.Year}` : ""}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); downloadAlbum(a); }}
            aria-label={t("media.music.downloadAlbum")}
            title={t("media.music.downloadAlbum")}
            className="absolute top-2 right-2 p-1.5 rounded-full bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--accent)] cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity-fast"
          >
            <Download size={ICON.sm} />
          </button>
        </div>
      ))}
    </MediaGrid>
  );
}

function ArtistsTab({
  artists,
  loading,
  isError,
  error,
  onRetry,
  onArtistClick,
}: {
  artists: Artist[];
  loading: boolean;
  isError?: boolean;
  error?: unknown;
  onRetry?: () => void;
  onArtistClick: (a: Artist) => void;
}) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  // Mirror the scroll-parent pattern from MediaGrid so Virtuoso's endReached
  // fires correctly inside AuthLayout's overflow-y-auto #main-content region.
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  useEffect(() => { setScrollParent(document.getElementById("main-content")); }, []);

  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  // A failed artist fetch must not collapse into "no artists found" - that
  // reads as an empty library when the server actually errored.
  if (isError) {
    return (
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
    );
  }
  if (!artists.length) return <p className="text-[var(--text-muted)]">{t("media.music.noArtists")}</p>;

  return (
    <Virtuoso
      customScrollParent={scrollParent ?? undefined}
      totalCount={artists.length}
      overscan={400}
      itemContent={(index) => {
        const a = artists[index];
        return (
          <div
            onClick={() => onArtistClick(a)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onArtistClick(a); } }}
            className="flex items-center justify-between px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] cursor-pointer hover:border-[var(--accent)] transition-colors-fast mb-1"
          >
            <div className="flex items-center gap-3 min-w-0">
              <div
                className="w-12 h-12 rounded-full bg-cover bg-center bg-[var(--surface)] flex items-center justify-center flex-shrink-0"
                style={{ backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL}${tokenParam})` : undefined }}
              >
                {!a.CoverArtURL && <Music size={ICON.lg} className="opacity-30 text-[var(--text-muted)]" />}
              </div>
              <span className="font-medium text-[var(--text)] truncate">{a.Name}</span>
            </div>
            <span className="text-sm text-[var(--text-muted)]">
              {t("media.music.albumCount", { count: a.AlbumCount })} · {t("media.music.trackCount", { count: a.TrackCount })}
            </span>
          </div>
        );
      }}
    />
  );
}
