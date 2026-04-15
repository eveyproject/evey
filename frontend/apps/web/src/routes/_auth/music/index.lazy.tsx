import { useState, useMemo, useRef, useEffect } from "react";
import { useDebounce } from "@/hooks/useDebounce";
import { createLazyFileRoute } from "@tanstack/react-router";
import {
  useLibraryServiceListLibraries,
  useMusicServiceListTracks,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Music, Play, Pause, SkipBack, SkipForward, X, Volume2, Shuffle } from "lucide-react";

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
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
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
      src={`/api/v1/music/tracks/${trackID}/art${tokenParam}`}
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

function useMusicLibraries() {
  return useLibraryServiceListLibraries({
    query: {
      select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC").map<LibraryItem>((l) => ({ id: l.id ?? "", name: l.name ?? "", type: l.type ?? "" })),
      staleTime: 60_000,
    },
  });
}

function useMusicTracks(libraryID: string | undefined, search: string) {
  return useMusicServiceListTracks(
    { libraryId: libraryID, limit: 200, search: search || undefined },
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

type Tab = "tracks" | "albums" | "artists";

function NowPlayingBar({
  playing,
  queue,
  shuffle,
  onToggleShuffle,
  onClose,
  onSelectTrack,
}: {
  playing: Track;
  queue: Track[];
  shuffle: boolean;
  onToggleShuffle: () => void;
  onClose: () => void;
  onSelectTrack: (t: Track) => void;
}) {
  const { t } = useTranslation();
  const audioRef = useRef<HTMLAudioElement>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";

  const currentIndex = queue.findIndex((t) => t.ID === playing.ID);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    const tryPlay = async () => {
      try {
        await audio.play();
        setIsPlaying(true);
      } catch (e: unknown) {
        setIsPlaying(false);
        if (e instanceof DOMException && e.name !== "AbortError") console.warn("Autoplay blocked:", e.message);
      }
    };
    tryPlay();
  }, [playing.ID]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume;
  }, [volume]);

  async function togglePlay() {
    const audio = audioRef.current;
    if (!audio) return;
    if (isPlaying) {
      audio.pause();
      setIsPlaying(false);
    } else {
      try {
        await audio.play();
        setIsPlaying(true);
      } catch (e: unknown) {
        if (e instanceof DOMException && e.name !== "AbortError") console.warn("Autoplay blocked:", e.message);
      }
    }
  }

  function seek(e: React.ChangeEvent<HTMLInputElement>) {
    const audio = audioRef.current;
    if (!audio) return;
    const val = Number(e.target.value);
    audio.currentTime = val;
    setCurrentTime(val);
  }

  function pickRandom(): Track | null {
    if (queue.length <= 1) return null;
    const others = queue.filter((t) => t.ID !== playing.ID);
    if (!others.length) return null;
    return others[Math.floor(Math.random() * others.length)];
  }

  function playPrev() {
    if (shuffle) {
      const t = pickRandom();
      if (t) onSelectTrack(t);
      return;
    }
    if (currentIndex > 0) onSelectTrack(queue[currentIndex - 1]);
  }

  function playNext() {
    if (shuffle) {
      const t = pickRandom();
      if (t) onSelectTrack(t);
      return;
    }
    if (currentIndex < queue.length - 1) onSelectTrack(queue[currentIndex + 1]);
  }

  return (
    <div
      className="fixed bottom-0 left-0 right-0 flex items-center gap-2 sm:gap-4 px-3 sm:px-6 py-3 bg-[var(--surface)] border-t border-[var(--border)] z-overlay"
    >
      <audio
        ref={audioRef}
        key={playing.ID}
        src={`/api/v1/music/tracks/${playing.ID}/stream${tokenParam}`}
        onTimeUpdate={() => setCurrentTime(audioRef.current?.currentTime ?? 0)}
        onLoadedMetadata={() => setDuration(audioRef.current?.duration ?? 0)}
        onEnded={playNext}
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
      />

      {/* Album art + info */}
      <div className="flex items-center gap-3 min-w-0 w-[120px] sm:w-[200px]">
        <TrackArt trackID={playing.ID} coverArtURL={playing.CoverArtURL} size={48} />
        <div className="min-w-0">
          <p className="font-medium truncate text-sm text-[var(--text)]">{playing.Title || "—"}</p>
          <p className="text-xs truncate text-[var(--text-muted)]">{playing.Artist || "—"}</p>
        </div>
      </div>

      {/* Controls */}
      <div className="flex flex-col items-center gap-1 flex-1">
        <div className="flex items-center gap-4">
          <button
            onClick={onToggleShuffle}
            className={`bg-transparent border-none cursor-pointer ${shuffle ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}
            aria-label={t("media.music.shuffle")}
            aria-pressed={shuffle}
            title={t("media.music.shuffle")}
          >
            <Shuffle size={16} />
          </button>
          <button
            onClick={playPrev}
            disabled={!shuffle && currentIndex <= 0}
            className="bg-transparent border-none text-[var(--text)] disabled:opacity-30 disabled:cursor-default cursor-pointer"
            aria-label="Previous"
          >
            <SkipBack size={18} />
          </button>
          <button
            onClick={togglePlay}
            className="bg-[var(--accent)] border-none cursor-pointer text-[var(--filter-active-text)] rounded-full w-9 h-9 flex items-center justify-center"
            aria-label={isPlaying ? "Pause" : "Play"}
          >
            {isPlaying ? <Pause size={16} /> : <Play size={16} />}
          </button>
          <button
            onClick={playNext}
            disabled={!shuffle && currentIndex >= queue.length - 1}
            className="bg-transparent border-none text-[var(--text)] disabled:opacity-30 disabled:cursor-default cursor-pointer"
            aria-label="Next"
          >
            <SkipForward size={18} />
          </button>
        </div>
        <div className="flex items-center gap-2 w-full max-w-[200px] sm:max-w-[400px]">
          <span className="text-[0.7rem] text-[var(--text-muted)] min-w-8 text-right">
            {formatDuration(currentTime)}
          </span>
          <input
            type="range"
            min={0}
            max={duration || 0}
            step={1}
            value={currentTime}
            onChange={seek}
            className="flex-1 accent-[var(--accent)]"
          />
          <span className="text-[0.7rem] text-[var(--text-muted)] min-w-8">
            {duration > 0 ? formatDuration(duration) : "—"}
          </span>
        </div>
      </div>

      {/* Volume */}
      <div className="hidden sm:flex items-center gap-2 min-w-[100px]">
        <Volume2 size={16} className="text-[var(--text-muted)]" />
        <input
          type="range"
          min={0}
          max={1}
          step={0.02}
          value={volume}
          onChange={(e) => setVolume(Number(e.target.value))}
          className="w-[72px] accent-[var(--accent)]"
        />
      </div>

      <button
        onClick={onClose}
        className="text-[var(--text-muted)] bg-transparent border-none cursor-pointer"
        aria-label="Close player"
      >
        <X size={18} />
      </button>
    </div>
  );
}

function MusicPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("tracks");
  const [selectedLibraryID, setSelectedLibraryID] = useState<string | undefined>(undefined);
  const [playing, setPlaying] = useState<Track | null>(null);
  const [queue, setQueue] = useState<Track[]>([]);
  const [searchInput, setSearchInput] = useState("");
  const [albumFilter, setAlbumFilter] = useState<string | null>(null);
  const [artistFilter, setArtistFilter] = useState<string | null>(null);
  const [genreFilter, setGenreFilter] = useState<string>("");
  const [yearFilter, setYearFilter] = useState<number>(0);
  const [artistSelectFilter, setArtistSelectFilter] = useState<string>("");
  const [shuffle, setShuffle] = useState<boolean>(() => {
    try { return localStorage.getItem("evey-music-shuffle") === "1"; } catch { return false; }
  });
  useEffect(() => {
    try { localStorage.setItem("evey-music-shuffle", shuffle ? "1" : "0"); } catch { /* ignore */ }
  }, [shuffle]);
  const search = useDebounce(searchInput, 300);

  const { data: libraries, isLoading: libLoading } = useMusicLibraries();

  const firstLib = libraries?.[0];
  const activeLibraryID = selectedLibraryID ?? firstLib?.id;

  const { data: tracks, isLoading: tracksLoading } = useMusicTracks(activeLibraryID, search);
  const { data: albums, isLoading: albumsLoading } = useMusicAlbums(activeLibraryID);
  const { data: artists, isLoading: artistsLoading } = useMusicArtists(activeLibraryID);

  function handlePlay(t: Track, allTracks: Track[]) {
    setQueue(allTracks);
    setPlaying(t);
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
            onChange={(e) => setSelectedLibraryID(e.target.value)}
            className="text-sm px-3 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer"
          >
            {libraries.map((lib) => (
              <option key={lib.id} value={lib.id}>{lib.name}</option>
            ))}
          </select>
        )}
      </div>

      <div className="flex gap-2 mb-6">
        {(["tracks", "albums", "artists"] as Tab[]).map((tabKey) => (
          <button key={tabKey} className={tabClass(tab === tabKey)} onClick={() => setTab(tabKey)}>
            {t(`media.music.${tabKey}`)}
          </button>
        ))}
      </div>

      {tab === "tracks" && (
        <TracksTab
          allTracks={tracks ?? []}
          loading={tracksLoading}
          playing={playing}
          onPlay={(tr, filtered) => handlePlay(tr, filtered)}
          search={searchInput}
          onSearchChange={setSearchInput}
          albumFilter={albumFilter}
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
          onAlbumClick={(a) => { setAlbumFilter(a.Title); setArtistFilter(null); setTab("tracks"); }}
        />
      )}
      {tab === "artists" && (
        <ArtistsTab
          artists={artists ?? []}
          loading={artistsLoading}
          onArtistClick={(a) => { setArtistFilter(a.Name); setAlbumFilter(null); setTab("tracks"); }}
        />
      )}

      {playing && (
        <NowPlayingBar
          playing={playing}
          queue={queue}
          shuffle={shuffle}
          onToggleShuffle={() => setShuffle((s) => !s)}
          onClose={() => setPlaying(null)}
          onSelectTrack={setPlaying}
        />
      )}
    </div>
  );
}

const TRACKS_PAGE_SIZE = 50;

function TracksTab({
  allTracks,
  loading,
  playing,
  onPlay,
  search,
  onSearchChange,
  albumFilter,
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
  playing: Track | null;
  onPlay: (t: Track, filtered: Track[]) => void;
  search: string;
  onSearchChange: (v: string) => void;
  albumFilter: string | null;
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

  const tracks = useMemo(
    () => allTracks.filter((x) => {
      if (albumFilter && x.Album !== albumFilter) return false;
      if (artistFilter && x.Artist !== artistFilter && x.AlbumArtist !== artistFilter) return false;
      if (genreFilter && x.Genre !== genreFilter) return false;
      if (yearFilter && x.Year !== yearFilter) return false;
      if (artistSelectFilter && x.Artist !== artistSelectFilter && x.AlbumArtist !== artistSelectFilter) return false;
      return true;
    }),
    [allTracks, albumFilter, artistFilter, genreFilter, yearFilter, artistSelectFilter],
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
          value={search}
          onChange={(e) => { onSearchChange(e.target.value); setPage(0); }}
          className="w-full max-w-[400px] px-3 py-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] text-sm"
        />
        {albumFilter && (
          <button
            onClick={onClearAlbumFilter}
            className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs bg-[var(--accent)] text-[var(--filter-active-text)] cursor-pointer border-none"
            title={t("common.clear")}
          >
            {t("media.music.albums")}: {albumFilter} <X size={12} />
          </button>
        )}
        {artistFilter && (
          <button
            onClick={onClearArtistFilter}
            className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs bg-[var(--accent)] text-[var(--filter-active-text)] cursor-pointer border-none"
            title={t("common.clear")}
          >
            {t("media.music.artists")}: {artistFilter} <X size={12} />
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
      {!loading && !tracks.length && <p className="text-[var(--text-muted)]">{search ? t("media.music.noTracksSearch") : t("media.music.noTracks")}</p>}
      {!loading && tracks.length > 0 && <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="text-[var(--text-muted)] text-left border-b border-[var(--border)]">
            <th className="px-3 py-2">#</th>
            <th className="px-3 py-2"></th>
            <th className="px-3 py-2">{t("media.library.titleSort")}</th>
            <th className="px-3 py-2">{t("media.music.artists")}</th>
            <th className="px-3 py-2">{t("media.music.albums")}</th>
            <th className="px-3 py-2 text-right">{t("media.library.duration")}</th>
          </tr>
        </thead>
        <tbody>
          {pageTrack.map((track, i) => {
            const isActive = playing?.ID === track.ID;
            return (
              <tr
                key={track.ID}
                onClick={() => onPlay(track, tracks)}
                className={`cursor-pointer border-b border-[var(--border)] hover:bg-[var(--surface-raised)] ${isActive ? "bg-[var(--surface-raised)]" : ""}`}
              >
                <td className="px-3 py-2 text-[var(--text-muted)]">{page * TRACKS_PAGE_SIZE + i + 1}</td>
                <td className="px-1 py-2">
                  <TrackArt trackID={track.ID} coverArtURL={track.CoverArtURL} size={36} />
                </td>
                <td className={`px-3 py-2 font-medium ${isActive ? "text-[var(--accent)]" : "text-[var(--text)]"}`}>{track.Title || "—"}</td>
                <td className="px-3 py-2 text-[var(--text-secondary)]">{track.Artist || "—"}</td>
                <td className="px-3 py-2 text-[var(--text-secondary)]">{track.Album || "—"}</td>
                <td className="px-3 py-2 text-[var(--text-muted)] text-right">
                  {track.Duration > 0 ? formatDuration(track.Duration) : "—"}
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

function AlbumsTab({ albums, loading, onAlbumClick }: { albums: Album[]; loading: boolean; onAlbumClick: (a: Album) => void }) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  if (!albums.length) return <p className="text-[var(--text-muted)]">{t("media.music.noAlbums")}</p>;

  return (
    <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(160px,1fr))]">
      {albums.map((a) => (
        <div
          key={a.ID}
          onClick={() => onAlbumClick(a)}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onAlbumClick(a); } }}
          className="rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] border border-[var(--border)] cursor-pointer hover:border-[var(--accent)] transition-colors-fast"
        >
          <div
            className="w-full flex items-center justify-center bg-[var(--surface)] bg-cover bg-center aspect-square"
            style={{
              backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL}${tokenParam})` : undefined,
            }}
          >
            {!a.CoverArtURL && (
              <Music size={32} className="opacity-30 text-[var(--text-muted)]" />
            )}
          </div>
          <div className="p-2">
            <p className="font-medium text-sm truncate text-[var(--text)]">{a.Title || "—"}</p>
            <p className="text-xs truncate text-[var(--text-muted)]">{a.Artist || "—"}</p>
            <p className="text-xs text-[var(--text-muted)]">
              {t("media.music.trackCount", { count: a.TrackCount })}
              {a.Year > 0 ? ` · ${a.Year}` : ""}
            </p>
          </div>
        </div>
      ))}
    </div>
  );
}

function ArtistsTab({ artists, loading, onArtistClick }: { artists: Artist[]; loading: boolean; onArtistClick: (a: Artist) => void }) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  if (!artists.length) return <p className="text-[var(--text-muted)]">{t("media.music.noArtists")}</p>;

  return (
    <div className="flex flex-col gap-1">
      {artists.map((a) => (
        <div
          key={a.ID}
          onClick={() => onArtistClick(a)}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onArtistClick(a); } }}
          className="flex items-center justify-between px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] cursor-pointer hover:border-[var(--accent)] transition-colors-fast"
        >
          <div className="flex items-center gap-3 min-w-0">
            <div
              className="w-12 h-12 rounded-full bg-cover bg-center bg-[var(--surface)] flex items-center justify-center flex-shrink-0"
              style={{ backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL}${tokenParam})` : undefined }}
            >
              {!a.CoverArtURL && <Music size={20} className="opacity-30 text-[var(--text-muted)]" />}
            </div>
            <span className="font-medium text-[var(--text)] truncate">{a.Name}</span>
          </div>
          <span className="text-sm text-[var(--text-muted)]">
            {t("media.music.albumCount", { count: a.AlbumCount })} · {t("media.music.trackCount", { count: a.TrackCount })}
          </span>
        </div>
      ))}
    </div>
  );
}
