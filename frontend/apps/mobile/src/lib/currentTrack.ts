// "Is this row the track currently playing?"
//
// It looks like a one-liner and it was wrong for months. The tracks list is
// FILTERED (by search, by album, by playlist) while the queue is not, so the
// two are only aligned by accident. Comparing positions therefore highlighted
// whichever track happened to sit at the queue's index inside the filtered
// list - a different song as soon as a filter, a shuffle or a playlist made
// them diverge. Web has always compared by id.
//
// It lives in its own function so the list and any future caller answer the
// question the same way, and so the answer can be tested without mounting a
// 42 kB screen.

export interface QueueEntry {
  id?: string;
}

/**
 * True when `trackId` is the entry the queue is currently on.
 *
 * Positions are never used: pass the queue and its cursor, and the id of the
 * row being rendered.
 */
export function isCurrentQueueTrack(
  queue: readonly QueueEntry[] | undefined,
  currentIndex: number,
  trackId: string | undefined,
): boolean {
  if (!queue || currentIndex < 0 || currentIndex >= queue.length) return false;
  // An absent id must never match an absent id: two unidentified rows are not
  // the same row.
  if (!trackId) return false;
  return queue[currentIndex]?.id === trackId;
}
