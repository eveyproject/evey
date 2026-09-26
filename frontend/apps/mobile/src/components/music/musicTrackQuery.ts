// Which URL the music tab asks for its tracks, kept out of the screen so the
// decision can be tested without mounting 900 lines of player, queue and
// download wiring.
//
// The plain listing is keyset-paginated. It used to ask for a flat limit=200,
// which simply lost everything past the 200th track of a library: no error, no
// indication, the list just ended. The web tab carried the same defect and was
// fixed the same day (2026-08-08).
//
// The other two modes do NOT page, and that is the server's contract rather
// than an omission:
//   - with an album id, the server returns that album's full track set and
//     ignores limit/search entirely;
//   - a free-text search is answered by the full-text index, which has no
//     keyset of its own. Honouring a cursor there would serve page 1 from the
//     search engine and page 2 from a SQL LIKE, changing the result set under
//     the reader.
// Both come back without a cursor, so pagination simply never starts.

// Matches the server's own default page for tracks and stays well under
// maxPageSize (500) - above that the server silently substitutes its default.
export const TRACKS_PAGE_SIZE = 200;

export interface TrackQueryInput {
  libraryID: string;
  /** Free-text query. Ignored when albumID is set. */
  search?: string;
  albumID?: string;
  /** Opaque cursor from the previous page. Empty/absent means page 1. */
  cursor?: string;
}

// SHUFFLE_QUEUE_SIZE is how many tracks a "shuffle play" draws: a long
// queue, not the whole library.
export const SHUFFLE_QUEUE_SIZE = 200;

// buildShuffleQueryUrl asks the server for random tracks from the WHOLE
// library, instead of shuffling the pages the screen happened to load.
export function buildShuffleQueryUrl(libraryID: string): string {
  return `/api/v1/music/tracks?libraryId=${encodeURIComponent(libraryID)}&shuffle=true&limit=${SHUFFLE_QUEUE_SIZE}`;
}

export function buildTrackQueryUrl(input: TrackQueryInput): string {
  const { libraryID, albumID, cursor } = input;
  const search = (input.search ?? "").trim();

  if (albumID) {
    return `/api/v1/music/tracks?libraryId=${libraryID}&albumId=${encodeURIComponent(albumID)}&limit=${TRACKS_PAGE_SIZE}`;
  }
  if (search) {
    return `/api/v1/music/tracks?libraryId=${libraryID}&search=${encodeURIComponent(search)}`;
  }
  // Page 1 takes the offset path - the only one that computes an exact total;
  // every page after it carries the previous response's cursor.
  const paging = cursor ? `&cursor=${encodeURIComponent(cursor)}` : "&offset=0";
  return `/api/v1/music/tracks?libraryId=${libraryID}&limit=${TRACKS_PAGE_SIZE}${paging}`;
}
