import { useState, useMemo, useRef, useEffect } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
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
        style={{
          width: size,
          height: size,
          minWidth: size,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          backgroundColor: "var(--surface)",
          borderRadius: "var(--radius-sm)",
        }}
      >
        <Music size={size * 0.5} style={{ color: "var(--text-muted)", opacity: 0.5 }} />
      </div>
    );
  }
  return (
    <img
      src={`/api/v1/music/tracks/${trackID}/art`}
      alt=""
      width={size}
      height={size}
      style={{
        width: size,
        height: size,
        minWidth: size,
        objectFit: "cover",
        borderRadius: "var(--radius-sm)",
        backgroundColor: "var(--surface)",
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
    audio.play().then(() => setIsPlaying(true)).catch((e) => {
      setIsPlaying(false);
      if (e.name !== "AbortError") console.warn("Autoplay blocked:", e.message);
    });
  }, [playing.ID]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume;
  }, [volume]);

  function togglePlay() {
    const audio = audioRef.current;
    if (!audio) return;
    if (isPlaying) {
      audio.pause();
      setIsPlaying(false);
    } else {
      audio.play().then(() => setIsPlaying(true)).catch((e) => {
        if (e.name !== "AbortError") console.warn("Autoplay blocked:", e.message);
      });
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
      className="fixed bottom-0 left-0 right-0 flex items-center gap-4 px-6 py-3"
      style={{
        backgroundColor: "var(--surface)",
        borderTop: "1px solid var(--border)",
        zIndex: 50,
      }}
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
      <div className="flex items-center gap-3" style={{ minWidth: 0, width: 200 }}>
        <TrackArt trackID={playing.ID} size={48} />
        <div style={{ minWidth: 0 }}>
          <p className="font-medium truncate text-sm" style={{ color: "var(--text)" }}>{playing.Title || "—"}</p>
          <p className="text-xs truncate" style={{ color: "var(--text-muted)" }}>{playing.Artist || "—"}</p>
        </div>
      </div>

      {/* Controls */}
      <div className="flex flex-col items-center gap-1 flex-1">
        <div className="flex items-center gap-4">
          <button
            onClick={playPrev}
            disabled={currentIndex <= 0}
            style={{ background: "none", border: "none", cursor: currentIndex <= 0 ? "default" : "pointer", opacity: currentIndex <= 0 ? 0.3 : 1, color: "var(--text)" }}
            aria-label="Previous"
          >
            <SkipBack size={18} />
          </button>
          <button
            onClick={togglePlay}
            style={{
              background: "var(--accent)",
              border: "none",
              cursor: "pointer",
              color: "#fff",
              borderRadius: "50%",
              width: 36,
              height: 36,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
            aria-label={isPlaying ? "Pause" : "Play"}
          >
            {isPlaying ? <Pause size={16} /> : <Play size={16} />}
          </button>
          <button
            onClick={playNext}
            disabled={currentIndex >= queue.length - 1}
            style={{ background: "none", border: "none", cursor: currentIndex >= queue.length - 1 ? "default" : "pointer", opacity: currentIndex >= queue.length - 1 ? 0.3 : 1, color: "var(--text)" }}
            aria-label="Next"
          >
            <SkipForward size={18} />
          </button>
        </div>
        <div className="flex items-center gap-2 w-full" style={{ maxWidth: 400 }}>
          <span style={{ fontSize: "0.7rem", color: "var(--text-muted)", minWidth: 32, textAlign: "right" }}>
            {formatDuration(currentTime)}
          </span>
          <input
            type="range"
            min={0}
            max={duration || 0}
            step={1}
            value={currentTime}
            onChange={seek}
            style={{ flex: 1, accentColor: "var(--accent)" }}
          />
          <span style={{ fontSize: "0.7rem", color: "var(--text-muted)", minWidth: 32 }}>
            {duration > 0 ? formatDuration(duration) : "—"}
          </span>
        </div>
      </div>

      {/* Volume */}
      <div className="flex items-center gap-2" style={{ minWidth: 100 }}>
        <Volume2 size={16} style={{ color: "var(--text-muted)" }} />
        <input
          type="range"
          min={0}
          max={1}
          step={0.02}
          value={volume}
          onChange={(e) => setVolume(Number(e.target.value))}
          style={{ width: 72, accentColor: "var(--accent)" }}
        />
      </div>

      <button
        onClick={onClose}
        style={{ color: "var(--text-muted)", background: "none", border: "none", cursor: "pointer" }}
        aria-label="Close player"
      >
        <X size={18} />
      </button>
    </div>
  );
}

function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(t);
  }, [value, delay]);
  return debounced;
}

function MusicPage() {
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
      toast.success("Track deleted");
    },
    onError: (err: Error) => toast.error(err.message || "Failed to delete track"),
  });

  function handleDeleteTrack(id: string) {
    if (!window.confirm("Delete this track? This cannot be undone.")) return;
    deleteTrackMutation.mutate(id);
  }

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: "6px 16px",
    borderRadius: "var(--radius-sm)",
    border: "none",
    cursor: "pointer",
    fontSize: "0.875rem",
    fontWeight: active ? 600 : 400,
    backgroundColor: active ? "var(--accent)" : "transparent",
    color: active ? "#fff" : "var(--text-secondary)",
  });

  if (libLoading) {
    return (
      <div className="flex-1 p-6">
        <div className="animate-pulse h-8 w-48 rounded mb-6" style={{ backgroundColor: "var(--surface-raised)" }} />
      </div>
    );
  }

  if (!libraries || libraries.length === 0) {
    return (
      <div className="flex-1 p-6">
        <p style={{ color: "var(--text-muted)" }}>No music libraries found. Add one in the Admin panel.</p>
      </div>
    );
  }

  return (
    <div className="flex-1 p-6" style={{ paddingBottom: playing ? 120 : undefined }}>
      <div className="flex flex-col sm:flex-row sm:items-center gap-4 mb-4">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Music
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
        {(["tracks", "albums", "artists"] as Tab[]).map((t) => (
          <button key={t} style={tabStyle(tab === t)} onClick={() => setTab(t)}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
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
  const [page, setPage] = useState(0);
  const totalPages = Math.ceil(tracks.length / TRACKS_PAGE_SIZE);
  const pageTrack = useMemo(
    () => tracks.slice(page * TRACKS_PAGE_SIZE, (page + 1) * TRACKS_PAGE_SIZE),
    [tracks, page],
  );

  return (
    <div>
      <div style={{ marginBottom: "16px" }}>
        <input
          type="search"
          placeholder="Search tracks, artists, albums…"
          value={search}
          onChange={(e) => { onSearchChange(e.target.value); setPage(0); }}
          style={{
            width: "100%",
            maxWidth: "400px",
            padding: "8px 12px",
            borderRadius: "var(--radius-sm)",
            border: "1px solid var(--border)",
            backgroundColor: "var(--surface-raised)",
            color: "var(--text)",
            fontSize: "0.875rem",
          }}
        />
      </div>
      {loading && <p style={{ color: "var(--text-muted)" }}>Loading…</p>}
      {!loading && !tracks.length && <p style={{ color: "var(--text-muted)" }}>{search ? "No tracks match your search." : "No tracks found."}</p>}
      {!loading && tracks.length > 0 && <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.875rem" }}>
        <thead>
          <tr style={{ color: "var(--text-muted)", textAlign: "left", borderBottom: "1px solid var(--border)" }}>
            <th style={{ padding: "8px 12px" }}>#</th>
            <th style={{ padding: "8px 12px" }}></th>
            <th style={{ padding: "8px 12px" }}>Title</th>
            <th style={{ padding: "8px 12px" }}>Artist</th>
            <th style={{ padding: "8px 12px" }}>Album</th>
            <th style={{ padding: "8px 12px", textAlign: "right" }}>Duration</th>
            {isAdmin && <th style={{ padding: "8px 12px" }}></th>}
          </tr>
        </thead>
        <tbody>
          {pageTrack.map((t, i) => {
            const isActive = playing?.ID === t.ID;
            return (
              <tr
                key={t.ID}
                onClick={() => onPlay(t)}
                style={{
                  cursor: "pointer",
                  borderBottom: "1px solid var(--border)",
                  backgroundColor: isActive ? "var(--surface-raised)" : undefined,
                }}
                className="hover:bg-[var(--surface-raised)]"
              >
                <td style={{ padding: "8px 12px", color: "var(--text-muted)" }}>{page * TRACKS_PAGE_SIZE + i + 1}</td>
                <td style={{ padding: "8px 4px" }}>
                  <TrackArt trackID={t.ID} size={36} />
                </td>
                <td style={{ padding: "8px 12px", color: isActive ? "var(--accent)" : "var(--text)", fontWeight: 500 }}>{t.Title || "—"}</td>
                <td style={{ padding: "8px 12px", color: "var(--text-secondary)" }}>{t.Artist || "—"}</td>
                <td style={{ padding: "8px 12px", color: "var(--text-secondary)" }}>{t.Album || "—"}</td>
                <td style={{ padding: "8px 12px", color: "var(--text-muted)", textAlign: "right" }}>
                  {t.Duration > 0 ? formatDuration(t.Duration) : "—"}
                </td>
                {isAdmin && (
                  <td style={{ padding: "8px 12px" }} onClick={(e) => e.stopPropagation()}>
                    <button
                      onClick={() => onDelete(t.ID)}
                      title="Delete track"
                      style={{
                        background: "none",
                        border: "none",
                        cursor: "pointer",
                        color: "var(--text-muted)",
                        padding: "4px",
                        borderRadius: "var(--radius-sm)",
                        display: "flex",
                        alignItems: "center",
                      }}
                      className="hover:text-red-500"
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
        <div className="flex items-center justify-between mt-4" style={{ color: "var(--text-muted)", fontSize: "0.875rem" }}>
          <button
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0}
            style={{
              padding: "6px 16px",
              borderRadius: "var(--radius-sm)",
              border: "1px solid var(--border)",
              backgroundColor: "var(--surface-raised)",
              color: page === 0 ? "var(--text-muted)" : "var(--text)",
              cursor: page === 0 ? "default" : "pointer",
              opacity: page === 0 ? 0.5 : 1,
            }}
          >
            Previous
          </button>
          <span>
            {page + 1} / {totalPages} · {tracks.length} tracks
          </span>
          <button
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={page === totalPages - 1}
            style={{
              padding: "6px 16px",
              borderRadius: "var(--radius-sm)",
              border: "1px solid var(--border)",
              backgroundColor: "var(--surface-raised)",
              color: page === totalPages - 1 ? "var(--text-muted)" : "var(--text)",
              cursor: page === totalPages - 1 ? "default" : "pointer",
              opacity: page === totalPages - 1 ? 0.5 : 1,
            }}
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}

function AlbumsTab({ albums, loading }: { albums: Album[]; loading: boolean }) {
  if (loading) return <p style={{ color: "var(--text-muted)" }}>Loading…</p>;
  if (!albums.length) return <p style={{ color: "var(--text-muted)" }}>No albums found.</p>;

  return (
    <div className="grid gap-4" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))" }}>
      {albums.map((a) => (
        <div
          key={a.ID}
          className="rounded-[var(--radius-md)] overflow-hidden"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)" }}
        >
          <div
            style={{
              width: "100%",
              aspectRatio: "1",
              backgroundColor: "var(--surface)",
              backgroundImage: a.CoverArtURL ? `url(${a.CoverArtURL})` : undefined,
              backgroundSize: "cover",
              backgroundPosition: "center",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            {!a.CoverArtURL && (
              <Music size={32} style={{ opacity: 0.3, color: "var(--text-muted)" }} />
            )}
          </div>
          <div className="p-2">
            <p className="font-medium text-sm truncate" style={{ color: "var(--text)" }}>{a.Title || "—"}</p>
            <p className="text-xs truncate" style={{ color: "var(--text-muted)" }}>{a.Artist || "—"}</p>
            <p className="text-xs" style={{ color: "var(--text-muted)" }}>
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
  if (loading) return <p style={{ color: "var(--text-muted)" }}>Loading…</p>;
  if (!artists.length) return <p style={{ color: "var(--text-muted)" }}>No artists found.</p>;

  return (
    <div className="flex flex-col gap-1">
      {artists.map((a) => (
        <div
          key={a.ID}
          className="flex items-center justify-between px-4 py-3 rounded-[var(--radius-md)]"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)" }}
        >
          <span className="font-medium" style={{ color: "var(--text)" }}>{a.Name}</span>
          <span className="text-sm" style={{ color: "var(--text-muted)" }}>
            {a.AlbumCount} {a.AlbumCount === 1 ? "album" : "albums"} · {a.TrackCount} tracks
          </span>
        </div>
      ))}
    </div>
  );
}
