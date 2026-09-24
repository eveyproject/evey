import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  formatTime,
  getBaseURL,
  useLibraryServiceListLibraries,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
  useMusicServiceListTracks,
  type V1Track,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { CARD_WIDTH_GRID, coverArtUri } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Pressable, Spinner } from "../components/ui";
import { Icon } from "../components/icons";
import { ScreenState } from "../components/ScreenState";
import { StackHeader } from "../components/StackHeader";
import { navigate } from "../app/router";
import { useMusicQueueStore, trackToQueueTrack } from "../music/queue";

const TRACKS_LIMIT = 100;
const ART = CARD_WIDTH_GRID / 10;
const ALBUM_ART = 16;

function art(url: string | undefined, width: number): string | undefined {
  if (!url) return undefined;
  const token = useAuthStore.getState().accessToken;
  const withToken = token ? `${url}${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}` : url;
  return coverArtUri(getBaseURL(), withToken, width);
}

function useCurrentTrackId(): string | undefined {
  const currentIndex = useMusicQueueStore((s) => s.currentIndex);
  const queue = useMusicQueueStore((s) => s.queue);
  return currentIndex >= 0 && currentIndex < queue.length ? queue[currentIndex].id : undefined;
}

function ArtCard({ title, subtitle, uri, round, playing, initial, autoFocus, onPress }: {
  title: string;
  subtitle: string;
  uri?: string;
  round?: boolean;
  playing?: boolean;
  initial?: string;
  autoFocus?: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable className="tv-card music-card" autoFocus={autoFocus} label={title} onPress={onPress}>
      <span className={`music-art ${round ? "round" : ""} ${playing ? "playing" : ""}`} style={{ width: `${ART}rem`, height: `${ART}rem` }}>
        {uri ? <img src={uri} alt="" loading="lazy" /> : initial ? <span className="music-initial">{initial}</span> : <Icon name="musical-notes" size={40} />}
      </span>
      <span className={`music-card-title ${playing ? "accent" : ""}`}>{title || "-"}</span>
      <span className="music-card-sub">{subtitle || "-"}</span>
    </Pressable>
  );
}

function Section({ title, loading, error, onRetry, empty, emptyLabel, children }: {
  title: string;
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  empty: boolean;
  emptyLabel: string;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <FocusRoot className="music-section">
      <h2 className="music-row-title">{title}</h2>
      {loading ? (
        <div className="music-row-note"><Spinner /></div>
      ) : error ? (
        <div className="music-row-note">
          <span className="muted">{t("common.loadError")}</span>
          <Pressable className="tv-card music-retry" onPress={onRetry}>{t("common.retry")}</Pressable>
        </div>
      ) : empty ? (
        <p className="music-row-note muted">{emptyLabel}</p>
      ) : (
        <div className="music-row">{children}</div>
      )}
    </FocusRoot>
  );
}

// The Music tab: artists, albums and tracks of the chosen library as rows. The
// first row that has something takes the remote.
export function MusicScreen({ libraryId }: { libraryId?: string }) {
  const { t } = useTranslation();
  const libs = useLibraryServiceListLibraries({
    query: { select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC"), staleTime: 60_000 },
  });
  const [chosen, setChosen] = useState<string | undefined>(libraryId);
  const activeId = chosen ?? libs.data?.[0]?.id;
  const lib = activeId ?? "";
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const currentTrackId = useCurrentTrackId();

  const artists = useMusicServiceGetArtists({ libraryId: activeId }, { query: { enabled: !!activeId, select: (d) => d.artists ?? [] } });
  const albums = useMusicServiceGetAlbums({ libraryId: activeId }, { query: { enabled: !!activeId, select: (d) => d.albums ?? [] } });
  const tracks = useMusicServiceListTracks({ libraryId: activeId, limit: TRACKS_LIMIT }, { query: { enabled: !!activeId, select: (d) => d.tracks ?? [] } });

  if (libs.isLoading || libs.isError) return <ScreenState loading={libs.isLoading} error={libs.isError} onRetry={() => void libs.refetch()} />;
  const musicLibs = libs.data ?? [];
  if (musicLibs.length === 0) return <div className="center-fill"><p className="muted big">{t("media.music.noLibraries")}</p></div>;

  const artistList = artists.data ?? [];
  const albumList = albums.data ?? [];
  const trackList = tracks.data ?? [];
  const focusSection = [artistList.length, albumList.length, trackList.length].findIndex((n) => n > 0);

  return (
    <FocusRoot autoFocus className="tab-page music-page">
      <h1 className="tab-title music-title">{t("media.music.title")}</h1>
      {musicLibs.length > 1 ? (
        <FocusRoot className="music-libs">
          {musicLibs.map((l) => (
            <Pressable key={l.id} className={`tv-card music-lib ${l.id === activeId ? "active" : ""}`} label={l.name} onPress={() => setChosen(l.id)}>{l.name}</Pressable>
          ))}
        </FocusRoot>
      ) : null}
      <Section title={t("media.music.artists")} loading={artists.isLoading} error={artists.isError} onRetry={() => void artists.refetch()} empty={artistList.length === 0} emptyLabel={t("media.music.noArtists")}>
        {artistList.map((a, i) => (
          <ArtCard
            key={a.id}
            round
            title={a.name ?? ""}
            subtitle={`${a.albumCount ?? 0} · ${a.trackCount ?? 0}`}
            uri={art(a.coverArtUrl, CARD_WIDTH_GRID * 2)}
            initial={a.name?.[0]?.toUpperCase() ?? "?"}
            autoFocus={i === 0 && focusSection === 0}
            onPress={() => a.id && navigate({ name: "artist", artistId: a.id, libraryId: lib })}
          />
        ))}
      </Section>
      <Section title={t("media.music.albums")} loading={albums.isLoading} error={albums.isError} onRetry={() => void albums.refetch()} empty={albumList.length === 0} emptyLabel={t("media.music.noAlbums")}>
        {albumList.map((a, i) => (
          <ArtCard
            key={a.id}
            title={a.title ?? ""}
            subtitle={a.artist ?? ""}
            uri={art(a.coverArtUrl, CARD_WIDTH_GRID * 2)}
            autoFocus={i === 0 && focusSection === 1}
            onPress={() => a.id && navigate({ name: "album", albumId: a.id, libraryId: lib })}
          />
        ))}
      </Section>
      <Section title={t("media.music.tracks")} loading={tracks.isLoading} error={tracks.isError} onRetry={() => void tracks.refetch()} empty={trackList.length === 0} emptyLabel={t("media.music.noTracks")}>
        {trackList.map((tr, i) => (
          <ArtCard
            key={tr.id}
            title={tr.title ?? ""}
            subtitle={tr.artist ?? ""}
            uri={art(tr.coverArtUrl, CARD_WIDTH_GRID * 2)}
            playing={!!tr.id && tr.id === currentTrackId}
            autoFocus={i === 0 && focusSection === 2}
            onPress={() => setQueue(trackList.map(trackToQueueTrack), i)}
          />
        ))}
      </Section>
    </FocusRoot>
  );
}

/** An album: its art and names, then the tracks; OK starts the album there. */
export function AlbumScreen({ albumId, libraryId }: { albumId: string; libraryId?: string }) {
  const { t } = useTranslation();
  const setQueue = useMusicQueueStore((s) => s.setQueue);
  const currentTrackId = useCurrentTrackId();
  const { data, isLoading, isError, refetch } = useMusicServiceListTracks(
    { libraryId, albumId },
    { query: { enabled: !!libraryId && !!albumId, select: (d) => d.tracks ?? [] } },
  );
  const items: V1Track[] = data ?? [];
  const title = items[0]?.album || albumId;
  const cover = art(items[0]?.coverArtUrl, ALBUM_ART * 20);
  return (
    <div className="deep-page">
      <StackHeader title={title} />
      {isLoading || isError ? (
        <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />
      ) : (
        <FocusRoot autoFocus className="library-body album-body">
          <div className="album-head">
            <span className="music-art" style={{ width: `${ALBUM_ART}rem`, height: `${ALBUM_ART}rem` }}>
              {cover ? <img src={cover} alt="" /> : <Icon name="musical-notes" size={48} />}
            </span>
            <div className="album-names">
              <h1 className="album-title">{title}</h1>
              <p className="album-artist">{items[0]?.artist ?? ""}</p>
            </div>
          </div>
          {items.length === 0 ? <div className="center-fill pad-top"><p className="muted big">{t("media.music.noTracks")}</p></div> : null}
          {items.map((tr, i) => {
            const playing = !!tr.id && tr.id === currentTrackId;
            return (
              <Pressable key={tr.id} className="tv-card album-track" autoFocus={i === 0} label={tr.title} onPress={() => setQueue(items.map(trackToQueueTrack), i)}>
                <span className="album-track-no">{tr.trackNumber || i + 1}</span>
                <span className="album-track-text">
                  <span className={`album-track-title ${playing ? "accent" : ""}`}>{tr.title || "-"}</span>
                  <span className="album-track-artist">{tr.artist || "-"}</span>
                </span>
                {tr.duration ? <span className="album-track-time">{formatTime(tr.duration)}</span> : null}
              </Pressable>
            );
          })}
        </FocusRoot>
      )}
    </div>
  );
}

/** An artist: their albums as a grid (the artist id is the display name). */
export function ArtistScreen({ artistId, libraryId }: { artistId: string; libraryId?: string }) {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch } = useMusicServiceGetAlbums(
    { libraryId },
    { query: { enabled: !!libraryId, select: (d) => (d.albums ?? []).filter((a) => a.artist === artistId) } },
  );
  const items = data ?? [];
  return (
    <div className="deep-page">
      <StackHeader title={artistId} />
      {isLoading || isError ? (
        <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />
      ) : items.length === 0 ? (
        <div className="center-fill pad-top"><p className="muted big">{t("media.music.noAlbums")}</p></div>
      ) : (
        <FocusRoot autoFocus grid className="library-body library-grid-wrap">
          <div className="grid library-grid">
            {items.map((a, i) => (
              <ArtCard
                key={a.id}
                title={a.title ?? ""}
                subtitle={a.year ? String(a.year) : ""}
                uri={art(a.coverArtUrl, CARD_WIDTH_GRID * 2)}
                autoFocus={i === 0}
                onPress={() => a.id && navigate({ name: "album", albumId: a.id, libraryId })}
              />
            ))}
          </div>
        </FocusRoot>
      )}
    </div>
  );
}
