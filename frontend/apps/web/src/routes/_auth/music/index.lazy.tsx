import { useState, useMemo, useRef, useEffect } from "react";
import { useDebounce } from "@/hooks/useDebounce";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { Music, Play, Pause, SkipBack, SkipForward, X, Volume2, Trash2 } from "lucide-react";

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

function TrackArt({ trackID, size = 36 }: { trackID: string; size?: number }) {
  const [error, setError] = useState(false);
  if (error) {
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
      src={`/api/v1/music/tracks/${trackID}/art`}
      alt=""
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
  return useQuery({
    queryKey: ["libraries", "music"],
    queryFn: () => eveyFetch<{ libraries: LibraryItem[] }>({ url: "/api/v1/libraries", method: "GET" }),
    select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC"),
    staleTime: 60_000,
  });
}

function useMusicTracks(libraryID: string | undefined, search: string) {
  return useQuery({
    queryKey: ["music", "tracks", libraryID, search],
    queryFn: () =>
      eveyFetch<{ tracks: Track[]; total: number }>({
        url: `/api/v1/music/tracks?library_id=${libraryID}&limit=200${search ? `&search=${encodeURIComponent(search)}` : ""}`,
        method: "GET",
      }),
    enabled: !!libraryID,
    select: (d) => d.tracks ?? [],
    staleTime: 60_000,
  });
}

function useMusicAlbums(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["music", "albums", libraryID],
    queryFn: () =>
      eveyFetch<{ albums: Album[] }>({
        url: `/api/v1/music/albums?library_id=${libraryID}`,
        method: "GET",
      }),
    enabled: !!libraryID,
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
}

function useMusicArtists(libraryID: string | undefined) {
  return useQuery({
    queryKey: ["music", "artists", libraryID],
    queryFn: () =>
      eveyFetch<{ artists: Artist[] }>({
        url: `/api/v1/music/artists?library_id=${libraryID}`,
        method: "GET",
      }),
    enabled: !!libraryID,
    select: (d) => d.artists ?? [],
    staleTime: 60_000,
  });
}

type Tab = "tracks" | "albums" | "artists";

function NowPlayingBar({
  playing,
  queue,
  onClose,
  onSelectTrack,
}: {
  playing: Track;
  queue: Track[];
  onClose: () => void;
  onSelectTrack: (t: Track) => void;
}) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);

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

  function playPrev() {
    if (currentIndex > 0) onSelectTrack(queue[currentIndex - 1]);
  }

  function playNext() {
    if (currentIndex < queue.length - 1) onSelectTrack(queue[currentIndex + 1]);
  }

  return (
    <div
      className="fixed bottom-0 left-0 right-0 flex items-center gap-2 sm:gap-4 px-3 sm:px-6 py-3 bg-[var(--surface)] border-t border-[var(--border)] z-overlay"
    >
      <audio
        ref={audioRef}
        key={playing.ID}
        src={`/api/v1/music/tracks/${playing.ID}/stream`}
        onTimeUpdate={() => setCurrentTime(audioRef.current?.currentTime ?? 0)}
        onLoadedMetadata={() => setDuration(audioRef.current?.duration ?? 0)}
        onEnded={playNext}
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
      />

      {/* Album art + info */}
      <div className="flex items-center gap-3 min-w-0 w-[120px] sm:w-[200px]">
        <TrackArt trackID={playing.ID} size={48} />
        <div className="min-w-0">
          <p className="font-medium truncate text-sm text-[var(--text)]">{playing.Title || "—"}</p>
          <p className="text-xs truncate text-[var(--text-muted)]">{playing.Artist || "—"}</p>
        </div>
      </div>

      {/* Controls */}
      <div className="flex flex-col items-center gap-1 flex-1">
        <div className="flex items-center gap-4">
          <button
            onClick={playPrev}
            disabled={currentIndex <= 0}
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
            disabled={currentIndex >= queue.length - 1}
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
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const qc = useQueryClient();
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

  const deleteTrackMutation = useMutation({
    mutationFn: (id: string) => eveyFetch({ url: `/api/v1/music/tracks/${id}`, method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["music", "tracks"] });
      toast.success(t("media.music.trackDeleted"));
    },
    onError: (err: Error) => toast.error(err.message || t("media.music.failedDeleteTrack")),
  });

  function handleDeleteTrack(id: string) {
    if (!window.confirm(t("media.music.deleteTrackConfirm"))) return;
    deleteTrackMutation.mutate(id);
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
          tracks={tracks ?? []}
          loading={tracksLoading}
          playing={playing}
          onPlay={(t) => handlePlay(t, tracks ?? [])}
          isAdmin={isAdmin}
          onDelete={handleDeleteTrack}
          search={searchInput}
          onSearchChange={setSearchInput}
        />
      )}
      {tab === "albums" && (
        <AlbumsTab albums={albums ?? []} loading={albumsLoading} />
      )}
      {tab === "artists" && (
        <ArtistsTab artists={artists ?? []} loading={artistsLoading} />
      )}

      {playing && (
        <NowPlayingBar
          playing={playing}
          queue={queue}
          onClose={() => setPlaying(null)}
          onSelectTrack={setPlaying}
        />
      )}
    </div>
  );
}

const TRACKS_PAGE_SIZE = 50;

function TracksTab({
  tracks,
  loading,
  playing,
  onPlay,
  isAdmin,
  onDelete,
  search,
  onSearchChange,
}: {
  tracks: Track[];
  loading: boolean;
  playing: Track | null;
  onPlay: (t: Track) => void;
  isAdmin: boolean;
  onDelete: (id: string) => void;
  search: string;
  onSearchChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(0);
  const totalPages = Math.ceil(tracks.length / TRACKS_PAGE_SIZE);
  const pageTrack = useMemo(
    () => tracks.slice(page * TRACKS_PAGE_SIZE, (page + 1) * TRACKS_PAGE_SIZE),
    [tracks, page],
  );

  return (
    <div>
      <div className="mb-4">
        <input
          type="search"
          placeholder={t("media.music.searchPlaceholder")}
          value={search}
          onChange={(e) => { onSearchChange(e.target.value); setPage(0); }}
          className="w-full max-w-[400px] px-3 py-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] text-sm"
        />
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
            {isAdmin && <th className="px-3 py-2"></th>}
          </tr>
        </thead>
        <tbody>
          {pageTrack.map((track, i) => {
            const isActive = playing?.ID === track.ID;
            return (
              <tr
                key={track.ID}
                onClick={() => onPlay(track)}
                className={`cursor-pointer border-b border-[var(--border)] hover:bg-[var(--surface-raised)] ${isActive ? "bg-[var(--surface-raised)]" : ""}`}
              >
                <td className="px-3 py-2 text-[var(--text-muted)]">{page * TRACKS_PAGE_SIZE + i + 1}</td>
                <td className="px-1 py-2">
                  <TrackArt trackID={track.ID} size={36} />
                </td>
                <td className={`px-3 py-2 font-medium ${isActive ? "text-[var(--accent)]" : "text-[var(--text)]"}`}>{track.Title || "—"}</td>
                <td className="px-3 py-2 text-[var(--text-secondary)]">{track.Artist || "—"}</td>
                <td className="px-3 py-2 text-[var(--text-secondary)]">{track.Album || "—"}</td>
                <td className="px-3 py-2 text-[var(--text-muted)] text-right">
                  {track.Duration > 0 ? formatDuration(track.Duration) : "—"}
                </td>
                {isAdmin && (
                  <td className="px-3 py-2" onClick={(e) => e.stopPropagation()}>
                    <button
                      onClick={() => onDelete(track.ID)}
                      title={t("media.music.deleteTrack")}
                      className="bg-transparent border-none cursor-pointer text-[var(--color-danger)] p-1 rounded-[var(--radius-sm)] flex items-center hover:opacity-80"
                    >
                      <Trash2 size={16} />
                    </button>
                  </td>
                )}
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
            {page + 1} / {totalPages} · {tracks.length} tracks
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

function AlbumsTab({ albums, loading }: { albums: Album[]; loading: boolean }) {
  const { t } = useTranslation();
  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  if (!albums.length) return <p className="text-[var(--text-muted)]">{t("media.music.noAlbums")}</p>;

  return (
    <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(160px,1fr))]">
      {albums.map((a) => (
        <div
          key={a.ID}
          className="rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] border border-[var(--border)]"
        >
          <div
            className="w-full flex items-center justify-center bg-[var(--surface)] bg-cover bg-center aspect-square"
            style={{
              backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL})` : undefined,
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
              {a.TrackCount} {a.TrackCount === 1 ? "track" : "tracks"}
              {a.Year > 0 ? ` · ${a.Year}` : ""}
            </p>
          </div>
        </div>
      ))}
    </div>
  );
}

function ArtistsTab({ artists, loading }: { artists: Artist[]; loading: boolean }) {
  const { t } = useTranslation();
  if (loading) return <p className="text-[var(--text-muted)]">{t("media.music.loading")}</p>;
  if (!artists.length) return <p className="text-[var(--text-muted)]">{t("media.music.noArtists")}</p>;

  return (
    <div className="flex flex-col gap-1">
      {artists.map((a) => (
        <div
          key={a.ID}
          className="flex items-center justify-between px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)]"
        >
          <span className="font-medium text-[var(--text)]">{a.Name}</span>
          <span className="text-sm text-[var(--text-muted)]">
            {a.AlbumCount} {a.AlbumCount === 1 ? "album" : "albums"} · {a.TrackCount} tracks
          </span>
        </div>
      ))}
    </div>
  );
}
