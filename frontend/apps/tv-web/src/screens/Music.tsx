import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getBaseURL,
  useLibraryServiceListLibraries,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
  useMusicServiceListTracks,
  type V1MusicAlbum,
  type V1Track,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { coverArtUri } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner } from "../components/ui";
import { navigate } from "../app/router";
import { useMusicQueueStore, trackToQueueTrack } from "../music/queue";

const TRACKS_LIMIT = 100;

function art(url: string | undefined, width: number): string | undefined {
  const token = useAuthStore.getState().accessToken;
  if (!url) return undefined;
  const withToken = token ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : url;
  return coverArtUri(getBaseURL(), withToken, width);
}

// The music library: artists, albums and tracks of the chosen library as
// rows, the way the Android TV lays it out. An album opens its page; a
// track starts the whole row as the queue from that track.
export function MusicScreen({ libraryId }: { libraryId?: string }) {
  const { t } = useTranslation();
  const { data: libs, isLoading: libsLoading } = useLibraryServiceListLibraries();
  const musicLibs = (libs?.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC" && l.id);
  const [chosen, setChosen] = useState<string | undefined>(libraryId);
  const activeId = chosen ?? musicLibs[0]?.id;
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const { data: artists } = useMusicServiceGetArtists({ libraryId: activeId }, { query: { enabled: !!activeId, select: (d) => d.artists ?? [] } });
  const { data: albums } = useMusicServiceGetAlbums({ libraryId: activeId }, { query: { enabled: !!activeId, select: (d) => d.albums ?? [] } });
  const { data: tracks } = useMusicServiceListTracks({ libraryId: activeId, limit: TRACKS_LIMIT }, { query: { enabled: !!activeId, select: (d) => d.tracks ?? [] } });

  if (libsLoading) return <div className="screen screen-center"><Spinner /></div>;
  if (musicLibs.length === 0) return <div className="screen screen-center"><p className="muted">{t("media.music.noMusicLibraries", "No music libraries")}</p></div>;

  const playTracks = (list: V1Track[], index: number) => setQueue(list.map(trackToQueueTrack), index);

  return (
    <div className="screen">
      <FocusRoot autoFocus className="header">
        <h1 className="title">{t("common.nav.music", "Music")}</h1>
        <div className="actions">
          {musicLibs.map((l, i) => (
            <Button key={l.id} autoFocus={i === 0} variant={l.id === activeId ? "primary" : "default"} onPress={() => setChosen(l.id)}>{l.name}</Button>
          ))}
        </div>
      </FocusRoot>
      {artists && artists.length > 0 ? (
        <FocusRoot>
          <h2 className="row-title">{t("media.music.artists", "Artists")}</h2>
          <div className="row">
            {artists.map((a) => (
              <Card key={a.id} title={a.name ?? ""} imageUrl={art(a.coverArtUrl, 400)} onPress={() => a.id && navigate({ name: "artist", artistId: a.id, libraryId: activeId })} />
            ))}
          </div>
        </FocusRoot>
      ) : null}
      {albums && albums.length > 0 ? (
        <FocusRoot>
          <h2 className="row-title">{t("media.music.albums", "Albums")}</h2>
          <div className="row">
            {albums.map((a: V1MusicAlbum) => (
              <Card key={a.id} title={`${a.title ?? ""}${a.artist ? ` · ${a.artist}` : ""}`} imageUrl={art(a.coverArtUrl, 400)} onPress={() => a.id && navigate({ name: "album", albumId: a.id, libraryId: activeId })} />
            ))}
          </div>
        </FocusRoot>
      ) : null}
      {tracks && tracks.length > 0 ? (
        <FocusRoot>
          <h2 className="row-title">{t("media.music.tracks", "Tracks")}</h2>
          <div className="row">
            {tracks.map((tr, i) => (
              <Card key={tr.id} title={`${tr.title ?? ""}${tr.artist ? ` · ${tr.artist}` : ""}`} imageUrl={art(tr.coverArtUrl, 400)} onPress={() => playTracks(tracks, i)} />
            ))}
          </div>
        </FocusRoot>
      ) : null}
    </div>
  );
}

// An album, or an artist's albums: the tracks in order, OK on one starts
// the album from there.
export function AlbumScreen({ albumId, artistId, libraryId }: { albumId?: string; artistId?: string; libraryId?: string }) {
  const { t } = useTranslation();
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const queue = useMusicQueueStore((s) => s.queue);
  const currentId = currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex].id : undefined;
  const { data: tracks, isLoading } = useMusicServiceListTracks(
    { libraryId, albumId, limit: 500 },
    { query: { enabled: !!libraryId, select: (d) => (d.tracks ?? []).filter((tr) => !artistId || tr.artist === artistId || tr.albumArtist === artistId) } },
  );
  if (isLoading) return <div className="screen screen-center"><Spinner /></div>;
  const list = (tracks ?? []).slice().sort((a, b) => (a.diskNumber ?? 0) - (b.diskNumber ?? 0) || (a.trackNumber ?? 0) - (b.trackNumber ?? 0));
  const first = list[0];
  const title = albumId ? first?.album ?? albumId : artistId ?? "";
  const subtitle = albumId ? first?.artist ?? "" : "";
  return (
    <div className="screen">
      <div className="hero">
        {first?.coverArtUrl ? <img src={art(first.coverArtUrl, 600)} alt="" /> : <div className="hero-placeholder">♪</div>}
        <div>
          <h1 className="title">{title}</h1>
          {subtitle ? <p className="muted">{subtitle}</p> : null}
          <p className="muted">{t("media.music.trackCount", { count: list.length, defaultValue: `${list.length} tracks` })}</p>
        </div>
      </div>
      <FocusRoot autoFocus>
        <div className="track-list">
          {list.map((tr, i) => (
            <Button key={tr.id} autoFocus={i === 0} variant={tr.id === currentId ? "primary" : "default"} onPress={() => setQueue(list.map(trackToQueueTrack), i)}>
              <span className="track-no">{tr.trackNumber ?? i + 1}</span>
              <span className="track-title">{tr.title}</span>
              <span className="muted">{tr.duration ? `${Math.floor(tr.duration / 60)}:${String(Math.floor(tr.duration % 60)).padStart(2, "0")}` : ""}</span>
            </Button>
          ))}
        </div>
      </FocusRoot>
    </div>
  );
}
