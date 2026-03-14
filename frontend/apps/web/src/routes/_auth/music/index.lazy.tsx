import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

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

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${String(s).padStart(2, "0")}`;
}

function useMusicTracks(libraryID: string) {
  return useQuery({
    queryKey: ["music", "tracks", libraryID],
    queryFn: () =>
      eveyFetch<{ tracks: Track[]; total: number }>({
        url: `/api/v1/music/tracks?library_id=${libraryID}&limit=200`,
        method: "GET",
      }),
    select: (d) => d.tracks ?? [],
    staleTime: 60_000,
  });
}

function useMusicAlbums(libraryID: string) {
  return useQuery({
    queryKey: ["music", "albums", libraryID],
    queryFn: () =>
      eveyFetch<{ albums: Album[] }>({
        url: `/api/v1/music/albums?library_id=${libraryID}`,
        method: "GET",
      }),
    select: (d) => d.albums ?? [],
    staleTime: 60_000,
  });
}

function useMusicArtists(libraryID: string) {
  return useQuery({
    queryKey: ["music", "artists", libraryID],
    queryFn: () =>
      eveyFetch<{ artists: Artist[] }>({
        url: `/api/v1/music/artists?library_id=${libraryID}`,
        method: "GET",
      }),
    select: (d) => d.artists ?? [],
    staleTime: 60_000,
  });
}

type Tab = "tracks" | "albums" | "artists";

function MusicPage() {
  const [tab, setTab] = useState<Tab>("tracks");
  const [libraryID] = useState("");
  const [playing, setPlaying] = useState<Track | null>(null);

  const { data: tracks, isLoading: tracksLoading } = useMusicTracks(libraryID);
  const { data: albums, isLoading: albumsLoading } = useMusicAlbums(libraryID);
  const { data: artists, isLoading: artistsLoading } = useMusicArtists(libraryID);

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

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4" style={{ color: "var(--text)" }}>
        Music
      </h1>

      <div className="flex gap-2 mb-6">
        {(["tracks", "albums", "artists"] as Tab[]).map((t) => (
          <button key={t} style={tabStyle(tab === t)} onClick={() => setTab(t)}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === "tracks" && (
        <TracksTab tracks={tracks ?? []} loading={tracksLoading} onPlay={setPlaying} />
      )}
      {tab === "albums" && (
        <AlbumsTab albums={albums ?? []} loading={albumsLoading} />
      )}
      {tab === "artists" && (
        <ArtistsTab artists={artists ?? []} loading={artistsLoading} />
      )}

      {playing && (
        <div
          className="fixed bottom-0 left-0 right-0 flex items-center gap-4 px-6 py-3"
          style={{
            backgroundColor: "var(--surface)",
            borderTop: "1px solid var(--border)",
            zIndex: 50,
          }}
        >
          <div className="flex flex-col min-w-0 flex-1">
            <span className="font-medium truncate" style={{ color: "var(--text)" }}>
              {playing.Title}
            </span>
            <span className="text-sm truncate" style={{ color: "var(--text-muted)" }}>
              {playing.Artist}
            </span>
          </div>
          <audio
            key={playing.ID}
            src={`/api/v1/music/tracks/${playing.ID}/stream`}
            controls
            autoPlay
            className="flex-1 max-w-lg"
          />
          <button
            onClick={() => setPlaying(null)}
            style={{ color: "var(--text-muted)", background: "none", border: "none", cursor: "pointer", fontSize: "1.2rem" }}
            aria-label="Close player"
          >
            ✕
          </button>
        </div>
      )}
    </div>
  );
}

function TracksTab({
  tracks,
  loading,
  onPlay,
}: {
  tracks: Track[];
  loading: boolean;
  onPlay: (t: Track) => void;
}) {
  if (loading) return <p style={{ color: "var(--text-muted)" }}>Loading…</p>;
  if (!tracks.length) return <p style={{ color: "var(--text-muted)" }}>No tracks found.</p>;

  return (
    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.875rem" }}>
      <thead>
        <tr style={{ color: "var(--text-muted)", textAlign: "left", borderBottom: "1px solid var(--border)" }}>
          <th style={{ padding: "8px 12px" }}>#</th>
          <th style={{ padding: "8px 12px" }}>Title</th>
          <th style={{ padding: "8px 12px" }}>Artist</th>
          <th style={{ padding: "8px 12px" }}>Album</th>
          <th style={{ padding: "8px 12px", textAlign: "right" }}>Duration</th>
        </tr>
      </thead>
      <tbody>
        {tracks.map((t, i) => (
          <tr
            key={t.ID}
            onClick={() => onPlay(t)}
            style={{ cursor: "pointer", borderBottom: "1px solid var(--border)" }}
            className="hover:bg-[var(--surface-raised)]"
          >
            <td style={{ padding: "8px 12px", color: "var(--text-muted)" }}>{i + 1}</td>
            <td style={{ padding: "8px 12px", color: "var(--text)", fontWeight: 500 }}>{t.Title || "—"}</td>
            <td style={{ padding: "8px 12px", color: "var(--text-secondary)" }}>{t.Artist || "—"}</td>
            <td style={{ padding: "8px 12px", color: "var(--text-secondary)" }}>{t.Album || "—"}</td>
            <td style={{ padding: "8px 12px", color: "var(--text-muted)", textAlign: "right" }}>
              {t.Duration > 0 ? formatDuration(t.Duration) : "—"}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
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
              <span style={{ fontSize: "2rem", opacity: 0.3 }}>♪</span>
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
