import { fetchAlbumTracks, fetchPlaylistTracks, type Album, type Track } from "./musicQueries";

// What the music screens drag: tracks, a whole album, a whole playlist, or a
// row of a list being reordered (a queue or a playlist), which names the list
// it comes from.
export type MusicDragData =
  | { kind: "music-tracks"; tracks: Track[] }
  | { kind: "music-album"; album: Album; libraryID: string }
  | { kind: "music-playlist"; id: string; name: string }
  | { kind: "music-row"; list: string; index: number; track: Track };

const KINDS = new Set(["music-tracks", "music-album", "music-playlist", "music-row"]);

export function isMusicDrag(d: unknown): d is MusicDragData {
  return typeof d === "object" && d !== null && KINDS.has((d as { kind?: string }).kind ?? "");
}

/** Anything that names tracks, but not a row of the list `list` itself. */
export function addsTracks(list: string) {
  return (d: unknown): d is MusicDragData => isMusicDrag(d) && !(d.kind === "music-row" && d.list === list);
}

/** A row of the list `list`, being reordered. */
export function isRowOf(list: string) {
  return (d: unknown): d is Extract<MusicDragData, { kind: "music-row" }> =>
    isMusicDrag(d) && d.kind === "music-row" && d.list === list;
}

/** The tracks a drag stands for, fetched when it is an album or a playlist. */
export async function resolveDragTracks(d: MusicDragData): Promise<Track[]> {
  switch (d.kind) {
    case "music-tracks":
      return d.tracks;
    case "music-row":
      return [d.track];
    case "music-album":
      return fetchAlbumTracks(d.libraryID, d.album.ID);
    case "music-playlist":
      return fetchPlaylistTracks(d.id);
  }
}
