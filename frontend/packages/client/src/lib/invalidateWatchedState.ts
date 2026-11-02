import type { QueryClient } from "@tanstack/react-query";

// Feed query keys that reflect watched state and must refresh after a
// mark-watched / mark-unwatched. Kept as an explicit predicate (not a prefix)
// because the watched-sensitive feeds do NOT all share one prefix: the home
// sliders live under "/api/v1/feeds/*", but "Most Popular" is "/api/v1/popular"
// and "Continue Watching" (with the next episode of each series in it) is
// "/api/v1/playback/resume" on all four homes - it drifted out of every
// hand-rolled predicate and left a finished title stuck in the row for up to
// the 5-minute global staleTime.
function isWatchedSensitiveFeed(key: unknown): boolean {
  if (typeof key !== "string") return false;
  return (
    key.startsWith("/api/v1/feeds/") ||
    key === "/api/v1/popular" ||
    key === "/api/v1/playback/resume" ||
    // A series' Play target moves on once an episode of it is marked.
    key.startsWith("/api/v1/playback/series/")
  );
}

// invalidateWatchedState refreshes everything that depends on a media item's
// watched status: the item itself, the watch-history map (green badges +
// library-slider filtering), and every watched-sensitive home feed. Shared by
// all four mark-watched entry points (card button, context menu, auto-mark at
// 90%, detail page) so a new watched-sensitive feed only has to be added here
// once, not in four hand-copied predicates.
export function invalidateWatchedState(qc: QueryClient, mediaId?: string): void {
  if (mediaId) {
    void qc.invalidateQueries({ queryKey: [`/api/v1/media/${mediaId}`] });
    void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${mediaId}/progress`] });
  }
  void qc.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
  void qc.invalidateQueries({ predicate: (q) => isWatchedSensitiveFeed(q.queryKey?.[0]) });
}
