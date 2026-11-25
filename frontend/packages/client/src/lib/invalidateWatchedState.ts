import type { QueryClient } from "@tanstack/react-query";

// Marking a title watched or unwatched refreshes EVERY query, not a chosen
// list of them. Since the server stamps the watched mark on every title it
// returns (MediaItem.user_data), any answer that holds a title can hold a
// stale mark: a library page, a search, a series' episodes, a home rail, the
// title page. The hand-kept list of "watched-sensitive" keys this replaces
// missed one row after another (Continue Watching, Most Popular, the series
// Play target, the library rails), each found by a viewer.
//
// Only the queries on screen refetch (the rest are marked stale and refetch
// when they are next shown), and a refetch already in flight is reused, so a
// bulk mark of fifty titles costs one round, not fifty: the marks within
// COALESCE_MS share a single pass.
//
// playingId names the title a player is showing. Its own page is marked stale
// but not refetched while it plays: its user_data changes with the mark, and a
// player whose effects read the title would see a new object mid-stream.

const COALESCE_MS = 150;

type Pending = { timer: ReturnType<typeof setTimeout>; playing: Set<string> };
const pendingByClient = new WeakMap<QueryClient, Pending>();

export function invalidateWatchedState(qc: QueryClient, _mediaId?: string, opts?: { playingId?: string }): void {
  let pending = pendingByClient.get(qc);
  if (!pending) {
    pending = { timer: setTimeout(() => flush(qc), COALESCE_MS), playing: new Set() };
    pendingByClient.set(qc, pending);
  }
  if (opts?.playingId) pending.playing.add(opts.playingId);
}

function flush(qc: QueryClient): void {
  const pending = pendingByClient.get(qc);
  pendingByClient.delete(qc);
  const held = new Set([...(pending?.playing ?? [])].map((id) => `/api/v1/media/${id}`));
  const isHeld = (key: unknown) => typeof key === "string" && held.has(key);
  void qc.invalidateQueries(
    { predicate: (q) => !isHeld(q.queryKey?.[0]) },
    { cancelRefetch: false },
  );
  if (held.size > 0) {
    void qc.invalidateQueries({ predicate: (q) => isHeld(q.queryKey?.[0]), refetchType: "none" });
  }
}

/** Runs a pending refresh now (tests, and a screen that must not wait). */
export function flushWatchedState(qc: QueryClient): void {
  const pending = pendingByClient.get(qc);
  if (!pending) return;
  clearTimeout(pending.timer);
  flush(qc);
}
