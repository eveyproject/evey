import type { MusicSource } from "../../stores/musicQueue";

export type MusicTab = "tracks" | "albums" | "artists" | "playlists";

// Builds the source tag for a queue launched from an album view, so the
// mini-player's "back to source" link can reopen that same album.
export function albumMusicSource(albumId: string, albumLabel: string): MusicSource {
  return { type: "album", id: albumId, label: albumLabel || albumId };
}

// Artist rows on mobile carry no stable id (MusicArtist is grouped purely by
// display name, and the tracks-tab artist filter matches on that same name),
// so the source id must be the name too for goToSource to round-trip.
export function artistMusicSource(artistName: string): MusicSource {
  return { type: "artist", id: artistName, label: artistName };
}

// Derives the queue source from whichever hard filter (album/artist card tap)
// is currently applied to the visible track list. A plain unfiltered listing
// or a free-text search has no single source view to link back to.
export function trackListMusicSource(
  albumFilter: string | null,
  albumFilterLabel: string,
  artistFilter: string | null,
): MusicSource | null {
  if (albumFilter) return albumMusicSource(albumFilter, albumFilterLabel);
  if (artistFilter) return artistMusicSource(artistFilter);
  return null;
}

// A search result only carries an albumId (see V1Track.albumId) - there is no
// artist or playlist page reachable from search, so anything else is null.
export function searchTrackMusicSource(albumId: string | undefined, albumLabel: string | undefined): MusicSource | null {
  if (!albumId) return null;
  return albumMusicSource(albumId, albumLabel || albumId);
}

export interface MusicRouteParams {
  tab?: string;
  album?: string;
  albumLabel?: string;
  artist?: string;
  playlistId?: string;
}

export interface RestoredMusicFilter {
  tab: MusicTab;
  albumFilter: string | null;
  albumFilterLabel: string;
  artistFilter: string | null;
  playlistId: string | null;
}

const VALID_TABS: readonly MusicTab[] = ["tracks", "albums", "artists", "playlists"];

// Turns the music screen's own navigation params back into filter state, so
// following a "back to source" link (or a deep link) reopens the exact
// album/artist/playlist view a queue was launched from. Returns null when no
// relevant param is present, so callers leave existing screen state alone
// instead of resetting it on every unrelated re-render.
export function restoreMusicFilterFromParams(params: MusicRouteParams): RestoredMusicFilter | null {
  const { tab, album, albumLabel, artist, playlistId } = params;
  if (!tab && !album && !artist && !playlistId) return null;

  if (playlistId) {
    return { tab: "playlists", albumFilter: null, albumFilterLabel: "", artistFilter: null, playlistId };
  }
  if (album) {
    return { tab: "tracks", albumFilter: album, albumFilterLabel: albumLabel ?? album, artistFilter: null, playlistId: null };
  }
  if (artist) {
    return { tab: "tracks", albumFilter: null, albumFilterLabel: "", artistFilter: artist, playlistId: null };
  }
  const safeTab = VALID_TABS.includes(tab as MusicTab) ? (tab as MusicTab) : "tracks";
  return { tab: safeTab, albumFilter: null, albumFilterLabel: "", artistFilter: null, playlistId: null };
}
